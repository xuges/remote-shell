# remote-shell

用 Go 和本机 OpenSSH 执行远程命令。启动一次后台服务，后续命令复用同一条 SSH 连接；远端无需安装代理或本项目程序。

第一版支持本机 Linux/macOS、远端 POSIX 兼容 Shell（如 sh、bash、zsh）。需要本机 Go 1.22+（编译时）和 OpenSSH 8.9+，远端开启 SSH，并提供常见的 `uname`、`hostname`、`id` 工具。Go 代码只依赖标准库。真实 SSH 集成测试在 Linux 上验证，macOS 仅验证交叉编译。

## 构建和使用

```sh
make build
export PATH="$PWD/bin:$PATH"

start-remote-shell -host=192.168.1.10 -user=root -password='your-password'
remote-shell ls -la /tmp
remote-shell-info
remote-shell-info -json
stop-remote-shell
```

`start-remote-shell` 在 SSH 登录成功且取得系统信息后返回，后台服务继续运行。重复启动会报错；切换远端时先停止已有服务。`stop-remote-shell` 会结束连接和正在运行的本地 SSH 命令进程，并等待服务清理完成。

也支持环境变量提供密码，以及私钥、ssh-agent 或默认 SSH 配置中的密钥：

```sh
REMOTE_SHELL_PASSWORD='your-password' start-remote-shell -host=192.168.1.10 -user=root
start-remote-shell -host=192.168.1.10 -user=root -port=2222 -identity="$HOME/.ssh/id_ed25519"
start-remote-shell -host=my-server
```

密码优先级为 `-password` > `REMOTE_SHELL_PASSWORD`；两者都未提供时使用非交互密钥认证。已加密的私钥请先用 `ssh-add` 加载到 ssh-agent。提供密码时会使用密码或单次 keyboard-interactive 认证，不支持需要多个不同答案的 MFA。

`-host` 支持 SSH config 别名，未指定 `-user` 时使用 SSH 的用户配置。端口由 `-port` 指定，默认固定为 22。`-timeout=20s` 控制 SSH 建连等待时间；随后系统信息探测最多等待 10 秒。

## 命令和数据流

普通模式保留每个参数的边界，参数里的空格、引号、美元符号不会被远端再次当作 Shell 语法执行：

```sh
remote-shell printf '%s\n' 'hello world' '$HOME'
printf 'hello\n' | remote-shell cat
remote-shell cat /var/log/app.log > app.log
remote-shell -c 'printf error >&2; exit 42'
echo "$?"  # 42
```

需要远端展开变量、通配符、管道或重定向时使用 `-c`，用本机的单引号保护完整表达式：

```sh
remote-shell -c 'cd /var/log && ls -lh *.log | head'
remote-shell -c 'echo "$HOME"; uname -a'
```

标准输入、标准输出和标准错误以数据块实时转发，支持二进制数据；stdout 与 stderr 独立保留，但不保证两者之间的全局顺序。远端正常退出码原样返回，SSH 传输失败使用 255，本地参数或服务错误使用非零退出码。多个客户端可以并发执行命令，并发数量受 SSH 服务端的 `MaxSessions` 限制。

每次执行创建独立的远端命令会话，因此 `cd`、`export` 等状态不会跨调用保留。需关联执行时放进同一个 `-c` 表达式。第一版不分配 PTY，不适合 vim、top 或需要终端密码提示的交互程序。Ctrl-C/客户端退出会关闭对应的本地 SSH 通道；已在远端自行脱离会话的后台进程不保证被终止。

## 状态和生命周期

`remote-shell-info` 显示连接状态、用户名、地址、系统发行版、内核、架构、主机名、登录 Shell 路径、本地服务 PID 和启动时间。`-json` 输出相同信息，便于脚本使用。系统发行版优先读取远端 `/etc/os-release`，否则显示 `uname -s`。

查询状态会执行一次最长 5 秒的远端探测，避免只根据本地进程存在判断连接正常。连接断开时状态查询返回 1，并尽可能附带上次已取得的系统信息；执行命令返回 255。后台 SSH 每 5 秒发送保活，连续两次无应答后会断开。第一版不自动重连，恢复方式为：

```sh
stop-remote-shell
start-remote-shell -host=192.168.1.10 -user=root
```

运行目录默认是系统临时目录下的 `remote-shell-<uid>`，包含本地服务 socket、SSH 复用 socket、进程锁和 `daemon.log`。可设置 `REMOTE_SHELL_DIR` 修改路径，所有客户端须使用相同值；绝对路径最多 75 字节，以满足 Unix socket 路径限制。这只是运行路径设置，第一版不提供远端连接命名和隔离管理。

正常退出会清理 socket；崩溃后再次启动并取得进程锁时，会尝试关闭残留 SSH 主连接，再清理残留 socket。不要删除正在运行的服务的锁文件。

## 实现

```text
remote-shell / remote-shell-info / stop-remote-shell
                  │ Unix socket，JSON 分帧
                  ▼
         start-remote-shell 后台服务
                  │ 调用本机 ssh、复用 ControlMaster
                  ▼
             远端 SSH 服务端
```

后台服务使用 `ssh -M -N` 保持连接，每条命令通过同一 ControlPath 创建 SSH 通道。连接复用失败时禁止退回另建连接，因此命令不会意外重新认证或连接到另一个会话。密码通过匿名管道传给后台服务，OpenSSH 再通过该程序内置的 SSH_ASKPASS 功能获取，不依赖 sshpass，不把密码写入运行文件或后台进程参数。用户直接使用 `-password` 时，该启动命令自身的参数仍可能被本机进程列表或 Shell 历史记录看到。

本地通信没有应用层认证，按第一版需求使用本机用户隔离的目录和 socket 权限。SSH 主机密钥使用 `accept-new`：首次连接记录新密钥，已知主机密钥变化时拒绝连接；保留 OpenSSH 的 known_hosts 校验。程序会关闭 agent/X11/端口转发及 LocalCommand。

代码入口位于 `cmd/`，实现位于 `internal/app/`，没有第三方 Go 依赖。

## 验证

```sh
make test              # 单元测试、race 检测、go vet
make integration-test  # 使用真实 OpenSSH 服务端的集成测试
```

集成测试需要 Linux、Python 3、Go、OpenSSH 服务端，以及 root 或可免密运行的 sudo。脚本创建临时测试账户和密钥，在随机本机回环端口启动独立 sshd，并在结束时删除账户和临时文件；不会修改系统 sshd 配置和当前用户的 known_hosts。

覆盖密码登录失败/成功、环境变量密码、密钥登录、重复启动、系统信息、参数引用、Shell 表达式、退出码、二进制输入输出、实时输出、并发命令、中断、断线检测、停止重启、服务崩溃恢复、执行中停止、建连超时和残留 socket 恢复。测试构建的程序也启用 race 检测。

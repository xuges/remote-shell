# remote-shell

用 Go 和本机 OpenSSH 执行远程命令。启动一次后台服务，后续命令复用同一条 SSH 连接；远端无需安装代理或本项目程序。

`remote-shell` 本身不做命令解释或兼容层——所有命令都原样发送到远端，由远端的默认 Shell 执行。`remote-shell-info` 在启动和查询时自动探测远端操作系统、Shell 类型等信息，供脚本和 AI 判断远端环境后选用正确的命令。本机需要 Go 1.22+（编译时）和 OpenSSH 8.9+；远端需要开启 SSH 服务。真实 SSH 集成测试在 Linux 上验证，macOS/Windows 验证交叉编译。预编译发布产物面向无 Go 工具链用户，见[发布与插件](#发布与 ai 编程插件)。

## 构建和使用

```sh
make build          # VERSION=v1.0.0, 注入 --version; 可覆盖: make build VERSION=v1.1.0
export PATH="$PWD/bin:$PATH"
make dist           # 交叉编译 6 平台 tar.gz/zip + sha256sums.txt 到 dist/
```

### 单连接（旧模式，向后兼容）

```sh
start-remote-shell -host=192.168.1.10 -user=root -password='your-password'
remote-shell ls -la /tmp
remote-shell-info
stop-remote-shell
```

### 多连接（TOML 配置文件）

在 `~/.remote-shell/config.toml`（或 `./remote-shell.toml`）中定义多个连接：

```toml
[connections.prod]
host = "192.168.1.10"
user = "root"
password = "your-password"

[connections.win]
host = "10.0.0.5"
user = "admin"
password = "win-password"
shell = "powershell"          # 可选：显式指定 "cmd" 或 "powershell"，不指定则自动探测

[connections.staging]
host = "staging.example.com"
user = "deploy"
identity = "~/.ssh/deploy_key"
timeout = "30s"
```

启动和操作：

```sh
# 自动选择（配置文件仅一个连接时）
start-remote-shell

# 指定连接名称
start-remote-shell -conn prod
start-remote-shell -conn win

# 启动所有连接
start-remote-shell --all

# 执行命令（需指定连接）
remote-shell -conn prod ls -la /tmp
remote-shell -conn win -c 'powershell -NoProfile -Command "Get-Process"'

# 查询状态
remote-shell-info -conn prod
remote-shell-info -all          # 列出所有运行中的连接
remote-shell-info -all -json    # JSON 格式

# 停止
stop-remote-shell -conn prod
stop-remote-shell -all          # 停止所有连接
```

配置文件查找顺序：`-config PATH` > `$REMOTE_SHELL_CONFIG` > `~/.remote-shell/config.toml` > `./remote-shell.toml`。密码可明文写在 TOML 中（本机可信环境），也可通过 `-password` 或 `REMOTE_SHELL_PASSWORD` 环境变量提供。CLI 参数优先于配置文件中的值。

### Windows 远端

远端 Windows 需要启用 OpenSSH Server（`Add-WindowsCapability OpenSSH.Server~~~~0.0.1.0`）。Windows OpenSSH 默认 Shell 是 `cmd.exe`，但部分服务器配置为 `powershell.exe`。`remote-shell-info` 会自动探测远端默认 Shell 类型（`cmd` 或 `powershell`），`shell` 和 `default_shell` 字段均为该值。也可在 TOML 中用 `shell` 字段显式指定，跳过自动探测：

```toml
[connections.win]
host = "10.0.0.5"
user = "admin"
shell = "powershell"   # 或 "cmd"
```

由于 `remote-shell` 不做命令兼容层，普通参数模式（`remote-shell ls -la`）的 POSIX Shell 引号语法在 Windows 下不适用。Windows 远端应始终使用 `-c` 模式，由调用方负责编写目标 Shell 的正确语法：

```sh
# PowerShell
remote-shell -conn win -c 'powershell -NoProfile -Command "Get-Process"'

# cmd.exe
remote-shell -conn win -c 'dir C:\'

# 非 -c 模式会自动检测 Windows 远端并报错提示
```

## 命令和数据流

`remote-shell` 执行命令时必须用 `-conn` 指定连接名称（不再隐式使用旧模式的默认连接）。`-connection` 保留为 `-conn` 的全称别名。

普通模式保留每个参数的边界，参数里的空格、引号、美元符号不会被远端再次当作 Shell 语法执行：

```sh
remote-shell -conn prod printf '%s\n' 'hello world' '$HOME'
printf 'hello\n' | remote-shell -conn prod cat
remote-shell -conn prod cat /var/log/app.log > app.log
remote-shell -conn prod -c 'printf error >&2; exit 42'
echo "$?"  # 42
```

需要远端展开变量、通配符、管道或重定向时使用 `-c`，用本机的单引号保护完整表达式：

```sh
remote-shell -conn prod -c 'cd /var/log && ls -lh *.log | head'
remote-shell -conn prod -c 'echo "$HOME"; uname -a'
```

标准输入、标准输出和标准错误以数据块实时转发，支持二进制数据；stdout 与 stderr 独立保留，但不保证两者之间的全局顺序。远端正常退出码原样返回，SSH 传输失败使用 255，本地参数或服务错误使用非零退出码。多个客户端可以并发执行命令，并发数量受 SSH 服务端的 `MaxSessions` 限制。

每次执行创建独立的远端命令会话，因此 `cd`、`export` 等状态不会跨调用保留。需关联执行时放进同一个 `-c` 表达式。第一版不分配 PTY，不适合 vim、top 或需要终端密码提示的交互程序。Ctrl-C/客户端退出会关闭对应的本地 SSH 通道；已在远端自行脱离会话的后台进程不保证被终止。

## 状态和生命周期

`remote-shell-info` 显示连接状态、用户名、地址、系统发行版、内核、架构、主机名、远端执行命令所用的默认 Shell、本地服务 PID 和启动时间。`-json` 输出相同信息，便于脚本使用。

探测逻辑：先尝试 POSIX 方式（`uname`、`hostname`、`id`；系统版本在 Linux 取 `/etc/os-release`，macOS 取 `sw_vers -productName/-productVersion`），适用于 Linux 和 macOS。若失败，自动回退到 PowerShell 探测（`$env:OS`、`Get-CimInstance`、`$PSVersionTable`），适用于 Windows 远端。两者输出格式统一为 7 字段 NUL 分隔，同一解析器处理。`default_shell` 表示远端执行 `-c` 命令时所用的默认 Shell：POSIX 下即登录 Shell（如 `/bin/bash`、`/bin/zsh`），Windows 下为 `cmd` 或 `powershell`（额外探测 `cmd.exe` 还是 `powershell.exe`）。TOML `shell` 字段可跳过 Windows 自动探测，直接指定 `cmd` 或 `powershell`。探测失败时报告最初的错误信息。

查询状态会执行一次最长 5 秒的远端探测，避免只根据本地进程存在判断连接正常。连接断开时状态查询返回 1，并尽可能附带上次已取得的系统信息；执行命令返回 255。后台 SSH 每 5 秒发送保活，连续两次无应答后会断开。第一版不自动重连，恢复方式为：

```sh
stop-remote-shell
start-remote-shell -host=192.168.1.10 -user=root
```

运行目录默认是系统临时目录下的 `remote-shell-<uid>`，包含本地服务 socket、SSH 复用 socket、进程锁和 `daemon.log`。可设置 `REMOTE_SHELL_DIR` 修改路径，所有客户端须使用相同值；绝对路径最多 75 字节，以满足 Unix socket 路径限制。

多连接时，每个连接使用独立文件：`<name>.service.sock`、`<name>.ssh.sock`、`<name>.daemon.lock`、`<name>.daemon.log`。空名称或 `default` 使用无前缀的旧文件名，保持向后兼容。

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

代码入口位于 `cmd/`，实现位于 `internal/app/`。TOML 配置解析使用 `github.com/BurntSushi/toml`，Windows 文件锁使用 `golang.org/x/sys`，其余依赖标准库。

## 发布与 AI 编程插件

预编译发布产物面向无 Go 工具链的用户。推 `v*` tag 触发 GitHub Actions（`.github/workflows/release.yml`）：跑测试、构建 `make dist`、上传 `remote-shell-vX.Y.Z-<os>-<arch>.tar.gz/.zip` 及 `sha256sums.txt` 到 GitHub Release。

插件（codex / opencode / claude code）以共享的 skill + 安装脚本分发，装在用户本机并复用同一套 `remote-shell` 二进制与 `~/.remote-shell`：

```text
plugins/
  skills-canonical/          # remote-shell + remote-computer-use 两份 skill 的唯一来源
  bin/bootstrap.sh|ps1       # 下载并校验预编译二进制到 ~/.remote-shell/bin（无需 Go）
  codex/remote-shell/        # Agent Plugins 可移植插件 (plugin.json)
  claude/remote-shell/       # Claude Code 插件 (.claude-plugin/plugin.json)
  opencode/remote-shell/     # OpenCode 插件 (remote-shell.ts 自定义工具 + shell.env 注入 PATH)
.claude-plugin/marketplace.json   # Claude 分发入口 "remote-shell-plugins"
```

```sh
make plugins-sync   # canonical → 三处适配器 + .opencode/skills 同步
make plugins-lint   # frontmatter/命名/副本一致性/清单 JSON 校验
```

各宿主安装与首次配置：
- **Codex**：`codex plugins install ./plugins/codex/remote-shell`
- **Claude Code**：`claude --plugin-dir ./plugins/claude/remote-shell`；或 `/plugin marketplace add <repo>` 后 `/plugin install remote-shell@remote-shell-plugins`
- **OpenCode**：复制 `plugins/opencode/remote-shell` 到 `~/.config/opencode/plugins/`（技能另见 `.opencode/skills/`）

首次使用任其运行 skill 中的 Setup：`bootstrap.sh` 下载并校验二进制到 `~/.remote-shell/bin`，配置连接写入 `~/.remote-shell/config.toml`。

## 验证

```sh
make test              # 单元测试、race 检测、go vet
make integration-test  # 使用真实 OpenSSH 服务端的集成测试
make plugins-lint      # 插件树校验
make dist              # 交叉编译 + 校验和
```

集成测试需要 Linux、Python 3、Go、OpenSSH 服务端，以及 root 或可免密运行的 sudo。脚本创建临时测试账户和密钥，在随机本机回环端口启动独立 sshd，并在结束时删除账户和临时文件；不会修改系统 sshd 配置和当前用户的 known_hosts。

覆盖密码登录失败/成功、环境变量密码、密钥登录、重复启动、系统信息、参数引用、Shell 表达式、退出码、二进制输入输出、实时输出、并发命令、中断、断线检测、停止重启、服务崩溃恢复、执行中停止、建连超时和残留 socket 恢复。测试构建的程序也启用 race 检测。

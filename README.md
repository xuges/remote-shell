# remote-shell

给 AI 一个稳定的远程命令与桌面操作入口。通过 SSH 连接 Linux、macOS、Windows，在 Codex、OpenCode、Claude Code 中执行命令、传输文件、查看截图和操作远程应用。

启动一次连接，后续命令复用 SSH 通道。远端无需安装 remote-shell 代理；命令执行只需 SSH，桌面操作还需要可访问的图形会话及对应截图、输入工具。

| 技能 | 用途 |
| --- | --- |
| `remote-shell` | 连接管理、远程构建与测试、日志排查、文件传输、长任务跟踪 |
| `remote-computer-use` | 观察远程截图、定位控件、键鼠输入、验证浏览器和桌面应用的操作结果 |

## 安装

在 **AI 客户端所在的机器**执行。Linux、macOS 或 Windows WSL，任选一行；不需要 Go、Node.js 或手动复制技能目录。

```sh
# Codex
curl -fsSL https://raw.githubusercontent.com/xuges/remote-shell/main/install.sh | bash -s -- --agent codex

# OpenCode
curl -fsSL https://raw.githubusercontent.com/xuges/remote-shell/main/install.sh | bash -s -- --agent opencode

# Claude Code
curl -fsSL https://raw.githubusercontent.com/xuges/remote-shell/main/install.sh | bash -s -- --agent claude-code
```

省略 `--agent` 会识别本机已安装的客户端；`--agent all` 一次安装到三种客户端。需要 Bash、curl、tar、OpenSSH 客户端和 `sha256sum` 或 `shasum`。

安装器会下载并校验本机平台的四个命令，安装两份完整技能及参考文档，最后检查命令版本。重复运行即可更新技能；被替换的技能会备份，连接配置保持原样。

安装完成后，新开一个 AI 会话即可提出任务，例如：

> 使用 remote-shell 连接 dev，在 /srv/app 运行测试并检查失败原因。
>
> 使用 remote-computer-use 查看 dev 的桌面，打开浏览器检查页面布局。

也可以直接把这句话发给 AI：

> 请阅读 https://raw.githubusercontent.com/xuges/remote-shell/main/INSTALL.md ，为当前客户端安装 remote-shell 和 remote-computer-use，并验证安装结果。

[完整安装指南](INSTALL.md)包含 AI 执行步骤、安装目录、更新、Windows 原生安装与故障处理。

## 连接第一台机器

在 `~/.remote-shell/config.toml` 定义连接；安装器提供了 `~/.remote-shell/config.example.toml` 模板。已有配置中只需追加所需连接。

```toml
[connections.dev]
host = "server.example.com"
user = "deploy"
port = 22
# 可使用现有 SSH 密钥/agent；指定密钥时填本机绝对路径。
# identity = "/home/you/.ssh/id_ed25519"
timeout = "20s"
```

TOML 不展开 `~` 或 `$HOME`。POSIX 主机上的配置文件建议设为 `0600`。如果使用密码，可由受保护的配置或 `REMOTE_SHELL_PASSWORD` 环境变量提供，避免把密码写进命令行和聊天记录。

```sh
# 当前终端使用；AI 技能也可以直接通过绝对路径调用命令。
export PATH="$HOME/.remote-shell/bin:$PATH"

start-remote-shell -conn dev
remote-shell-info -conn dev -json
remote-shell -conn dev -c 'cd /srv/app && ls -la'
```

配置查找顺序：`start-remote-shell -config PATH` > `REMOTE_SHELL_CONFIG` > `~/.remote-shell/config.toml` > `./remote-shell.toml`。自定义配置位置时，给各个命令设置相同的 `REMOTE_SHELL_CONFIG`。

## 日常使用

```sh
# 查询已配置和运行中的连接，不输出密码。
remote-shell-info --all -json

# POSIX 远端：按原有参数边界传递文本。
remote-shell -conn dev printf '%s\n' 'hello world' '$HOME'

# 管道、变量、通配符或重定向由远端 Shell 解释。
remote-shell -conn dev -c 'cd /srv/app && ls -lh *.log'

# 下载：外层重定向写本机文件；仅成功后使用最终文件名。
remote-shell -conn dev cat /srv/app/report.json > report.json.part && mv report.json.part report.json

# 停止指定连接。
stop-remote-shell -conn dev
```

每条执行命令都显式指定 `-conn`。多台机器可配置多个 `[connections.NAME]`；`start-remote-shell --all` 和 `stop-remote-shell --all` 用于明确需要批量启停的场景。

每次执行都是独立的远端 Shell 会话，`cd`、`export` 和变量不会跨调用保留。相关步骤应放在同一个 `-c` 表达式中，并使用 `&&` 检查前一步是否成功。

stdout、stderr、stdin 支持实时二进制传输，远端退出码原样返回，SSH 传输故障返回 255。不要把 stderr 合并到截图或压缩包的 stdout 中。命令不分配 PTY；使用非交互模式，长任务保留执行句柄或远端任务记录，超时后先查状态再决定是否重试。

### Windows 远端

读取 `remote-shell-info -conn win -json` 中的 `default_shell`，始终使用 `-c` 和对应的语法。下面的调用写法用于本机 Bash：

```sh
# default_shell = powershell
remote-shell -conn win -c 'Get-Process | Select-Object -First 5'

# default_shell = cmd
remote-shell -conn win -c 'dir C:\'
```

配置中的 `shell = "powershell"` 或 `shell = "cmd"` 可声明实际默认 Shell，省略则自动探测。此配置不会更改远端 OpenSSH 的 Shell。复杂 PowerShell 脚本、引号处理和二进制传输见[命令执行指南](plugins/skills-canonical/remote-shell/references/execution.md)。

### 远程桌面

AI 按“截图 → 观察 → 操作 → 再截图验证”的流程工作。桌面能否控制取决于会话和权限：

| 远端 | 条件 |
| --- | --- |
| Linux X11 | 实际 DISPLAY/XAUTHORITY、截图工具和 xdotool 等输入工具 |
| Linux Wayland | 当前 compositor 支持的截图和输入接口 |
| macOS | 可访问的 Aqua 会话、Screen Recording 与 Accessibility 授权 |
| Windows | 命令确实运行在可操作目标桌面的会话中；SSH 服务会话常与桌面隔离 |

已解锁桌面不等于 SSH 进程有权访问该桌面。[桌面操作指南](docs/computer-use.md)提供各平台命令、坐标缩放、文本输入与排障方法。

## 排障

| 现象 | 处理 |
| --- | --- |
| 找不到 CLI | 使用 `~/.remote-shell/bin/remote-shell`，或在当前调用设置 PATH |
| 查询状态退出码为 1 | 先看 JSON，可能只是尚无连接或某条连接未启动 |
| 无法连接本地服务 | 对目标运行 `start-remote-shell -conn NAME` |
| 连接中断 | 先确认远端任务是否已执行，再对该连接 stop/start；随后检查状态 |
| 主机密钥变化 | 核实新指纹，不要关闭 known_hosts 校验 |
| 安装校验失败 | 停止使用该下载，排查下载来源和发布资产，不跳过 SHA-256 校验 |

状态查询会进行远端探测。连接恢复需要重新启动对应连接；恢复连接不会自动恢复之前的任务。运行目录可通过 `REMOTE_SHELL_DIR` 指定，各命令须使用相同值，并保持路径简短。不要删除运行中服务的锁或 socket。

## 开发与验证

编译需要 Go 1.22+；运行依赖本机 OpenSSH。真实 SSH 集成测试在 Linux 上执行，macOS/Windows 还提供交叉编译产物；桌面能力需在目标图形会话中验证。

```sh
make build              # 四个 CLI → bin/
make test               # Go 单元测试、race 检测、vet
make installer-test     # 隔离安装、更新、校验失败与客户端目录测试
make integration-test   # 真实本机回环 SSH 测试
make plugins-lint       # 技能与安装脚本副本一致性、frontmatter、manifest
make dist               # 六种 OS/架构发布包与 SHA-256 清单
```

SSH 集成测试需要 Linux、Python 3、OpenSSH Server，以及 root 或免密 sudo。测试创建临时账户和独立回环 sshd，结束后清理；不修改系统 sshd 配置或当前用户的 known_hosts。

技能只编辑 `plugins/skills-canonical/`，二进制安装器只编辑 `plugins/bin/`，随后运行 `make plugins-sync` 同步到客户端包和 `.opencode/skills`。入口 `install.sh` 为三个客户端安装相同的技能；可选客户端插件的说明位于各自目录。

推送 `v*` tag 后，GitHub Actions 运行验证、构建发布包并上传 Release。发布前将 `plugins/bin/VERSION` 设为对应二进制版本并同步副本。

MIT License，见 [LICENSE](LICENSE)。

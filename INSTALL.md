# 安装 remote-shell 与 remote-computer-use

这份文档可以直接交给 AI 执行。目标是把两个技能及四个 CLI 安装到 **AI 运行所在的机器**，然后验证技能文件与命令。远端主机只需提供 SSH；GUI 任务另需可操作的桌面会话。

## 一行安装

Linux、macOS 或 WSL 中执行，选择当前使用的客户端：

```sh
curl -fsSL https://raw.githubusercontent.com/xuges/remote-shell/main/install.sh | bash -s -- --agent codex
curl -fsSL https://raw.githubusercontent.com/xuges/remote-shell/main/install.sh | bash -s -- --agent opencode
curl -fsSL https://raw.githubusercontent.com/xuges/remote-shell/main/install.sh | bash -s -- --agent claude-code
```

已有仓库时，在仓库根目录执行 `bash install.sh --agent codex`，把客户端名换成所需值即可。可先运行 `bash install.sh --help`。本地安装使用当前 checkout 的技能和安装脚本。

省略 `--agent` 会按命令或配置目录识别已安装的客户端，`--agent all` 安装到三处。不需要 Go、Git 或 Node.js；需要 Bash、curl、tar、OpenSSH 及 `sha256sum`/`shasum`。安装只下载技能与 CLI，不安装 AI 客户端本身。

脚本下载完整技能（包括 `references/`），按 OS/架构选择发布包并校验 SHA-256，然后运行四个命令的 `--version`。出现下载、校验或版本检查失败时，以实际错误为准，不报告安装成功。

## 给 AI 的执行步骤

1. 确认当前 AI 客户端与本机 OS，优先使用上面的对应命令。若用户指定安装范围/路径，遵循用户选择。WSL 中的安装属于 WSL 内运行的客户端，不会安装到 Windows 原生客户端。
2. 运行安装器并检查退出状态。若仓库已存在，使用本地脚本即可，无需重新克隆。缺少依赖时报告具体缺项，按用户已有授权补齐。
3. 检查下表对应目录中的两个 `SKILL.md` 及其 `references/`，并运行四个二进制的 `--version`。不要把“成功下载一个脚本”当作安装完成。
4. 用绝对路径调用 `remote-shell-info --all -json`。退出码 1 可能只是尚无连接或连接未运行；看 JSON 内容。`[]` 不意味着安装失败。
5. 只有任务还包括连接机器时，才继续配置/验证 SSH。复用已有连接，或只询问缺少的主机、用户名、端口、认证方式。不要求用户在聊天中发送私钥或明文密码。优先用已有 SSH 密钥/agent。
6. 若需要新配置，参考 `config.example.toml`，保留已有条目，使用绝对密钥路径。对自定义配置设置 `REMOTE_SHELL_CONFIG`，再启动目标连接并查询系统信息。只在用户授权的目标上执行只读验证命令。
7. 告知实际安装位置、CLI 版本、技能文件验证结果，以及是否已验证连接。新开客户端会话加载技能；如果当前客户端已自动发现，则直接使用。未能实际观察技能加载或远端桌面时，不声称它们已验证。

安装后技能会使用 CLI 的绝对路径，或在当前命令调用中设置 PATH，无需修改用户的 shell 启动文件、AI 全局提示词、权限策略或 MCP 配置。

## 安装位置

| 内容 | 默认路径 | 约定 |
| --- | --- | --- |
| Codex 两个技能 | `~/.agents/skills/remote-{shell,computer-use}/` | [Codex 技能目录](https://learn.chatgpt.com/docs/build-skills) |
| OpenCode 两个技能 | `~/.config/opencode/skills/remote-{shell,computer-use}/` | [OpenCode 技能目录](https://opencode.ai/docs/skills/)；支持 `XDG_CONFIG_HOME` |
| Claude Code 两个技能 | `~/.claude/skills/remote-{shell,computer-use}/` | [Claude Code 技能目录](https://code.claude.com/docs/en/skills)；支持 `CLAUDE_CONFIG_DIR` |
| 四个 CLI | `~/.remote-shell/bin/` | `start-remote-shell`、`remote-shell`、`remote-shell-info`、`stop-remote-shell` |
| 配置模板 | `~/.remote-shell/config.example.toml` | 不覆盖实际连接配置 |
| 连接配置 | `~/.remote-shell/config.toml` | 由用户或获授权的 AI 配置 |
| 被替换的技能 | `~/.remote-shell/backups/skills.*/` | 安装器输出具体备份位置 |

安装器尊重已有技能的内容：完全一致时跳过，内容不同时先备份整个目录，再安装完整副本。同名符号链接也会先保存，不修改链接指向的目录。其他技能目录保持原样。

## 更新、指定版本和自定义目录

重复执行安装命令即可更新技能；二进制使用随技能提供的 `plugins/bin/VERSION`。`--version latest` 或 `--version vX.Y.Z` 可选择已发布的二进制版本；`--ref` 选择下载技能的 Git ref。这两个参数分别控制技能来源与 CLI 版本。

```sh
# 使用当前 checkout，安装到全部客户端。
bash install.sh --agent all

# 显式更新二进制到最新发布版本。
bash install.sh --agent codex --version latest

# 在别处运行时指定 checkout 与二进制目录。
bash /path/to/remote-shell/install.sh --agent opencode --source /path/to/remote-shell --prefix /absolute/path/remote-shell
```

使用自定义 `--prefix` 时，为后续 AI 进程设置相同的 `REMOTE_SHELL_PREFIX`，或让 AI 使用安装器输出的绝对命令路径。此选项只改变二进制、模板和备份位置；不会改变技能发现目录，也不会改变 CLI 的连接配置查找规则。自定义配置使用 `REMOTE_SHELL_CONFIG`。

需要回退技能时，可把安装器输出的备份中对应技能目录恢复到原位置。卸载时仅删除所选客户端的这两个技能目录；确认没有其他客户端使用后，再删除 `~/.remote-shell/bin`。保留连接配置和备份，除非用户明确要求一并删除。

## Windows 原生客户端

若 AI 运行在 WSL，直接使用前面的一行命令。Windows 原生客户端可下载并解压仓库，在仓库根目录的 PowerShell 中安装二进制与技能；下面以 Codex 为例：

```powershell
& .\plugins\bin\bootstrap.ps1
$skillRoot = Join-Path $HOME '.agents\skills'
New-Item -ItemType Directory -Force $skillRoot | Out-Null
foreach ($name in @('remote-shell', 'remote-computer-use')) {
    $dest = Join-Path $skillRoot $name
    if (Test-Path $dest) { throw "Skill already exists: $dest. Back it up before replacing it." }
    Copy-Item -Recurse (Join-Path '.\plugins\skills-canonical' $name) $dest
}
& "$HOME\.remote-shell\bin\remote-shell.exe" --version
```

OpenCode 将 `$skillRoot` 改为 `$HOME\.config\opencode\skills`（设置了 `XDG_CONFIG_HOME` 时使用其 `opencode\skills`）；Claude Code 改为 `$HOME\.claude\skills`（设置了 `CLAUDE_CONFIG_DIR` 时使用其 `skills`）。已有技能先备份整个目录再替换。需要 OpenSSH 客户端；脚本执行策略受组织管理时遵循现有策略。

Windows 发布包不等于桌面已可操作：原生 SSH 连接复用及桌面访问需在实际环境中验证，GUI 还受交互会话隔离限制，见 [Windows 操作指南](plugins/skills-canonical/remote-computer-use/references/windows.md)。

## 常见问题

| 现象 | 下一步 |
| --- | --- |
| 未识别客户端 | 显式传 `--agent codex`、`opencode` 或 `claude-code` |
| `curl` 失败或资源 404 | 检查网络、Git ref 和发布版本；不使用不完整下载继续安装 |
| SHA-256 不匹配/缺失 | 停止安装并核对发布资产，不跳过校验 |
| 新会话找不到技能 | 核对当前客户端实际 HOME/配置目录及完整 `SKILL.md` 路径 |
| 终端找不到命令 | 使用 `~/.remote-shell/bin/命令`；也可在当前终端 `export PATH="$HOME/.remote-shell/bin:$PATH"` |
| info 显示未连接 | 安装与连接是两步，配置后运行 `start-remote-shell -conn NAME` |

维护者可用 `make installer-test` 在临时目录测试安装，不接触真实用户配置。测试通过 `REMOTE_SHELL_DIST_BASE_URL` 和 `REMOTE_SHELL_SOURCE_URL` 指向本机测试资产；普通安装无需设置它们。

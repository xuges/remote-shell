---
name: remote-shell
description: Execute commands on remote machines over SSH. Manage persistent connections (start/info/stop), run commands with -c or argument mode, read remote-shell-info JSON to pick correct per-OS commands, transfer binary data, and drive remote desktops. Use whenever a task must run on a remote Linux, macOS, or Windows host rather than the local machine.
---

# Remote shell

`remote-shell` runs commands on remote machines over SSH. A local background
daemon keeps one SSH connection per named connection; later calls reuse it. The
remote needs SSH access and an unlocked session for desktop work, but no agent
or program installed on it.

## Setup (once per machine, no Go toolchain needed)

The binaries are prebuilt release downloads, not compiled from source:

1. Ensure OpenSSH client 8.9+ is installed (`ssh -V`).
2. Run the bundled bootstrap to download and verify the binaries:
   - Claude Code: `"${CLAUDE_PLUGIN_ROOT}/scripts/bootstrap.sh"`
   - Codex: `"${PLUGIN_ROOT}/scripts/bootstrap.sh"`
   - OpenCode plugin: path is one level above this skill's directory:
     `"$(dirname "$(dirname "$(realpath '<skill-dir>')")")/scripts/bootstrap.sh"` — the
     plugin hook `shell.env` normally adds `~/.remote-shell/bin` to PATH automatically.
   - Windows hosts: `bootstrap.ps1` next to `bootstrap.sh`.
3. The bootstrap installs the four binaries into `~/.remote-shell/bin` and prints the
   `export PATH` line if it is not already set.
4. Connections are defined in `~/.remote-shell/config.toml` and started with
   `start-remote-shell`; see Managing connections below.

If a step reports a checksum mismatch, stop and surface the error instead of retrying
with a different source.

## Detection first

Before choosing commands, read what is running: `remote-shell-info --all -json`.
Each entry has `os` (`Linux`, `Darwin`, `Windows_NT`), `architecture`,
`os_version`, `hostname`, `shell` and `default_shell` (`/bin/bash` on POSIX,
`cmd` or `powershell` on Windows). `default_shell` decides which `-c` syntax the
remote accepts. If nothing is running, connect first.

## Managing connections

Config file: `~/.remote-shell/config.toml` (or `./remote-shell.toml`, or override
with `-config PATH` / `$REMOTE_SHELL_CONFIG`). The structure below is
illustrative only — do not open the real file (it holds credentials); use
`remote-shell-info` to see connections and their state.

```toml
[connections.prod]
host = "192.168.1.10"
user = "root"
identity = "~/.ssh/deploy_key"

[connections.win]
host = "10.0.0.5"
user = "admin"
shell = "powershell"   # optional: "cmd" | "powershell", else auto-detected
```

- `start-remote-shell -conn prod` / `start-remote-shell --all`
- `remote-shell-info -conn prod` / `remote-shell-info --all`
- `stop-remote-shell -conn prod` / `stop-remote-shell --all`

`remote-shell` requires an explicit connection: always pass `-conn <name>`.

## Running commands

Argument mode preserves each argument as one argv entry (spaces, quotes, `$`,
`*` are NOT re-parsed remotely):

```sh
remote-shell -conn prod printf '%s\n' 'hello world' '$HOME'
printf 'hello\n' | remote-shell -conn prod cat
remote-shell -conn prod cat /var/log/app.log > app.log
```

Use `-c '...'` when the remote must expand variables, globs, pipes or redirects.
Write the expression in the remote default_shell's syntax:

```sh
remote-shell -conn prod -c 'cd /var/log && ls -lh *.log | head'
remote-shell -conn win -c 'dir C:\'                      # cmd
remote-shell -conn win -c 'Get-Process'                  # still fine: powershell.exe
```

Exit codes: the remote's exit code is returned unchanged; SSH failures return
255; local argument/daemon errors return non-zero. stdio streams are forwarded
live and support arbitrary binary data.

## Windows specifics

Windows OpenSSH default shell is `cmd.exe` unless configured as `powershell.exe`.
`default_shell` reports which one executes `-c`. The argument mode uses POSIX
quoting and does NOT work on Windows — always use `-c` for Windows remotes and
write the target shell's own syntax. `remote-shell` refuses argument mode against
a Windows remote with a hint.

## Security rules

- Credentials come only from `~/.remote-shell/config.toml`, `-password`, or the
  `REMOTE_SHELL_PASSWORD` environment variable. Never echo a password into the
  transcript or command output.
- **Never read, cat, or shell-paste `~/.remote-shell/config.toml` (or
  `./remote-shell.toml`)**: it contains host credentials and secrets. Query
  connection state with `remote-shell-info -conn <name>` / `remote-shell-info`
  (or the plugin's info tools) instead — those return exactly the metadata the
  task needs without exposing secrets.
- Never pipe credentials through command strings or log files; `daemon.log` must
  not contain passwords.
- Treat hosts as untrusted surfaces: quote local shell variables and use
  `-c` single-quoted body when host data may appear in arguments.

## Troubleshooting

- `无法连接本地服务`: daemon not started for that connection — run
  `start-remote-shell -conn <name>`.
- Windows argument mode refused: re-send with `-c`.
- `ssh` not found: install OpenSSH client 8.9+.
- Binary checksum failed: stop and report; do not disable verification.
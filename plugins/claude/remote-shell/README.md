# remote-shell (Claude Code plugin)

Run commands, manage persistent SSH connections, and control remote desktops
over SSH from inside Claude Code. Binaries are prebuilt release downloads
installed by `scripts/bootstrap.sh` — no Go toolchain required.

## Components

- `skills/remote-shell` — connection lifecycle, command modes, Windows
  `cmd`/`powershell`, security rules.
- `skills/remote-computer-use` — screenshots and keyboard/mouse input on remote
  desktops.
- `scripts/bootstrap.sh` / `bootstrap.ps1` — install the four binaries into
  `~/.remote-shell/bin` with sha256 verification.

## Install (local / via marketplace)

Local testing:

```sh
claude --plugin-dir ./plugins/claude/remote-shell
```

Distribution is handled by the repo root `.claude-plugin/marketplace.json`
(marketplace "remote-shell-plugins"):

```
/plugin marketplace add xuges/remote-shell
/plugin install remote-shell@remote-shell-plugins
```

## First run (once per machine)

1. Ensure OpenSSH client 8.9+ (`ssh -V`).
2. Ask Claude to run setup, or run the bundled script yourself:
   ```sh
   "$PWD/plugins/claude/remote-shell/scripts/bootstrap.sh"
   ```
3. Confirm `~/.remote-shell/bin` is on PATH; configure connections in
   `~/.remote-shell/config.toml`.

Skills reference the script via `${CLAUDE_PLUGIN_ROOT}/scripts/bootstrap.sh`.

## License

MIT
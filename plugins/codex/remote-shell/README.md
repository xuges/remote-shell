# remote-shell (Codex plugin)

Run commands, manage persistent SSH connections, and control remote desktops
over SSH from inside Codex. No remote agent is needed; the binaries are
prebuilt release downloads (no Go toolchain required).

## What it ships

- Skill `remote-shell` — connection lifecycle (`start/info/stop`), argument and
  `-c` command modes, Windows `cmd`/`powershell` handling, security rules.
- Skill `remote-computer-use` — screenshots and keyboard/mouse input for remote
  Linux/macOS/Windows desktops.
- `scripts/bootstrap.sh` (+ `bootstrap.ps1`) — downloads and verifies the
  binaries into `~/.remote-shell/bin` (sha256-checked against the release).

## Install

```sh
codex plugins install ./plugins/codex/remote-shell
```

Then ask Codex to run the skill ("run remote-shell setup" or just start using
it — the skill loads from your prompt matching its description).

## Distribution

Publish `plugins/codex/remote-shell` to a marketplace (or point `codex plugins
install` at this repo). The plugin requires no Go toolchain: install it, run
`bootstrap.sh` once per machine, and configure connections in
`~/.remote-shell/config.toml`.

## License

MIT
# remote-shell (OpenCode plugin)

Run commands, manage persistent SSH connections, and control remote desktops
over SSH from inside OpenCode. Binaries are prebuilt releases downloaded by
`bootstrap.sh` — no Go toolchain required.

## Components

- `remote-shell.ts` — plugin that:
  - runs `scripts/bootstrap.sh` on load so the binaries are ready;
  - prepends `~/.remote-shell/bin` to PATH for every shell (via the
    `shell.env` hook);
  - registers `start_remote_shell`, `remote_shell`, `remote_shell_info` and
    `stop_remote_shell` tools.
- `skills/remote-shell` and `skills/remote-computer-use` — reusable instructions
  loaded via the skill tool for connection and desktop-work flows.

## Install

OpenCode's local plugin loader picks up plugin modules (`.ts`/`.js`) placed
flat inside the plugin directory, so copy the module and its `scripts/`
sidecar directly:

```sh
# global scope (all projects)
cp remote-shell.ts               "$HOME/.config/opencode/plugins/remote-shell.ts"
cp -r scripts                    "$HOME/.config/opencode/plugins/scripts"
mkdir -p "$HOME/.config/opencode/skills/remote-shell" \
         "$HOME/.config/opencode/skills/remote-computer-use"
cp skills/remote-shell/SKILL.md          "$HOME/.config/opencode/skills/remote-shell/"
cp skills/remote-computer-use/SKILL.md   "$HOME/.config/opencode/skills/remote-computer-use/"

# or project scope: use .opencode/plugins/ and the repo's .opencode/skills/
```

Restart OpenCode. The plugin then:
- runs `scripts/bootstrap.sh` on load (installs binaries to
  `~/.remote-shell/bin`; skips silently when already present);
- prepends `~/.remote-shell/bin` to PATH for every shell (`shell.env`);
- registers `start_remote_shell`, `remote_shell`, `remote_shell_info` and
  `stop_remote_shell` tools.

First use of each tool triggers an OpenCode permission prompt; approve it (or
allow `remote_*` in `opencode.json` permissions to skip prompts).

Connection config: `~/.remote-shell/config.toml`.

## License

MIT
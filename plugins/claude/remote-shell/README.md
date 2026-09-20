# remote-shell for Claude Code

Install both skills and the verified CLI binaries:

```sh
curl -fsSL https://raw.githubusercontent.com/xuges/remote-shell/main/install.sh | bash -s -- --agent claude-code
```

From a checkout, run `bash install.sh --agent claude-code` at the repository root.
The skills go to `~/.claude/skills/` (or `$CLAUDE_CONFIG_DIR/skills/`), binaries to
`~/.remote-shell/bin/`. Open a new Claude Code session and ask it to use the skills.
See [INSTALL.md](../../../INSTALL.md) for configuration and validation.

## Optional plugin installation

For a Claude Code plugin workflow, add this repository's marketplace in Claude:

```text
/plugin marketplace add xuges/remote-shell
/plugin install remote-shell@remote-shell-plugins
```

The plugin includes the same two skills. Install binaries once with the bundled
`scripts/bootstrap.sh` or `bootstrap.ps1`, resolved from the installed package.
Local plugin testing uses `claude --plugin-dir ./plugins/claude/remote-shell`
from the repository root. Choose one skill installation method to avoid duplicate
skill entries.

Generated `skills/` and `scripts/` are synchronized with `make plugins-sync`.

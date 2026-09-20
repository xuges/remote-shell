# remote-shell for Codex

Install `remote-shell`, including its reference
files and the verified CLI binaries:

```sh
curl -fsSL https://raw.githubusercontent.com/xuges/remote-shell/main/install.sh | bash -s -- --agent codex
```

From a checkout, run `bash install.sh --agent codex` at the repository root.
The skills go to `~/.agents/skills/`, binaries to `~/.remote-shell/bin/`.
Open a new Codex session and ask it to use remote-shell for a named remote host.

See [INSTALL.md](../../../INSTALL.md) for configuration, updates, Windows setup
and validation. This package's `skills/` and `scripts/` are generated copies;
edit the canonical sources and run `make plugins-sync`.

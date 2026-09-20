# remote-shell for OpenCode

Install the skill and the verified CLI binaries:

```sh
curl -fsSL https://raw.githubusercontent.com/xuges/remote-shell/main/install.sh | bash -s -- --agent opencode
```

From a checkout, run `bash install.sh --agent opencode` at the repository root.
Skills go to `${XDG_CONFIG_HOME:-$HOME/.config}/opencode/skills/`, binaries to
`~/.remote-shell/bin/`. Open a new OpenCode session and ask it to use remote-shell.
The skills invoke the CLI through the shell tool; no extra tool registration is
needed. See [INSTALL.md](../../../INSTALL.md) for setup and validation.

## Optional custom tools

`remote-shell.ts` supplies `start_remote_shell`, `remote_shell`,
`remote_shell_info`, and `stop_remote_shell`, plus a PATH hook. For users who
want those additional tools, copy the module and its scripts sidecar from the
repository root after running the installer:

```sh
oc_plugins="${XDG_CONFIG_HOME:-$HOME/.config}/opencode/plugins"
mkdir -p "$oc_plugins/remote-shell-scripts"
cp plugins/opencode/remote-shell/remote-shell.ts "$oc_plugins/remote-shell.ts"
cp plugins/opencode/remote-shell/scripts/* "$oc_plugins/remote-shell-scripts/"
```

Restart OpenCode to load the module. Its bootstrap uses the installed package's
pinned binary version. For screenshots or other binary output, use the shell
CLI with file redirection; these custom tools return text. For long commands,
use a shell execution tool with a resumable process handle: the custom command
tool has a five-minute timeout.

Generated `skills/` and `scripts/` are synchronized with `make plugins-sync`.

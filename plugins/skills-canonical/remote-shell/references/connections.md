# Connections and authentication

Use `remote-shell-info --all -json` to discover names and state. It includes
stopped configured connections; status fields for a stopped host can be empty.
Do not parse a real config by printing it: it may contain passwords.

## Create a connection when the task needs one

Config discovery order is `start-remote-shell -config PATH`,
`REMOTE_SHELL_CONFIG`, `~/.remote-shell/config.toml`, then `./remote-shell.toml`.
Only the start command accepts `-config`; use `REMOTE_SHELL_CONFIG` consistently
across local calls if info should discover names in a nonstandard config too.

Example with placeholder values:

```toml
[connections.dev]
host = "server.example.com"
user = "deploy"
port = 22
identity = "/home/operator/.ssh/id_ed25519"
timeout = "20s"

[connections.win]
host = "windows.example.com"
user = "operator"
shell = "powershell"
```

- Use an existing SSH key/agent when available; the private key stays local.
  `identity` is a **local absolute path**. TOML does not expand `~` or `$HOME`.
  Relative identity paths resolve against the local process's working directory.
- Names use 1–32 lowercase letters, digits, `_` or `-`, beginning with a letter
  or digit. Commands use the name, not the hostname.
- `shell` is optional and records the Windows server's actual `cmd` or
  `powershell` default. Setting it does not reconfigure OpenSSH or change the
  shell. Leave it unset for detection unless the correct value is known.
- When authorized to configure a host, edit only that connection and preserve
  unrelated entries. Keep the directory private and config mode `0600` on POSIX.
  Inspect only fields needed for troubleshooting; never echo secrets or include
  them in a patch/transcript. Do not overwrite an existing config with a template.
- If password authentication is required, prefer an already supplied secret
  environment variable (`REMOTE_SHELL_PASSWORD`) or the user's protected config.
  Do not embed passwords into `-password` command strings or shell history.
  A password in the selected TOML entry takes precedence over the environment;
  avoid assuming that changing the environment replaces it.

```sh
start-remote-shell -conn dev
remote-shell-info -conn dev -json
remote-shell -conn dev uname -s
```

An initial `remote-shell-info` result of `[]` means there are no discovered
connections. Ask only for missing host/user/authentication details; do not invent
a destination or start every configured host to find one that works.

## Recover a connection

| Evidence | Next action |
| --- | --- |
| Local CLI missing | Use the installation guide / actual bundled bootstrap path |
| Service not running | Start the selected connection, then query it again |
| Authentication failure | Check user, key path/access and auth method without logging secrets |
| Host key changed | Report the verification failure; verify the new fingerprint through a trusted source before changing known_hosts |
| `connected=false` after a network break | Inspect task effects, then stop/start only this connection |
| Runtime/socket path too long | Use a short private `REMOTE_SHELL_DIR` consistently for all local CLI calls |

```sh
stop-remote-shell -conn dev
start-remote-shell -conn dev
remote-shell-info -conn dev -json
```

These calls repair the transport, not partially executed remote work. A healthy
status probe proves connectivity, not that an earlier build or deployment ended.
Runtime files are local; do not remove a live service's socket or lock file.
For an isolated test, set a short private `REMOTE_SHELL_DIR` as well as a separate
config/home directory. Changing HOME alone does not isolate already running
daemons, which are discovered in the per-user runtime directory.

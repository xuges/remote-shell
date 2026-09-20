# Reliable command execution

## Shell boundaries and scripts

There are two parsing boundaries: the **local shell** processes your CLI call,
then the **remote default shell** interprets the transmitted command. Argument
mode quotes individual arguments for POSIX. A local double-quoted `$HOME` expands
locally even in argument mode; use single quotes when a literal value is intended.

For a POSIX remote, send a script over stdin to an explicit interpreter:

```sh
remote-shell -conn dev sh -s -- '/srv/app with spaces' <<'REMOTE'
set -eu
cd "$1"
printf 'Working directory: %s\n' "$PWD"
git status --short
REMOTE
```

The quoted heredoc delimiter prevents local expansion. This uses stdin for the
script, so it cannot simultaneously carry a separate binary payload. If Bash
features such as `pipefail` are needed, first confirm remote Bash exists, then
use `bash -s` and `set -euo pipefail`. A pipeline normally returns its last
command's status; `build | tee log` can hide a failing build without `pipefail`.

For dynamic POSIX arguments, build a local subprocess argv list. If an expression
is necessary, use a POSIX quoting function (e.g. Python `shlex.quote`) separately
on each data argument. Do not concatenate filenames, UI text or log contents
directly into executable shell syntax. POSIX quoting is not Windows quoting.

## Windows commands without nested quoting

Always use `-c`. The following calls are written in a local Bash/POSIX shell:

```sh
# default_shell = powershell
remote-shell -conn win -c 'Get-Location; Get-Process | Select-Object -First 5'
# default_shell = cmd
remote-shell -conn win -c 'cd /d C:\Work && dir'
```

For a complex script, write a **local** UTF-8 `task.ps1` and use Windows
PowerShell's UTF-16LE `-EncodedCommand`. This avoids passing PowerShell variables
through another double-quoted shell string, and works with either remote default
shell. This local helper needs Python 3:

```python
import base64
import os
from pathlib import Path
import subprocess

prefix = Path(os.environ.get("REMOTE_SHELL_PREFIX", Path.home() / ".remote-shell"))
binary = prefix / "bin" / ("remote-shell.exe" if os.name == "nt" else "remote-shell")
script = Path("task.ps1").read_text(encoding="utf-8-sig")
encoded = base64.b64encode(script.encode("utf-16le")).decode("ascii")
subprocess.run([str(binary), "-conn", "win", "-c",
                "powershell.exe -NoProfile -NonInteractive -EncodedCommand " + encoded],
               check=True)
```

Use `$ErrorActionPreference = 'Stop'` and `try { ... } catch { ...; exit 1 }`
when script success depends on PowerShell cmdlets. Native programs have their
own exit codes: check `$LASTEXITCODE` immediately and `exit` with it as needed.
Encoding is for quoting, not secrecy; never place credentials in the script or
encoded command. Large scripts can exceed Windows command-line limits; upload
them as files and invoke `-File` instead.

See [Microsoft's powershell.exe reference](https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.core/about/about_powershell_exe?view=powershell-5.1).

## File transfer and binary data

For POSIX remotes, stdout/stdin stream bytes unchanged through remote-shell:

```sh
# Download. Publish the local filename only after success.
remote-shell -conn dev cat /srv/app/output.tar.gz > output.tar.gz.part &&
  mv output.tar.gz.part output.tar.gz

# Upload to a staging file, then move only if the transfer succeeded.
remote-shell -conn dev -c 'umask 077; cat > /srv/app/input.bin.part' < input.bin &&
  remote-shell -conn dev mv /srv/app/input.bin.part /srv/app/input.bin
```

Use task-specific staging names if there may be concurrent transfers. Keep stderr
separate; `2>&1` corrupts an image/archive when diagnostics occur. Check file size
or a hash when verifying a transfer matters. Never infer that an empty or partial
local file is valid because the shell created it before the remote command failed.

On Windows, PowerShell `cat` is `Get-Content`, which is text-oriented by default.
Use .NET byte APIs plus Base64 for small binary artifacts, or an available file
transfer tool for large ones. A binary-safe transport does not make text-oriented
commands binary-safe. On a local PowerShell host, capture bytes through a binary
file API (e.g. Python subprocess with an opened `wb` file) if shell redirection's
byte preservation is uncertain.

## Long-running commands

Run bounded tasks in the foreground when possible and preserve the local tool's
session handle. Poll it until it actually exits; do not replace polling with a
second invocation of the same command. Keep stdout limits reasonable and retain
the full log remotely or locally if needed for diagnosis.

For a job that must outlive the SSH command, use the remote's existing job manager,
service manager, or a deliberate `nohup` wrapper. Arrange these **before** launch:

- A unique job directory and an explicit working directory/environment.
- stdin detached from SSH; both stdout and stderr redirected to a log.
- A PID/job identifier and a completion record containing the actual exit code.
- A way to query/cancel only that job without killing unrelated processes.

Launching with `&` alone is not reliable detachment. A successful launch is not a
successful job. A PID can be reused; verify its command/start identity before
signalling it. After interruption or reconnection, query this existing job and its
completion record before starting another. If cancellation is requested, inspect
whether children survived rather than claiming that closing SSH killed them all.

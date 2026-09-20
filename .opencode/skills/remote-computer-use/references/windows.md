# Windows desktop

## Check interactive-session access first

Use `remote-shell-info -conn win -json` to select `cmd` or `powershell` syntax.
Probe the executing process's session ID and compare with the target logged-in
user session (`query user`, when available). For a PowerShell default shell:

```sh
remote-shell -conn win -c '[Diagnostics.Process]::GetCurrentProcess().SessionId; query user'
```

Windows OpenSSH commonly launches commands in a service session. A logged-in or
unlocked RDP/console session does **not** make that SSH process interactive.
`CopyFromScreen`, cursor movement, SendKeys and clipboard APIs may fail or affect
the wrong desktop. See [Microsoft's interactive services documentation](https://learn.microsoft.com/en-us/windows/win32/services/interactive-services).

If the process cannot reach the intended desktop, stop GUI attempts and report
the session mismatch. Use an already authorized mechanism that runs inside that
interactive session, or have the user arrange one. Do not create SYSTEM tasks,
install a bridge, or change service privileges merely to bypass this boundary.
Command-line work over SSH can still proceed independently.

## Transport PowerShell without quoting errors

For either remote default shell, encode a local UTF-8 script as UTF-16LE Base64
and call `powershell.exe -EncodedCommand`. Only Base64 characters pass through
the default shell. The local helper below requires Python 3; save it as
`run-remote-ps.py` in a task temporary directory:

```python
import base64
import os
from pathlib import Path
import subprocess
import sys

prefix = Path(os.environ.get("REMOTE_SHELL_PREFIX", Path.home() / ".remote-shell"))
binary = prefix / "bin" / ("remote-shell.exe" if os.name == "nt" else "remote-shell")
script = Path(sys.argv[2]).read_text(encoding="utf-8-sig")
encoded = base64.b64encode(script.encode("utf-16le")).decode("ascii")
result = subprocess.run([str(binary), "-conn", sys.argv[1], "-c",
                         "powershell.exe -NoProfile -NonInteractive -STA -EncodedCommand " + encoded],
                        stdout=subprocess.PIPE, check=True)
sys.stdout.buffer.write(result.stdout)
```

Use `python3 run-remote-ps.py win task.ps1`. Keep diagnostics on stderr; a failure
must exit nonzero. Encoding prevents quoting problems, not secret disclosure.

## Screenshot with geometry

Save this as local `screen.ps1`. It captures the virtual desktop, including
monitors with negative origins, and returns JSON with a Base64 PNG. Text transport
avoids PowerShell's text conversion of native stdout or `Get-Content` (`cat`).

```powershell
$ErrorActionPreference = 'Stop'
try {
    Add-Type -AssemblyName System.Windows.Forms,System.Drawing
    Add-Type 'using System.Runtime.InteropServices; public class DesktopDpi {
        [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
    }'
    [void][DesktopDpi]::SetProcessDPIAware()
    $r = [System.Windows.Forms.SystemInformation]::VirtualScreen
    $b = New-Object System.Drawing.Bitmap($r.Width, $r.Height)
    $g = [System.Drawing.Graphics]::FromImage($b)
    $m = New-Object System.IO.MemoryStream
    try {
        $g.CopyFromScreen($r.Left, $r.Top, 0, 0, $b.Size)
        $b.Save($m, [System.Drawing.Imaging.ImageFormat]::Png)
        @{left=$r.Left; top=$r.Top; width=$r.Width; height=$r.Height;
          png=[Convert]::ToBase64String($m.ToArray())} | ConvertTo-Json -Compress
    } finally {
        $m.Dispose(); $g.Dispose(); $b.Dispose()
    }
} catch {
    [Console]::Error.WriteLine($_.Exception.Message)
    exit 1
}
```

Capture and decode locally; do not send the JSON's large Base64 field to chat:

```sh
python3 run-remote-ps.py win screen.ps1 > screen.json.part && mv screen.json.part screen.json
```

Only after that command succeeds:

```python
import base64
import json
from pathlib import Path

data = json.loads(Path("screen.json").read_text(encoding="utf-8-sig"))
png = base64.b64decode(data.pop("png"), validate=True)
if not png.startswith(b"\x89PNG\r\n\x1a\n"):
    raise ValueError("Capture is not a PNG")
Path("screen.png").write_bytes(png)
print(data)  # Retain origin and dimensions for input mapping.
```

Open `screen.png`. An image point maps to desktop coordinates as
`x = left + x_image`, `y = top + y_image` at the original image size. Check DPI
and monitor mapping with a harmless pointer move before a consequential click.
Blank captures are not permission to keep clicking blindly.

## Input in the same interactive session

For a click, save a script using the same error handling as `screen.ps1`, then
execute it with `run-remote-ps.py`. Replace coordinates with observed values:

```powershell
$ErrorActionPreference = 'Stop'
try {
    Add-Type @'
using System.Runtime.InteropServices;
public class DesktopMouse {
    [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
    [DllImport("user32.dll")] public static extern bool SetCursorPos(int x, int y);
    [DllImport("user32.dll")] public static extern void mouse_event(uint flags, uint x, uint y, uint data, System.UIntPtr extra);
}
'@
    [void][DesktopMouse]::SetProcessDPIAware()
    if (-not [DesktopMouse]::SetCursorPos(500, 300)) { throw 'Cannot move cursor on this desktop' }
    [DesktopMouse]::mouse_event(0x0002, 0, 0, 0, [UIntPtr]::Zero)
    [DesktopMouse]::mouse_event(0x0004, 0, 0, 0, [UIntPtr]::Zero)
} catch {
    [Console]::Error.WriteLine($_.Exception.Message)
    exit 1
}
```

For shortcuts, use `Add-Type -AssemblyName System.Windows.Forms`, then
`[System.Windows.Forms.SendKeys]::SendWait('^a')` for Ctrl-A or
`[System.Windows.Forms.SendKeys]::SendWait('{ENTER}')` for Enter in the focused
control. SendKeys interprets `+ ^ % ~ ( ) { }`; it is not a literal-text API.

For Unicode/literal text, in the same STA/interactive context set the clipboard
with `[System.Windows.Forms.Clipboard]::SetText($text)` and paste with
`[System.Windows.Forms.SendKeys]::SendWait('^v')`. Encode dynamic text separately
as UTF-8 Base64 and decode with `[Text.Encoding]::UTF8.GetString(...)`; do not
interpolate it as PowerShell source. Verify focus before pasting and inspect the
field afterward. A locked clipboard is a retryable observation, not a reason to
send repeated submissions; preserve clipboard data when the task requires it.

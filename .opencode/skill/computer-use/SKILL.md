# Computer Use Skill

Use `remote-shell` to operate remote desktops via SSH. All screenshots and input commands go through the same `remote-shell` binary — no extra agents needed.

## Prerequisites

- `start-remote-shell` already connected to the target machine
- Remote machine has an active, unlocked desktop session
- For screenshots: remote has `xwd`/`maim`/`import` (Linux), `screencapture` (macOS), or PowerShell (Windows)
- For input: remote has `xdotool` (Linux), `osascript` (macOS), or `SendKeys` (Windows)

## Detection

Always pass `-conn <conn>` to `remote-shell` — it is required. Use `remote-shell-info -json` to list connections and check:
- `os` field — remote platform, determines which OS recipes to use
- `default_shell` field — shell that executes `-c` commands: login shell on POSIX (`/bin/bash` etc.), `cmd` or `powershell` on Windows (determines which `-c` syntax to use)

TOML config allows `shell = "cmd"` or `shell = "powershell"` to override auto-detection.

## Screenshot

Capture the remote screen to a local PNG file:

```sh
# Linux
remote-shell -conn <conn> -c 'DISPLAY="${DISPLAY:-:0}"; XAUTHORITY="${XAUTHORITY:-$HOME/.Xauthority}"; command -v xwd >/dev/null 2>&1 && { xwd -root -silent | convert xwd:- png:-; } || command -v maim >/dev/null 2>&1 && maim --format png /dev/stdout || import -window root png:-' > screen.png

# macOS
remote-shell -conn <conn> -c 'screencapture -x -t png /tmp/rs.png && cat /tmp/rs.png' > screen.png

# Windows (powershell) — use when default_shell is "powershell"
remote-shell -conn <conn> -c 'powershell -NoProfile -ExecutionPolicy Bypass -Command "Add-Type -AssemblyName System.Drawing,System.Windows.Forms; $b=New-Object Drawing.Bitmap([Windows.Forms.Screen]::PrimaryScreen.Bounds.Width,[Windows.Forms.Screen]::PrimaryScreen.Bounds.Height); [Drawing.Graphics]::FromImage($b).CopyFromScreen(0,0,0,0,$b.Size); $f=[IO.Path]::Combine([IO.Path]::GetTempPath(),[IO.Path]::GetRandomFileName()+\".png\"); $b.Save($f,[Drawing.Imaging.ImageFormat]::Png); cat $f; Remove-Item $f -ErrorAction SilentlyContinue"' > screen.png

# Windows (cmd) — use when default_shell is "cmd"
remote-shell -conn <conn> -c 'powershell -NoProfile -ExecutionPolicy Bypass -Command "Add-Type -AssemblyName System.Drawing,System.Windows.Forms; $b=New-Object Drawing.Bitmap([Windows.Forms.Screen]::PrimaryScreen.Bounds.Width,[Windows.Forms.Screen]::PrimaryScreen.Bounds.Height); [Drawing.Graphics]::FromImage($b).CopyFromScreen(0,0,0,0,$b.Size); $f=[IO.Path]::Combine([IO.Path]::GetTempPath(),[IO.Path]::GetRandomFileName()+\".png\"); $b.Save($f,[Drawing.Imaging.ImageFormat]::Png); cat $f; Remove-Item $f -ErrorAction SilentlyContinue"' > screen.png
```

## Mouse Click

```sh
# Linux
remote-shell -conn <conn> -c 'DISPLAY=:0 xdotool mousemove X Y click 1'

# macOS
remote-shell -conn <conn> -c 'osascript -e "tell application \"System Events\" to click at {X,Y}"'

# Windows (powershell)
remote-shell -conn <conn> -c 'powershell -NoProfile -Command "Add-Type System.Windows.Forms; [System.Windows.Forms.Cursor]::Position=New-Object Drawing.Point(X,Y); Add-Type \"using System;using System.Runtime.InteropServices;public class M{[DllImport(\\\"user32.dll\\\")]public static extern void mouse_event(int f,int x,int y,int d,int t);}\"; [M]::mouse_event(0x0002,0,0,0,0); [M]::mouse_event(0x0004,0,0,0,0)"'

# Windows (cmd) — same command, cmd.exe passes it to PowerShell
remote-shell -conn <conn> -c 'powershell -NoProfile -Command "Add-Type System.Windows.Forms; [System.Windows.Forms.Cursor]::Position=New-Object Drawing.Point(X,Y); Add-Type \"using System;using System.Runtime.InteropServices;public class M{[DllImport(\\\"user32.dll\\\")]public static extern void mouse_event(int f,int x,int y,int d,int t);}\"; [M]::mouse_event(0x0002,0,0,0,0); [M]::mouse_event(0x0004,0,0,0,0)"'
```

## Keyboard Input

```sh
# Linux — type text
remote-shell -conn <conn> -c 'DISPLAY=:0 xdotool type --delay 50 "hello"'

# macOS — type text
remote-shell -conn <conn> -c 'osascript -e "tell application \"System Events\" to keystroke \"hello\""'

# Windows (powershell) — type text
remote-shell -conn <conn> -c 'powershell -NoProfile -Command "Add-Type System.Windows.Forms; [System.Windows.Forms.SendKeys]::SendWait(\"hello\")"'

# Windows (cmd) — same command
remote-shell -conn <conn> -c 'powershell -NoProfile -Command "Add-Type System.Windows.Forms; [System.Windows.Forms.SendKeys]::SendWait(\"hello\")"'
```

## Keyboard Shortcut

```sh
# Linux
remote-shell -conn <conn> -c 'DISPLAY=:0 xdotool key ctrl+c'

# macOS
remote-shell -conn <conn> -c 'osascript -e "tell application \"System Events\" to keystroke \"c\" using command down"'

# Windows (powershell)
remote-shell -conn <conn> -c 'powershell -NoProfile -Command "Add-Type System.Windows.Forms; [System.Windows.Forms.SendKeys]::SendWait(\"^(c)\")"'

# Windows (cmd)
remote-shell -conn <conn> -c 'powershell -NoProfile -Command "Add-Type System.Windows.Forms; [System.Windows.Forms.SendKeys]::SendWait(\"^(c)\")"'
```

## Troubleshooting

| Symptom | Fix |
|---------|-----|
| Black screenshot | Remote desktop is locked or in Session 0 — unlock it |
| macOS permission error | Grant Screen Recording + Accessibility in System Settings |
| Linux "No protocol specified" | Set `XAUTHORITY=$HOME/.Xauthority` |
| Windows type error | Ensure `Add-Type -AssemblyName System.Windows.Forms` succeeds |

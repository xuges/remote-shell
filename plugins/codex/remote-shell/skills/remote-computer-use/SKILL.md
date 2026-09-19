---
name: remote-computer-use
description: Drive a remote Linux, macOS, or Windows desktop through remote-shell over SSH — capture screenshots to local PNG files, move/click the mouse, type text, and send keyboard shortcuts. Use when a task requires seeing or controlling the remote GUI (browser automation, app interaction) and the desktop session is already unlocked.
---

# Computer Use

The `remote-shell` binaries stream binary data, so screenshots and input work
through plain shell commands over the kept-alive SSH connection. The remote
needs an active, unlocked desktop session. No agent is installed remotely.

## Prerequisites

- A connection already started (`remote-shell-info --all` shows it as 已连接).
- Remote desktop session is unlocked.
- Screenshot tooling on the remote: `xwd`/`maim`/ImageMagick `import` (Linux),
  built-in `screencapture` (macOS), or PowerShell `CopyFromScreen` (Windows).
- Input tooling: `xdotool` (Linux), `osascript` (macOS), or
  `System.Windows.Forms` (Windows).

All commands require the target connection name: always pass `-conn <conn>`.

## Detection

Read `remote-shell-info --all -json`:
- `os` — `Linux`, `Darwin`, or `Windows_NT`: decides which OS recipes to use.
- `default_shell` — POSIX: the login shell (`/bin/bash` ...). Windows: `cmd` or
  `powershell`, which decides the `-c` syntax. TOML `shell = "cmd"` (or
  `"powershell"`) can set it explicitly.

## Screenshot

Linux (X11), auto-probes DISPLAY and prefers `xwd`, falls back to maim/import:

```sh
remote-shell -conn <conn> -c 'DISPLAY="${DISPLAY:-:0}"; XAUTHORITY="${XAUTHORITY:-$HOME/.Xauthority}"; command -v xwd >/dev/null 2>&1 && { xwd -root -silent | convert xwd:- png:-; } || command -v maim >/dev/null 2>&1 && maim --format png /dev/stdout || import -window root png:-' > screen.png
```

macOS (built-in `screencapture`):

```sh
remote-shell -conn <conn> -c 'screencapture -x -t png /tmp/rs.png && cat /tmp/rs.png' > screen.png
```

Windows, PowerShell edition (use when `default_shell` is `powershell`):

```sh
remote-shell -conn <conn> -c 'powershell -NoProfile -ExecutionPolicy Bypass -Command "Add-Type -AssemblyName System.Drawing,System.Windows.Forms; $b=New-Object Drawing.Bitmap([Windows.Forms.Screen]::PrimaryScreen.Bounds.Width,[Windows.Forms.Screen]::PrimaryScreen.Bounds.Height); [Drawing.Graphics]::FromImage($b).CopyFromScreen(0,0,0,0,$b.Size); $f=[IO.Path]::Combine([IO.Path]::GetTempPath(),[IO.Path]::GetRandomFileName()+\".png\"); $b.Save($f,[Drawing.Imaging.ImageFormat]::Png); cat $f; Remove-Item $f -ErrorAction SilentlyContinue"' > screen.png
```

Windows, cmd edition (use when `default_shell` is `cmd`):

```sh
remote-shell -conn <conn> -c 'powershell -NoProfile -ExecutionPolicy Bypass -Command "Add-Type -AssemblyName System.Drawing,System.Windows.Forms; $b=New-Object Drawing.Bitmap([Windows.Forms.Screen]::PrimaryScreen.Bounds.Width,[Windows.Forms.Screen]::PrimaryScreen.Bounds.Height); [Drawing.Graphics]::FromImage($b).CopyFromScreen(0,0,0,0,$b.Size); $f=[IO.Path]::Combine([IO.Path]::GetTempPath(),[IO.Path]::GetRandomFileName()+\".png\"); $b.Save($f,[Drawing.Imaging.ImageFormat]::Png); cat $f; Remove-Item $f -ErrorAction SilentlyContinue"' > screen.png
```

The PNG streams to stdout; redirect locally with `> screen.png`. Do not re-block on
the screenshot command.

## Mouse Click

Linux:

```sh
remote-shell -conn <conn> -c 'DISPLAY=:0 xdotool mousemove X Y click 1'
```

macOS (needs Accessibility):

```sh
remote-shell -conn <conn> -c 'osascript -e "tell application \"System Events\" to click at {X,Y}"'
```

Windows (path identical under `cmd`); click via user32:

```sh
remote-shell -conn <conn> -c 'powershell -NoProfile -Command "Add-Type System.Windows.Forms; [System.Windows.Forms.Cursor]::Position=New-Object Drawing.Point(X,Y); Add-Type \"using System;using System.Runtime.InteropServices;public class M{[DllImport(\\\"user32.dll\\\")]public static extern void mouse_event(int f,int x,int y,int d,int t);}\"; [M]::mouse_event(0x0002,0,0,0,0); [M]::mouse_event(0x0004,0,0,0,0)"'
```

## Keyboard Input

```sh
# Linux
remote-shell -conn <conn> -c 'DISPLAY=:0 xdotool type --delay 50 "hello"'
# macOS
remote-shell -conn <conn> -c 'osascript -e "tell application \"System Events\" to keystroke \"hello\""'
# Windows (cmd or powershell)
remote-shell -conn <conn> -c 'powershell -NoProfile -Command "Add-Type System.Windows.Forms; [System.Windows.Forms.SendKeys]::SendWait(\"hello\")"'
```

## Keyboard Shortcut

```sh
# Linux
remote-shell -conn <conn> -c 'DISPLAY=:0 xdotool key ctrl+c'
# macOS
remote-shell -conn <conn> -c 'osascript -e "tell application \"System Events\" to keystroke \"c\" using command down"'
# Windows
remote-shell -conn <conn> -c 'powershell -NoProfile -Command "Add-Type System.Windows.Forms; [System.Windows.Forms.SendKeys]::SendWait(\"^(c)\")"'
```

## Troubleshooting

| Symptom | Fix |
|---------|-----|
| Black screenshot | Desktop locked or Session 0 isolation — unlock/relogin the remote desktop session |
| macOS permission error | Grant Screen Recording + Accessibility in System Settings |
| Linux "No protocol specified" | Set `XAUTHORITY=$HOME/.Xauthority` and pick the active display |
| Windows type load failed | Ensure `Add-Type -AssemblyName System.Windows.Forms` succeeds |
| Nothing connected | `start-remote-shell -conn <conn>` and re-check with `remote-shell-info` |
# Linux desktop

Examples use a local POSIX shell and connection `linux`. The CLI directory must
be on that invocation's PATH. Replace display/authentication values with values
from the target's actual session, not guessed defaults.

## Find the desktop

```sh
remote-shell -conn linux -c 'id -un; printf "DISPLAY=%s\nXAUTHORITY=%s\nWAYLAND_DISPLAY=%s\nXDG_RUNTIME_DIR=%s\n" "$DISPLAY" "$XAUTHORITY" "$WAYLAND_DISPLAY" "$XDG_RUNTIME_DIR"; ls /tmp/.X11-unix'
```

SSH environments often have these variables unset. Use `loginctl list-sessions`
and `loginctl show-session SESSION -p Type -p Name -p Display -p State` if available;
otherwise use the known desktop launch configuration. The display socket list
identifies candidates, not which user owns an unlocked desktop. Do not dump
entire process environments, copy another user's Xauthority cookies, or use
`xhost +` to bypass access control.

For X11, confirm `DISPLAY` and, when required, the session's real `XAUTHORITY`
file. It may live under `/run/user/...`, not `~/.Xauthority`. Use the **same values
on every capture and input call**; SSH channels do not preserve exports. Probe
installed tools with `command -v maim`, `command -v import`, `command -v xdotool`.

## X11 screenshot and input

These examples assume you confirmed `DISPLAY=:1` and
`XAUTHORITY=/home/operator/.Xauthority`. `env` exports both to the remote program:

```sh
remote-shell -conn linux env DISPLAY=:1 XAUTHORITY=/home/operator/.Xauthority \
  maim --format=png > screen.png.part && mv screen.png.part screen.png

# Alternative when ImageMagick import is installed; choose one capture command.
remote-shell -conn linux env DISPLAY=:1 XAUTHORITY=/home/operator/.Xauthority \
  import -window root png:- > screen.png.part && mv screen.png.part screen.png

# Coordinates must come from the screenshot you just opened.
remote-shell -conn linux env DISPLAY=:1 XAUTHORITY=/home/operator/.Xauthority \
  xdotool mousemove --sync 500 300 click 1

remote-shell -conn linux env DISPLAY=:1 XAUTHORITY=/home/operator/.Xauthority \
  xdotool type --clearmodifiers --delay 20 -- 'hello world'

remote-shell -conn linux env DISPLAY=:1 XAUTHORITY=/home/operator/.Xauthority \
  xdotool key --clearmodifiers ctrl+a

# Scroll down a small amount, then capture again before choosing a target.
remote-shell -conn linux env DISPLAY=:1 XAUTHORITY=/home/operator/.Xauthority \
  xdotool click --repeat 3 --delay 100 5
```

`xwd` produces XWD, not PNG. If it is the only capture tool, you also need a
working ImageMagick converter. Run `xwd -root -silent | convert xwd:- png:-` under
a shell with `pipefail` (or stage the XWD file and check each exit code). Do not
use `A && B || C && D` as a capture fallback: it can run more than one capture
and concatenate two images.

`xdotool type` is affected by keymaps and may mishandle Unicode. For text that
does not type correctly, an available `xclip` can fill the clipboard from stdin:

```sh
printf '%s' '你好，world' | remote-shell -conn linux \
  env DISPLAY=:1 XAUTHORITY=/home/operator/.Xauthority xclip -selection clipboard
remote-shell -conn linux env DISPLAY=:1 XAUTHORITY=/home/operator/.Xauthority \
  xdotool key --clearmodifiers ctrl+v
```

Confirm the target is a text field before pasting, and verify the actual text.
Clipboard managers and key bindings vary; do not assume clipboard contents are
preserved. See the [xdotool manual](https://github.com/jordansissel/xdotool/blob/main/xdotool.pod)
for window selection and key syntax.

## Wayland

X11 recipes do not control a whole Wayland desktop; XWayland covers only some
windows. Detect the compositor and available capture/input interfaces first.
For a compositor supporting `grim`, and a confirmed user runtime directory and
Wayland socket, capture with:

```sh
remote-shell -conn linux env XDG_RUNTIME_DIR=/run/user/1000 WAYLAND_DISPLAY=wayland-0 \
  grim - > screen.png.part && mv screen.png.part screen.png
```

GNOME/KDE may instead need their own screenshot facility or an authorized desktop
portal session. Input needs a compositor-supported mechanism or an already
configured tool such as `ydotool`; inspect its installed version/help and daemon
permissions before using it. Do not assume `sudo`, input-device access, or an
unattended portal grant. If no usable interface exists, report the prerequisite
and ask for the required desktop setup; do not switch session type or install a
privileged input daemon just to make a click work.

If capture is blank or input does nothing, recheck lock state, session identity,
display/auth values, focus, and tool stderr before trying new coordinates.

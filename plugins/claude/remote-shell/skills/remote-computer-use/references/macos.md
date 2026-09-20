# macOS desktop

Use connection `mac` below. Confirm the SSH account can access the logged-in
console user's Aqua session. `stat -f %Su /dev/console` identifies the console
user; it does not itself grant access to that session.

Screen capture requires Screen Recording permission for the process context
performing capture. UI scripting requires Accessibility, and may also trigger
Automation permission prompts. If access is denied, identify the process named
by the prompt and let the user grant the needed permission in System Settings.
Do not disable TCC or repeatedly retry a denied action. An unlocked screen does
not prove the SSH-launched process has these permissions.

## Screenshot

From a local POSIX shell, capture the main display, stream it to a fresh local
file, and clean up the unique remote temporary file even on failure:

```sh
remote-shell -conn mac sh -s > screen.png.part <<'REMOTE'
set -eu
tmp=$(mktemp "${TMPDIR:-/tmp}/remote-screen.XXXXXX")
trap 'rm -f "$tmp"' EXIT
screencapture -x -m -t png "$tmp"
cat "$tmp"
REMOTE
capture_status=$?
if [ "$capture_status" -eq 0 ]; then mv screen.png.part screen.png; fi
```

Open the PNG before acting. Retina screenshots can have more pixels than the
screen's logical point coordinates. Confirm display geometry and the input tool's
coordinate units; do not hard-code a factor of two. `-m` captures the main display;
if the target is elsewhere, choose that display with the installed
`screencapture` options and record its origin.

## Focus, keys and clicks

Send AppleScript through stdin to avoid multiple layers of shell escaping:

```sh
# Activate only the app relevant to the task, then capture to verify its state.
remote-shell -conn mac osascript - <<'APPLESCRIPT'
tell application "Safari" to activate
APPLESCRIPT

# Example: focus Safari's address bar. Verify before typing a URL.
remote-shell -conn mac osascript - <<'APPLESCRIPT'
tell application "System Events"
    keystroke "l" using command down
end tell
APPLESCRIPT

# Type simple text into the already focused field.
remote-shell -conn mac osascript - <<'APPLESCRIPT'
tell application "System Events" to keystroke "hello world"
APPLESCRIPT

# Return. Take a fresh screenshot after the navigation/submission.
remote-shell -conn mac osascript - <<'APPLESCRIPT'
tell application "System Events" to key code 36
APPLESCRIPT
```

For clicks, a known accessibility element is often more reliable than pixels:
inspect the current app's UI hierarchy and target its actual window/control.
Do not assume a button is named `OK` or reuse an example hierarchy on another app.
If an installed coordinate tool such as [cliclick](https://github.com/BlueM/cliclick) is available, use its documented
coordinates (`cliclick c:500,300`) after mapping the screenshot to logical points.
Confirm the tool and permissions before use; macOS does not include `cliclick`.
In cliclick, signed coordinates normally mean relative movement. For an absolute
negative monitor coordinate, prefix it with `=`, e.g. `c:=-200,100`.

For Unicode or long literal text, use `pbcopy` over stdin, then paste with
Command-V into the verified field:

```sh
printf '%s' '你好，world' | remote-shell -conn mac env LANG=en_US.UTF-8 pbcopy
remote-shell -conn mac osascript - <<'APPLESCRIPT'
tell application "System Events" to keystroke "v" using command down
APPLESCRIPT
```

This replaces the clipboard. If preserving it matters, use an appropriate
clipboard API; a plain-text backup does not preserve image or rich-text formats.
After every UI transition, observe again and check the result rather than relying
on `osascript`'s exit code alone.

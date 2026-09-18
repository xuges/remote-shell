package app

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// boundedBuffer keeps SSH diagnostics bounded for a long-lived master process.
type boundedBuffer struct {
	mu   sync.Mutex
	data []byte
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data = append(b.data, p...)
	if len(b.data) > 16384 {
		b.data = b.data[len(b.data)-16384:]
	}
	return len(p), nil
}
func (b *boundedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.TrimSpace(string(b.data))
}

func (c config) baseArgs() []string {
	seconds := int64((c.Timeout + time.Second - 1) / time.Second)
	if seconds < 1 {
		seconds = 1
	}
	a := []string{"-T", "-S", controlPath(c.Dir, c.Name), "-p", fmt.Sprint(c.Port),
		"-o", fmt.Sprintf("ConnectTimeout=%d", seconds), "-o", "ConnectionAttempts=1", "-o", "ServerAliveInterval=5",
		"-o", "ServerAliveCountMax=2", "-o", "StrictHostKeyChecking=accept-new",
		"-o", "ForwardAgent=no", "-o", "ForwardX11=no", "-o", "ClearAllForwardings=yes",
		"-o", "PermitLocalCommand=no", "-o", "RequestTTY=no"}
	if c.User != "" {
		a = append(a, "-l", c.User)
	}
	return a
}

func (c config) command(ctx context.Context, command string) *exec.Cmd {
	a := append(c.baseArgs(), "-o", "ControlMaster=no", "-o", "BatchMode=yes", "-o", "ProxyCommand=false", "--", c.Host, command)
	return exec.CommandContext(ctx, c.SSH, a...)
}

// A POSIX login shell and standard system utilities are sufficient on the remote.
// NUL separators avoid ambiguities in OS release names and startup banners.
// OS release: /etc/os-release on Linux, sw_vers on macOS (no os-release),
// and uname -s as a last resort.
const probeCommand = `printf '\000REMOTE_SHELL_INFO\000'; uname -s; uname -r; uname -m; hostname; printf '%s\n' "$SHELL"; id -un; if [ -r /etc/os-release ]; then . /etc/os-release; printf '%s\n' "$PRETTY_NAME"; elif command -v sw_vers >/dev/null 2>&1; then printf '%s %s\n' "$(sw_vers -productName 2>/dev/null)" "$(sw_vers -productVersion 2>/dev/null)"; else uname -s; fi`

// PowerShell probe fallback for Windows remotes. Windows OpenSSH's default
// shell is cmd.exe, so the probe explicitly invokes powershell.exe and forces
// UTF-8 output so the NUL byte marker survives. Produces the same 7-field
// NUL-separated output as the POSIX probe (with additional CRLF line endings).
const probeCommandPS = `powershell -NoProfile -NonInteractive -Command "[Console]::OutputEncoding=[Text.Encoding]::UTF8; $m=[char]0+'REMOTE_SHELL_INFO'+[char]0; $f=@($env:OS,[Environment]::OSVersion.Version.ToString(),$env:PROCESSOR_ARCHITECTURE,$env:COMPUTERNAME,$PSVersionTable.PSEdition,$env:USERNAME,(Get-CimInstance Win32_OperatingSystem).Caption); [Console]::Write($m); [Console]::WriteLine(); $f|%{[Console]::WriteLine($_)}"`

func (c config) probe(ctx context.Context, info *connectionInfo) error {
	out, err := c.command(ctx, probeCommand).Output()
	if err != nil || parseProbe(out, info) != nil {
		// POSIX probe failed or produced unparseable output. Try the
		// PowerShell fallback for Windows remotes.
		out, err = c.command(ctx, probeCommandPS).Output()
		if err != nil {
			return fmt.Errorf("SSH 探测失败: %v", err)
		}
	}
	if err := parseProbe(out, info); err != nil {
		return err
	}
	if info.OS == "Windows_NT" {
		// Windows has a single execution shell (cmd or powershell), decided
		// by the OpenSSH server configuration. Fields[4] from the probe is
		// $PSVersionTable.PSEdition ("Desktop"), which is not a shell, so
		// replace it with the detected default shell.
		info.Shell = c.detectDefaultShell(ctx)
		info.DefaultShell = info.Shell
	} else {
		// POSIX: the login shell is what executes -c commands. Fields[4] is
		// $SHELL (e.g. /bin/bash, /bin/zsh).
		info.DefaultShell = info.Shell
	}
	return nil
}

// parseProbe parses NUL-marker-separated probe output into info. It tolerates
// both LF (POSIX) and CRLF (Windows PowerShell) line endings.
func parseProbe(out []byte, info *connectionInfo) error {
	_, payload, found := strings.Cut(string(out), "\x00REMOTE_SHELL_INFO\x00")
	if !found {
		return fmt.Errorf("无法解析远端系统信息；需要 POSIX 兼容 Shell 和 uname、hostname、id")
	}
	fields := strings.FieldsFunc(payload, func(r rune) bool { return r == '\n' || r == '\r' })
	if len(fields) < 7 {
		return fmt.Errorf("无法解析远端系统信息；需要 POSIX 兼容 Shell 和 uname、hostname、id")
	}
	info.OS, info.Kernel, info.Architecture = fields[0], fields[1], fields[2]
	info.Hostname, info.Shell, info.User, info.OSVersion = fields[3], fields[4], fields[5], fields[6]
	return nil
}

// detectDefaultShell probes whether the remote Windows OpenSSH default shell
// is cmd.exe or powershell.exe. Returns "cmd", "powershell", or "" on failure.
// If the user set a TOML shell override, the caller should use that instead.
func (c config) detectDefaultShell(ctx context.Context) string {
	if c.ShellOverride != "" {
		return c.ShellOverride
	}
	// Use a cmd-native command to detect: echo %COMSPEC% produces
	// "C:\WINDOWS\system32\cmd.exe" on cmd, while on PowerShell it would
	// not expand (returns literal "%COMSPEC%"). Combined with a second
	// signal from Get-Process, we can reliably distinguish.
	out, err := c.command(ctx, `echo %COMSPEC%`).Output()
	if err != nil {
		return ""
	}
	s := strings.TrimSpace(string(out))
	if strings.Contains(strings.ToLower(s), "cmd.exe") {
		return "cmd"
	}
	// COMSPEC did not expand — PowerShell or other shell.
	return "powershell"
}

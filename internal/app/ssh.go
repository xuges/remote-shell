package app

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
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
	a := []string{"-T", "-S", filepath.Join(c.Dir, "ssh.sock"), "-p", fmt.Sprint(c.Port),
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
const probeCommand = `printf '\000REMOTE_SHELL_INFO\000'; uname -s; uname -r; uname -m; hostname; printf '%s\n' "$SHELL"; id -un; if [ -r /etc/os-release ]; then . /etc/os-release; printf '%s\n' "$PRETTY_NAME"; else uname -s; fi`

func (c config) probe(ctx context.Context, info *connectionInfo) error {
	out, err := c.command(ctx, probeCommand).Output()
	if err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("SSH 探测失败: %s: %s", err, strings.TrimSpace(string(e.Stderr)))
		}
		return fmt.Errorf("SSH 探测失败: %w", err)
	}
	_, payload, found := strings.Cut(string(out), "\x00REMOTE_SHELL_INFO\x00")
	fields := strings.Split(strings.TrimSuffix(payload, "\n"), "\n")
	if !found || len(fields) < 7 {
		return fmt.Errorf("无法解析远端系统信息；需要 POSIX 兼容 Shell 和 uname、hostname、id")
	}
	info.OS, info.Kernel, info.Architecture = fields[0], fields[1], fields[2]
	info.Hostname, info.Shell, info.User, info.OSVersion = fields[3], fields[4], fields[5], fields[6]
	return nil
}

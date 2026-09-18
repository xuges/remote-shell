// Package app implements the local daemon and the command-line clients.
package app

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type config struct {
	Name          string
	Host          string
	User          string
	Port          int
	Identity      string
	Password      string
	HasPassword   bool
	Dir           string
	SSH           string
	Timeout       time.Duration
	ShellOverride string // "cmd" or "powershell"; empty = auto-detect
}

type request struct {
	Action  string `json:"action"`
	Name    string `json:"name,omitempty"`
	Command string `json:"command,omitempty"`
}

type packet struct {
	Type  string            `json:"type"`
	Data  []byte            `json:"data,omitempty"`
	Code  int               `json:"code,omitempty"`
	Error string            `json:"error,omitempty"`
	Info  *connectionInfo   `json:"info,omitempty"`
	Infos []*connectionInfo `json:"infos,omitempty"`
}

type connectionInfo struct {
	Name         string    `json:"name,omitempty"`
	Connected    bool      `json:"connected"`
	Host         string    `json:"host"`
	User         string    `json:"user"`
	Port         int       `json:"port"`
	PID          int       `json:"pid"`
	StartedAt    time.Time `json:"started_at"`
	OS           string    `json:"os,omitempty"`
	OSVersion    string    `json:"os_version,omitempty"`
	Kernel       string    `json:"kernel,omitempty"`
	Architecture string    `json:"architecture,omitempty"`
	Hostname     string    `json:"hostname,omitempty"`
	Shell        string    `json:"shell,omitempty"`
	DefaultShell string    `json:"default_shell,omitempty"`
	Error        string    `json:"error,omitempty"`
}

func runtimeDir() (string, error) {
	dir := os.Getenv("REMOTE_SHELL_DIR")
	if dir == "" {
		dir = filepath.Join(os.TempDir(), fmt.Sprintf("remote-shell-%d", os.Getuid()))
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if len(dir) > 75 {
		return "", fmt.Errorf("REMOTE_SHELL_DIR 路径过长（最多 75 字节）")
	}
	return dir, nil
}

// Per-name path helpers. When name is "" or "default", use legacy filenames
// (service.sock, daemon.lock, ssh.sock, daemon.log) for backward compatibility.

func socketPath(dir, name string) string {
	if name == "" || name == "default" {
		return filepath.Join(dir, "service.sock")
	}
	return filepath.Join(dir, name+".service.sock")
}

func lockPath(dir, name string) string {
	if name == "" || name == "default" {
		return filepath.Join(dir, "daemon.lock")
	}
	return filepath.Join(dir, name+".daemon.lock")
}

func controlPath(dir, name string) string {
	if name == "" || name == "default" {
		return filepath.Join(dir, "ssh.sock")
	}
	return filepath.Join(dir, name+".ssh.sock")
}

func logPath(dir, name string) string {
	if name == "" || name == "default" {
		return filepath.Join(dir, "daemon.log")
	}
	return filepath.Join(dir, name+".daemon.log")
}

// checkPathLength verifies the socket path fits within the Unix sun_path limit.
func checkPathLength(dir, name string) error {
	p := socketPath(dir, name)
	// 108 is the typical sun_path limit; leave margin for internal use.
	if len(p) > 104 {
		return fmt.Errorf("socket 路径过长（%d 字节，最多 104）: %s", len(p), p)
	}
	return nil
}

func dial(dir string) (net.Conn, error) {
	return dialNamed(dir, "")
}

func dialNamed(dir, name string) (net.Conn, error) {
	c, err := net.DialTimeout("unix", socketPath(dir, name), 2*time.Second)
	if err != nil {
		return nil, fmt.Errorf("无法连接本地服务，请先运行 start-remote-shell: %w", err)
	}
	return c, nil
}

func exchange(action string) (packet, error) {
	return exchangeNamed(action, "")
}

func exchangeNamed(action, name string) (packet, error) {
	var p packet
	dir, err := runtimeDir()
	if err != nil {
		return p, err
	}
	c, err := dialNamed(dir, name)
	if err != nil {
		return p, err
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(12 * time.Second))
	if err = json.NewEncoder(c).Encode(request{Action: action, Name: name}); err != nil {
		return p, err
	}
	err = json.NewDecoder(c).Decode(&p)
	if err == nil && p.Error != "" {
		err = fmt.Errorf("%s", p.Error)
	}
	return p, err
}

type sender struct {
	mu  sync.Mutex
	enc *json.Encoder
}

func (s *sender) send(p packet) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.enc.Encode(p)
}

type streamWriter struct {
	s    *sender
	kind string
}

func (w streamWriter) Write(b []byte) (int, error) {
	if err := w.s.send(packet{Type: w.kind, Data: b}); err != nil {
		return 0, err
	}
	return len(b), nil
}

// quoteArgs preserves argument boundaries through the remote POSIX shell.
func quoteArgs(args []string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = "'" + strings.ReplaceAll(arg, "'", "'\"'\"'") + "'"
	}
	return strings.Join(quoted, " ")
}

func fail(err error) int { fmt.Fprintln(os.Stderr, "remote-shell:", err); return 1 }

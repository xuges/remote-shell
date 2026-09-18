package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

type daemon struct {
	cfg    config
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.RWMutex
	info   connectionInfo
	ready  bool
	wg     sync.WaitGroup
}

func runDaemon() (result int) {
	startup := json.NewEncoder(os.Stdout)
	reported := false
	report := func(err error) {
		p := packet{Type: "ready"}
		if err != nil {
			p.Error = err.Error()
		}
		startup.Encode(p)
		reported = true
	}
	defer func() {
		if !reported {
			report(fmt.Errorf("服务启动失败"))
		}
	}()
	var cfg config
	if err := json.NewDecoder(os.Stdin).Decode(&cfg); err != nil {
		report(err)
		return 1
	}
	lock, err := os.OpenFile(lockPath(cfg.Dir, cfg.Name), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		report(err)
		return 1
	}
	defer lock.Close()
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		report(fmt.Errorf("已有服务运行，请先运行 stop-remote-shell"))
		return 1
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	// A daemon killed with SIGKILL can leave its SSH child alive. Under the
	// exclusive lock, close that old master before replacing its control socket.
	cp := controlPath(cfg.Dir, cfg.Name)
	if _, err := os.Stat(cp); err == nil {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 2*time.Second)
		exec.CommandContext(cleanupCtx, cfg.SSH, "-S", cp, "-O", "exit", "--", cfg.Host).Run()
		cleanupCancel()
	}
	socket := socketPath(cfg.Dir, cfg.Name)
	os.Remove(socket)
	os.Remove(cp)
	listener, err := net.Listen("unix", socket)
	if err != nil {
		report(err)
		return 1
	}
	defer os.Remove(socket)
	defer listener.Close()
	if err := os.Chmod(socket, 0600); err != nil {
		report(err)
		return 1
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	defer cancel()
	d := &daemon{cfg: cfg, ctx: ctx, cancel: cancel, info: connectionInfo{
		Name: cfg.Name, Host: cfg.Host, User: cfg.User, Port: cfg.Port, PID: os.Getpid(), StartedAt: time.Now().UTC(),
	}}
	acceptDone := make(chan struct{})
	go func() {
		defer close(acceptDone)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			d.wg.Add(1)
			go func() { defer d.wg.Done(); d.handle(conn) }()
		}
	}()
	defer func() { cancel(); listener.Close(); <-acceptDone; d.wg.Wait() }()

	exe, err := os.Executable()
	if err != nil {
		report(err)
		return 1
	}
	args := append(cfg.baseArgs(), "-M", "-N", "-o", "ControlPersist=no", "-o", "ForkAfterAuthentication=no")
	if cfg.Identity != "" {
		args = append(args, "-i", cfg.Identity, "-o", "IdentitiesOnly=yes")
	}
	if cfg.HasPassword {
		args = append(args, "-o", "BatchMode=no", "-o", "NumberOfPasswordPrompts=1", "-o", "PreferredAuthentications=password,keyboard-interactive", "-o", "PubkeyAuthentication=no")
	} else {
		args = append(args, "-o", "BatchMode=yes")
	}
	args = append(args, "--", cfg.Host)
	master := exec.CommandContext(ctx, cfg.SSH, args...)
	master.Env = append(os.Environ(), "SSH_ASKPASS="+exe, "SSH_ASKPASS_REQUIRE=force", "REMOTE_SHELL_ASKPASS=1", "REMOTE_SHELL_DIR="+cfg.Dir, "REMOTE_SHELL_NAME="+cfg.Name, "DISPLAY=remote-shell:0")
	var diagnostics boundedBuffer
	master.Stderr = &diagnostics
	if err := master.Start(); err != nil {
		report(err)
		return 1
	}
	masterDone := make(chan struct{})
	go func() { master.Wait(); close(masterDone) }()
	defer func() { cancel(); <-masterDone; os.Remove(controlPath(cfg.Dir, cfg.Name)) }()
	timer := time.NewTimer(cfg.Timeout)
	defer timer.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			report(ctx.Err())
			return 1
		case <-masterDone:
			report(fmt.Errorf("SSH 连接失败: %s", diagnostics.String()))
			return 1
		case <-timer.C:
			report(fmt.Errorf("SSH 连接超时: %s", diagnostics.String()))
			return 1
		case <-ticker.C:
			if _, err := os.Stat(controlPath(cfg.Dir, cfg.Name)); err == nil {
				goto connected
			}
		}
	}
connected:
	probeCtx, probeCancel := context.WithTimeout(ctx, 10*time.Second)
	info := d.info
	err = cfg.probe(probeCtx, &info)
	probeCancel()
	if err != nil {
		report(err)
		return 1
	}
	info.Connected = true
	d.mu.Lock()
	d.info = info
	d.ready = true
	d.mu.Unlock()
	report(nil)
	select {
	case <-ctx.Done():
	case <-masterDone:
		d.mu.Lock()
		d.info.Connected = false
		d.info.Error = "SSH 连接已断开: " + diagnostics.String()
		d.mu.Unlock()
		<-ctx.Done()
	}
	return 0
}

func (d *daemon) handle(conn net.Conn) {
	defer conn.Close()
	ctx, cancel := context.WithCancel(d.ctx)
	defer cancel()
	watchDone := make(chan struct{})
	defer close(watchDone)
	go func() {
		select {
		case <-ctx.Done():
			conn.Close()
		case <-watchDone:
		}
	}()
	dec := json.NewDecoder(conn)
	s := &sender{enc: json.NewEncoder(conn)}
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var req request
	if err := dec.Decode(&req); err != nil {
		return
	}
	conn.SetReadDeadline(time.Time{})
	if req.Action == "password" {
		s.send(packet{Type: "password", Data: []byte(d.cfg.Password)})
		return
	}
	if req.Action == "stop" {
		s.send(packet{Type: "stopped"})
		d.cancel()
		return
	}
	d.mu.RLock()
	ready, info := d.ready, d.info
	d.mu.RUnlock()
	if !ready {
		s.send(packet{Type: "error", Error: "服务正在连接远端"})
		return
	}
	switch req.Action {
	case "info":
		if info.Connected {
			probeCtx, probeCancel := context.WithTimeout(ctx, 5*time.Second)
			err := d.cfg.probe(probeCtx, &info)
			probeCancel()
			if err != nil {
				info.Connected = false
				info.Error = err.Error()
			}
		}
		s.send(packet{Type: "info", Info: &info})
	case "exec":
		if !info.Connected {
			s.send(packet{Type: "exit", Code: 255, Error: info.Error})
			return
		}
		if req.Command == "" {
			s.send(packet{Type: "exit", Code: 2, Error: "命令不能为空"})
			return
		}
		cmd := d.cfg.command(ctx, req.Command)
		stdin, err := cmd.StdinPipe()
		if err != nil {
			s.send(packet{Type: "exit", Code: 255, Error: err.Error()})
			return
		}
		cmd.Stdout, cmd.Stderr = streamWriter{s, "stdout"}, streamWriter{s, "stderr"}
		cmd.WaitDelay = 2 * time.Second
		if err := cmd.Start(); err != nil {
			stdin.Close()
			s.send(packet{Type: "exit", Code: 255, Error: err.Error()})
			return
		}
		inputDone := make(chan struct{})
		go func() {
			defer close(inputDone)
			defer stdin.Close()
			ended := false
			for {
				var p packet
				if err := dec.Decode(&p); err != nil {
					cancel()
					return
				}
				switch p.Type {
				case "stdin":
					if !ended {
						if _, err := stdin.Write(p.Data); err != nil {
							ended = true
							stdin.Close()
						}
					}
				case "eof":
					ended = true
					stdin.Close()
				case "cancel":
					cancel()
					return
				}
			}
		}()
		err = cmd.Wait()
		code := 0
		message := ""
		if err != nil {
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				code = exitErr.ExitCode()
				if code < 0 {
					code = 255
				}
			} else {
				code = 255
				message = err.Error()
			}
		}
		s.send(packet{Type: "exit", Code: code, Error: message})
		conn.Close()
		stdin.Close()
		<-inputDone
	default:
		s.send(packet{Type: "error", Error: "未知请求"})
	}
}

func askpass() int {
	name := os.Getenv("REMOTE_SHELL_NAME")
	p, err := exchangeNamed("password", name)
	if err != nil {
		return 1
	}
	_, err = io.WriteString(os.Stdout, string(p.Data)+"\n")
	if err != nil {
		return 1
	}
	return 0
}

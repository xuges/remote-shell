package app

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

func Start(args []string) int {
	if os.Getenv("REMOTE_SHELL_ASKPASS") == "1" {
		return askpass()
	}
	if len(args) == 1 && args[0] == "--daemon" {
		return runDaemon()
	}
	var cfg config
	f := flag.NewFlagSet("start-remote-shell", flag.ContinueOnError)
	f.StringVar(&cfg.Host, "host", "", "远端主机名、IP 或 SSH config 别名（必填）")
	f.StringVar(&cfg.User, "user", "", "远端用户名；默认使用 SSH 配置")
	f.IntVar(&cfg.Port, "port", 22, "SSH 端口")
	f.StringVar(&cfg.Password, "password", "", "SSH 密码；也可设置 REMOTE_SHELL_PASSWORD")
	f.StringVar(&cfg.Identity, "identity", "", "SSH 私钥文件")
	f.DurationVar(&cfg.Timeout, "timeout", 20*time.Second, "建立 SSH 连接的超时时间")
	if err := f.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if f.NArg() != 0 || cfg.Host == "" || strings.HasPrefix(cfg.Host, "-") || strings.ContainsAny(cfg.Host, "\x00\r\n \t") {
		return fail(fmt.Errorf("请用 -host 指定有效主机"))
	}
	if cfg.Port < 1 || cfg.Port > 65535 || cfg.Timeout <= 0 {
		return fail(fmt.Errorf("端口必须在 1–65535，超时必须大于零"))
	}
	f.Visit(func(v *flag.Flag) {
		if v.Name == "password" {
			cfg.HasPassword = true
		}
	})
	if !cfg.HasPassword {
		cfg.Password, cfg.HasPassword = os.LookupEnv("REMOTE_SHELL_PASSWORD")
	}
	if strings.ContainsAny(cfg.Password, "\r\n") {
		return fail(fmt.Errorf("密码不能包含换行符"))
	}
	var err error
	cfg.Dir, err = runtimeDir()
	if err != nil {
		return fail(err)
	}
	cfg.SSH, err = exec.LookPath("ssh")
	if err != nil {
		return fail(fmt.Errorf("找不到本机 OpenSSH 客户端: %w", err))
	}
	if cfg.Identity != "" {
		cfg.Identity, err = filepath.Abs(cfg.Identity)
		if err != nil {
			return fail(err)
		}
	}
	if err := os.MkdirAll(cfg.Dir, 0700); err != nil {
		return fail(err)
	}
	if err := os.Chmod(cfg.Dir, 0700); err != nil {
		return fail(err)
	}
	exe, err := os.Executable()
	if err != nil {
		return fail(err)
	}
	logFile, err := os.OpenFile(filepath.Join(cfg.Dir, "daemon.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return fail(err)
	}
	defer logFile.Close()
	cmd := exec.Command(exe, "--daemon")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	// Credentials cross an anonymous pipe, not daemon argv or the filesystem.
	data, err := json.Marshal(cfg)
	if err != nil {
		return fail(err)
	}
	cmd.Stdin = strings.NewReader(string(data))
	cmd.Stderr = logFile
	for _, env := range os.Environ() {
		if !strings.HasPrefix(env, "REMOTE_SHELL_PASSWORD=") {
			cmd.Env = append(cmd.Env, env)
		}
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fail(err)
	}
	if err := cmd.Start(); err != nil {
		return fail(err)
	}
	result := make(chan error, 1)
	go func() {
		var p packet
		err := json.NewDecoder(stdout).Decode(&p)
		if err == nil && p.Error != "" {
			err = fmt.Errorf("%s", p.Error)
		}
		result <- err
	}()
	select {
	case err = <-result:
	case <-time.After(cfg.Timeout + 15*time.Second):
		err = fmt.Errorf("本地服务启动超时")
	}
	stdout.Close()
	if err != nil {
		cmd.Process.Signal(syscall.SIGTERM)
		cmd.Wait()
		return fail(err)
	}
	cmd.Process.Release()
	fmt.Printf("远程 Shell 已连接：%s:%d\n", cfg.Host, cfg.Port)
	return 0
}

func Execute(args []string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprintln(os.Stdout, "用法：remote-shell 命令 [参数...]\n      remote-shell -c 'Shell 表达式'\n标准输入、标准输出、标准错误和退出码会转发；不分配终端。")
		if len(args) == 0 {
			return 2
		}
		return 0
	}
	command := ""
	if args[0] == "-c" {
		if len(args) != 2 {
			return fail(fmt.Errorf("-c 需要一个完整的 Shell 表达式"))
		}
		command = args[1]
	} else {
		if args[0] == "--" {
			args = args[1:]
		}
		if len(args) == 0 {
			return fail(fmt.Errorf("缺少命令"))
		}
		command = quoteArgs(args)
	}
	dir, err := runtimeDir()
	if err != nil {
		return fail(err)
	}
	conn, err := dial(dir)
	if err != nil {
		return fail(err)
	}
	defer conn.Close()
	if err := json.NewEncoder(conn).Encode(request{Action: "exec", Command: command}); err != nil {
		return fail(err)
	}
	// Closing the client connection also cancels its SSH channel on the daemon.
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(signals)
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-signals:
			conn.Close()
		case <-done:
		}
	}()
	go copyInput(conn)
	dec := json.NewDecoder(conn)
	for {
		var p packet
		if err := dec.Decode(&p); err != nil {
			fmt.Fprintln(os.Stderr, "remote-shell: 连接中断:", err)
			return 255
		}
		var writeErr error
		switch p.Type {
		case "stdout":
			_, writeErr = os.Stdout.Write(p.Data)
		case "stderr":
			_, writeErr = os.Stderr.Write(p.Data)
		case "exit":
			if p.Error != "" {
				fmt.Fprintln(os.Stderr, "remote-shell:", p.Error)
			}
			return p.Code
		case "error":
			return fail(fmt.Errorf("%s", p.Error))
		}
		if writeErr != nil {
			return fail(writeErr)
		}
	}
}

func copyInput(conn net.Conn) {
	enc := json.NewEncoder(conn)
	buf := make([]byte, 32*1024)
	for {
		n, err := os.Stdin.Read(buf)
		if n > 0 {
			if enc.Encode(packet{Type: "stdin", Data: buf[:n]}) != nil {
				return
			}
		}
		if err != nil {
			if err != io.EOF {
				fmt.Fprintln(os.Stderr, "remote-shell: 读取标准输入:", err)
			}
			enc.Encode(packet{Type: "eof"})
			return
		}
	}
}

func Info(args []string) int {
	f := flag.NewFlagSet("remote-shell-info", flag.ContinueOnError)
	asJSON := f.Bool("json", false, "输出 JSON")
	if err := f.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if f.NArg() != 0 {
		return fail(fmt.Errorf("不支持位置参数"))
	}
	p, err := exchange("info")
	if err != nil {
		return fail(err)
	}
	if p.Info == nil {
		return fail(fmt.Errorf("服务返回了无效连接信息"))
	}
	i := p.Info
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(i); err != nil {
			return fail(err)
		}
	} else {
		status := "已断开"
		if i.Connected {
			status = "已连接"
		}
		fmt.Printf("连接状态：%s\n远端地址：%s@%s:%d\n系统版本：%s\n内核版本：%s\n架构：%s\n主机名：%s\nShell：%s\n服务 PID：%d\n启动时间：%s\n", status, i.User, i.Host, i.Port, i.OSVersion, i.Kernel, i.Architecture, i.Hostname, i.Shell, i.PID, i.StartedAt.Local().Format(time.RFC3339))
		if i.Error != "" {
			fmt.Fprintln(os.Stdout, "错误："+i.Error)
		}
	}
	if !i.Connected {
		return 1
	}
	return 0
}

func Stop(args []string) int {
	if len(args) != 0 {
		if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
			fmt.Println("用法：stop-remote-shell")
			return 0
		}
		return fail(fmt.Errorf("stop-remote-shell 不接受参数"))
	}
	_, err := exchange("stop")
	if err != nil {
		return fail(err)
	}
	// Wait for cleanup and lock release so an immediate start is reliable.
	dir, _ := runtimeDir()
	lock, err := os.OpenFile(filepath.Join(dir, "daemon.lock"), os.O_RDWR, 0600)
	if err != nil {
		return fail(err)
	}
	defer lock.Close()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
			syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
			break
		}
		if time.Now().After(deadline) {
			return fail(fmt.Errorf("等待服务退出超时"))
		}
		time.Sleep(50 * time.Millisecond)
	}
	fmt.Println("远程 Shell 服务已停止")
	return 0
}

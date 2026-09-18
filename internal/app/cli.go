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
	"sort"
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
	var configPath, connectionName string
	allFlag := false
	f := flag.NewFlagSet("start-remote-shell", flag.ContinueOnError)
	f.StringVar(&configPath, "config", "", "配置文件路径")
	f.StringVar(&connectionName, "conn", "", "连接名称（TOML 配置中的名称）")
	f.StringVar(&connectionName, "connection", "", "连接名称（-conn 的全称别名）")
	f.BoolVar(&allFlag, "all", false, "启动配置文件中的所有连接")
	f.StringVar(&cfg.Host, "host", "", "远端主机名、IP 或 SSH config 别名（无配置文件时必填）")
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
	if f.NArg() != 0 {
		return fail(fmt.Errorf("start-remote-shell 不接受位置参数"))
	}

	// Validate --all is not combined with per-connection flags.
	if allFlag {
		used := connectionName != ""
		f.Visit(func(v *flag.Flag) {
			switch v.Name {
			case "host", "user", "port", "password", "identity", "timeout", "conn", "connection":
				used = true
			}
		})
		if used {
			return fail(fmt.Errorf("--all 不能与 -conn 等单连接参数同时使用"))
		}
	}

	// Find and parse config file.
	configFile, err := findConfig(configPath)
	if err != nil {
		return fail(err)
	}
	if configFile != "" && configPath != "" && configFile != configPath {
		configFile = configPath
	}

	// --all mode: start every connection defined in the config file.
	if allFlag {
		if configFile == "" {
			return fail(fmt.Errorf("没有找到配置文件，无法使用 --all"))
		}
		fc, err := loadConfigFile(configFile)
		if err != nil {
			return fail(err)
		}
		if len(fc.Connections) == 0 {
			return fail(fmt.Errorf("配置文件中没有定义任何连接"))
		}
		var names []string
		for name := range fc.Connections {
			names = append(names, name)
		}
		sort.Strings(names)
		ok, skip, failCount := 0, 0, 0
		dir, _ := runtimeDir()
		for _, name := range names {
			// Check if already running.
			p, err := exchangeNamed("info", name)
			if err == nil && p.Info != nil && p.Info.Connected {
				fmt.Printf("  [%s] 已在运行，跳过\n", name)
				skip++
				continue
			}
			tomlCfg, err := lookupConnection(fc, name)
			if err != nil {
				fmt.Fprintf(os.Stderr, "  [%s] 启动失败：%v\n", name, err)
				failCount++
				continue
			}
			tomlCfg.Dir = dir
			if err := spawnDaemon(tomlCfg); err != nil {
				fmt.Fprintf(os.Stderr, "  [%s] 启动失败：%v\n", name, err)
				failCount++
				continue
			}
			fmt.Printf("  [%s] %s:%d\n", name, tomlCfg.Host, tomlCfg.Port)
			ok++
		}
		fmt.Printf("启动完成：成功 %d，跳过 %d，失败 %d\n", ok, skip, failCount)
		if failCount > 0 {
			return 1
		}
		return 0
	}

	// Single-connection mode.
	if configFile != "" {
		fc, err := loadConfigFile(configFile)
		if err != nil {
			return fail(err)
		}
		if connectionName == "" {
			if len(fc.Connections) == 1 {
				for name := range fc.Connections {
					connectionName = name
				}
			} else if len(fc.Connections) > 1 {
				return fail(fmt.Errorf("配置文件包含多个连接，请用 -conn 指定名称"))
			} else {
				return fail(fmt.Errorf("配置文件中没有定义任何连接"))
			}
		}
		if err := validateName(connectionName); err != nil {
			return fail(err)
		}
		cfg.Name = connectionName
		tomlCfg, err := lookupConnection(fc, connectionName)
		if err != nil {
			return fail(err)
		}
		if cfg.Host == "" {
			cfg.Host = tomlCfg.Host
		}
		if cfg.User == "" {
			cfg.User = tomlCfg.User
		}
		if !isFlagSet(f, "port") {
			cfg.Port = tomlCfg.Port
		}
		if cfg.Password == "" {
			cfg.Password = tomlCfg.Password
			cfg.HasPassword = tomlCfg.HasPassword
		}
		if cfg.Identity == "" {
			cfg.Identity = tomlCfg.Identity
		}
		if !isFlagSet(f, "timeout") {
			cfg.Timeout = tomlCfg.Timeout
		}
		if cfg.ShellOverride == "" {
			cfg.ShellOverride = tomlCfg.ShellOverride
		}
	} else {
		if cfg.Host == "" || strings.HasPrefix(cfg.Host, "-") || strings.ContainsAny(cfg.Host, "\x00\r\n \t") {
			return fail(fmt.Errorf("请用 -host 指定有效主机，或创建配置文件"))
		}
		cfg.Name = connectionName
	}

	// Validate connection name.
	if cfg.Name != "" {
		if err := validateName(cfg.Name); err != nil {
			return fail(err)
		}
	}

	// Validate host and credentials.
	if cfg.Host == "" || strings.HasPrefix(cfg.Host, "-") || strings.ContainsAny(cfg.Host, "\x00\r\n \t") {
		return fail(fmt.Errorf("无效主机: %q", cfg.Host))
	}
	if cfg.Port < 1 || cfg.Port > 65535 || cfg.Timeout <= 0 {
		return fail(fmt.Errorf("端口必须在 1–65535，超时必须大于零"))
	}
	if isFlagSet(f, "password") {
		cfg.HasPassword = true
	}
	if !cfg.HasPassword {
		cfg.Password, cfg.HasPassword = os.LookupEnv("REMOTE_SHELL_PASSWORD")
	}
	if strings.ContainsAny(cfg.Password, "\r\n") {
		return fail(fmt.Errorf("密码不能包含换行符"))
	}

	if err := spawnDaemon(&cfg); err != nil {
		return fail(err)
	}
	if cfg.Name != "" {
		fmt.Printf("远程 Shell 已连接 [%s]：%s:%d\n", cfg.Name, cfg.Host, cfg.Port)
	} else {
		fmt.Printf("远程 Shell 已连接：%s:%d\n", cfg.Host, cfg.Port)
	}
	return 0
}

// spawnDaemon resolves runtime paths, creates the runtime directory, and
// spawns a daemon process. It returns an error if any step fails.
func spawnDaemon(cfg *config) error {
	var err error
	cfg.Dir, err = runtimeDir()
	if err != nil {
		return err
	}
	if err := checkPathLength(cfg.Dir, cfg.Name); err != nil {
		return err
	}
	cfg.SSH, err = exec.LookPath("ssh")
	if err != nil {
		return fmt.Errorf("找不到本机 OpenSSH 客户端: %w", err)
	}
	if cfg.Identity != "" {
		cfg.Identity, err = filepath.Abs(cfg.Identity)
		if err != nil {
			return err
		}
	}
	if err := os.MkdirAll(cfg.Dir, 0700); err != nil {
		return err
	}
	if err := os.Chmod(cfg.Dir, 0700); err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	logFile, err := os.OpenFile(logPath(cfg.Dir, cfg.Name), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer logFile.Close()
	cmd := exec.Command(exe, "--daemon")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	data, err := json.Marshal(cfg)
	if err != nil {
		return err
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
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
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
		return err
	}
	cmd.Process.Release()
	return nil
}

// isFlagSet checks if a flag was explicitly set by the user.
func isFlagSet(f *flag.FlagSet, name string) bool {
	found := false
	f.Visit(func(v *flag.Flag) {
		if v.Name == name {
			found = true
		}
	})
	return found
}

// parseConnectionFlag scans args for -conn/-connection/-name and returns it
// plus the remaining args (without the flag and its value).
func parseConnectionFlag(args []string) (string, []string) {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "-conn" || args[i] == "--conn" ||
			args[i] == "-connection" || args[i] == "--connection" ||
			args[i] == "-name" || args[i] == "--name" {
			name := args[i+1]
			remaining := make([]string, 0, len(args)-2)
			remaining = append(remaining, args[:i]...)
			remaining = append(remaining, args[i+2:]...)
			return name, remaining
		}
		if strings.HasPrefix(args[i], "-conn=") || strings.HasPrefix(args[i], "--conn=") ||
			strings.HasPrefix(args[i], "-connection=") || strings.HasPrefix(args[i], "--connection=") ||
			strings.HasPrefix(args[i], "-name=") || strings.HasPrefix(args[i], "--name=") {
			parts := strings.SplitN(args[i], "=", 2)
			name := parts[1]
			remaining := make([]string, 0, len(args)-1)
			remaining = append(remaining, args[:i]...)
			remaining = append(remaining, args[i+1:]...)
			return name, remaining
		}
	}
	return "", args
}

func Execute(args []string) int {
	connName, remaining := parseConnectionFlag(args)
	args = remaining

	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		fmt.Fprintln(os.Stdout, "用法：remote-shell -conn 名称 命令 [参数...]\n      remote-shell -conn 名称 -c 'Shell 表达式'\n标准输入、标准输出、标准错误和退出码会转发；不分配终端。")
		if len(args) == 0 {
			return 2
		}
		return 0
	}
	if connName == "" {
		return fail(fmt.Errorf("必须用 -conn 指定连接名称；可用 remote-shell-info --all 查看运行中的连接"))
	}
	command := ""
	isCMode := args[0] == "-c"
	if isCMode {
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

	// Windows guard: non -c mode uses POSIX quoting which does not work on
	// Windows default shells (cmd.exe / powershell.exe). Detect and advise.
	if !isCMode {
		p, err := exchangeNamed("info", connName)
		if err == nil && p.Info != nil && p.Info.OS == "Windows_NT" {
			return fail(fmt.Errorf("远端默认 Shell 是 Windows (%s)，请用 -c 模式编写对应 Shell 的命令：\n  cmd:    remote-shell -c 'dir C:\\\\'\n  powershell: remote-shell -c 'Get-Process'", p.Info.DefaultShell))
		}
	}

	conn, err := dialNamed(dir, connName)
	if err != nil {
		return fail(err)
	}
	defer conn.Close()
	if err := json.NewEncoder(conn).Encode(request{Action: "exec", Name: connName, Command: command}); err != nil {
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
	connName := f.String("conn", "", "指定连接名称")
	f.StringVar(connName, "connection", "", "指定连接名称（-conn 的全称别名）")
	allFlag := f.Bool("all", false, "列出所有连接")
	if err := f.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if f.NArg() != 0 {
		return fail(fmt.Errorf("不支持位置参数"))
	}
	if *allFlag {
		return infoAll(*asJSON)
	}
	name := *connName
	if name == "" {
		// No flag: auto-select if single connection.
		dir, err := runtimeDir()
		if err != nil {
			return fail(err)
		}
		names := scanConnections(dir)
		if len(names) == 1 {
			name = names[0]
		} else if len(names) > 1 {
			return fail(fmt.Errorf("存在多个连接，请用 -conn 指定名称"))
		}
		// If 0 names, try legacy single connection (empty name).
	}
	return infoOne(name, *asJSON)
}

func infoOne(name string, asJSON bool) int {
	p, err := exchangeNamed("info", name)
	if err != nil {
		return fail(err)
	}
	if p.Info == nil {
		return fail(fmt.Errorf("服务返回了无效连接信息"))
	}
	i := p.Info
	if asJSON {
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
		if i.Name != "" {
			fmt.Printf("[%s] ", i.Name)
		}
		fmt.Printf("连接状态：%s\n远端地址：%s@%s:%d\n系统版本：%s\n内核版本：%s\n架构：%s\n主机名：%s\nShell：%s\n", status, i.User, i.Host, i.Port, i.OSVersion, i.Kernel, i.Architecture, i.Hostname, i.Shell)
		if i.DefaultShell != "" && i.DefaultShell != i.Shell {
			fmt.Printf("默认 Shell：%s\n", i.DefaultShell)
		}
		fmt.Printf("服务 PID：%d\n启动时间：%s\n", i.PID, i.StartedAt.Local().Format(time.RFC3339))
		if i.Error != "" {
			fmt.Fprintln(os.Stdout, "错误："+i.Error)
		}
	}
	if !i.Connected {
		return 1
	}
	return 0
}

func infoAll(asJSON bool) int {
	dir, err := runtimeDir()
	if err != nil {
		return fail(err)
	}
	names := scanConnections(dir)
	if len(names) == 0 {
		if asJSON {
			fmt.Println("[]")
		} else {
			fmt.Println("没有运行中的连接")
		}
		return 1
	}
	if !asJSON {
		fmt.Printf("运行中的连接（%d）：\n", len(names))
	}
	infos := make([]*connectionInfo, 0, len(names))
	var lastErr int
	for _, name := range names {
		p, err := exchangeNamed("info", name)
		if err != nil {
			lastErr = 1
			continue
		}
		if p.Info != nil {
			infos = append(infos, p.Info)
		}
	}
	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(infos); err != nil {
			return fail(err)
		}
	} else {
		for _, i := range infos {
			status := "已断开"
			if i.Connected {
				status = "已连接"
			}
			fmt.Printf("  [%s] %s  %s@%s:%d  %s  shell=%s\n", i.Name, status, i.User, i.Host, i.Port, i.OSVersion, i.DefaultShell)
		}
	}
	return lastErr
}

// scanConnections lists connection names by looking for *.service.sock files
// in the runtime directory.
func scanConnections(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, ".service.sock") {
			names = append(names, strings.TrimSuffix(name, ".service.sock"))
		}
	}
	// Also check legacy single-connection socket.
	if _, err := os.Stat(filepath.Join(dir, "service.sock")); err == nil {
		hasDefault := false
		for _, n := range names {
			if n == "default" || n == "" {
				hasDefault = true
				break
			}
		}
		if !hasDefault {
			names = append(names, "default")
		}
	}
	return names
}

func Stop(args []string) int {
	f := flag.NewFlagSet("stop-remote-shell", flag.ContinueOnError)
	connName := f.String("conn", "", "指定连接名称")
	f.StringVar(connName, "connection", "", "指定连接名称（-conn 的全称别名）")
	allFlag := f.Bool("all", false, "停止所有连接")
	if err := f.Parse(args); err != nil {
		if err == flag.ErrHelp {
			fmt.Println("用法：stop-remote-shell [-conn 名称] [--all]")
			return 0
		}
		return 2
	}
	if f.NArg() != 0 {
		return fail(fmt.Errorf("stop-remote-shell 不接受位置参数"))
	}
	if *allFlag {
		return stopAll()
	}
	name := *connName
	if name == "" {
		dir, err := runtimeDir()
		if err != nil {
			return fail(err)
		}
		names := scanConnections(dir)
		if len(names) == 1 {
			name = names[0]
		} else if len(names) > 1 {
			return fail(fmt.Errorf("存在多个连接，请用 -conn 指定名称"))
		}
	}
	return stopOne(name)
}

func stopOne(name string) int {
	_, err := exchangeNamed("stop", name)
	if err != nil {
		return fail(err)
	}
	// Wait for cleanup and lock release so an immediate start is reliable.
	dir, _ := runtimeDir()
	lock, err := os.OpenFile(lockPath(dir, name), os.O_RDWR, 0600)
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
	if name != "" {
		fmt.Printf("远程 Shell 服务 [%s] 已停止\n", name)
	} else {
		fmt.Println("远程 Shell 服务已停止")
	}
	return 0
}

func stopAll() int {
	dir, err := runtimeDir()
	if err != nil {
		return fail(err)
	}
	names := scanConnections(dir)
	if len(names) == 0 {
		fmt.Println("没有运行中的连接")
		return 0
	}
	var lastErr int
	for _, name := range names {
		if rc := stopOne(name); rc != 0 {
			fmt.Fprintf(os.Stderr, "remote-shell: 停止 %s 失败\n", name)
			lastErr = 1
		}
	}
	return lastErr
}

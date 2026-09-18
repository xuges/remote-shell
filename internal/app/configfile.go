package app

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// FileConfig represents the TOML configuration file structure.
type FileConfig struct {
	Connections map[string]*ConnectionConfig `toml:"connections"`
}

// ConnectionConfig represents a single named connection entry.
type ConnectionConfig struct {
	Host     string `toml:"host"`
	User     string `toml:"user"`
	Port     int    `toml:"port"`
	Password string `toml:"password"`
	Identity string `toml:"identity"`
	Timeout  string `toml:"timeout"`
	Shell    string `toml:"shell"`
}

var validName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

// validateName checks a connection name for validity.
func validateName(name string) error {
	if name == "" {
		return fmt.Errorf("连接名称不能为空")
	}
	if !validName.MatchString(name) {
		return fmt.Errorf("连接名称 %q 无效；允许小写字母、数字、下划线和连字符，长度 1-32", name)
	}
	return nil
}

// loadConfigFile parses a TOML config file at the given path.
func loadConfigFile(path string) (*FileConfig, error) {
	var fc FileConfig
	if _, err := toml.DecodeFile(path, &fc); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}
	if fc.Connections == nil {
		fc.Connections = make(map[string]*ConnectionConfig)
	}
	return &fc, nil
}

// findConfig resolves the config file path using:
// 1. explicitPath (--config flag)  2. REMOTE_SHELL_CONFIG env
// 3. ~/.config/remote-shell/config.toml  4. ./remote-shell.toml
func findConfig(explicitPath string) (string, error) {
	if explicitPath != "" {
		abs, err := filepath.Abs(explicitPath)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(abs); err != nil {
			return "", fmt.Errorf("配置文件不存在: %s", abs)
		}
		return abs, nil
	}
	if env := os.Getenv("REMOTE_SHELL_CONFIG"); env != "" {
		abs, err := filepath.Abs(env)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(abs); err != nil {
			return "", fmt.Errorf("配置文件不存在: %s", abs)
		}
		return abs, nil
	}
	if home, err := os.UserHomeDir(); err == nil {
		p := filepath.Join(home, ".config", "remote-shell", "config.toml")
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	if _, err := os.Stat("remote-shell.toml"); err == nil {
		return "remote-shell.toml", nil
	}
	return "", nil
}

// lookupConnection finds a named connection in the config file and converts
// it to a config struct (without Dir/SSH which are runtime-injected).
func lookupConnection(fc *FileConfig, name string) (*config, error) {
	cc, ok := fc.Connections[name]
	if !ok {
		return nil, fmt.Errorf("连接 %q 未在配置文件中定义", name)
	}
	return connectionFromTOML(name, cc)
}

// connectionFromTOML builds a config from a TOML entry.
func connectionFromTOML(name string, cc *ConnectionConfig) (*config, error) {
	if cc.Host == "" {
		return nil, fmt.Errorf("连接 %q 缺少 host", name)
	}
	if cc.Port == 0 {
		cc.Port = 22
	}
	if cc.Port < 1 || cc.Port > 65535 {
		return nil, fmt.Errorf("连接 %q 的端口 %d 无效（1–65535）", name, cc.Port)
	}
	timeout := 20 * time.Second
	if cc.Timeout != "" {
		if d, err := time.ParseDuration(cc.Timeout); err == nil {
			timeout = d
		} else {
			return nil, fmt.Errorf("连接 %q 的 timeout 格式无效: %w", name, err)
		}
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("连接 %q 的 timeout 必须大于零", name)
	}
	if strings.ContainsAny(cc.Password, "\r\n") {
		return nil, fmt.Errorf("连接 %q 的密码不能包含换行符", name)
	}
	if cc.Shell != "" && cc.Shell != "cmd" && cc.Shell != "powershell" {
		return nil, fmt.Errorf("连接 %q 的 shell 必须是 cmd 或 powershell，当前值: %q", name, cc.Shell)
	}
	identity := ""
	if cc.Identity != "" {
		abs, err := filepath.Abs(cc.Identity)
		if err != nil {
			return nil, fmt.Errorf("连接 %q 的 identity 路径无效: %w", name, err)
		}
		identity = abs
	}
	return &config{
		Name:          name,
		Host:          cc.Host,
		User:          cc.User,
		Port:          cc.Port,
		Password:      cc.Password,
		HasPassword:   cc.Password != "",
		Identity:      identity,
		Timeout:       timeout,
		ShellOverride: cc.Shell,
	}, nil
}

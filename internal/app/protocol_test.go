package app

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func TestQuoteArgs(t *testing.T) {
	args := []string{"", "a b", "one'two", `"quoted"`, "line\nbreak", "$(touch SHOULD_NOT_EXIST)", "; exit 99", "*", "中文", `a\b`}
	// Let a real POSIX shell parse the generated command and compare argv.
	out, err := exec.Command("sh", "-c", "printf '%s\\000' "+quoteArgs(args)).Output()
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00")
	if !reflect.DeepEqual(got, args) {
		t.Fatalf("argv = %#v, want %#v", got, args)
	}
}

func TestConcurrentStreams(t *testing.T) {
	var buf bytes.Buffer
	s := &sender{enc: json.NewEncoder(&buf)}
	var wg sync.WaitGroup
	for _, kind := range []string{"stdout", "stderr"} {
		wg.Add(1)
		go func(kind string) {
			defer wg.Done()
			w := streamWriter{s, kind}
			for i := 0; i < 100; i++ {
				if _, err := w.Write([]byte{0, 255, '\n'}); err != nil {
					t.Error(err)
				}
			}
		}(kind)
	}
	wg.Wait()
	dec := json.NewDecoder(&buf)
	counts := map[string]int{}
	for i := 0; i < 200; i++ {
		var p packet
		if err := dec.Decode(&p); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(p.Data, []byte{0, 255, '\n'}) {
			t.Fatalf("corrupt data: %v", p.Data)
		}
		counts[p.Type]++
	}
	if counts["stdout"] != 100 || counts["stderr"] != 100 {
		t.Fatal(counts)
	}
}

func TestBoundedDiagnostics(t *testing.T) {
	var b boundedBuffer
	b.Write(bytes.Repeat([]byte("x"), 20000))
	b.Write([]byte("tail"))
	if got := b.String(); len(got) != 16384 || !strings.HasSuffix(got, "tail") {
		t.Fatal("diagnostics must retain a bounded tail")
	}
}

func TestRuntimeDir(t *testing.T) {
	t.Setenv("REMOTE_SHELL_DIR", strings.Repeat("a", 100))
	if _, err := runtimeDir(); err == nil {
		t.Fatal("accepted path exceeding Unix socket limit")
	}
}

func TestValidateName(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"prod", false},
		{"win-server", false},
		{"my_vm_1", false},
		{"a", false},
		{"0", false},
		{"", true},
		{"PROD", true},
		{"has space", true},
		{"has\x00null", true},
		{"very-long-connection-name-thats-more-than-32", true},
		{"special!chars", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateName(tt.name)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateName(%q) error = %v, wantErr %v", tt.name, err, tt.wantErr)
			}
		})
	}
}

func TestParseProbeCRLF(t *testing.T) {
	// Linux LF format
	inputLF := []byte("\x00REMOTE_SHELL_INFO\x00Linux\n5.15.0\nx86_64\nhost1\n/bin/bash\nuser1\nUbuntu 22.04\n")
	var info connectionInfo
	if err := parseProbe(inputLF, &info); err != nil {
		t.Fatal(err)
	}
	if info.OS != "Linux" || info.Kernel != "5.15.0" || info.Architecture != "x86_64" || info.DefaultShell != "" {
		t.Fatalf("unexpected info: %+v", info)
	}

	// Windows CRLF format
	inputCRLF := []byte("\x00REMOTE_SHELL_INFO\x00Windows_NT\r\n10.0.19045\r\nAMD64\r\nWIN-HOST\r\nDesktop\r\nadmin\r\nMicrosoft Windows 10 Pro\r\n")
	var info2 connectionInfo
	if err := parseProbe(inputCRLF, &info2); err != nil {
		t.Fatal(err)
	}
	if info2.OS != "Windows_NT" || info2.Kernel != "10.0.19045" || info2.Architecture != "AMD64" || info2.Hostname != "WIN-HOST" {
		t.Fatalf("unexpected info: %+v", info2)
	}

	// Missing NUL marker
	if err := parseProbe([]byte("no marker"), &info); err == nil {
		t.Fatal("expected error for missing NUL marker")
	}

	// Too few fields
	if err := parseProbe([]byte("\x00REMOTE_SHELL_INFO\x00Linux\n5.15\n"), &info); err == nil {
		t.Fatal("expected error for too few fields")
	}
}

func TestConnectionFromTOML(t *testing.T) {
	tests := []struct {
		name       string
		cc         ConnectionConfig
		wantShell  string
		wantErr    bool
		wantErrStr string
	}{
		{
			name:      "no shell",
			cc:        ConnectionConfig{Host: "1.2.3.4"},
			wantShell: "",
		},
		{
			name:      "cmd",
			cc:        ConnectionConfig{Host: "1.2.3.4", Shell: "cmd"},
			wantShell: "cmd",
		},
		{
			name:      "powershell",
			cc:        ConnectionConfig{Host: "1.2.3.4", Shell: "powershell"},
			wantShell: "powershell",
		},
		{
			name:       "invalid shell",
			cc:         ConnectionConfig{Host: "1.2.3.4", Shell: "zsh"},
			wantErr:    true,
			wantErrStr: "shell 必须是 cmd 或 powershell",
		},
		{
			name:       "missing host",
			cc:         ConnectionConfig{},
			wantErr:    true,
			wantErrStr: "缺少 host",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := connectionFromTOML("test", &tt.cc)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				if !strings.Contains(err.Error(), tt.wantErrStr) {
					t.Errorf("error = %q, want containing %q", err.Error(), tt.wantErrStr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.ShellOverride != tt.wantShell {
				t.Errorf("ShellOverride = %q, want %q", cfg.ShellOverride, tt.wantShell)
			}
		})
	}
}

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

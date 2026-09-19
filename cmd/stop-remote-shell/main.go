package main

import (
	"fmt"
	"os"
	"path/filepath"
	"remote-shell/internal/app"
)

// version is injected at build time via -ldflags "-X main.version=<tag>".
var version = "dev"

func main() {
	if len(os.Args) == 2 && (os.Args[1] == "-version" || os.Args[1] == "--version") {
		fmt.Printf("%s %s\n", filepath.Base(os.Args[0]), version)
		return
	}
	os.Exit(app.Stop(os.Args[1:]))
}
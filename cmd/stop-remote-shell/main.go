package main

import (
	"os"
	"remote-shell/internal/app"
)

func main() { os.Exit(app.Stop(os.Args[1:])) }

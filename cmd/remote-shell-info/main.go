package main

import (
	"os"
	"remote-shell/internal/app"
)

func main() { os.Exit(app.Info(os.Args[1:])) }

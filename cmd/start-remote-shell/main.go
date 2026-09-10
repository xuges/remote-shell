package main

import (
	"os"
	"remote-shell/internal/app"
)

func main() { os.Exit(app.Start(os.Args[1:])) }

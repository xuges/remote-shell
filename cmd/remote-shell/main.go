package main

import (
	"os"
	"remote-shell/internal/app"
)

func main() { os.Exit(app.Execute(os.Args[1:])) }

//go:build windows

package app

import (
	"syscall"

	"golang.org/x/sys/windows"
)

// daemonSysProcAttr starts the daemon in a detached process so it can outlive
// the launching terminal.
func daemonSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: windows.DETACHED_PROCESS}
}
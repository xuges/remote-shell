//go:build !windows

package app

import "syscall"

// daemonSysProcAttr detaches the daemon from the controlling terminal by
// starting it in its own session.
func daemonSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
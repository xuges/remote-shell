//go:build !windows

package app

import (
	"os"
	"syscall"
)

// flockTryLock acquires a non-blocking exclusive advisory lock. It returns an
// error if the lock is already held.
func flockTryLock(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}

// flockUnlock releases the lock held on f.
func flockUnlock(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
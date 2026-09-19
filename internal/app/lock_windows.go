//go:build windows

package app

import (
	"os"

	"golang.org/x/sys/windows"
)

// flockTryLock acquires a non-blocking exclusive lock on the first byte of the
// file via LockFileEx. It returns an error if the lock is already held.
func flockTryLock(f *os.File) error {
	var ol windows.Overlapped
	return windows.LockFileEx(windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0, 1, 0, &ol)
}

// flockUnlock releases the lock held on f.
func flockUnlock(f *os.File) error {
	var ol windows.Overlapped
	return windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, &ol)
}
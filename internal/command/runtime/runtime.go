// Package runtime coordinates access to files in the runtime directory.
package runtime

import (
	"fmt"
	"os"
	"syscall"
)

// WithLock runs callback while holding an exclusive lock on runtimeDir.
func WithLock(runtimeDir string, callback func() error) error {
	dir, err := Lock(runtimeDir)
	if err != nil {
		return err
	}
	defer Unlock(dir)
	return callback()
}

// Lock acquires an exclusive advisory lock on runtimeDir and returns the open
// directory handle that holds the lock. The caller must pass the handle to Unlock.
func Lock(runtimeDir string) (*os.File, error) {
	dir, err := os.Open(runtimeDir)
	if err != nil {
		return nil, fmt.Errorf("cannot lock runtime directory: %w", err)
	}

	for {
		err = syscall.Flock(int(dir.Fd()), syscall.LOCK_EX)
		if err != syscall.EINTR {
			break
		}
	}
	if err != nil {
		dir.Close()
		return nil, fmt.Errorf("cannot lock runtime directory: %w", err)
	}

	return dir, nil
}

// Unlock releases the advisory lock held by dir and closes the directory handle.
// It reports failures to release the lock to standard error.
func Unlock(dir *os.File) {
	if err := syscall.Flock(int(dir.Fd()), syscall.LOCK_UN); err != nil {
		fmt.Fprintf(os.Stderr, "talk2text-nvim: cannot unlock runtime directory: %s\n", err)
	}
	dir.Close()
}

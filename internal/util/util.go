// Package util provides small shared helpers for the talk2text integration.
package util

import (
	"os"
	"os/exec"
	"syscall"
)

// RemovePath removes path and treats an already-missing path as success.
func RemovePath(path string) error {
	err := os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// RunCmdDetached starts a command in a new session with inherited stderr and
// releases its process handle instead of waiting for it.
func RunCmdDetached(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stderr, cmd.SysProcAttr = os.Stderr, &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

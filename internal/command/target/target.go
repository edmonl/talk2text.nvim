// Package target reads and validates Neovim target files.
package target

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/edmonl/talk2text.nvim/internal/command/runtime"
	"github.com/edmonl/talk2text.nvim/internal/util"
)

const (
	NormalTarget  = "nvim-target"
	DefaultTarget = "default-nvim-target"
)

// Read returns the value from the named target file while holding the runtime
// directory lock. A missing target file produces an empty value without an error.
func Read(runtimeDir, targetName string) (string, error) {
	path := filepath.Join(runtimeDir, targetName)
	lock, err := runtime.Lock(runtimeDir)
	if err != nil {
		return "", err
	}
	defer runtime.Unlock(lock)
	return readTarget(path)
}

// readTarget returns the trimmed first line of path. It removes the file when
// the file cannot be read or closed, or when its first line is empty.
func readTarget(path string) (string, error) {
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", removeInvalid(path, fmt.Errorf("cannot open target file %s: %w", path, err))
	}

	line, readErr := bufio.NewReader(file).ReadString('\n')
	closeErr := file.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return "", removeInvalid(path, fmt.Errorf("cannot read target file %s: %w", path, readErr))
	}
	if closeErr != nil {
		return "", removeInvalid(path, fmt.Errorf("cannot close target file %s: %w", path, closeErr))
	}
	if line == "" && errors.Is(readErr, io.EOF) {
		return "", nil
	}

	value := strings.TrimSpace(line)
	if value == "" {
		return "", removeInvalid(path, fmt.Errorf("empty value in target file %s", path))
	}
	return value, nil
}

// removeInvalid removes an invalid target file and returns cause, augmented
// with the removal error when cleanup fails.
func removeInvalid(path string, cause error) error {
	if err := util.RemovePath(path); err != nil {
		return fmt.Errorf("%w; cannot remove invalid target file %s: %w", cause, path, err)
	}
	return cause
}

// Delete removes the named target file only when its current value still
// matches originalValue. It reports whether the file was removed.
func Delete(runtimeDir, targetName, originalValue string) (bool, error) {
	path := filepath.Join(runtimeDir, targetName)
	lock, err := runtime.Lock(runtimeDir)
	if err != nil {
		return false, err
	}
	defer runtime.Unlock(lock)

	currentValue, err := readTarget(path)
	if err != nil {
		return false, err
	}
	if currentValue != originalValue {
		return false, nil
	}
	if err := util.RemovePath(path); err != nil {
		return false, fmt.Errorf("cannot remove target file %s: %w", path, err)
	}
	return true, nil
}

package runtime

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestWithLockRunsCallback(t *testing.T) {
	want := errors.New("callback error")
	if err := WithLock(t.TempDir(), func() error { return want }); !errors.Is(err, want) {
		t.Fatalf("WithLock() error = %s, want callback error", err)
	}
}

func TestWithLockRejectsMissingDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing")
	if err := WithLock(path, func() error {
		t.Fatal("WithLock() called callback for a missing directory")
		return nil
	}); err == nil {
		t.Fatal("WithLock() succeeded for a missing directory")
	}
}

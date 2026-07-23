package util

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemovePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := RemovePath(path); err != nil {
		t.Fatalf("RemovePath() error = %s", err)
	}
	if err := RemovePath(path); err != nil {
		t.Fatalf("RemovePath() missing-path error = %s", err)
	}
}

func TestRunCmdDetachedReportsStartFailure(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if err := RunCmdDetached("missing-command"); err == nil {
		t.Fatal("RunCmdDetached() succeeded for a missing command")
	}
}

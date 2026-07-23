package target

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadTarget(t *testing.T) {
	t.Run("reports missing path as absent", func(t *testing.T) {
		value, err := readTarget(filepath.Join(t.TempDir(), "missing"))
		if err != nil {
			t.Fatalf("readTarget() error = %s", err)
		}
		if value != "" {
			t.Fatalf("readTarget() = %q, want empty", value)
		}
	})

	t.Run("accepts final line without newline", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "target")
		if err := os.WriteFile(path, []byte("\u2003socket\u00a0"), 0o600); err != nil {
			t.Fatal(err)
		}

		value, err := readTarget(path)
		if err != nil {
			t.Fatalf("readTarget() error = %s", err)
		}
		if value != "socket" {
			t.Fatalf("readTarget() = %q, want socket", value)
		}
	})

	t.Run("deletes nonempty target with blank first line", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "target")
		if err := os.WriteFile(path, []byte("\nignored"), 0o600); err != nil {
			t.Fatal(err)
		}

		value, err := readTarget(path)
		if err == nil || value != "" {
			t.Fatalf("readTarget() = %q, %s, want empty and error", value, err)
		}
		if _, statErr := os.Lstat(path); !os.IsNotExist(statErr) {
			t.Fatalf("invalid target was not removed: %s", statErr)
		}
	})

	t.Run("deletes empty directory after read failure", func(t *testing.T) {
		path := t.TempDir()
		value, err := readTarget(path)
		if err == nil || value != "" {
			t.Fatalf("readTarget() = %q, %s, want empty and error", value, err)
		}
		if _, statErr := os.Lstat(path); !os.IsNotExist(statErr) {
			t.Fatalf("invalid target directory was not removed: %s", statErr)
		}
	})
}

func TestDeletePreservesReplacement(t *testing.T) {
	runtimeDir := t.TempDir()
	path := filepath.Join(runtimeDir, NormalTarget)
	replacement := "/tmp/replacement.sock"
	if err := os.WriteFile(path, []byte(replacement+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	removed, err := Delete(runtimeDir, NormalTarget, "/tmp/stale.sock")
	if err != nil {
		t.Fatalf("Delete() error = %s", err)
	}
	if removed {
		t.Fatal("Delete() removed a replacement target")
	}
	value, err := readTarget(path)
	if err != nil {
		t.Fatalf("readTarget() error = %s", err)
	}
	if value != replacement {
		t.Fatalf("replacement target = %q, want %q", value, replacement)
	}
}

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
			t.Fatalf("readTarget() error = %v", err)
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
			t.Fatalf("readTarget() error = %v", err)
		}
		if value != "socket" {
			t.Fatalf("readTarget() = %q, want socket", value)
		}
	})

	t.Run("preserves a zero-byte target as not yet published", func(t *testing.T) {
		runtimeDir := t.TempDir()
		path := filepath.Join(runtimeDir, NormalTarget)
		if err := os.WriteFile(path, nil, 0o600); err != nil {
			t.Fatal(err)
		}

		value, err := Read(runtimeDir, NormalTarget)
		if err != nil {
			t.Fatalf("Read() error = %v", err)
		}
		if value != "" {
			t.Fatalf("Read() = %q, want empty", value)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("zero-byte target was removed: %v", err)
		}
	})

	t.Run("deletes nonempty target with blank first line", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "target")
		if err := os.WriteFile(path, []byte("\nignored"), 0o600); err != nil {
			t.Fatal(err)
		}

		value, err := readTarget(path)
		if err == nil || value != "" {
			t.Fatalf("readTarget() = %q, %v, want empty and error", value, err)
		}
		if _, statErr := os.Lstat(path); !os.IsNotExist(statErr) {
			t.Fatalf("invalid target was not removed: %v", statErr)
		}
	})

	t.Run("deletes empty directory after read failure", func(t *testing.T) {
		path := t.TempDir()
		value, err := readTarget(path)
		if err == nil || value != "" {
			t.Fatalf("readTarget() = %q, %v, want empty and error", value, err)
		}
		if _, statErr := os.Lstat(path); !os.IsNotExist(statErr) {
			t.Fatalf("invalid target directory was not removed: %v", statErr)
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
		t.Fatalf("Delete() error = %v", err)
	}
	if removed {
		t.Fatal("Delete() removed a replacement target")
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read replacement target: %v", err)
	}
	if value := string(contents); value != replacement+"\n" {
		t.Fatalf("replacement target = %q, want %q", value, replacement+"\n")
	}
}

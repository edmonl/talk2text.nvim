package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRuntimeDirectoryPreservesLiteralPath(t *testing.T) {
	root := t.TempDir()
	runtimeDir := filepath.Join(root, "runtime")
	if err := os.MkdirAll(filepath.Join(runtimeDir, "transcripts"), 0o700); err != nil {
		t.Fatal(err)
	}

	transcript := runtimeDir + "/transcripts/../transcripts/file.txt"
	got, err := runtimeDirectory(transcript)
	if err != nil {
		t.Fatalf("runtimeDirectory() error = %v", err)
	}
	want := runtimeDir + "/transcripts/.."
	if got != want {
		t.Fatalf("runtimeDirectory() = %q, want %q", got, want)
	}
}

func TestRuntimeDirectoryRejectsRoot(t *testing.T) {
	if _, err := runtimeDirectory("/transcripts/file.txt"); err == nil {
		t.Fatal("runtimeDirectory() accepted the filesystem root")
	}
}

func TestTranscriptID(t *testing.T) {
	got, err := transcriptID("/tmp/runtime/transcripts/42.txt")
	if err != nil {
		t.Fatalf("transcriptID() error = %v", err)
	}
	if got != 42 {
		t.Fatalf("transcriptID() = %d, want 42", got)
	}
}

func TestTranscriptIDRejectsMalformedNames(t *testing.T) {
	for _, path := range []string{
		"/tmp/runtime/transcripts/0.txt",
		"/tmp/runtime/transcripts/-1.txt",
		"/tmp/runtime/transcripts/01.txt",
		"/tmp/runtime/transcripts/one.txt",
		"/tmp/runtime/transcripts/1.log",
	} {
		if _, err := transcriptID(path); err == nil {
			t.Errorf("transcriptID(%q) accepted a malformed filename", path)
		}
	}
}

func TestRemoveNonDirectoryDoesNotRemoveDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "transcript")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := removeNonDirectory(dir); err == nil {
		t.Fatal("removeNonDirectory() removed or accepted a directory")
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("directory was removed: %v", err)
	}
}

func TestDetachedHookStartErrorsAreReported(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	for _, test := range []struct {
		name   string
		invoke func(*command)
	}{
		{
			name: "notification",
			invoke: func(c *command) {
				c.notify("message")
			},
		},
		{
			name: "focus",
			invoke: func(c *command) {
				c.focusDefaultEditor()
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			stderr := filepath.Join(t.TempDir(), "stderr")
			file, err := os.Create(stderr)
			if err != nil {
				t.Fatal(err)
			}

			original := os.Stderr
			os.Stderr = file
			test.invoke(&command{notifyCmd: "true", focusCmd: "true"})
			os.Stderr = original
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}

			contents, err := os.ReadFile(stderr)
			if err != nil {
				t.Fatal(err)
			}
			want := "cannot start " + test.name + " hook:"
			if !strings.Contains(string(contents), want) {
				t.Fatalf("stderr = %q, want text containing %q", contents, want)
			}
		})
	}
}

func TestDetachedHookInheritsStderr(t *testing.T) {
	const missingCommand = "talk2text-nvim-command-that-does-not-exist"
	stderr := filepath.Join(t.TempDir(), "stderr")
	file, err := os.Create(stderr)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	original := os.Stderr
	os.Stderr = file
	startErr := startDetachedHook(missingCommand)
	os.Stderr = original
	if startErr != nil {
		t.Fatalf("startDetachedHook() error = %v", startErr)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		contents, err := os.ReadFile(stderr)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(contents), missingCommand) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("detached hook stderr = %q, want text containing %q", contents, missingCommand)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

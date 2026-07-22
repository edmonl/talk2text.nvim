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

func TestHandleBlank(t *testing.T) {
	t.Run("removes transcript", func(t *testing.T) {
		transcript := filepath.Join(t.TempDir(), "transcript.txt")
		if err := os.WriteFile(transcript, nil, 0o600); err != nil {
			t.Fatal(err)
		}

		(&command{transcript: transcript}).handleBlank()
		if _, err := os.Lstat(transcript); !os.IsNotExist(err) {
			t.Fatalf("blank transcript was not removed: %v", err)
		}
	})

	t.Run("cleanup is best effort", func(t *testing.T) {
		transcript := filepath.Join(t.TempDir(), "transcript.txt")
		if err := os.Mkdir(transcript, 0o700); err != nil {
			t.Fatal(err)
		}

		stderr := captureStderr(t, func() {
			(&command{transcript: transcript}).handleBlank()
		})
		if !strings.Contains(stderr, "cannot remove transcript") {
			t.Fatalf("stderr = %q, want transcript cleanup failure", stderr)
		}
		if info, err := os.Stat(transcript); err != nil || !info.IsDir() {
			t.Fatalf("transcript directory was removed or changed: %v", err)
		}
	})
}

func TestHandleShort(t *testing.T) {
	t.Run("removes transcript and explicit target", func(t *testing.T) {
		runtimeDir := t.TempDir()
		transcript := filepath.Join(runtimeDir, "transcript.txt")
		target := filepath.Join(runtimeDir, targetName)
		defaultTarget := filepath.Join(runtimeDir, defaultTargetName)
		for _, path := range []string{transcript, target, defaultTarget} {
			if err := os.WriteFile(path, nil, 0o600); err != nil {
				t.Fatal(err)
			}
		}

		if err := (&command{runtimeDir: runtimeDir, transcript: transcript}).handleShort(); err != nil {
			t.Fatalf("handleShort() error = %v", err)
		}
		if _, err := os.Lstat(target); !os.IsNotExist(err) {
			t.Fatalf("explicit target was not removed: %v", err)
		}
		if _, err := os.Stat(defaultTarget); err != nil {
			t.Fatalf("default target was changed: %v", err)
		}
	})

	t.Run("transcript cleanup failure does not prevent target reset", func(t *testing.T) {
		runtimeDir := t.TempDir()
		transcript := filepath.Join(runtimeDir, "transcript.txt")
		target := filepath.Join(runtimeDir, targetName)
		if err := os.Mkdir(transcript, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, nil, 0o600); err != nil {
			t.Fatal(err)
		}

		captureStderr(t, func() {
			if err := (&command{runtimeDir: runtimeDir, transcript: transcript}).handleShort(); err != nil {
				t.Fatalf("handleShort() error = %v", err)
			}
		})
		if _, err := os.Lstat(target); !os.IsNotExist(err) {
			t.Fatalf("target was not reset after transcript cleanup failure: %v", err)
		}
	})

	t.Run("target reset failure happens after transcript cleanup", func(t *testing.T) {
		runtimeDir := t.TempDir()
		transcript := filepath.Join(runtimeDir, "transcript.txt")
		target := filepath.Join(runtimeDir, targetName)
		if err := os.WriteFile(transcript, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(target, 0o700); err != nil {
			t.Fatal(err)
		}

		if err := (&command{runtimeDir: runtimeDir, transcript: transcript}).handleShort(); err == nil {
			t.Fatal("handleShort() succeeded when the target could not be removed")
		}
		if _, err := os.Lstat(transcript); !os.IsNotExist(err) {
			t.Fatalf("transcript was not removed before target reset failed: %v", err)
		}
	})
}

func TestDefaultEditorInvocation(t *testing.T) {
	command := &command{nvimCmd: "nvim", transcriptID: 3}
	gotCommand, gotArgs := command.defaultEditorInvocation()
	if gotCommand != "nvim" {
		t.Fatalf("direct command = %q, want nvim", gotCommand)
	}
	wantArgs := []string{"-c", `lua require("talk2text")._default_start(3)`}
	if strings.Join(gotArgs, "\n") != strings.Join(wantArgs, "\n") {
		t.Fatalf("arguments = %q, want %q", gotArgs, wantArgs)
	}

	command.launchCmd = "launcher"
	gotCommand, _ = command.defaultEditorInvocation()
	if gotCommand != "launcher nvim" {
		t.Fatalf("launch command = %q, want %q", gotCommand, "launcher nvim")
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
			contents := captureStderr(t, func() {
				test.invoke(&command{notifyCmd: "true", focusCmd: "true"})
			})
			want := "cannot start " + test.name + " hook:"
			if !strings.Contains(contents, want) {
				t.Fatalf("stderr = %q, want text containing %q", contents, want)
			}
		})
	}
}

func captureStderr(t *testing.T, callback func()) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stderr")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}

	original := os.Stderr
	os.Stderr = file
	defer func() { os.Stderr = original }()
	callback()
	os.Stderr = original
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(contents)
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

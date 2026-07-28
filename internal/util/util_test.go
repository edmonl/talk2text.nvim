package util

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestRemovePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "target")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := RemovePath(path); err != nil {
		t.Fatalf("RemovePath() error = %v", err)
	}
	if err := RemovePath(path); err != nil {
		t.Fatalf("RemovePath() missing-path error = %v", err)
	}
}

func TestRunCmdDetachedReportsStartFailure(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if err := RunCmdDetached(exec.Command("missing-command")); err == nil {
		t.Fatal("RunCmdDetached() succeeded for a missing command")
	}
}

func TestRunCmdDetachedInheritsEnvironment(t *testing.T) {
	const (
		helperEnv    = "TALK2TEXT_TEST_DETACHED_HELPER"
		inheritedEnv = "TALK2TEXT_TEST_DETACHED_INHERITED"
		outputEnv    = "TALK2TEXT_TEST_DETACHED_OUTPUT"
	)
	if os.Getenv(helperEnv) == "1" {
		output := os.Getenv(inheritedEnv)
		outputPath := os.Getenv(outputEnv)
		pendingPath := outputPath + ".pending"
		if err := os.WriteFile(pendingPath, []byte(output), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(pendingPath, outputPath); err != nil {
			t.Fatal(err)
		}
		return
	}

	t.Setenv(helperEnv, "1")
	t.Setenv(inheritedEnv, "inherited")
	outputPath := filepath.Join(t.TempDir(), "environment")
	t.Setenv(outputEnv, outputPath)
	cmd := exec.Command(os.Args[0], "-test.run=^TestRunCmdDetachedInheritsEnvironment$")
	if err := RunCmdDetached(cmd); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		output, err := os.ReadFile(outputPath)
		if err == nil {
			if got, want := string(output), "inherited"; got != want {
				t.Fatalf("detached environment = %q, want %q", got, want)
			}
			return
		}
		if !os.IsNotExist(err) {
			t.Fatal(err)
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for detached command")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

package command

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	targetfile "github.com/edmonl/talk2text.nvim/internal/command/target"
)

func TestNewCommand(t *testing.T) {
	t.Run("uses default when notify-send is available", func(t *testing.T) {
		unsetEnvironment(t, "TALK2TEXT_NVIM_NOTIFY_CMD")
		binDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(binDir, "notify-send"), nil, 0o700); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", binDir)

		if got := New("", "", 0).notifyCmd; got != defaultNotifyCmd {
			t.Fatalf("notification command = %q, want %q", got, defaultNotifyCmd)
		}
	})

	t.Run("uses fallback configuration", func(t *testing.T) {
		unsetEnvironment(t, "TALK2TEXT_NVIM_NOTIFY_CMD")
		unsetEnvironment(t, "TALK2TEXT_NVIM_LAUNCH_CMD")
		unsetEnvironment(t, "TALK2TEXT_NVIM_FOCUS_CMD")
		t.Setenv("PATH", t.TempDir())

		got := New("", "", 0)
		if got.notifyCmd != "" {
			t.Fatalf("notification command = %q, want disabled", got.notifyCmd)
		}
		if got.launchCmd != "nvim" {
			t.Fatalf("launch command = %q, want nvim", got.launchCmd)
		}
		if got.focusCmd != "" {
			t.Fatalf("focus command = %q, want empty", got.focusCmd)
		}
	})

	t.Run("trims explicit configuration without checking notification command", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		t.Setenv("TALK2TEXT_NVIM_NOTIFY_CMD", " \tmissing-notifier --flag\n")
		t.Setenv("TALK2TEXT_NVIM_LAUNCH_CMD", " \tterminal -- nvim\n")
		t.Setenv("TALK2TEXT_NVIM_FOCUS_CMD", " \tfocus-window --current\n")

		got := New("", "", 0)
		if got.notifyCmd != "missing-notifier --flag" {
			t.Fatalf("notification command = %q, want explicit command", got.notifyCmd)
		}
		if got.launchCmd != "terminal -- nvim" {
			t.Fatalf("launch command = %q, want explicit command", got.launchCmd)
		}
		if got.focusCmd != "focus-window --current" {
			t.Fatalf("focus command = %q, want explicit command", got.focusCmd)
		}
	})

	t.Run("preserves explicit empty command", func(t *testing.T) {
		t.Setenv("TALK2TEXT_NVIM_NOTIFY_CMD", " \t\n")

		if got := New("", "", 0).notifyCmd; got != "" {
			t.Fatalf("notification command = %q, want disabled", got)
		}
	})
}

func unsetEnvironment(t *testing.T, name string) {
	t.Helper()
	value, exists := os.LookupEnv(name)
	if err := os.Unsetenv(name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if exists {
			_ = os.Setenv(name, value)
		} else {
			_ = os.Unsetenv(name)
		}
	})
}

func TestHandleBlank(t *testing.T) {
	t.Run("removes transcript", func(t *testing.T) {
		transcript := filepath.Join(t.TempDir(), "transcript")
		if err := os.WriteFile(transcript, nil, 0o600); err != nil {
			t.Fatal(err)
		}

		(&Command{transcriptPath: transcript}).HandleBlank()
		if _, err := os.Lstat(transcript); !os.IsNotExist(err) {
			t.Fatalf("blank transcript was not removed: %v", err)
		}
	})

	t.Run("cleanup is best effort", func(t *testing.T) {
		transcript := filepath.Join(t.TempDir(), "transcript")
		if err := os.Mkdir(transcript, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(transcript, "entry"), nil, 0o600); err != nil {
			t.Fatal(err)
		}

		stderr := captureStderr(t, func() {
			(&Command{transcriptPath: transcript}).HandleBlank()
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
		transcript := filepath.Join(runtimeDir, "transcript")
		target := filepath.Join(runtimeDir, targetfile.NormalTarget)
		defaultTarget := filepath.Join(runtimeDir, targetfile.DefaultTarget)
		for _, path := range []string{transcript, target, defaultTarget} {
			if err := os.WriteFile(path, nil, 0o600); err != nil {
				t.Fatal(err)
			}
		}

		if err := (&Command{runtimeDir: runtimeDir, transcriptPath: transcript}).HandleShort(); err != nil {
			t.Fatalf("HandleShort() error = %v", err)
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
		transcript := filepath.Join(runtimeDir, "transcript")
		target := filepath.Join(runtimeDir, targetfile.NormalTarget)
		if err := os.Mkdir(transcript, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(transcript, "entry"), nil, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, nil, 0o600); err != nil {
			t.Fatal(err)
		}

		captureStderr(t, func() {
			if err := (&Command{runtimeDir: runtimeDir, transcriptPath: transcript}).HandleShort(); err != nil {
				t.Fatalf("HandleShort() error = %v", err)
			}
		})
		if _, err := os.Lstat(target); !os.IsNotExist(err) {
			t.Fatalf("target was not reset after transcript cleanup failure: %v", err)
		}
	})

	t.Run("target reset failure happens after transcript cleanup", func(t *testing.T) {
		runtimeDir := t.TempDir()
		transcript := filepath.Join(runtimeDir, "transcript")
		target := filepath.Join(runtimeDir, targetfile.NormalTarget)
		if err := os.WriteFile(transcript, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(target, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(target, "entry"), nil, 0o600); err != nil {
			t.Fatal(err)
		}

		if err := (&Command{runtimeDir: runtimeDir, transcriptPath: transcript}).HandleShort(); err == nil {
			t.Fatal("HandleShort() succeeded after target reset failure")
		}
		if _, err := os.Lstat(transcript); !os.IsNotExist(err) {
			t.Fatalf("transcript was not removed before target reset failed: %v", err)
		}
		if info, err := os.Stat(target); err != nil || !info.IsDir() {
			t.Fatalf("target directory was removed or changed: %v", err)
		}
	})
}

func TestDefaultEditorInvocation(t *testing.T) {
	if os.Getenv("TALK2TEXT_NVIM_TEST_DEFAULT_EDITOR") == "1" {
		if err := (&Command{
			launchCmd:      `printf '%s\n'`,
			transcriptPath: "/tmp/runtime with spaces/transcripts/3",
		}).launchDefault(); err != nil {
			t.Fatal(err)
		}
		return
	}

	t.Run("passes the transcript path to the launch command", func(t *testing.T) {
		cmd := exec.Command(os.Args[0], "-test.run=^TestDefaultEditorInvocation$")
		cmd.Env = append(os.Environ(), "TALK2TEXT_NVIM_TEST_DEFAULT_EDITOR=1")
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("launchDefault() process error = %v: %s", err, output)
		}
		want := "/tmp/runtime with spaces/transcripts/3"
		if got := strings.TrimSuffix(string(output), "\n"); got != want {
			t.Fatalf("argument = %q, want %q", got, want)
		}
	})

	t.Run("requires a launch command", func(t *testing.T) {
		if err := (&Command{}).launchDefault(); err == nil || !strings.Contains(err.Error(), "TALK2TEXT_NVIM_LAUNCH_CMD") {
			t.Fatalf("empty launch command error = %v, want required-setting error", err)
		}
	})
}

func TestShellPathIsCached(t *testing.T) {
	command := &Command{}
	first, err := command.shell()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	second, err := command.shell()
	if err != nil {
		t.Fatal(err)
	}
	if second != first {
		t.Fatalf("second shell path = %q, want cached path %q", second, first)
	}
	if _, err := (&Command{}).shell(); err == nil {
		t.Fatal("uncached shell lookup succeeded without PATH")
	}
}

func TestTryTargetRejectsRelativeAddress(t *testing.T) {
	runtimeDir := t.TempDir()
	path := filepath.Join(runtimeDir, targetfile.NormalTarget)
	if err := os.WriteFile(path, []byte("relative.sock\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := (&Command{runtimeDir: runtimeDir}).tryTarget(targetfile.NormalTarget)
	if result != targetFatal || err == nil || !strings.Contains(err.Error(), "must be absolute") {
		t.Fatalf("tryTarget() = %d, %v, want fatal absolute-path error", result, err)
	}
	if _, statErr := os.Lstat(path); !os.IsNotExist(statErr) {
		t.Fatalf("relative target was not removed: %v", statErr)
	}
}

func TestDetachedHookStartErrorsAreReported(t *testing.T) {
	for _, test := range []struct {
		name string
		run  func(*Command)
	}{
		{"notification", func(command *Command) { command.notify("message") }},
		{"focus", func(command *Command) { command.focusDefault() }},
	} {
		t.Run(test.name, func(t *testing.T) {
			command := &Command{
				notifyCmd: "true",
				focusCmd:  "true",
				shellPath: filepath.Join(t.TempDir(), "missing-shell"),
			}
			stderr := captureStderr(t, func() { test.run(command) })
			want := "cannot start " + test.name + " command:"
			if !strings.Contains(stderr, want) {
				t.Fatalf("stderr = %q, want text containing %q", stderr, want)
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
	(&Command{focusCmd: missingCommand}).focusDefault()
	os.Stderr = original

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

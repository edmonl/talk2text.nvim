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

func TestNewCommandNotification(t *testing.T) {
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

	t.Run("disables unavailable default", func(t *testing.T) {
		unsetEnvironment(t, "TALK2TEXT_NVIM_NOTIFY_CMD")
		t.Setenv("PATH", t.TempDir())

		if got := New("", "", 0).notifyCmd; got != "" {
			t.Fatalf("notification command = %q, want disabled", got)
		}
	})

	t.Run("preserves explicit command without checking it", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		t.Setenv("TALK2TEXT_NVIM_NOTIFY_CMD", "missing-notifier --flag")

		if got := New("", "", 0).notifyCmd; got != "missing-notifier --flag" {
			t.Fatalf("notification command = %q, want explicit command", got)
		}
	})

	t.Run("preserves explicit empty command", func(t *testing.T) {
		t.Setenv("TALK2TEXT_NVIM_NOTIFY_CMD", "")

		if got := New("", "", 0).notifyCmd; got != "" {
			t.Fatalf("notification command = %q, want disabled", got)
		}
	})
}

func TestNewCommandLaunch(t *testing.T) {
	t.Run("uses nvim by default", func(t *testing.T) {
		unsetEnvironment(t, "TALK2TEXT_NVIM_LAUNCH_CMD")

		if got := New("", "", 0).launchCmd; got != "nvim" {
			t.Fatalf("launch command = %q, want nvim", got)
		}
	})

	t.Run("preserves explicit command", func(t *testing.T) {
		t.Setenv("TALK2TEXT_NVIM_LAUNCH_CMD", "terminal -- nvim")

		if got := New("", "", 0).launchCmd; got != "terminal -- nvim" {
			t.Fatalf("launch command = %q, want explicit command", got)
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
		transcript := filepath.Join(t.TempDir(), "transcript.txt")
		if err := os.WriteFile(transcript, nil, 0o600); err != nil {
			t.Fatal(err)
		}

		(&Command{transcriptPath: transcript}).HandleBlank()
		if _, err := os.Lstat(transcript); !os.IsNotExist(err) {
			t.Fatalf("blank transcript was not removed: %s", err)
		}
	})

	t.Run("cleanup is best effort", func(t *testing.T) {
		transcript := filepath.Join(t.TempDir(), "transcript.txt")
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
			t.Fatalf("transcript directory was removed or changed: %s", err)
		}
	})

	t.Run("already absent is already cleaned", func(t *testing.T) {
		transcript := filepath.Join(t.TempDir(), "transcript.txt")
		stderr := captureStderr(t, func() {
			(&Command{transcriptPath: transcript}).HandleBlank()
		})
		if strings.Contains(stderr, "cannot remove transcript") {
			t.Fatalf("stderr = %q, want no transcript cleanup failure", stderr)
		}
	})
}

func TestHandleShort(t *testing.T) {
	t.Run("removes transcript and explicit target", func(t *testing.T) {
		runtimeDir := t.TempDir()
		transcript := filepath.Join(runtimeDir, "transcript.txt")
		target := filepath.Join(runtimeDir, targetfile.NormalTarget)
		defaultTarget := filepath.Join(runtimeDir, targetfile.DefaultTarget)
		for _, path := range []string{transcript, target, defaultTarget} {
			if err := os.WriteFile(path, nil, 0o600); err != nil {
				t.Fatal(err)
			}
		}

		if err := (&Command{runtimeDir: runtimeDir, transcriptPath: transcript}).HandleShort(); err != nil {
			t.Fatalf("HandleShort() error = %s", err)
		}
		if _, err := os.Lstat(target); !os.IsNotExist(err) {
			t.Fatalf("explicit target was not removed: %s", err)
		}
		if _, err := os.Stat(defaultTarget); err != nil {
			t.Fatalf("default target was changed: %s", err)
		}
	})

	t.Run("transcript cleanup failure does not prevent target reset", func(t *testing.T) {
		runtimeDir := t.TempDir()
		transcript := filepath.Join(runtimeDir, "transcript.txt")
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
				t.Fatalf("HandleShort() error = %s", err)
			}
		})
		if _, err := os.Lstat(target); !os.IsNotExist(err) {
			t.Fatalf("target was not reset after transcript cleanup failure: %s", err)
		}
	})

	t.Run("target reset failure happens after transcript cleanup", func(t *testing.T) {
		runtimeDir := t.TempDir()
		transcript := filepath.Join(runtimeDir, "transcript.txt")
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
			t.Fatalf("transcript was not removed before target reset failed: %s", err)
		}
		if info, err := os.Stat(target); err != nil || !info.IsDir() {
			t.Fatalf("target directory was removed or changed: %s", err)
		}
	})
}

func TestDefaultEditorInvocation(t *testing.T) {
	t.Run("passes generated arguments to the launch command", func(t *testing.T) {
		cmd := exec.Command(os.Args[0], "-test.run=^TestDefaultEditorInvocationHelper$")
		cmd.Env = append(os.Environ(), "TALK2TEXT_NVIM_TEST_DEFAULT_EDITOR=1")
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("launchDefault() process error = %s: %s", err, output)
		}
		gotArgs := strings.Split(strings.TrimSuffix(string(output), "\n"), "\n")
		wantArgs := []string{"-c", `lua require("talk2text")._default_start(3)`}
		if strings.Join(gotArgs, "\n") != strings.Join(wantArgs, "\n") {
			t.Fatalf("arguments = %q, want %q", gotArgs, wantArgs)
		}
	})

	t.Run("requires a launch command", func(t *testing.T) {
		if err := (&Command{}).launchDefault(); err == nil || !strings.Contains(err.Error(), "TALK2TEXT_NVIM_LAUNCH_CMD") {
			t.Fatalf("empty launch command error = %s, want required-setting error", err)
		}
	})
}

func TestDefaultEditorInvocationHelper(t *testing.T) {
	if os.Getenv("TALK2TEXT_NVIM_TEST_DEFAULT_EDITOR") != "1" {
		return
	}
	if err := (&Command{launchCmd: `printf '%s\n'`, transcriptID: 3}).launchDefault(); err != nil {
		t.Fatal(err)
	}
}

func TestShellPathIsCached(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestShellPathCacheHelper$")
	cmd.Env = append(os.Environ(), "TALK2TEXT_NVIM_TEST_SHELL_CACHE=1")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("shell cache process error = %s: %s", err, output)
	}
}

func TestShellPathCacheHelper(t *testing.T) {
	if os.Getenv("TALK2TEXT_NVIM_TEST_SHELL_CACHE") != "1" {
		return
	}
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
}

func TestReadTargetTreatsZeroByteTargetAsMissing(t *testing.T) {
	runtimeDir := t.TempDir()
	path := filepath.Join(runtimeDir, targetfile.NormalTarget)
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	value, err := targetfile.Read(runtimeDir, targetfile.NormalTarget)
	if err != nil {
		t.Fatalf("target.Read() error = %s", err)
	}
	if value != "" {
		t.Fatalf("target.Read() = %q, want empty", value)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("zero-byte target was removed: %s", err)
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
		t.Fatalf("tryTarget() = %d, %s, want fatal absolute-path error", result, err)
	}
	if _, statErr := os.Lstat(path); !os.IsNotExist(statErr) {
		t.Fatalf("relative target was not removed: %s", statErr)
	}
}

func TestDetachedHookStartErrorsAreReported(t *testing.T) {
	for _, name := range []string{"notification", "focus"} {
		t.Run(name, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestDetachedHookStartErrorHelper$")
			cmd.Env = append(os.Environ(), "PATH="+t.TempDir(), "TALK2TEXT_NVIM_TEST_HOOK="+name)
			contents, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("hook process error = %s: %s", err, contents)
			}
			want := "cannot start " + name + " command:"
			if !strings.Contains(string(contents), want) {
				t.Fatalf("stderr = %q, want text containing %q", contents, want)
			}
		})
	}
}

func TestDetachedHookStartErrorHelper(t *testing.T) {
	switch os.Getenv("TALK2TEXT_NVIM_TEST_HOOK") {
	case "notification":
		(&Command{notifyCmd: "true"}).notify("message")
	case "focus":
		(&Command{focusCmd: "true"}).focusDefault()
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

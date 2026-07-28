package command

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	targetfile "github.com/edmonl/talk2text.nvim/internal/command/target"
)

func TestNewCommand(t *testing.T) {
	t.Run("uses fallback configuration", func(t *testing.T) {
		unsetEnvironment(t, "TALK2TEXT_NVIM_LAUNCH_CMD")
		unsetEnvironment(t, "TALK2TEXT_NVIM_FOCUS_CMD")

		got := newTestCommand(t)
		if got.launchCmd != "nvim" {
			t.Fatalf("launch command = %q, want nvim", got.launchCmd)
		}
		if got.focusCmd != "" {
			t.Fatalf("focus command = %q, want empty", got.focusCmd)
		}
		if got.TranscriptID() != 1 {
			t.Fatalf("transcript ID = %d, want 1", got.TranscriptID())
		}
	})

	t.Run("trims explicit configuration", func(t *testing.T) {
		t.Setenv("TALK2TEXT_NVIM_LAUNCH_CMD", " \tterminal -- nvim\n")
		t.Setenv("TALK2TEXT_NVIM_FOCUS_CMD", " \tfocus-window --current\n")

		got := newTestCommand(t)
		if got.launchCmd != "terminal -- nvim" {
			t.Fatalf("launch command = %q, want explicit command", got.launchCmd)
		}
		if got.focusCmd != "focus-window --current" {
			t.Fatalf("focus command = %q, want explicit command", got.focusCmd)
		}
	})

	t.Run("preserves notification executable", func(t *testing.T) {
		const executable = " \t/path/to/notification command\n"
		t.Setenv("TALK2TEXT_NOTIFY_CMD", executable)

		if got := newTestCommand(t).notifyCmd; got != executable {
			t.Fatalf("notification command = %q, want %q", got, executable)
		}
	})

}

func TestChildEnvironmentRemovesNvimVariables(t *testing.T) {
	t.Setenv("TALK2TEXT_NVIM_FOCUS_CMD", "focus")
	t.Setenv("TALK2TEXT_NVIM_CUSTOM", "custom")
	t.Setenv(outputKindEnv, "text")
	t.Setenv("TALK2TEXT_NOTIFY_CMD", "notify")

	environment := make(map[string]string)
	for _, setting := range childEnvironment() {
		name, value, _ := strings.Cut(setting, "=")
		if strings.HasPrefix(name, nvimEnvPrefix) {
			t.Fatalf("child environment contains %s", name)
		}
		environment[name] = value
	}
	for name, want := range map[string]string{
		outputKindEnv:          "text",
		"TALK2TEXT_NOTIFY_CMD": "notify",
	} {
		if got := environment[name]; got != want {
			t.Errorf("child environment %s = %q, want %q", name, got, want)
		}
	}
}

func newTestCommand(t *testing.T) *Command {
	t.Helper()
	runtimeDir := t.TempDir()
	transcriptDir := filepath.Join(runtimeDir, "transcripts")
	if err := os.Mkdir(transcriptDir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv(outputKindEnv, "text")

	got, _, err := Parse([]string{filepath.Join(transcriptDir, "1")})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	return got
}

func TestParseTranscriptPathPreservesRuntimePath(t *testing.T) {
	root := t.TempDir()
	runtimeDir := filepath.Join(root, "runtime with spaces")
	resolvedTranscriptDir := filepath.Join(root, "recordings")
	if err := os.Mkdir(runtimeDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(resolvedTranscriptDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(resolvedTranscriptDir, filepath.Join(runtimeDir, "transcripts")); err != nil {
		t.Fatal(err)
	}

	transcript := filepath.Join(runtimeDir, "transcripts", "42")
	gotRuntime, gotID, err := parseTranscriptPath(transcript)
	if err != nil {
		t.Fatalf("parseTranscriptPath() error = %v", err)
	}
	if gotRuntime != runtimeDir {
		t.Fatalf("parseTranscriptPath() runtime directory = %q, want symlink path %q", gotRuntime, runtimeDir)
	}
	if gotID != 42 {
		t.Fatalf("parseTranscriptPath() transcript ID = %d, want 42", gotID)
	}
}

func TestParseTranscriptPathRejectsRoot(t *testing.T) {
	if _, _, err := parseTranscriptPath("/transcripts/1"); err == nil {
		t.Fatal("parseTranscriptPath() accepted the filesystem root")
	}
}

func TestParseTranscriptPathRejectsInvalidPath(t *testing.T) {
	for _, path := range []string{"runtime/transcripts/1", "/runtime/transcripts/"} {
		if _, _, err := parseTranscriptPath(path); err == nil || err.Error() != "transcript path must be an absolute file path" {
			t.Errorf("parseTranscriptPath(%q) error = %v, want absolute-path error", path, err)
		}
	}
}

func TestParseTranscriptPathRejectsMalformedNames(t *testing.T) {
	runtimeDir := t.TempDir()
	for _, filename := range []string{
		"0",
		"01",
		"1.txt",
		strconv.FormatUint(uint64(^uint(0)>>1)+1, 10),
	} {
		path := filepath.Join(runtimeDir, "transcripts", filename)
		if _, _, err := parseTranscriptPath(path); err == nil {
			t.Errorf("parseTranscriptPath(%q) accepted a malformed filename", path)
		}
	}
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

func TestHandleBlankReportsCleanupFailure(t *testing.T) {
	t.Setenv("TALK2TEXT_NOTIFY_CMD", "")

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
}

func TestHandleShort(t *testing.T) {
	t.Setenv("TALK2TEXT_NOTIFY_CMD", "")

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

func TestDetachedStartErrorsAreReported(t *testing.T) {
	for _, test := range []struct {
		name      string
		configure func(*testing.T, *Command)
		run       func(*Command)
	}{
		{
			name: "notification",
			configure: func(t *testing.T, command *Command) {
				command.notifyCmd = filepath.Join(t.TempDir(), "missing-notifier")
			},
			run: func(command *Command) { command.notifyInfo("message") },
		},
		{
			name: "focus",
			configure: func(t *testing.T, command *Command) {
				command.focusCmd = "true"
				command.shellPath = filepath.Join(t.TempDir(), "missing-shell")
			},
			run: (*Command).focusDefault,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			command := &Command{transcriptID: 17}
			test.configure(t, command)
			stderr := captureStderr(t, func() { test.run(command) })
			for _, want := range []string{"transcript 17", test.name + " command error"} {
				if !strings.Contains(stderr, want) {
					t.Fatalf("stderr = %q, want text containing %q", stderr, want)
				}
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

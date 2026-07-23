package main

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	targetName        = "nvim-target"
	defaultTargetName = "default-nvim-target"
)

// Embedding the Lua inputs makes changes to them invalidate Go's test cache.
//
//go:embed lua/talk2text/*.lua tests/plugin.lua
var integrationInputs embed.FS

func TestNeovimIntegration(t *testing.T) {
	if _, err := exec.LookPath("nvim"); err != nil {
		t.Fatal("nvim is required for integration tests")
	}

	projectRoot, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	testDir := t.TempDir()

	// Consult an embedded input so the dependency on the Lua sources remains
	// explicit even though Neovim loads their filesystem copies below.
	if _, err := integrationInputs.ReadFile("tests/plugin.lua"); err != nil {
		t.Fatal(err)
	}

	pluginDir := filepath.Join(testDir, "plugin")
	runSuccessfulProcess(t, projectRoot, integrationEnvironment(map[string]string{
		"TALK2TEXT_TEST_DIR": pluginDir,
		"NVIM_LOG_FILE":      filepath.Join(testDir, "plugin-nvim.log"),
	}), "nvim", "--headless", "-u", "NONE", "-i", "NONE", "-n",
		"--cmd", "set runtimepath^="+projectRoot,
		"-l", filepath.Join(projectRoot, "tests", "plugin.lua"))

	binary := filepath.Join(testDir, "talk2text-nvim")
	runSuccessfulProcess(t, projectRoot, integrationEnvironment(nil),
		"go", "build", "-o", binary, projectRoot)

	runtimeDir := filepath.Join(testDir, "runtime with spaces")
	transcriptDir := filepath.Join(runtimeDir, "transcripts")
	if err := os.MkdirAll(transcriptDir, 0o700); err != nil {
		t.Fatal(err)
	}

	focusLog := filepath.Join(testDir, "focus.log")
	notifyLog := filepath.Join(testDir, "notify.log")
	hookEnvironment := map[string]string{
		"TALK2TEXT_TEST_FOCUS_LOG":  focusLog,
		"TALK2TEXT_TEST_NOTIFY_LOG": notifyLog,
		"TALK2TEXT_NVIM_FOCUS_CMD":  `printf "focused\n" >> "$TALK2TEXT_TEST_FOCUS_LOG"`,
		"TALK2TEXT_NVIM_NOTIFY_CMD": `record_notification() { printf "%s\n" "$1" >> "$TALK2TEXT_TEST_NOTIFY_LOG"; }; record_notification`,
	}

	socket := filepath.Join(testDir, "server.sock")
	serverLogPath := filepath.Join(testDir, "server.log")
	serverLog, err := os.Create(serverLogPath)
	if err != nil {
		t.Fatal(err)
	}
	server := exec.Command("nvim", "--headless", "-u", "NONE", "-i", "NONE", "-n", "--listen", socket,
		"--cmd", "set runtimepath^="+projectRoot,
		"-c", fmt.Sprintf("lua vim.fn.serverstart(%q); require('talk2text').setup({runtime_dir=%q}); require('talk2text').set_target()", filepath.Join(runtimeDir, "daemon.sock"), runtimeDir))
	server.Env = integrationEnvironment(map[string]string{"NVIM_LOG_FILE": filepath.Join(testDir, "server-nvim.log")})
	server.Stdout = serverLog
	server.Stderr = serverLog
	if err := server.Start(); err != nil {
		serverLog.Close()
		t.Fatal(err)
	}
	serverDone := make(chan error, 1)
	go func() { serverDone <- server.Wait() }()
	serverRunning := true
	t.Cleanup(func() {
		if serverRunning {
			_ = server.Process.Kill()
			<-serverDone
		}
		_ = serverLog.Close()
	})
	waitFor(t, "Neovim target", func() bool {
		return isRegularFile(filepath.Join(runtimeDir, targetName))
	})

	firstPath := filepath.Join(transcriptDir, "1")
	writeFile(t, firstPath, "first line\nsecond line\n")
	hookEnvironment["TALK2TEXT_NVIM_LAUNCH_CMD"] = ""
	runOutputCommand(t, projectRoot, hookEnvironment, binary, "text", firstPath)
	loaded := runNvimExpression(t, projectRoot, socket, `join(getline(1,"$"), "|")`)
	if loaded != "first line|second line" {
		t.Fatalf("unexpected RPC buffer: %q", loaded)
	}

	defaultSetupPath := filepath.Join(transcriptDir, "2")
	writeFile(t, defaultSetupPath, "")
	runNvimExpression(t, projectRoot, socket, `execute("enew")`)
	runNvimExpression(t, projectRoot, socket, `luaeval('require("talk2text").start_default_target(2)')`)
	writeFile(t, filepath.Join(runtimeDir, targetName), "/tmp/stale-talk2text-nvim.sock\n")
	runNvimExpression(t, projectRoot, socket, `execute("enew")`)
	fallbackPath := filepath.Join(transcriptDir, "3")
	writeFile(t, fallbackPath, "fallback")
	runOutputCommand(t, projectRoot, hookEnvironment, binary, "text", fallbackPath)
	assertAbsent(t, filepath.Join(runtimeDir, targetName))
	assertExists(t, filepath.Join(runtimeDir, defaultTargetName))
	if got := runNvimExpression(t, projectRoot, socket, `getline(1)`); got != "fallback" {
		t.Fatalf("default target buffer = %q, want fallback", got)
	}
	waitFor(t, "focus hook", func() bool {
		contents, err := os.ReadFile(focusLog)
		return err == nil && strings.TrimSpace(string(contents)) == "focused"
	})
	waitFor(t, "stale target notification", func() bool {
		contents, err := os.ReadFile(notifyLog)
		return err == nil && strings.Contains(string(contents), "Stale target /tmp/stale-talk2text-nvim.sock removed")
	})

	runNvimExpression(t, projectRoot, socket, `execute("setlocal nomodifiable")`)
	failedPath := filepath.Join(transcriptDir, "4")
	writeFile(t, failedPath, "retry me")
	if output, err := runProcess(projectRoot, integrationEnvironment(hookEnvironment), binary, "text", failedPath); err == nil {
		t.Fatalf("reachable target load failure returned success; output: %s", output)
	}
	assertExists(t, failedPath)
	assertExists(t, filepath.Join(runtimeDir, defaultTargetName))
	waitFor(t, "target error notification", func() bool {
		contents, err := os.ReadFile(notifyLog)
		return err == nil && strings.Contains(string(contents), "Error: failed to load transcript 4:")
	})
	runNvimExpression(t, projectRoot, socket, `execute("setlocal modifiable")`)

	runNvimExpression(t, projectRoot, socket, `luaeval("require(\"talk2text\").set_target()")`)
	writeFile(t, filepath.Join(runtimeDir, targetName), "replacement\n")
	_, _ = runProcess(projectRoot, integrationEnvironment(nil), "nvim", "--server", socket, "--remote-expr", `execute("qa!")`)
	select {
	case <-serverDone:
		serverRunning = false
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for Neovim server to exit")
	}
	if got := firstLine(t, filepath.Join(runtimeDir, targetName)); got != "replacement" {
		t.Fatalf("replacement target = %q, want replacement", got)
	}
	assertAbsent(t, filepath.Join(runtimeDir, defaultTargetName))

	startupBase := filepath.Join(testDir, "xdg")
	startupRuntime := filepath.Join(startupBase, "talk2text")
	startupTranscriptDir := filepath.Join(startupRuntime, "transcripts")
	if err := os.MkdirAll(startupTranscriptDir, 0o700); err != nil {
		t.Fatal(err)
	}
	startupPath := filepath.Join(startupTranscriptDir, "1")
	startupCWDLog := filepath.Join(testDir, "startup-cwd.log")
	writeFile(t, startupPath, "started by launch command")
	startupEnvironment := map[string]string{
		"TALK2TEXT_TEST_ROOT":            projectRoot,
		"TALK2TEXT_TEST_CWD_LOG":         startupCWDLog,
		"TALK2TEXT_TEST_STARTUP_RUNTIME": startupRuntime,
		"TALK2TEXT_NVIM_LAUNCH_CMD":      `pwd > "$TALK2TEXT_TEST_CWD_LOG"; run_launch_command() { "$@" +q; }; run_launch_command nvim --headless -u NONE -i NONE -n --cmd "set runtimepath^=$TALK2TEXT_TEST_ROOT" --cmd "lua vim.fn.serverstart(vim.env.TALK2TEXT_TEST_STARTUP_RUNTIME .. \"/daemon.sock\"); require(\"talk2text\").setup({runtime_dir=vim.env.TALK2TEXT_TEST_STARTUP_RUNTIME})"`,
		"XDG_RUNTIME_DIR":                startupBase,
		"NVIM_LOG_FILE":                  filepath.Join(testDir, "startup-nvim.log"),
	}
	runOutputCommand(t, testDir, startupEnvironment, binary, "text", startupPath)
	waitForAbsence(t, startupPath)
	if got := strings.TrimSpace(readFile(t, startupCWDLog)); got != testDir {
		t.Fatalf("launch command working directory = %q, want %q", got, testDir)
	}

	directRuntime := filepath.Join(testDir, "direct-runtime")
	directTranscriptDir := filepath.Join(directRuntime, "transcripts")
	if err := os.MkdirAll(directTranscriptDir, 0o700); err != nil {
		t.Fatal(err)
	}
	directFailurePath := filepath.Join(directTranscriptDir, "2")
	writeFile(t, directFailurePath, "retain after failed direct startup")
	failedDirectEnvironment := map[string]string{
		"TALK2TEXT_NVIM_LAUNCH_CMD": `failed_direct_launch() { return 23; }; failed_direct_launch`,
	}
	if output, err := runProcess(projectRoot, integrationEnvironment(failedDirectEnvironment), binary, "text", directFailurePath); err == nil {
		t.Fatalf("failed direct Neovim launch returned success; output: %s", output)
	}
	assertExists(t, directFailurePath)
}

func runOutputCommand(t *testing.T, directory string, environment map[string]string, binary string, args ...string) {
	t.Helper()
	runSuccessfulProcess(t, directory, integrationEnvironment(environment), binary, args...)
}

func runNvimExpression(t *testing.T, directory, socket, expression string) string {
	t.Helper()
	return strings.TrimSpace(runSuccessfulProcess(t, directory, integrationEnvironment(nil),
		"nvim", "--server", socket, "--remote-expr", expression))
}

func runSuccessfulProcess(t *testing.T, directory string, environment []string, name string, args ...string) string {
	t.Helper()
	output, err := runProcess(directory, environment, name, args...)
	if err != nil {
		t.Fatalf("%s %s failed: %s\n%s", name, strings.Join(args, " "), err, output)
	}
	return output
}

func runProcess(directory string, environment []string, name string, args ...string) (string, error) {
	command := exec.Command(name, args...)
	command.Dir = directory
	command.Env = environment
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	err := command.Run()
	return output.String(), err
}

func integrationEnvironment(overrides map[string]string) []string {
	controlled := map[string]bool{
		"NVIM_LOG_FILE":                  true,
		"TALK2TEXT_NVIM_FOCUS_CMD":       true,
		"TALK2TEXT_NVIM_LAUNCH_CMD":      true,
		"TALK2TEXT_NVIM_NOTIFY_CMD":      true,
		"TALK2TEXT_TEST_CWD_LOG":         true,
		"TALK2TEXT_TEST_DIR":             true,
		"TALK2TEXT_TEST_FOCUS_LOG":       true,
		"TALK2TEXT_TEST_NOTIFY_LOG":      true,
		"TALK2TEXT_TEST_ROOT":            true,
		"TALK2TEXT_TEST_STARTUP_RUNTIME": true,
		"XDG_RUNTIME_DIR":                true,
	}
	for name := range overrides {
		controlled[name] = true
	}

	environment := make([]string, 0, len(os.Environ())+len(overrides))
	for _, setting := range os.Environ() {
		name, _, _ := strings.Cut(setting, "=")
		if !controlled[name] {
			environment = append(environment, setting)
		}
	}
	for name, value := range overrides {
		environment = append(environment, name+"="+value)
	}
	return environment
}

func waitFor(t *testing.T, description string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", description)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func waitForAbsence(t *testing.T, path string) {
	t.Helper()
	waitFor(t, path+" to disappear", func() bool {
		_, err := os.Lstat(path)
		return os.IsNotExist(err)
	})
}

func isRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(contents)
}

func firstLine(t *testing.T, path string) string {
	t.Helper()
	line, _, _ := strings.Cut(readFile(t, path), "\n")
	return line
}

func assertExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); err != nil {
		t.Fatalf("expected %s to exist: %s", path, err)
	}
}

func assertAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("expected %s to be absent; stat error: %s", path, err)
	}
}

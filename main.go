package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"

	"github.com/neovim/go-client/nvim"
)

const (
	targetName        = "nvim-target"
	defaultTargetName = "default-nvim-target"
	remoteOK          = "ok"
	probeLua          = "return true"
	remoteLoadLua     = `return require("talk2text")._remote_load(...)`
	defaultNotifyCmd  = "notify-send -a talk2text -u normal -t 5000 Talk2text"
)

type targetResult int

const (
	targetUnavailable targetResult = iota
	targetDelivered
	targetFatal
)

type command struct {
	notifyCmd    string
	nvimCmd      string
	launchCmd    string
	focusCmd     string
	runtimeDir   string
	transcript   string
	transcriptID int64
}

func main() {
	cmd := newCommand()
	if err := cmd.run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "talk2text-nvim: %v\n", err)
		os.Exit(1)
	}
}

func newCommand() *command {
	return &command{
		notifyCmd: environmentOrDefault("TALK2TEXT_NVIM_NOTIFY_CMD", defaultNotifyCmd),
		nvimCmd:   environmentOrDefault("TALK2TEXT_NVIM_CMD", "nvim"),
		launchCmd: environmentOrDefault("TALK2TEXT_NVIM_LAUNCH_CMD", ""),
		focusCmd:  environmentOrDefault("TALK2TEXT_NVIM_FOCUS_CMD", ""),
	}
}

func environmentOrDefault(name, fallback string) string {
	if value, ok := os.LookupEnv(name); ok {
		return value
	}
	return fallback
}

func (c *command) run(args []string) error {
	if len(args) != 2 {
		return errors.New("usage: talk2text-nvim <text|blank|short> <path>")
	}

	kind := args[0]
	c.transcript = args[1]
	if !strings.HasPrefix(c.transcript, "/") || strings.HasSuffix(c.transcript, "/") {
		return errors.New("transcript path must be an absolute file path")
	}

	runtimeDir, err := runtimeDirectory(c.transcript)
	if err != nil {
		return err
	}
	c.runtimeDir = runtimeDir
	clipID, err := transcriptID(c.transcript)
	if err != nil {
		return err
	}
	c.transcriptID = clipID

	switch kind {
	case "text":
		return c.handleText()
	case "blank":
		c.handleBlank()
		return nil
	case "short":
		return c.handleShort()
	default:
		return fmt.Errorf("unknown transcript kind: %s", kind)
	}
}

func runtimeDirectory(transcript string) (string, error) {
	lastSlash := strings.LastIndexByte(transcript, '/')
	transcriptDir := transcript[:lastSlash]
	dirSlash := strings.LastIndexByte(transcriptDir, '/')
	if transcriptDir[dirSlash+1:] != "transcripts" {
		return "", errors.New("transcript path must be directly under a transcripts directory")
	}

	runtimeDir := transcriptDir[:dirSlash]
	if strings.Trim(runtimeDir, "/") == "" {
		return "", errors.New("runtime directory must not be the filesystem root")
	}

	info, err := os.Stat(runtimeDir)
	if err != nil || !info.IsDir() {
		return "", errors.New("runtime directory is unavailable")
	}
	return runtimeDir, nil
}

func transcriptID(transcript string) (int64, error) {
	lastSlash := strings.LastIndexByte(transcript, '/')
	filename := transcript[lastSlash+1:]
	if !strings.HasSuffix(filename, ".txt") {
		return 0, errors.New("transcript filename must be <positive-id>.txt")
	}

	value := strings.TrimSuffix(filename, ".txt")
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id < 1 || strconv.FormatInt(id, 10) != value {
		return 0, errors.New("transcript filename must be <positive-id>.txt")
	}
	return id, nil
}

func (c *command) handleText() error {
	if c.nvimCmd == "" {
		return errors.New("TALK2TEXT_NVIM_CMD is required for handling transcripts")
	}
	if err := readableRegularFile(c.transcript); err != nil {
		return errors.New("text transcript must be a readable regular file")
	}

	switch result, err := c.tryTarget(targetName); result {
	case targetDelivered:
		return nil
	case targetFatal:
		return err
	}

	switch result, err := c.tryTarget(defaultTargetName); result {
	case targetDelivered:
		c.focusDefaultEditor()
		return nil
	case targetFatal:
		return err
	}

	return c.startDefaultEditor()
}

func readableRegularFile(path string) error {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("not a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	return file.Close()
}

func (c *command) tryTarget(name string) (targetResult, error) {
	path := c.runtimeDir + "/" + name
	exists, address, err := c.readTarget(path)
	if err != nil {
		return targetFatal, err
	}
	if !exists {
		return targetUnavailable, nil
	}
	if address == "" {
		if err := c.deleteTargetIfUnchanged(path, address); err != nil {
			return targetFatal, err
		}
		return targetUnavailable, nil
	}

	client, err := dialNvim(address)
	if err != nil {
		if cleanupErr := c.deleteTargetIfUnchanged(path, address); cleanupErr != nil {
			return targetFatal, cleanupErr
		}
		return targetUnavailable, nil
	}
	defer client.Close()

	var probe bool
	if err := client.ExecLua(probeLua, &probe); err != nil {
		if cleanupErr := c.deleteTargetIfUnchanged(path, address); cleanupErr != nil {
			return targetFatal, cleanupErr
		}
		return targetUnavailable, nil
	}

	var response string
	if err := client.ExecLua(remoteLoadLua, &response, c.transcriptID); err != nil || response != remoteOK {
		return targetFatal, errors.New("Neovim target was reachable but transcript loading failed")
	}
	return targetDelivered, nil
}

func dialNvim(address string) (*nvim.Nvim, error) {
	var dialer net.Dialer
	return nvim.Dial(
		address,
		nvim.DialNetDial(func(ctx context.Context, _ string, address string) (net.Conn, error) {
			return dialer.DialContext(ctx, "unix", address)
		}),
		nvim.DialLogf(func(string, ...interface{}) {}),
	)
}

func (c *command) readTarget(path string) (bool, string, error) {
	var exists bool
	var value string
	err := c.withRuntimeLock(func() error {
		exists, value = readTargetLocked(path)
		return nil
	})
	return exists, value, err
}

func readTargetLocked(path string) (bool, string) {
	if _, err := os.Lstat(path); err != nil {
		return false, ""
	}

	file, err := os.Open(path)
	if err != nil {
		return true, ""
	}
	defer file.Close()

	line, _ := bufio.NewReader(file).ReadString('\n')
	return true, strings.Trim(line, " \t\r\n\v\f")
}

func (c *command) deleteTargetIfUnchanged(path, expected string) error {
	return c.withRuntimeLock(func() error {
		exists, value := readTargetLocked(path)
		if !exists || value != expected {
			return nil
		}
		if err := removeNonDirectory(path); err != nil {
			return fmt.Errorf("cannot remove unusable target: %s", path)
		}
		return nil
	})
}

func (c *command) withRuntimeLock(fn func() error) error {
	dir, err := os.Open(c.runtimeDir)
	if err != nil {
		return errors.New("cannot open runtime directory for locking")
	}
	defer dir.Close()

	for {
		err = syscall.Flock(int(dir.Fd()), syscall.LOCK_EX)
		if err != syscall.EINTR {
			break
		}
	}
	if err != nil {
		return errors.New("cannot lock runtime directory")
	}
	defer syscall.Flock(int(dir.Fd()), syscall.LOCK_UN)

	return fn()
}

func (c *command) startDefaultEditor() error {
	command, args := c.defaultEditorInvocation()
	return execShellCommand(command, args...)
}

func (c *command) defaultEditorInvocation() (string, []string) {
	command := c.nvimCmd
	if c.launchCmd != "" {
		command = c.launchCmd + " " + c.nvimCmd
	}

	startup := fmt.Sprintf(`lua require("talk2text")._default_start(%d)`, c.transcriptID)
	return command, []string{"-c", startup}
}

func (c *command) handleBlank() {
	c.removeTranscript()
	c.notify("Blank transcript")
}

func (c *command) handleShort() error {
	c.removeTranscript()
	path := c.runtimeDir + "/" + targetName
	if err := c.withRuntimeLock(func() error { return removeNonDirectory(path) }); err != nil {
		return errors.New("cannot reset target to default")
	}
	c.notify("Target reset to default")
	return nil
}

func (c *command) removeTranscript() {
	if err := removeNonDirectory(c.transcript); err != nil {
		fmt.Fprintln(os.Stderr, "talk2text-nvim: cannot remove transcript")
	}
}

func removeNonDirectory(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.IsDir() {
		return errors.New("path is a directory")
	}
	return os.Remove(path)
}

func (c *command) notify(message string) {
	if c.notifyCmd != "" {
		if err := startDetachedHook(c.notifyCmd, message); err != nil {
			fmt.Fprintf(os.Stderr, "talk2text-nvim: cannot start notification hook: %v\n", err)
		}
	}
}

func (c *command) focusDefaultEditor() {
	if c.focusCmd != "" {
		if err := startDetachedHook(c.focusCmd); err != nil {
			fmt.Fprintf(os.Stderr, "talk2text-nvim: cannot start focus hook: %v\n", err)
		}
	}
}

func startDetachedHook(code string, args ...string) error {
	commandArgs := []string{"-c", code + ` "$@"`, "talk2text-nvim-hook"}
	commandArgs = append(commandArgs, args...)
	cmd := exec.Command("sh", commandArgs...)
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	_ = cmd.Process.Release()
	return nil
}

func execShellCommand(code string, args ...string) error {
	commandArgs := []string{"sh", "-c", code + ` "$@"`, "talk2text-nvim-hook"}
	commandArgs = append(commandArgs, args...)
	return syscall.Exec("/bin/sh", commandArgs, os.Environ())
}

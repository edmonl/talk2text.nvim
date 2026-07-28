// Package command implements the talk2text Neovim output command.
package command

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/edmonl/talk2text.nvim/internal/command/runtime"
	"github.com/edmonl/talk2text.nvim/internal/command/target"
	"github.com/edmonl/talk2text.nvim/internal/util"
	"github.com/neovim/go-client/nvim"
)

const (
	nvimEnvPrefix = "TALK2TEXT_NVIM_"
	outputKindEnv = "TALK2TEXT_OUTPUT_KIND"
	remoteLoadLua = `local called, loaded = pcall(require("talk2text").load, ...); return called and loaded`
)

type targetResult int

const (
	targetUnavailable targetResult = iota
	targetDelivered
	targetFatal
)

// Command routes a transcript event to Neovim.
type Command struct {
	notifyCmd      string
	launchCmd      string
	focusCmd       string
	runtimeDir     string
	transcriptPath string
	transcriptID   int
	shellPath      string
}

// TranscriptID returns the validated transcript ID handled by the command.
func (c *Command) TranscriptID() int {
	return c.transcriptID
}

// Parse validates output-command arguments and creates a Command using the
// configured hooks.
func Parse(args []string) (*Command, string, error) {
	if len(args) != 1 {
		return nil, "", errors.New("usage: TALK2TEXT_OUTPUT_KIND=<text|blank|short> talk2text-nvim <path>")
	}

	kind, transcriptPath := os.Getenv(outputKindEnv), args[0]
	switch kind {
	case "text", "blank", "short":
	default:
		return nil, "", fmt.Errorf("unknown transcript kind %s", kind)
	}

	runtimeDir, transcriptID, err := parseTranscriptPath(transcriptPath)
	if err != nil {
		return nil, "", err
	}

	return &Command{
		// The daemon supplies an executable name or path, so preserve it verbatim.
		notifyCmd:      os.Getenv("TALK2TEXT_NOTIFY_CMD"),
		launchCmd:      strings.TrimSpace(environmentOrDefault("TALK2TEXT_NVIM_LAUNCH_CMD", "nvim")),
		focusCmd:       strings.TrimSpace(os.Getenv("TALK2TEXT_NVIM_FOCUS_CMD")),
		runtimeDir:     runtimeDir,
		transcriptPath: transcriptPath,
		transcriptID:   transcriptID,
	}, kind, nil
}

func parseTranscriptPath(transcriptPath string) (string, int, error) {
	transcriptDir, filename := filepath.Split(transcriptPath)
	if !filepath.IsAbs(transcriptPath) || filename == "" {
		return "", 0, errors.New("transcript path must be an absolute file path")
	}

	transcriptDir = filepath.Clean(transcriptDir)
	if filepath.Base(transcriptDir) != "transcripts" {
		return "", 0, errors.New("transcript path must be directly under a transcripts directory")
	}

	runtimeDir := filepath.Dir(transcriptDir)
	if filepath.Dir(runtimeDir) == runtimeDir {
		return "", 0, errors.New("runtime directory must not be the filesystem root")
	}

	info, err := os.Stat(runtimeDir)
	if err != nil || !info.IsDir() {
		return "", 0, errors.New("runtime directory is unavailable")
	}

	id, err := strconv.Atoi(filename)
	if err != nil || id < 1 || strconv.Itoa(id) != filename {
		return "", 0, errors.New("transcript filename must be <positive-id>")
	}
	return runtimeDir, id, nil
}

func environmentOrDefault(name, fallback string) string {
	if value, ok := os.LookupEnv(name); ok {
		return value
	}
	return fallback
}

// HandleText routes a text transcript to a target or starts the default editor.
func (c *Command) HandleText() error {
	switch result, err := c.tryTarget(target.NormalTarget); result {
	case targetDelivered:
		return nil
	case targetFatal:
		return c.notifyError(err)
	}

	switch result, err := c.tryTarget(target.DefaultTarget); result {
	case targetDelivered:
		c.focusDefault()
		return nil
	case targetFatal:
		return c.notifyError(err)
	}

	err := c.launchDefault()
	if err != nil {
		return c.notifyError(fmt.Errorf("cannot launch default: %w", err))
	}
	return nil
}

func (c *Command) tryTarget(name string) (targetResult, error) {
	address, err := target.Read(c.runtimeDir, name)
	if err != nil {
		return targetFatal, err
	}
	if address == "" {
		return targetUnavailable, nil
	}
	if !filepath.IsAbs(address) {
		invalidErr := fmt.Errorf("target socket path must be absolute: %s", address)
		if _, cleanupErr := target.Delete(c.runtimeDir, name, address); cleanupErr != nil {
			return targetFatal, fmt.Errorf("%w; %w", invalidErr, cleanupErr)
		}
		return targetFatal, invalidErr
	}

	client, err := dialNvim(address)
	if err != nil {
		return c.handleStaleTarget(name, address)
	}
	defer client.Close()

	var probe bool
	if err := client.ExecLua("return true", &probe); err != nil {
		return c.handleStaleTarget(name, address)
	}

	var loaded bool
	if err := client.ExecLua(remoteLoadLua, &loaded, c.transcriptID); err != nil {
		return targetFatal, fmt.Errorf("failed to load transcript to %s: %w", address, err)
	}
	if !loaded {
		return targetFatal, fmt.Errorf("transcript rejected by %s", address)
	}
	return targetDelivered, nil
}

func (c *Command) handleStaleTarget(name, address string) (targetResult, error) {
	removed, err := target.Delete(c.runtimeDir, name, address)
	if err != nil {
		return targetFatal, err
	}
	if removed {
		c.notifyInfo(fmt.Sprintf("Stale target %s removed", address))
	}
	return targetUnavailable, nil
}

func dialNvim(address string) (*nvim.Nvim, error) {
	var dialer net.Dialer
	return nvim.Dial(
		address,
		nvim.DialNetDial(func(ctx context.Context, _ string, address string) (net.Conn, error) {
			return dialer.DialContext(ctx, "unix", address)
		}),
		nvim.DialLogf(func(string, ...any) {}),
	)
}

func (c *Command) launchDefault() error {
	if c.launchCmd == "" {
		return errors.New("TALK2TEXT_NVIM_LAUNCH_CMD is required")
	}
	shell, err := c.shell()
	if err != nil {
		return err
	}
	code := c.launchCmd + ` "$@"`
	return syscall.Exec(shell, []string{"sh", "-c", code, "talk2text-nvim-launch", c.transcriptPath}, childEnvironment())
}

// HandleBlank removes a blank transcript and notifies the user.
func (c *Command) HandleBlank() {
	c.removeTranscript()
	c.notifyInfo(fmt.Sprintf("Blank transcript %d", c.transcriptID))
}

// HandleShort removes a short transcript and resets the explicit target.
func (c *Command) HandleShort() error {
	c.removeTranscript()
	path := filepath.Join(c.runtimeDir, target.NormalTarget)
	if err := runtime.WithLock(c.runtimeDir, func() error { return util.RemovePath(path) }); err != nil {
		return c.notifyError(fmt.Errorf("cannot reset target to default: %w", err))
	}
	c.notifyInfo("Target reset to default")
	return nil
}

func (c *Command) removeTranscript() {
	if err := util.RemovePath(c.transcriptPath); err != nil {
		fmt.Fprintf(os.Stderr, "talk2text-nvim: cannot remove transcript %s: %v\n", c.transcriptPath, err)
	}
}

func (c *Command) notifyError(err error) error {
	c.notify("error", "output-command", fmt.Sprintf("Error: %v", err))
	return err
}

func (c *Command) notifyInfo(message string) {
	c.notify("info", "nvim", message)
}

func (c *Command) notify(level, code, message string) {
	if c.notifyCmd == "" {
		return
	}

	cmd := exec.Command(c.notifyCmd, message)
	cmd.Env = append(childEnvironment(),
		"TALK2TEXT_NOTIFY_LEVEL="+level,
		"TALK2TEXT_NOTIFY_CODE="+code,
	)
	if err := util.RunCmdDetached(cmd); err != nil {
		fmt.Fprintf(os.Stderr, "talk2text-nvim: transcript %d has notification command error: %v\n", c.transcriptID, err)
	}
}

func (c *Command) focusDefault() {
	if c.focusCmd == "" {
		return
	}

	shell, err := c.shell()
	if err == nil {
		cmd := exec.Command(shell, "-c", c.focusCmd)
		cmd.Env = childEnvironment()
		err = util.RunCmdDetached(cmd)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "talk2text-nvim: transcript %d has focus command error: %v\n", c.transcriptID, err)
	}
}

func (c *Command) shell() (string, error) {
	if c.shellPath == "" {
		path, err := exec.LookPath("sh")
		if err != nil {
			return "", err
		}
		c.shellPath = path
	}
	return c.shellPath, nil
}

func childEnvironment() []string {
	environment := os.Environ()
	filtered := environment[:0]
	for _, setting := range environment {
		if !strings.HasPrefix(setting, nvimEnvPrefix) {
			filtered = append(filtered, setting)
		}
	}
	return filtered
}

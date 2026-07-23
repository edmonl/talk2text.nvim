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
	"syscall"

	"github.com/edmonl/talk2text.nvim/internal/command/runtime"
	"github.com/edmonl/talk2text.nvim/internal/command/target"
	"github.com/edmonl/talk2text.nvim/internal/util"
	"github.com/neovim/go-client/nvim"
)

const (
	remoteOK         = "ok"
	probeLua         = "return true"
	remoteLoadLua    = `return require("talk2text")._remote_load(...)`
	defaultNotifyCmd = "notify-send -a talk2text -u normal -t 5000 Talk2text"
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

// New creates a Command using the configured editor and notification hooks.
func New(runtimeDir, transcriptPath string, transcriptID int) *Command {
	return &Command{
		notifyCmd:      notificationCommand(),
		launchCmd:      environmentOrDefault("TALK2TEXT_NVIM_LAUNCH_CMD", "nvim"),
		focusCmd:       environmentOrDefault("TALK2TEXT_NVIM_FOCUS_CMD", ""),
		runtimeDir:     runtimeDir,
		transcriptPath: transcriptPath,
		transcriptID:   transcriptID,
	}
}

func notificationCommand() string {
	if value, ok := os.LookupEnv("TALK2TEXT_NVIM_NOTIFY_CMD"); ok {
		return value
	}
	if _, err := exec.LookPath("notify-send"); err != nil {
		return ""
	}
	return defaultNotifyCmd
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
		return c.notifyError(fmt.Errorf("cannot launch editor: %w", err))
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
	if err := client.ExecLua(probeLua, &probe); err != nil {
		return c.handleStaleTarget(name, address)
	}

	var response string
	if err := client.ExecLua(remoteLoadLua, &response, c.transcriptID); err != nil || response != remoteOK {
		if err == nil {
			err = errors.New(response)
		}
		return targetFatal, fmt.Errorf("failed to load transcript %d: %w", c.transcriptID, err)
	}
	return targetDelivered, nil
}

func (c *Command) handleStaleTarget(name, address string) (targetResult, error) {
	removed, err := target.Delete(c.runtimeDir, name, address)
	if err != nil {
		return targetFatal, err
	}
	if removed {
		c.notify(fmt.Sprintf("Stale target %s removed", address))
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
	code := fmt.Sprintf(`%s -c 'lua require("talk2text")._default_start(%d)'`, c.launchCmd, c.transcriptID)
	return syscall.Exec(shell, []string{"sh", "-c", code}, os.Environ())
}

// HandleBlank removes a blank transcript and notifies the user.
func (c *Command) HandleBlank() {
	c.removeTranscript()
	c.notify("Blank transcript")
}

// HandleShort removes a short transcript and resets the explicit target.
func (c *Command) HandleShort() error {
	c.removeTranscript()
	path := filepath.Join(c.runtimeDir, target.NormalTarget)
	if err := runtime.WithLock(c.runtimeDir, func() error { return util.RemovePath(path) }); err != nil {
		return c.notifyError(fmt.Errorf("cannot reset target to default: %w", err))
	}
	c.notify("Target reset to default")
	return nil
}

func (c *Command) removeTranscript() {
	if err := util.RemovePath(c.transcriptPath); err != nil {
		fmt.Fprintf(os.Stderr, "talk2text-nvim: cannot remove transcript %s: %s\n", c.transcriptPath, err)
	}
}

func (c *Command) notifyError(err error) error {
	c.notify("Error: " + err.Error())
	return err
}

func (c *Command) notify(message string) {
	if c.notifyCmd == "" {
		return
	}

	shell, err := c.shell()
	if err == nil {
		commandArgs := []string{"-c", c.notifyCmd + ` "$@"`, "talk2text-nvim-hook", message}
		err = util.RunCmdDetached(shell, commandArgs...)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "talk2text-nvim: cannot start notification command: %s\n", err)
	}
}

func (c *Command) focusDefault() {
	if c.focusCmd == "" {
		return
	}

	shell, err := c.shell()
	if err == nil {
		err = util.RunCmdDetached(shell, "-c", c.focusCmd)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "talk2text-nvim: cannot start focus command: %s\n", err)
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

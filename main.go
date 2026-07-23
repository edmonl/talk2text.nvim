package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/edmonl/talk2text.nvim/internal/command"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "talk2text-nvim: %s\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) != 2 {
		return errors.New("usage: talk2text-nvim <text|blank|short> <path>")
	}

	kind, transcriptPath := args[0], args[1]
	runtimeDir, clipID, err := parseTranscriptPath(transcriptPath)
	if err != nil {
		return err
	}
	c := command.New(runtimeDir, transcriptPath, clipID)

	switch kind {
	case "text":
		return c.HandleText()
	case "blank":
		c.HandleBlank()
	case "short":
		return c.HandleShort()
	default:
		return fmt.Errorf("unknown transcript kind: %s", kind)
	}

	return nil
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

	value, found := strings.CutSuffix(filename, ".txt")
	if !found {
		return "", 0, errors.New("transcript filename must be <positive-id>.txt")
	}

	id, err := strconv.Atoi(value)
	if err != nil || id < 1 || strconv.Itoa(id) != value {
		return "", 0, errors.New("transcript filename must be <positive-id>.txt")
	}
	return runtimeDir, id, nil
}

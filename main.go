package main

import (
	"fmt"
	"os"

	"github.com/edmonl/talk2text.nvim/internal/command"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "talk2text-nvim: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	c, kind, err := command.Parse(args)
	if err != nil {
		return err
	}

	switch kind {
	case "text":
		err = c.HandleText()
	case "blank":
		c.HandleBlank()
	case "short":
		err = c.HandleShort()
	}

	if err != nil {
		err = fmt.Errorf("failed to process transcript %d: %w", c.TranscriptID(), err)
	}

	return err
}

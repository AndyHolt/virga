package cmd

import (
	"os"

	"github.com/charmbracelet/x/term"
)

func isInteractiveTerminal() bool {
	return term.IsTerminal(os.Stdin.Fd()) && term.IsTerminal(os.Stderr.Fd())
}

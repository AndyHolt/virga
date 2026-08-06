package cmd

import (
	"fmt"
	"io"

	"github.com/charmbracelet/huh"
)

type branchSelector func(io.Reader, io.Writer, []string) (string, error)

func selectHuhBranch(input io.Reader, prompt io.Writer, branches []string) (string, error) {
	if len(branches) == 0 {
		return "", fmt.Errorf("no local branches are available to select")
	}

	var selected string
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select a base branch").
				Options(huh.NewOptions(branches...)...).
				Value(&selected),
		),
	).WithInput(input).WithOutput(prompt)
	if err := form.Run(); err != nil {
		return "", fmt.Errorf("run branch selection: %w", err)
	}
	return selected, nil
}

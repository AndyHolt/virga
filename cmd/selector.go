package cmd

import (
	"fmt"
	"io"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

type branchSelector func(io.Reader, io.Writer, []string) (string, error)

const terminalAccent lipgloss.ANSIColor = 4

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
	).WithTheme(branchSelectorTheme()).WithInput(input).WithOutput(prompt)
	if err := form.Run(); err != nil {
		return "", fmt.Errorf("run branch selection: %w", err)
	}
	return selected, nil
}

// branchSelectorTheme uses terminal-default colors for text and the terminal's
// ANSI blue palette entry as an accent, so it remains legible across themes.
func branchSelectorTheme() *huh.Theme {
	theme := huh.ThemeBase()
	theme.Focused.Base = theme.Focused.Base.BorderForeground(terminalAccent)
	theme.Focused.Title = lipgloss.NewStyle().Bold(true).Foreground(terminalAccent)
	theme.Focused.SelectSelector = lipgloss.NewStyle().
		Bold(true).
		Foreground(terminalAccent).
		SetString("> ")
	theme.Focused.SelectedOption = lipgloss.NewStyle().Bold(true).Reverse(true)
	theme.Focused.UnselectedOption = lipgloss.NewStyle()

	theme.Help.Ellipsis = lipgloss.NewStyle()
	theme.Help.ShortKey = lipgloss.NewStyle().Bold(true).Foreground(terminalAccent)
	theme.Help.ShortDesc = lipgloss.NewStyle()
	theme.Help.ShortSeparator = lipgloss.NewStyle()
	theme.Help.FullKey = lipgloss.NewStyle().Bold(true).Foreground(terminalAccent)
	theme.Help.FullDesc = lipgloss.NewStyle()
	theme.Help.FullSeparator = lipgloss.NewStyle()
	return theme
}

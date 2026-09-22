package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestBranchSelectorThemeUsesTerminalDefaultsAndANSIColours(t *testing.T) {
	theme := branchSelectorTheme()
	assertTerminalDefault(t, theme.Focused.UnselectedOption)
	for _, style := range []lipgloss.Style{
		theme.Focused.Title,
		theme.Focused.SelectSelector,
		theme.Help.ShortKey,
		theme.Help.FullKey,
	} {
		assertANSIColour(t, style, terminalAccent)
	}
	for _, style := range []lipgloss.Style{
		theme.Help.Ellipsis,
		theme.Help.ShortDesc,
		theme.Help.ShortSeparator,
		theme.Help.FullDesc,
		theme.Help.FullSeparator,
	} {
		assertANSIColour(t, style, terminalMuted)
	}
	borderColour := theme.Focused.Base.GetBorderLeftForeground()
	if colour, ok := borderColour.(lipgloss.ANSIColor); !ok || colour != terminalAccent {
		t.Errorf("border foreground = %v, want ANSI colour %d", borderColour, terminalAccent)
	}
	if !theme.Focused.SelectedOption.GetReverse() {
		t.Error("selected option is not reverse video")
	}
}

func assertTerminalDefault(t *testing.T, style lipgloss.Style) {
	t.Helper()
	if _, ok := style.GetForeground().(lipgloss.NoColor); !ok {
		t.Errorf("foreground = %T, want terminal default", style.GetForeground())
	}
}

func assertANSIColour(t *testing.T, style lipgloss.Style, want lipgloss.ANSIColor) {
	t.Helper()
	colour, ok := style.GetForeground().(lipgloss.ANSIColor)
	if !ok || colour != want {
		t.Errorf("foreground = %v, want ANSI colour %d", style.GetForeground(), want)
	}
}

func TestSelectHuhBranchRejectsEmptyBranches(t *testing.T) {
	_, err := selectHuhBranch(strings.NewReader(""), &bytes.Buffer{}, nil)
	if err == nil || !strings.Contains(err.Error(), "no local branches are available") {
		t.Fatalf("selectHuhBranch() error = %v, want empty branch list error", err)
	}
}

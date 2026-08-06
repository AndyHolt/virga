package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestBranchSelectorThemeUsesTerminalDefaultsAndANSIAccent(t *testing.T) {
	theme := branchSelectorTheme()
	for _, style := range []lipgloss.Style{
		theme.Focused.UnselectedOption,
		theme.Help.Ellipsis,
		theme.Help.ShortDesc,
		theme.Help.ShortSeparator,
		theme.Help.FullDesc,
		theme.Help.FullSeparator,
	} {
		if _, ok := style.GetForeground().(lipgloss.NoColor); !ok {
			t.Errorf("foreground = %T, want terminal default", style.GetForeground())
		}
	}
	for _, style := range []lipgloss.Style{
		theme.Focused.Title,
		theme.Focused.SelectSelector,
		theme.Help.ShortKey,
		theme.Help.FullKey,
	} {
		color, ok := style.GetForeground().(lipgloss.ANSIColor)
		if !ok || color != terminalAccent {
			t.Errorf("foreground = %v, want ANSI color %d", style.GetForeground(), terminalAccent)
		}
	}
	borderColor := theme.Focused.Base.GetBorderLeftForeground()
	if color, ok := borderColor.(lipgloss.ANSIColor); !ok || color != terminalAccent {
		t.Errorf("border foreground = %v, want ANSI color %d", borderColor, terminalAccent)
	}
	if !theme.Focused.SelectedOption.GetReverse() {
		t.Error("selected option is not reverse video")
	}
}

func TestSelectHuhBranchRejectsEmptyBranches(t *testing.T) {
	_, err := selectHuhBranch(strings.NewReader(""), &bytes.Buffer{}, nil)
	if err == nil || !strings.Contains(err.Error(), "no local branches are available") {
		t.Fatalf("selectHuhBranch() error = %v, want empty branch list error", err)
	}
}

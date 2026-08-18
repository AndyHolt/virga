// Package tmux creates tmux sessions for Virga worktrees.
package tmux

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/AndyHolt/virga/internal/config"
)

// ErrNotInstalled reports that the tmux executable could not be found.
var ErrNotInstalled = errors.New("tmux is not installed")

// Command describes one tmux subprocess invocation.
type Command struct {
	Path       string
	Args       []string
	WorkingDir string
}

// Runner runs a tmux subprocess.
type Runner func(context.Context, Command) error

// Dependencies contains external operations used by Manager. Nil fields use the
// process environment and os/exec.
type ManagerDependencies struct {
	LookPath func(string) (string, error)
	Run      Runner
}

// Manager creates tmux sessions.
type Manager struct {
	lookPath func(string) (string, error)
	run      Runner
}

// CreateSessionOptions describes the session Virga should create.
type CreateSessionOptions struct {
	RepositoryRoot string
	Branch         string
	WorktreeRoot   string
	Tmux           config.TmuxConfig
}

// CreateSession creates a tmux session using the default process dependencies.
func CreateSession(ctx context.Context, options CreateSessionOptions) (string, error) {
	return NewManager(ManagerDependencies{}).CreateSession(ctx, options)
}

// NewManager constructs a tmux session manager.
func NewManager(dependencies ManagerDependencies) Manager {
	manager := Manager{
		lookPath: dependencies.LookPath,
		run:      dependencies.Run,
	}
	if manager.lookPath == nil {
		manager.lookPath = exec.LookPath
	}
	if manager.run == nil {
		manager.run = runCommand
	}
	return manager
}

// CreateSession creates a detached tmux session for a worktree and returns its
// deterministic session name.
func (m Manager) CreateSession(ctx context.Context, options CreateSessionOptions) (string, error) {
	if strings.TrimSpace(options.RepositoryRoot) == "" {
		return "", fmt.Errorf("create tmux session: repository root is required")
	}
	if strings.TrimSpace(options.Branch) == "" {
		return "", fmt.Errorf("create tmux session: branch is required")
	}
	if strings.TrimSpace(options.WorktreeRoot) == "" {
		return "", fmt.Errorf("create tmux session: worktree root is required")
	}

	windows, err := sessionWindows(options.Tmux)
	if err != nil {
		return "", err
	}

	tmuxPath, err := m.lookPath("tmux")
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrNotInstalled, err)
	}

	sessionName := SessionName(options.RepositoryRoot, options.Branch)
	firstWindow := windows[0]
	args := []string{"new-session", "-d", "-s", sessionName, "-c", options.WorktreeRoot}
	if firstWindow.Name != "" {
		args = append(args, "-n", firstWindow.Name)
	}
	if err := m.runTmux(ctx, tmuxPath, options.WorktreeRoot, args...); err != nil {
		return "", fmt.Errorf("create tmux session %q: %w", sessionName, err)
	}

	for windowIndex, window := range windows {
		if windowIndex > 0 {
			if err := m.runTmux(
				ctx,
				tmuxPath,
				options.WorktreeRoot,
				"new-window", "-t", sessionName, "-c", options.WorktreeRoot, "-n", window.Name,
			); err != nil {
				return "", fmt.Errorf("create tmux window %q in session %q: %w", window.Name, sessionName, err)
			}
		}

		windowTarget := fmt.Sprintf("%s:%d", sessionName, windowIndex)
		for paneIndex, pane := range window.Panes {
			paneTarget := fmt.Sprintf("%s.%d", windowTarget, paneIndex)
			if paneIndex > 0 {
				if err := m.runTmux(ctx, tmuxPath, options.WorktreeRoot, "split-window", "-t", windowTarget, "-c", options.WorktreeRoot); err != nil {
					return "", fmt.Errorf("create tmux pane %d in window %q: %w", paneIndex, window.Name, err)
				}
			}
			if pane.Command != "" {
				if err := m.runTmux(ctx, tmuxPath, options.WorktreeRoot, "send-keys", "-t", paneTarget, pane.Command, "C-m"); err != nil {
					return "", fmt.Errorf("start command in tmux pane %d of window %q: %w", paneIndex, window.Name, err)
				}
			}
		}
	}

	return sessionName, nil
}

func (m Manager) runTmux(ctx context.Context, tmuxPath, worktreeRoot string, args ...string) error {
	return m.run(ctx, Command{
		Path:       tmuxPath,
		Args:       append([]string(nil), args...),
		WorkingDir: worktreeRoot,
	})
}

// sessionWindows applies tmux session creation defaults and validates the
// requirements needed to create tmux windows and panes.
func sessionWindows(configuration config.TmuxConfig) ([]config.TmuxWindow, error) {
	if len(configuration.Windows) == 0 {
		return []config.TmuxWindow{{Panes: []config.TmuxPane{{}}}}, nil
	}

	windows := make([]config.TmuxWindow, len(configuration.Windows))
	for index, window := range configuration.Windows {
		name := strings.TrimSpace(window.Name)
		if name == "" {
			return nil, fmt.Errorf("create tmux session: windows[%d].name is required", index)
		}
		if len(window.Panes) == 0 {
			return nil, fmt.Errorf("create tmux session: windows[%d] must have at least one pane", index)
		}
		panes := append([]config.TmuxPane(nil), window.Panes...)
		windows[index] = config.TmuxWindow{Name: name, Panes: panes}
	}
	return windows, nil
}

func runCommand(ctx context.Context, command Command) error {
	process := exec.CommandContext(ctx, command.Path, command.Args...)
	process.Dir = command.WorkingDir
	process.Env = append(os.Environ(), "LC_ALL=C")
	output, err := process.CombinedOutput()
	if err != nil {
		if len(output) > 0 {
			return fmt.Errorf("%w: %s", err, bytes.TrimSpace(output))
		}
		return err
	}
	return nil
}

// SessionName returns Virga's deterministic tmux session name for a repository
// and branch.
func SessionName(repositoryRoot, branch string) string {
	repository := filepath.Base(filepath.Clean(repositoryRoot))
	prefix := strings.Trim(sanitizeSessionComponent(repository)+"_"+sanitizeSessionComponent(branch), "-_")
	if prefix == "" {
		prefix = "virga"
	}
	const maxPrefixLength = 80
	if len(prefix) > maxPrefixLength {
		prefix = strings.TrimRight(prefix[:maxPrefixLength], "-_")
		if prefix == "" {
			prefix = "virga"
		}
	}

	hash := sha256.Sum256([]byte(filepath.Clean(repositoryRoot) + "\x00" + branch))
	return prefix + "_" + hex.EncodeToString(hash[:])[:8]
}

func sanitizeSessionComponent(value string) string {
	var builder strings.Builder
	lastWasSeparator := false
	for _, character := range value {
		if character == '_' || character == '-' || isASCIILetter(character) || isASCIIDigit(character) {
			builder.WriteRune(character)
			lastWasSeparator = false
			continue
		}
		if !lastWasSeparator {
			builder.WriteByte('-')
			lastWasSeparator = true
		}
	}
	return strings.Trim(builder.String(), "-_")
}

func isASCIILetter(character rune) bool {
	return (character >= 'A' && character <= 'Z') || (character >= 'a' && character <= 'z')
}

func isASCIIDigit(character rune) bool {
	return character >= '0' && character <= '9'
}

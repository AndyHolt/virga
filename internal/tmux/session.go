// Package tmux creates tmux sessions for Virga worktrees.
package tmux

import (
	"bytes"
	"context"
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

// ManagerDependencies contains external operations used by Manager. Nil fields
// use the process environment and os/exec.
type ManagerDependencies struct {
	LookPath       func(string) (string, error)
	LookupEnv      func(string) (string, bool)
	Run            Runner
	RunInteractive Runner
}

// Manager creates tmux sessions.
type Manager struct {
	lookPath       func(string) (string, error)
	lookupEnv      func(string) (string, bool)
	run            Runner
	runInteractive Runner
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

// AttachSession attaches the current terminal to a tmux session using the
// default process dependencies.
func AttachSession(ctx context.Context, sessionName string) error {
	return NewManager(ManagerDependencies{}).AttachSession(ctx, sessionName)
}

// HasSession reports whether a tmux session exists using the default process
// dependencies.
func HasSession(ctx context.Context, sessionName string) (bool, error) {
	return NewManager(ManagerDependencies{}).HasSession(ctx, sessionName)
}

// NewManager constructs a tmux session manager.
func NewManager(dependencies ManagerDependencies) Manager {
	manager := Manager{
		lookPath:       dependencies.LookPath,
		lookupEnv:      dependencies.LookupEnv,
		run:            dependencies.Run,
		runInteractive: dependencies.RunInteractive,
	}
	if manager.lookPath == nil {
		manager.lookPath = exec.LookPath
	}
	if manager.lookupEnv == nil {
		manager.lookupEnv = os.LookupEnv
	}
	if manager.run == nil {
		manager.run = runCommand
	}
	if manager.runInteractive == nil {
		manager.runInteractive = runInteractiveCommand
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

	sessionName := SessionName(options.WorktreeRoot)
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

// AttachSession attaches the current terminal to an existing tmux session.
func (m Manager) AttachSession(ctx context.Context, sessionName string) error {
	if strings.TrimSpace(sessionName) == "" {
		return fmt.Errorf("attach tmux session: session name is required")
	}

	tmuxPath, err := m.lookPath("tmux")
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNotInstalled, err)
	}
	args := []string{"attach-session", "-t", sessionName}
	if _, insideTmux := m.lookupEnv("TMUX"); insideTmux {
		args = []string{"switch-client", "-t", sessionName}
	}
	if err := m.runInteractive(ctx, Command{
		Path: tmuxPath,
		Args: args,
	}); err != nil {
		return fmt.Errorf("attach tmux session %q: %w", sessionName, err)
	}
	return nil
}

// HasSession reports whether an existing tmux session is available.
func (m Manager) HasSession(ctx context.Context, sessionName string) (bool, error) {
	if strings.TrimSpace(sessionName) == "" {
		return false, fmt.Errorf("check tmux session: session name is required")
	}

	tmuxPath, err := m.lookPath("tmux")
	if err != nil {
		return false, fmt.Errorf("%w: %v", ErrNotInstalled, err)
	}
	if err := m.run(ctx, Command{
		Path: tmuxPath,
		Args: []string{"has-session", "-t", sessionName},
	}); err != nil {
		if hasExitCode(err, 1) {
			return false, nil
		}
		return false, fmt.Errorf("check tmux session %q: %w", sessionName, err)
	}
	return true, nil
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

func runInteractiveCommand(ctx context.Context, command Command) error {
	process := exec.CommandContext(ctx, command.Path, command.Args...)
	process.Dir = command.WorkingDir
	process.Env = append(os.Environ(), "LC_ALL=C")
	process.Stdin = os.Stdin
	process.Stdout = os.Stdout
	process.Stderr = os.Stderr
	return process.Run()
}

type exitCoder interface {
	ExitCode() int
}

func hasExitCode(err error, code int) bool {
	var exitErr exitCoder
	return errors.As(err, &exitErr) && exitErr.ExitCode() == code
}

// SessionName returns Virga's deterministic tmux session name for a worktree.
func SessionName(worktreeRoot string) string {
	sessionName := sanitizeSessionComponent(filepath.Base(filepath.Clean(worktreeRoot)))
	if sessionName == "" {
		sessionName = "virga"
	}
	const maxSessionNameLength = 80
	if len(sessionName) > maxSessionNameLength {
		sessionName = strings.TrimRight(sessionName[:maxSessionNameLength], "-_")
		if sessionName == "" {
			sessionName = "virga"
		}
	}
	return sessionName
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

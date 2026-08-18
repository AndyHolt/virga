package tmux

import (
	"context"
	"errors"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/AndyHolt/virga/internal/config"
)

func TestCreateSessionCreatesDefaultSession(t *testing.T) {
	runner := &recordingRunner{}
	manager := NewManager(ManagerDependencies{
		LookPath: func(name string) (string, error) {
			if name != "tmux" {
				t.Errorf("LookPath(%q), want tmux", name)
			}
			return "/usr/bin/tmux", nil
		},
		Run: runner.run,
	})
	options := CreateSessionOptions{
		RepositoryRoot: "/repositories/virga",
		Branch:         "feature/login",
		WorktreeRoot:   "/repositories/virga_feature-login",
	}

	session, err := manager.CreateSession(context.Background(), options)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	wantSession := SessionName(options.RepositoryRoot, options.Branch)
	if session != wantSession {
		t.Fatalf("session = %q, want %q", session, wantSession)
	}
	want := []Command{{
		Path:       "/usr/bin/tmux",
		Args:       []string{"new-session", "-d", "-s", wantSession, "-c", options.WorktreeRoot},
		WorkingDir: options.WorktreeRoot,
	}}
	if !reflect.DeepEqual(runner.commands, want) {
		t.Fatalf("commands = %#v, want %#v", runner.commands, want)
	}
}

func TestCreateSessionCreatesConfiguredWindowsAndPanes(t *testing.T) {
	runner := &recordingRunner{}
	manager := NewManager(ManagerDependencies{
		LookPath: func(string) (string, error) { return "/opt/bin/tmux", nil },
		Run:      runner.run,
	})
	options := CreateSessionOptions{
		RepositoryRoot: "/repositories/main repository",
		Branch:         "feature/login",
		WorktreeRoot:   "/repositories/main repository_feature-login",
		Tmux: config.TmuxConfig{Windows: []config.TmuxWindow{
			{
				Name: "editor",
				Panes: []config.TmuxPane{
					{Command: "nvim"},
					{Command: "make test"},
				},
			},
			{
				Name:  "server",
				Panes: []config.TmuxPane{{Command: "go run ./cmd/server"}},
			},
		}},
	}

	session, err := manager.CreateSession(context.Background(), options)
	if err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	wantSession := SessionName(options.RepositoryRoot, options.Branch)
	if session != wantSession {
		t.Fatalf("session = %q, want %q", session, wantSession)
	}
	want := []Command{
		{Path: "/opt/bin/tmux", Args: []string{"new-session", "-d", "-s", wantSession, "-c", options.WorktreeRoot, "-n", "editor"}, WorkingDir: options.WorktreeRoot},
		{Path: "/opt/bin/tmux", Args: []string{"send-keys", "-t", wantSession + ":0.0", "nvim", "C-m"}, WorkingDir: options.WorktreeRoot},
		{Path: "/opt/bin/tmux", Args: []string{"split-window", "-t", wantSession + ":0", "-c", options.WorktreeRoot}, WorkingDir: options.WorktreeRoot},
		{Path: "/opt/bin/tmux", Args: []string{"send-keys", "-t", wantSession + ":0.1", "make test", "C-m"}, WorkingDir: options.WorktreeRoot},
		{Path: "/opt/bin/tmux", Args: []string{"new-window", "-t", wantSession, "-c", options.WorktreeRoot, "-n", "server"}, WorkingDir: options.WorktreeRoot},
		{Path: "/opt/bin/tmux", Args: []string{"send-keys", "-t", wantSession + ":1.0", "go run ./cmd/server", "C-m"}, WorkingDir: options.WorktreeRoot},
	}
	if !reflect.DeepEqual(runner.commands, want) {
		t.Fatalf("commands = %#v, want %#v", runner.commands, want)
	}
}

func TestCreateSessionReportsMissingTmux(t *testing.T) {
	runner := &recordingRunner{}
	manager := NewManager(ManagerDependencies{
		LookPath: func(string) (string, error) { return "", exec.ErrNotFound },
		Run:      runner.run,
	})

	_, err := manager.CreateSession(context.Background(), CreateSessionOptions{
		RepositoryRoot: "/repositories/virga",
		Branch:         "feature",
		WorktreeRoot:   "/repositories/virga_feature",
	})
	if !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("CreateSession() error = %v, want %v", err, ErrNotInstalled)
	}
	if len(runner.commands) != 0 {
		t.Fatalf("commands = %#v, want no commands", runner.commands)
	}
}

func TestCreateSessionWrapsCommandFailure(t *testing.T) {
	runnerErr := errors.New("split failed")
	runner := &recordingRunner{failAt: 3, err: runnerErr}
	manager := NewManager(ManagerDependencies{
		LookPath: func(string) (string, error) { return "/usr/bin/tmux", nil },
		Run:      runner.run,
	})

	session, err := manager.CreateSession(context.Background(), CreateSessionOptions{
		RepositoryRoot: "/repositories/virga",
		Branch:         "feature",
		WorktreeRoot:   "/repositories/virga_feature",
		Tmux: config.TmuxConfig{Windows: []config.TmuxWindow{{
			Name:  "editor",
			Panes: []config.TmuxPane{{Command: "nvim"}, {Command: "make test"}},
		}}},
	})
	if session != "" {
		t.Errorf("session = %q, want empty on failure", session)
	}
	if !errors.Is(err, runnerErr) {
		t.Fatalf("CreateSession() error = %v, want wrapped %v", err, runnerErr)
	}
	if !strings.Contains(err.Error(), "create tmux pane 1") || !strings.Contains(err.Error(), "editor") {
		t.Fatalf("CreateSession() error = %v, want pane context", err)
	}
}

func TestCreateSessionValidatesOptions(t *testing.T) {
	valid := CreateSessionOptions{
		RepositoryRoot: "/repositories/virga",
		Branch:         "feature",
		WorktreeRoot:   "/repositories/virga_feature",
	}
	tests := []struct {
		name    string
		mutate  func(*CreateSessionOptions)
		wantErr string
	}{
		{name: "repository root", mutate: func(options *CreateSessionOptions) { options.RepositoryRoot = "" }, wantErr: "repository root is required"},
		{name: "branch", mutate: func(options *CreateSessionOptions) { options.Branch = "" }, wantErr: "branch is required"},
		{name: "worktree root", mutate: func(options *CreateSessionOptions) { options.WorktreeRoot = "" }, wantErr: "worktree root is required"},
		{name: "window name", mutate: func(options *CreateSessionOptions) {
			options.Tmux.Windows = []config.TmuxWindow{{Panes: []config.TmuxPane{{}}}}
		}, wantErr: "windows[0].name is required"},
		{name: "window panes", mutate: func(options *CreateSessionOptions) {
			options.Tmux.Windows = []config.TmuxWindow{{Name: "editor"}}
		}, wantErr: "windows[0] must have at least one pane"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			options := valid
			test.mutate(&options)
			manager := NewManager(ManagerDependencies{
				LookPath: func(string) (string, error) {
					t.Fatal("LookPath called after invalid options")
					return "", nil
				},
				Run: func(context.Context, Command) error {
					t.Fatal("Run called after invalid options")
					return nil
				},
			})

			_, err := manager.CreateSession(context.Background(), options)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("CreateSession() error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}

func TestSessionNameIsDeterministicAndTmuxSafe(t *testing.T) {
	first := SessionName("/repositories/main repository", "feature/login:api")
	second := SessionName("/repositories/main repository", "feature/login:api")
	if first != second {
		t.Fatalf("SessionName() = %q then %q, want deterministic", first, second)
	}
	if !strings.HasPrefix(first, "main-repository_feature-login-api_") {
		t.Fatalf("SessionName() = %q, want sanitized repository and branch prefix", first)
	}
	if strings.ContainsAny(first, ":/ ") {
		t.Fatalf("SessionName() = %q, want no tmux target separators or whitespace", first)
	}
	if other := SessionName("/repositories/main repository", "feature/login-api"); other == first {
		t.Fatalf("SessionName() collision for distinct branches: %q", first)
	}
}

type recordingRunner struct {
	commands []Command
	failAt   int
	err      error
}

func (runner *recordingRunner) run(_ context.Context, command Command) error {
	runner.commands = append(runner.commands, Command{
		Path:       command.Path,
		Args:       append([]string(nil), command.Args...),
		WorkingDir: command.WorkingDir,
	})
	if runner.failAt > 0 && len(runner.commands) == runner.failAt {
		return runner.err
	}
	return nil
}

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

	wantSession := SessionName(options.WorktreeRoot)
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

	wantSession := SessionName(options.WorktreeRoot)
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

func TestHasSessionReportsExistingSession(t *testing.T) {
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

	exists, err := manager.HasSession(context.Background(), "virga_feature")
	if err != nil {
		t.Fatalf("HasSession() error = %v", err)
	}
	if !exists {
		t.Fatal("HasSession() = false, want true")
	}

	want := []Command{{
		Path: "/usr/bin/tmux",
		Args: []string{"has-session", "-t", "virga_feature"},
	}}
	if !reflect.DeepEqual(runner.commands, want) {
		t.Fatalf("commands = %#v, want %#v", runner.commands, want)
	}
}

func TestHasSessionReportsMissingSession(t *testing.T) {
	runner := &recordingRunner{failAt: 1, err: exitCodeError{code: 1}}
	manager := NewManager(ManagerDependencies{
		LookPath: func(string) (string, error) { return "/usr/bin/tmux", nil },
		Run:      runner.run,
	})

	exists, err := manager.HasSession(context.Background(), "virga_feature")
	if err != nil {
		t.Fatalf("HasSession() error = %v", err)
	}
	if exists {
		t.Fatal("HasSession() = true, want false")
	}
}

func TestHasSessionReportsMissingTmux(t *testing.T) {
	runner := &recordingRunner{}
	manager := NewManager(ManagerDependencies{
		LookPath: func(string) (string, error) { return "", exec.ErrNotFound },
		Run:      runner.run,
	})

	exists, err := manager.HasSession(context.Background(), "virga_feature")
	if exists {
		t.Fatal("HasSession() = true, want false")
	}
	if !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("HasSession() error = %v, want %v", err, ErrNotInstalled)
	}
	if len(runner.commands) != 0 {
		t.Fatalf("commands = %#v, want no commands", runner.commands)
	}
}

func TestHasSessionWrapsCommandFailure(t *testing.T) {
	runnerErr := errors.New("tmux failed")
	runner := &recordingRunner{failAt: 1, err: runnerErr}
	manager := NewManager(ManagerDependencies{
		LookPath: func(string) (string, error) { return "/usr/bin/tmux", nil },
		Run:      runner.run,
	})

	exists, err := manager.HasSession(context.Background(), "virga_feature")
	if exists {
		t.Fatal("HasSession() = true, want false")
	}
	if !errors.Is(err, runnerErr) {
		t.Fatalf("HasSession() error = %v, want wrapped %v", err, runnerErr)
	}
	if !strings.Contains(err.Error(), "check tmux session \"virga_feature\"") {
		t.Fatalf("HasSession() error = %v, want session context", err)
	}
}

func TestHasSessionValidatesName(t *testing.T) {
	manager := NewManager(ManagerDependencies{
		LookPath: func(string) (string, error) {
			t.Fatal("LookPath called after invalid session name")
			return "", nil
		},
	})

	if exists, err := manager.HasSession(context.Background(), " "); err == nil || exists || !strings.Contains(err.Error(), "session name is required") {
		t.Fatalf("HasSession() = %v, error = %v; want validation error", exists, err)
	}
}

func TestEnsureSessionReusesExistingSession(t *testing.T) {
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
		Branch:         "feature",
		WorktreeRoot:   "/repositories/virga_feature",
	}

	result, err := manager.EnsureSession(context.Background(), options)
	if err != nil {
		t.Fatalf("EnsureSession() error = %v", err)
	}

	wantResult := EnsureSessionResult{Name: SessionName(options.WorktreeRoot), Action: SessionReused}
	if result != wantResult {
		t.Fatalf("EnsureSession() = %#v, want %#v", result, wantResult)
	}
	wantCommands := []Command{{
		Path: "/usr/bin/tmux",
		Args: []string{"has-session", "-t", wantResult.Name},
	}}
	if !reflect.DeepEqual(runner.commands, wantCommands) {
		t.Fatalf("commands = %#v, want %#v", runner.commands, wantCommands)
	}
}

func TestEnsureSessionCreatesMissingSession(t *testing.T) {
	runner := &recordingRunner{failAt: 1, err: exitCodeError{code: 1}}
	manager := NewManager(ManagerDependencies{
		LookPath: func(string) (string, error) { return "/usr/bin/tmux", nil },
		Run:      runner.run,
	})
	options := CreateSessionOptions{
		RepositoryRoot: "/repositories/virga",
		Branch:         "feature",
		WorktreeRoot:   "/repositories/virga_feature",
	}

	result, err := manager.EnsureSession(context.Background(), options)
	if err != nil {
		t.Fatalf("EnsureSession() error = %v", err)
	}

	sessionName := SessionName(options.WorktreeRoot)
	wantResult := EnsureSessionResult{Name: sessionName, Action: SessionCreated}
	if result != wantResult {
		t.Fatalf("EnsureSession() = %#v, want %#v", result, wantResult)
	}
	wantCommands := []Command{
		{Path: "/usr/bin/tmux", Args: []string{"has-session", "-t", sessionName}},
		{Path: "/usr/bin/tmux", Args: []string{"new-session", "-d", "-s", sessionName, "-c", options.WorktreeRoot}, WorkingDir: options.WorktreeRoot},
	}
	if !reflect.DeepEqual(runner.commands, wantCommands) {
		t.Fatalf("commands = %#v, want %#v", runner.commands, wantCommands)
	}
}

func TestEnsureSessionReportsMissingTmux(t *testing.T) {
	runner := &recordingRunner{}
	manager := NewManager(ManagerDependencies{
		LookPath: func(string) (string, error) { return "", exec.ErrNotFound },
		Run:      runner.run,
	})

	_, err := manager.EnsureSession(context.Background(), CreateSessionOptions{
		RepositoryRoot: "/repositories/virga",
		Branch:         "feature",
		WorktreeRoot:   "/repositories/virga_feature",
	})
	if !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("EnsureSession() error = %v, want %v", err, ErrNotInstalled)
	}
	if !strings.Contains(err.Error(), "ensure tmux session") {
		t.Fatalf("EnsureSession() error = %v, want ensure context", err)
	}
	if len(runner.commands) != 0 {
		t.Fatalf("commands = %#v, want no commands", runner.commands)
	}
}

func TestEnsureSessionWrapsCheckFailure(t *testing.T) {
	runnerErr := errors.New("tmux failed")
	runner := &recordingRunner{failAt: 1, err: runnerErr}
	manager := NewManager(ManagerDependencies{
		LookPath: func(string) (string, error) { return "/usr/bin/tmux", nil },
		Run:      runner.run,
	})

	_, err := manager.EnsureSession(context.Background(), CreateSessionOptions{
		RepositoryRoot: "/repositories/virga",
		Branch:         "feature",
		WorktreeRoot:   "/repositories/virga_feature",
	})
	if !errors.Is(err, runnerErr) {
		t.Fatalf("EnsureSession() error = %v, want wrapped %v", err, runnerErr)
	}
	if !strings.Contains(err.Error(), "ensure tmux session") || !strings.Contains(err.Error(), "check tmux session") {
		t.Fatalf("EnsureSession() error = %v, want ensure and check context", err)
	}
}

func TestEnsureSessionWrapsCreateFailure(t *testing.T) {
	createErr := errors.New("create failed")
	var commands []Command
	calls := 0
	manager := NewManager(ManagerDependencies{
		LookPath: func(string) (string, error) { return "/usr/bin/tmux", nil },
		Run: func(_ context.Context, command Command) error {
			commands = append(commands, Command{
				Path:       command.Path,
				Args:       append([]string(nil), command.Args...),
				WorkingDir: command.WorkingDir,
			})
			calls++
			if calls == 1 {
				return exitCodeError{code: 1}
			}
			if calls == 2 {
				return createErr
			}
			return nil
		},
	})
	options := CreateSessionOptions{
		RepositoryRoot: "/repositories/virga",
		Branch:         "feature",
		WorktreeRoot:   "/repositories/virga_feature",
	}

	_, err := manager.EnsureSession(context.Background(), options)
	if !errors.Is(err, createErr) {
		t.Fatalf("EnsureSession() error = %v, want wrapped %v", err, createErr)
	}
	if !strings.Contains(err.Error(), "ensure tmux session") || !strings.Contains(err.Error(), "create tmux session") {
		t.Fatalf("EnsureSession() error = %v, want ensure and create context", err)
	}
	if len(commands) != 2 {
		t.Fatalf("commands = %#v, want check and create commands", commands)
	}
}

func TestEnsureSessionValidatesOptions(t *testing.T) {
	manager := NewManager(ManagerDependencies{
		LookPath: func(string) (string, error) {
			t.Fatal("LookPath called after invalid options")
			return "", nil
		},
	})

	if result, err := manager.EnsureSession(context.Background(), CreateSessionOptions{WorktreeRoot: " "}); err == nil || result != (EnsureSessionResult{}) || !strings.Contains(err.Error(), "worktree root is required") {
		t.Fatalf("EnsureSession() = %#v, error = %v; want validation error", result, err)
	}
}

func TestAttachSessionAttachesToExistingSession(t *testing.T) {
	runner := &recordingRunner{}
	manager := NewManager(ManagerDependencies{
		LookPath: func(name string) (string, error) {
			if name != "tmux" {
				t.Errorf("LookPath(%q), want tmux", name)
			}
			return "/usr/bin/tmux", nil
		},
		LookupEnv:      func(string) (string, bool) { return "", false },
		RunInteractive: runner.run,
	})

	if err := manager.AttachSession(context.Background(), "virga_feature"); err != nil {
		t.Fatalf("AttachSession() error = %v", err)
	}

	want := []Command{{
		Path: "/usr/bin/tmux",
		Args: []string{"attach-session", "-t", "virga_feature"},
	}}
	if !reflect.DeepEqual(runner.commands, want) {
		t.Fatalf("commands = %#v, want %#v", runner.commands, want)
	}
}

func TestAttachSessionSwitchesClientInsideTmux(t *testing.T) {
	runner := &recordingRunner{}
	manager := NewManager(ManagerDependencies{
		LookPath:       func(string) (string, error) { return "/usr/bin/tmux", nil },
		LookupEnv:      func(string) (string, bool) { return "/tmp/tmux-501/default,123,0", true },
		RunInteractive: runner.run,
	})

	if err := manager.AttachSession(context.Background(), "virga_feature"); err != nil {
		t.Fatalf("AttachSession() error = %v", err)
	}

	want := []Command{{
		Path: "/usr/bin/tmux",
		Args: []string{"switch-client", "-t", "virga_feature"},
	}}
	if !reflect.DeepEqual(runner.commands, want) {
		t.Fatalf("commands = %#v, want %#v", runner.commands, want)
	}
}

func TestAttachSessionReportsMissingTmux(t *testing.T) {
	runner := &recordingRunner{}
	manager := NewManager(ManagerDependencies{
		LookPath:       func(string) (string, error) { return "", exec.ErrNotFound },
		RunInteractive: runner.run,
	})

	err := manager.AttachSession(context.Background(), "virga_feature")
	if !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("AttachSession() error = %v, want %v", err, ErrNotInstalled)
	}
	if len(runner.commands) != 0 {
		t.Fatalf("commands = %#v, want no commands", runner.commands)
	}
}

func TestAttachSessionValidatesName(t *testing.T) {
	manager := NewManager(ManagerDependencies{
		LookPath: func(string) (string, error) {
			t.Fatal("LookPath called after invalid session name")
			return "", nil
		},
	})

	if err := manager.AttachSession(context.Background(), " "); err == nil || !strings.Contains(err.Error(), "session name is required") {
		t.Fatalf("AttachSession() error = %v, want session name validation", err)
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

func TestSessionNameUsesWorktreeDirectoryName(t *testing.T) {
	first := SessionName("/repositories/virga_fix-tmux-attach")
	second := SessionName("/repositories/virga_fix-tmux-attach")
	if first != second {
		t.Fatalf("SessionName() = %q then %q, want deterministic", first, second)
	}
	if first != "virga_fix-tmux-attach" {
		t.Fatalf("SessionName() = %q, want worktree directory name", first)
	}
}

func TestSessionNameSanitizesTmuxTargetUnsafeCharacters(t *testing.T) {
	name := SessionName("/repositories/main repository:feature api")
	if name != "main-repository-feature-api" {
		t.Fatalf("SessionName() = %q, want sanitized worktree directory name", name)
	}
	if strings.ContainsAny(name, ":/ ") {
		t.Fatalf("SessionName() = %q, want no tmux target separators or whitespace", name)
	}
}

func TestSessionNameFallsBackWhenWorktreeNameHasNoSafeCharacters(t *testing.T) {
	if got, want := SessionName("/repositories/!!!"), "virga"; got != want {
		t.Fatalf("SessionName() = %q, want %q", got, want)
	}
}

type exitCodeError struct {
	code int
}

func (err exitCodeError) Error() string {
	return "exit status"
}

func (err exitCodeError) ExitCode() int {
	return err.code
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

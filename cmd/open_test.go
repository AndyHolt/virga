package cmd

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/AndyHolt/virga/internal/config"
	"github.com/AndyHolt/virga/internal/files"
	"github.com/AndyHolt/virga/internal/git"
	"github.com/AndyHolt/virga/internal/tmux"
)

func TestOpenCommandReusesExistingTmuxSession(t *testing.T) {
	var output bytes.Buffer
	configuration := config.Config{Tmux: config.TmuxConfig{Windows: []config.TmuxWindow{{
		Name:  "editor",
		Panes: []config.TmuxPane{{Command: "nvim"}},
	}}}}
	command := newOpenCmd(
		func() (string, error) { return "/repo/nested", nil },
		openWorktreeOptions{
			listBranches: func(_ context.Context, directory string) ([]string, error) {
				if directory != "/repo/nested" {
					t.Errorf("list branch directory = %q, want /repo/nested", directory)
				}
				return []string{"feature/login", "main"}, nil
			},
			listWorktrees: func(_ context.Context, directory string) ([]git.ListedWorktree, error) {
				if directory != "/repo/nested" {
					t.Errorf("list worktree directory = %q, want /repo/nested", directory)
				}
				return []git.ListedWorktree{{
					Path:      "/repo_feature-login",
					Kind:      git.LinkedWorktree,
					Branch:    "feature/login",
					BranchRef: "refs/heads/feature/login",
				}}, nil
			},
			inspect: func(_ context.Context, directory string) (git.WorktreeInfo, error) {
				if directory != "/repo/nested" {
					t.Errorf("inspect directory = %q, want /repo/nested", directory)
				}
				return git.WorktreeInfo{Kind: git.MainWorktree, WorktreeRoot: "/repo", MainWorktreeRoot: "/repo"}, nil
			},
			loadConfiguration: func(_ context.Context, directory, explicitPath string) (config.Config, error) {
				if directory != "/repo/nested" || explicitPath != "local.yaml" {
					t.Errorf("loadConfiguration(%q, %q), want /repo/nested and local.yaml", directory, explicitPath)
				}
				return configuration, nil
			},
			ensureSession: func(_ context.Context, options tmux.CreateSessionOptions) (tmux.EnsureSessionResult, error) {
				if options.RepositoryRoot != "/repo" || options.Branch != "feature/login" || options.WorktreeRoot != "/repo_feature-login" {
					t.Errorf("tmux options = %#v, want repository, branch, and worktree", options)
				}
				if !reflect.DeepEqual(options.Tmux, configuration.Tmux) {
					t.Errorf("tmux config = %#v, want %#v", options.Tmux, configuration.Tmux)
				}
				return tmux.EnsureSessionResult{Name: "repo_feature-login", Action: tmux.SessionReused}, nil
			},
			materializeFiles: func(context.Context, files.Options) error {
				t.Fatal("materializeFiles called for reused worktree")
				return nil
			},
			isInteractive: func() bool { return false },
			attachSession: func(context.Context, string) error {
				t.Fatal("attach called in non-interactive mode")
				return nil
			},
		},
	)
	command.SetOut(&output)
	command.SetArgs([]string{"feature/login", "--config", "local.yaml"})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := "Branch: feature/login\n" +
		"Worktree: /repo_feature-login\n" +
		"Worktree action: reused\n" +
		"Tmux session: repo_feature-login\n" +
		"Tmux action: reused\n"
	if got := output.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestOpenCommandCreatesMissingTmuxSession(t *testing.T) {
	var output bytes.Buffer
	command := newOpenCmd(
		func() (string, error) { return "/repo", nil },
		openWorktreeOptions{
			listBranches:  func(context.Context, string) ([]string, error) { return []string{"feature"}, nil },
			listWorktrees: openTestWorktrees,
			inspect: func(context.Context, string) (git.WorktreeInfo, error) {
				return git.WorktreeInfo{Kind: git.MainWorktree, WorktreeRoot: "/repo", MainWorktreeRoot: "/repo"}, nil
			},
			loadConfiguration: func(context.Context, string, string) (config.Config, error) { return config.Config{}, nil },
			ensureSession: func(context.Context, tmux.CreateSessionOptions) (tmux.EnsureSessionResult, error) {
				return tmux.EnsureSessionResult{Name: "repo_feature", Action: tmux.SessionCreated}, nil
			},
			isInteractive: func() bool { return false },
		},
	)
	command.SetOut(&output)
	command.SetArgs([]string{"feature"})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := output.String(); !strings.Contains(got, "Tmux action: created\n") {
		t.Fatalf("output = %q, want created tmux action", got)
	}
}

func TestOpenCommandNoTmuxSkipsTmuxSetup(t *testing.T) {
	var output bytes.Buffer
	command := newOpenCmd(
		func() (string, error) { return "/repo", nil },
		openWorktreeOptions{
			listBranches:  func(context.Context, string) ([]string, error) { return []string{"feature"}, nil },
			listWorktrees: openTestWorktrees,
			inspect: func(context.Context, string) (git.WorktreeInfo, error) {
				t.Fatal("inspect called with --no-tmux")
				return git.WorktreeInfo{}, nil
			},
			loadConfiguration: func(context.Context, string, string) (config.Config, error) {
				t.Fatal("loadConfiguration called with --no-tmux")
				return config.Config{}, nil
			},
			ensureSession: func(context.Context, tmux.CreateSessionOptions) (tmux.EnsureSessionResult, error) {
				t.Fatal("ensureSession called with --no-tmux")
				return tmux.EnsureSessionResult{}, nil
			},
			materializeFiles: func(context.Context, files.Options) error {
				t.Fatal("materializeFiles called for reused worktree")
				return nil
			},
			attachSession: func(context.Context, string) error {
				t.Fatal("attach called with --no-tmux")
				return nil
			},
		},
	)
	command.SetOut(&output)
	command.SetArgs([]string{"feature", "--no-tmux"})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := "Branch: feature\nWorktree: /repo_feature\nWorktree action: reused\n"
	if got := output.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestOpenCommandNoAttachLeavesSessionDetached(t *testing.T) {
	command := newOpenCmd(
		func() (string, error) { return "/repo", nil },
		openWorktreeOptions{
			listBranches:  func(context.Context, string) ([]string, error) { return []string{"feature"}, nil },
			listWorktrees: openTestWorktrees,
			inspect: func(context.Context, string) (git.WorktreeInfo, error) {
				return git.WorktreeInfo{Kind: git.MainWorktree, MainWorktreeRoot: "/repo"}, nil
			},
			loadConfiguration: func(context.Context, string, string) (config.Config, error) { return config.Config{}, nil },
			ensureSession: func(context.Context, tmux.CreateSessionOptions) (tmux.EnsureSessionResult, error) {
				return tmux.EnsureSessionResult{Name: "repo_feature", Action: tmux.SessionReused}, nil
			},
			isInteractive: func() bool { return true },
			attachSession: func(context.Context, string) error {
				t.Fatal("attach called with --no-attach")
				return nil
			},
		},
	)
	command.SetArgs([]string{"feature", "--no-attach"})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestOpenCommandAttachesInteractiveSession(t *testing.T) {
	var attached string
	command := newOpenCmd(
		func() (string, error) { return "/repo", nil },
		openWorktreeOptions{
			listBranches:  func(context.Context, string) ([]string, error) { return []string{"feature"}, nil },
			listWorktrees: openTestWorktrees,
			inspect: func(context.Context, string) (git.WorktreeInfo, error) {
				return git.WorktreeInfo{Kind: git.MainWorktree, MainWorktreeRoot: "/repo"}, nil
			},
			loadConfiguration: func(context.Context, string, string) (config.Config, error) { return config.Config{}, nil },
			ensureSession: func(context.Context, tmux.CreateSessionOptions) (tmux.EnsureSessionResult, error) {
				return tmux.EnsureSessionResult{Name: "repo_feature", Action: tmux.SessionReused}, nil
			},
			isInteractive: func() bool { return true },
			attachSession: func(_ context.Context, session string) error {
				attached = session
				return nil
			},
		},
	)
	command.SetArgs([]string{"feature"})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if attached != "repo_feature" {
		t.Fatalf("attached session = %q, want repo_feature", attached)
	}
}

func TestOpenCommandDoesNotAttachWhenNonInteractive(t *testing.T) {
	command := newOpenCmd(
		func() (string, error) { return "/repo", nil },
		openWorktreeOptions{
			listBranches:  func(context.Context, string) ([]string, error) { return []string{"feature"}, nil },
			listWorktrees: openTestWorktrees,
			inspect: func(context.Context, string) (git.WorktreeInfo, error) {
				return git.WorktreeInfo{Kind: git.MainWorktree, MainWorktreeRoot: "/repo"}, nil
			},
			loadConfiguration: func(context.Context, string, string) (config.Config, error) { return config.Config{}, nil },
			ensureSession: func(context.Context, tmux.CreateSessionOptions) (tmux.EnsureSessionResult, error) {
				return tmux.EnsureSessionResult{Name: "repo_feature", Action: tmux.SessionReused}, nil
			},
			isInteractive: func() bool { return false },
			attachSession: func(context.Context, string) error {
				t.Fatal("attach called in non-interactive mode")
				return nil
			},
		},
	)
	command.SetArgs([]string{"feature"})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestOpenCommandCreatesMissingWorktree(t *testing.T) {
	var output bytes.Buffer
	var calls []string
	configuration := config.Config{
		Files: []files.Entry{{Source: ".env", Mode: files.ModeCopy}},
		Tmux:  config.TmuxConfig{Windows: []config.TmuxWindow{{Name: "shell"}}},
	}
	command := newOpenCmd(
		func() (string, error) { return "/repo/nested", nil },
		openWorktreeOptions{
			listBranches: func(_ context.Context, directory string) ([]string, error) {
				if directory != "/repo/nested" {
					t.Errorf("list branch directory = %q, want /repo/nested", directory)
				}
				return []string{"feature", "main"}, nil
			},
			listWorktrees: func(_ context.Context, directory string) ([]git.ListedWorktree, error) {
				if directory != "/repo/nested" {
					t.Errorf("list worktree directory = %q, want /repo/nested", directory)
				}
				return []git.ListedWorktree{{Path: "/repo", Kind: git.MainWorktree, Branch: "main", BranchRef: "refs/heads/main"}}, nil
			},
			loadConfiguration: func(_ context.Context, directory, explicitPath string) (config.Config, error) {
				calls = append(calls, "load")
				if directory != "/repo/nested" || explicitPath != "local.yaml" {
					t.Errorf("loadConfiguration(%q, %q), want /repo/nested and local.yaml", directory, explicitPath)
				}
				return configuration, nil
			},
			inspect: func(_ context.Context, directory string) (git.WorktreeInfo, error) {
				calls = append(calls, "inspect")
				if directory != "/repo/nested" {
					t.Errorf("inspect directory = %q, want /repo/nested", directory)
				}
				return git.WorktreeInfo{Kind: git.LinkedWorktree, WorktreeRoot: "/repo_source", MainWorktreeRoot: "/repo"}, nil
			},
			addWorktree: func(_ context.Context, directory, branch string) (string, error) {
				calls = append(calls, "add")
				if directory != "/repo/nested" || branch != "feature" {
					t.Errorf("addWorktree(%q, %q), want /repo/nested and feature", directory, branch)
				}
				return "/repo_feature", nil
			},
			materializeFiles: func(_ context.Context, options files.Options) error {
				calls = append(calls, "materialize")
				want := files.Options{RepositoryRoot: "/repo", WorktreeRoot: "/repo_feature", Entries: configuration.Files}
				if !reflect.DeepEqual(options, want) {
					t.Errorf("materialize options = %#v, want %#v", options, want)
				}
				return nil
			},
			ensureSession: func(_ context.Context, options tmux.CreateSessionOptions) (tmux.EnsureSessionResult, error) {
				calls = append(calls, "ensure")
				if options.RepositoryRoot != "/repo" || options.Branch != "feature" || options.WorktreeRoot != "/repo_feature" {
					t.Errorf("tmux options = %#v, want repository, branch, and worktree", options)
				}
				if !reflect.DeepEqual(options.Tmux, configuration.Tmux) {
					t.Errorf("tmux config = %#v, want %#v", options.Tmux, configuration.Tmux)
				}
				return tmux.EnsureSessionResult{Name: "repo_feature", Action: tmux.SessionCreated}, nil
			},
			isInteractive: func() bool { return false },
		},
	)
	command.SetOut(&output)
	command.SetArgs([]string{"feature", "--config", "local.yaml"})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	wantOutput := "Branch: feature\n" +
		"Worktree: /repo_feature\n" +
		"Worktree action: created\n" +
		"Tmux session: repo_feature\n" +
		"Tmux action: created\n"
	if got := output.String(); got != wantOutput {
		t.Fatalf("output = %q, want %q", got, wantOutput)
	}
	if wantCalls := []string{"load", "inspect", "add", "materialize", "ensure"}; !reflect.DeepEqual(calls, wantCalls) {
		t.Fatalf("calls = %q, want %q", calls, wantCalls)
	}
}

func TestOpenCommandMaterializesFilesWithoutTmuxForCreatedWorktree(t *testing.T) {
	var output bytes.Buffer
	materialized := false
	command := newOpenCmd(
		func() (string, error) { return "/repo", nil },
		openWorktreeOptions{
			listBranches:  func(context.Context, string) ([]string, error) { return []string{"feature"}, nil },
			listWorktrees: func(context.Context, string) ([]git.ListedWorktree, error) { return nil, nil },
			loadConfiguration: func(context.Context, string, string) (config.Config, error) {
				return config.Config{Files: []files.Entry{{Source: ".env", Mode: files.ModeCopy}}}, nil
			},
			inspect: func(context.Context, string) (git.WorktreeInfo, error) {
				return git.WorktreeInfo{Kind: git.MainWorktree, MainWorktreeRoot: "/repo"}, nil
			},
			addWorktree: func(context.Context, string, string) (string, error) { return "/repo_feature", nil },
			materializeFiles: func(context.Context, files.Options) error {
				materialized = true
				return nil
			},
			ensureSession: func(context.Context, tmux.CreateSessionOptions) (tmux.EnsureSessionResult, error) {
				t.Fatal("ensureSession called with --no-tmux")
				return tmux.EnsureSessionResult{}, nil
			},
		},
	)
	command.SetOut(&output)
	command.SetArgs([]string{"feature", "--no-tmux"})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !materialized {
		t.Fatal("materializeFiles was not called")
	}
	want := "Branch: feature\nWorktree: /repo_feature\nWorktree action: created\n"
	if got := output.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestOpenCommandReportsCreatedWorktreeMaterializationFailure(t *testing.T) {
	materializeErr := errors.New("copy failed")
	ensureCalled := false
	command := newOpenCmd(
		func() (string, error) { return "/repo", nil },
		openWorktreeOptions{
			listBranches:  func(context.Context, string) ([]string, error) { return []string{"feature"}, nil },
			listWorktrees: func(context.Context, string) ([]git.ListedWorktree, error) { return nil, nil },
			loadConfiguration: func(context.Context, string, string) (config.Config, error) {
				return config.Config{Files: []files.Entry{{Source: ".env", Mode: files.ModeCopy}}}, nil
			},
			inspect: func(context.Context, string) (git.WorktreeInfo, error) {
				return git.WorktreeInfo{Kind: git.MainWorktree, MainWorktreeRoot: "/repo"}, nil
			},
			addWorktree:      func(context.Context, string, string) (string, error) { return "/repo_feature", nil },
			materializeFiles: func(context.Context, files.Options) error { return materializeErr },
			ensureSession: func(context.Context, tmux.CreateSessionOptions) (tmux.EnsureSessionResult, error) {
				ensureCalled = true
				return tmux.EnsureSessionResult{}, nil
			},
		},
	)
	command.SetArgs([]string{"feature"})

	err := command.Execute()
	if !errors.Is(err, materializeErr) {
		t.Fatalf("Execute() error = %v, want wrapped materialize error", err)
	}
	if !strings.Contains(err.Error(), "created worktree for branch \"feature\" at \"/repo_feature\", but materialize files") {
		t.Fatalf("Execute() error = %v, want created-worktree context", err)
	}
	if ensureCalled {
		t.Fatal("ensureSession called after materialization failure")
	}
}

func TestOpenCommandReportsCreatedWorktreeTmuxFailure(t *testing.T) {
	ensureErr := errors.New("tmux failed")
	command := newOpenCmd(
		func() (string, error) { return "/repo", nil },
		openWorktreeOptions{
			listBranches:      func(context.Context, string) ([]string, error) { return []string{"feature"}, nil },
			listWorktrees:     func(context.Context, string) ([]git.ListedWorktree, error) { return nil, nil },
			loadConfiguration: func(context.Context, string, string) (config.Config, error) { return config.Config{}, nil },
			inspect: func(context.Context, string) (git.WorktreeInfo, error) {
				return git.WorktreeInfo{Kind: git.MainWorktree, MainWorktreeRoot: "/repo"}, nil
			},
			addWorktree: func(context.Context, string, string) (string, error) { return "/repo_feature", nil },
			ensureSession: func(context.Context, tmux.CreateSessionOptions) (tmux.EnsureSessionResult, error) {
				return tmux.EnsureSessionResult{}, ensureErr
			},
		},
	)
	command.SetArgs([]string{"feature"})

	err := command.Execute()
	if !errors.Is(err, ensureErr) {
		t.Fatalf("Execute() error = %v, want wrapped tmux error", err)
	}
	if !strings.Contains(err.Error(), "created worktree for branch \"feature\" at \"/repo_feature\", but ensure tmux session") {
		t.Fatalf("Execute() error = %v, want created-worktree context", err)
	}
}

func TestOpenCommandErrors(t *testing.T) {
	getwdErr := errors.New("getwd failed")
	command := newOpenCmd(
		func() (string, error) { return "", getwdErr },
		openWorktreeOptions{
			listBranches: func(context.Context, string) ([]string, error) {
				t.Fatal("listBranches called after getwd failure")
				return nil, nil
			},
		},
	)
	command.SetArgs([]string{"feature"})
	if err := command.Execute(); !errors.Is(err, getwdErr) {
		t.Fatalf("Execute() error = %v, want wrapped getwd error", err)
	}

	listErr := errors.New("list branches failed")
	command = newOpenCmd(
		func() (string, error) { return "/repo", nil },
		openWorktreeOptions{
			listBranches: func(context.Context, string) ([]string, error) { return nil, listErr },
		},
	)
	command.SetArgs([]string{"feature"})
	if err := command.Execute(); !errors.Is(err, listErr) {
		t.Fatalf("Execute() error = %v, want wrapped list branches error", err)
	}

	command = newOpenCmd(
		func() (string, error) { return "/repo", nil },
		openWorktreeOptions{
			listBranches: func(context.Context, string) ([]string, error) { return []string{"main"}, nil },
			listWorktrees: func(context.Context, string) ([]git.ListedWorktree, error) {
				t.Fatal("listWorktrees called after missing branch")
				return nil, nil
			},
		},
	)
	command.SetArgs([]string{"feature"})
	if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "local branch \"feature\" does not exist") {
		t.Fatalf("Execute() error = %v, want missing branch error", err)
	}

	worktreeErr := errors.New("list worktrees failed")
	command = newOpenCmd(
		func() (string, error) { return "/repo", nil },
		openWorktreeOptions{
			listBranches:  func(context.Context, string) ([]string, error) { return []string{"feature"}, nil },
			listWorktrees: func(context.Context, string) ([]git.ListedWorktree, error) { return nil, worktreeErr },
		},
	)
	command.SetArgs([]string{"feature"})
	if err := command.Execute(); !errors.Is(err, worktreeErr) {
		t.Fatalf("Execute() error = %v, want wrapped list worktrees error", err)
	}
}

func TestOpenCommandReportsUnavailableWorktreeCreation(t *testing.T) {
	tests := []struct {
		name      string
		worktrees []git.ListedWorktree
	}{
		{
			name: "not checked out",
			worktrees: []git.ListedWorktree{{
				Path:      "/repo",
				Kind:      git.MainWorktree,
				Branch:    "main",
				BranchRef: "refs/heads/main",
			}},
		},
		{
			name: "detached worktree",
			worktrees: []git.ListedWorktree{{
				Path:     "/repo_detached",
				Kind:     git.LinkedWorktree,
				Detached: true,
			}},
		},
		{
			name: "prunable worktree",
			worktrees: []git.ListedWorktree{{
				Path:      "/repo_feature",
				Kind:      git.LinkedWorktree,
				Branch:    "feature",
				BranchRef: "refs/heads/feature",
				Prunable:  true,
			}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command := newOpenCmd(
				func() (string, error) { return "/repo", nil },
				openWorktreeOptions{
					listBranches:  func(context.Context, string) ([]string, error) { return []string{"feature"}, nil },
					listWorktrees: func(context.Context, string) ([]git.ListedWorktree, error) { return test.worktrees, nil },
				},
			)
			command.SetArgs([]string{"feature"})

			err := command.Execute()
			if err == nil || !strings.Contains(err.Error(), "local branch \"feature\" exists but has no usable worktree") {
				t.Fatalf("Execute() error = %v, want no usable worktree error", err)
			}
			if !strings.Contains(err.Error(), "worktree creation is unavailable") {
				t.Fatalf("Execute() error = %v, want unavailable creation guidance", err)
			}
		})
	}
}

func TestOpenCommandReportsNonGitRepository(t *testing.T) {
	command := newOpenCmd(
		func() (string, error) { return "/tmp", nil },
		openWorktreeOptions{
			listBranches: func(context.Context, string) ([]string, error) { return nil, git.ErrNotGitRepository },
		},
	)
	command.SetArgs([]string{"feature"})

	err := command.Execute()
	if !errors.Is(err, git.ErrNotGitRepository) {
		t.Fatalf("Execute() error = %v, want %v", err, git.ErrNotGitRepository)
	}
	if !command.SilenceUsage {
		t.Fatal("SilenceUsage = false, want true for not-a-repository error")
	}
}

func TestOpenCommandReportsTmuxEnsureFailure(t *testing.T) {
	ensureErr := tmux.ErrNotInstalled
	command := newOpenCmd(
		func() (string, error) { return "/repo", nil },
		openWorktreeOptions{
			listBranches:  func(context.Context, string) ([]string, error) { return []string{"feature"}, nil },
			listWorktrees: openTestWorktrees,
			inspect: func(context.Context, string) (git.WorktreeInfo, error) {
				return git.WorktreeInfo{Kind: git.MainWorktree, MainWorktreeRoot: "/repo"}, nil
			},
			loadConfiguration: func(context.Context, string, string) (config.Config, error) { return config.Config{}, nil },
			ensureSession: func(context.Context, tmux.CreateSessionOptions) (tmux.EnsureSessionResult, error) {
				return tmux.EnsureSessionResult{}, ensureErr
			},
		},
	)
	command.SetArgs([]string{"feature"})

	err := command.Execute()
	if !errors.Is(err, ensureErr) {
		t.Fatalf("Execute() error = %v, want wrapped %v", err, ensureErr)
	}
	if !strings.Contains(err.Error(), "open branch \"feature\" in worktree \"/repo_feature\", but ensure tmux session") {
		t.Fatalf("Execute() error = %v, want open context", err)
	}
}

func TestOpenCommandRejectsInvalidArgumentCounts(t *testing.T) {
	for _, args := range [][]string{nil, {"feature", "unexpected"}} {
		command := newOpenCmd(
			func() (string, error) { return "/repo", nil },
			openWorktreeOptions{
				listBranches: func(context.Context, string) ([]string, error) {
					t.Fatal("listBranches called with invalid arguments")
					return nil, nil
				},
			},
		)
		command.SetArgs(args)

		if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "accepts 1 arg(s), received") {
			t.Errorf("Execute(%q) error = %v, want argument error", args, err)
		}
	}
}

func TestOpenCommandCreatesGitWorktreeForExistingBranch(t *testing.T) {
	root := newCLITestRepository(t)
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("TOKEN=value\n"), 0o600); err != nil {
		t.Fatalf("write source file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".virga.yaml"), []byte(`files:
  - source: .env
    mode: copy
tmux:
  windows:
    - name: shell
      panes:
        - command: make test
`), 0o644); err != nil {
		t.Fatalf("write repository config: %v", err)
	}
	branchCommit := cliGitOutput(t, "-C", root, "rev-parse", "HEAD")
	cliRunGit(t, "-C", root, "commit", "--allow-empty", "-m", "main commit")
	cliRunGit(t, "-C", root, "branch", "feature", branchCommit)

	configurationLoader := config.NewLoader(config.LoaderDependencies{})
	var output bytes.Buffer
	var sessionOptions tmux.CreateSessionOptions
	command := newRootCommand(
		func() (string, error) { return root, nil },
		git.InspectWorktree,
		git.CreateWorktree,
		listWorktreeOptions{},
		openWorktreeOptions{
			inspect:           git.InspectWorktree,
			listBranches:      git.ListLocalBranches,
			listWorktrees:     git.ListWorktrees,
			addWorktree:       git.AddExistingBranchWorktree,
			loadConfiguration: configurationLoader.Load,
			materializeFiles:  files.Materialize,
			ensureSession: func(_ context.Context, options tmux.CreateSessionOptions) (tmux.EnsureSessionResult, error) {
				sessionOptions = options
				assertFileContents(t, filepath.Join(options.WorktreeRoot, ".env"), "TOKEN=value\n")
				return tmux.EnsureSessionResult{Name: "repository_feature", Action: tmux.SessionCreated}, nil
			},
			isInteractive: func() bool { return true },
			attachSession: func(context.Context, string) error {
				t.Fatal("attach called with --no-attach")
				return nil
			},
		},
		newWorktreeOptions{},
	)
	command.SetOut(&output)
	command.SetArgs([]string{"open", "feature", "--no-attach"})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	worktree := filepath.Join(filepath.Dir(root), "repository_feature")
	want := "Branch: feature\n" +
		"Worktree: " + worktree + "\n" +
		"Worktree action: created\n" +
		"Tmux session: repository_feature\n" +
		"Tmux action: created\n"
	if got := output.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if got := cliGitOutput(t, "-C", worktree, "branch", "--show-current"); got != "feature" {
		t.Errorf("checked-out branch = %q, want feature", got)
	}
	if got := cliGitOutput(t, "-C", worktree, "rev-parse", "HEAD"); got != branchCommit {
		t.Errorf("worktree commit = %q, want %q", got, branchCommit)
	}
	if sessionOptions.RepositoryRoot != root || sessionOptions.Branch != "feature" || sessionOptions.WorktreeRoot != worktree {
		t.Errorf("session options = %#v, want root, branch, and created worktree", sessionOptions)
	}
}

func TestOpenCommandRetainsCreatedGitWorktreeAfterFileMaterializationFailure(t *testing.T) {
	root := newCLITestRepository(t)
	if err := os.WriteFile(filepath.Join(root, "tracked.env"), []byte("tracked\n"), 0o600); err != nil {
		t.Fatalf("write tracked file: %v", err)
	}
	cliRunGit(t, "-C", root, "add", "tracked.env")
	cliRunGit(t, "-C", root, "commit", "-m", "add tracked file")
	cliRunGit(t, "-C", root, "branch", "feature")
	if err := os.WriteFile(filepath.Join(root, ".virga.yaml"), []byte(`files:
  - source: tracked.env
    mode: copy
`), 0o644); err != nil {
		t.Fatalf("write repository config: %v", err)
	}

	configurationLoader := config.NewLoader(config.LoaderDependencies{})
	command := newRootCommand(
		func() (string, error) { return root, nil },
		git.InspectWorktree,
		git.CreateWorktree,
		listWorktreeOptions{},
		openWorktreeOptions{
			inspect:           git.InspectWorktree,
			listBranches:      git.ListLocalBranches,
			listWorktrees:     git.ListWorktrees,
			addWorktree:       git.AddExistingBranchWorktree,
			loadConfiguration: configurationLoader.Load,
			materializeFiles:  files.Materialize,
			ensureSession: func(context.Context, tmux.CreateSessionOptions) (tmux.EnsureSessionResult, error) {
				t.Fatal("tmux session ensured after configured file collision")
				return tmux.EnsureSessionResult{}, nil
			},
		},
		newWorktreeOptions{},
	)
	command.SetArgs([]string{"open", "feature"})

	err := command.Execute()
	if err == nil {
		t.Fatal("Execute() error = nil, want configured file collision")
	}
	worktree := filepath.Join(filepath.Dir(root), "repository_feature")
	if !strings.Contains(err.Error(), "created worktree for branch \"feature\" at \""+worktree+"\"") {
		t.Fatalf("Execute() error = %v, want created worktree context", err)
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("Execute() error = %v, want destination collision", err)
	}
	if got := cliGitOutput(t, "-C", worktree, "branch", "--show-current"); got != "feature" {
		t.Errorf("created worktree branch = %q, want feature", got)
	}
	assertFileContents(t, filepath.Join(worktree, "tracked.env"), "tracked\n")
}

func TestOpenCommandRetainsCreatedGitWorktreeAfterTmuxFailure(t *testing.T) {
	root := newCLITestRepository(t)
	cliRunGit(t, "-C", root, "branch", "feature")
	ensureErr := errors.New("tmux failed")
	configurationLoader := config.NewLoader(config.LoaderDependencies{})
	command := newRootCommand(
		func() (string, error) { return root, nil },
		git.InspectWorktree,
		git.CreateWorktree,
		listWorktreeOptions{},
		openWorktreeOptions{
			inspect:           git.InspectWorktree,
			listBranches:      git.ListLocalBranches,
			listWorktrees:     git.ListWorktrees,
			addWorktree:       git.AddExistingBranchWorktree,
			loadConfiguration: configurationLoader.Load,
			materializeFiles:  files.Materialize,
			ensureSession: func(context.Context, tmux.CreateSessionOptions) (tmux.EnsureSessionResult, error) {
				return tmux.EnsureSessionResult{}, ensureErr
			},
		},
		newWorktreeOptions{},
	)
	command.SetArgs([]string{"open", "feature"})

	err := command.Execute()
	if !errors.Is(err, ensureErr) {
		t.Fatalf("Execute() error = %v, want wrapped tmux error", err)
	}
	worktree := filepath.Join(filepath.Dir(root), "repository_feature")
	if !strings.Contains(err.Error(), "created worktree for branch \"feature\" at \""+worktree+"\"") {
		t.Fatalf("Execute() error = %v, want created worktree context", err)
	}
	if got := cliGitOutput(t, "-C", worktree, "branch", "--show-current"); got != "feature" {
		t.Errorf("created worktree branch = %q, want feature", got)
	}
}

func TestOpenCommandReusesExistingGitWorktreeFromNestedDirectory(t *testing.T) {
	root := newCLITestRepository(t)
	cliRunGit(t, "-C", root, "branch", "feature")
	worktree := filepath.Join(filepath.Dir(root), "repository_feature")
	cliRunGit(t, "-C", root, "worktree", "add", worktree, "feature")
	nested := filepath.Join(root, "nested")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatalf("create nested directory: %v", err)
	}

	var output bytes.Buffer
	command := newRootCommand(
		func() (string, error) { return nested, nil },
		git.InspectWorktree,
		git.CreateWorktree,
		listWorktreeOptions{},
		openWorktreeOptions{
			listBranches:  git.ListLocalBranches,
			listWorktrees: git.ListWorktrees,
		},
		newWorktreeOptions{},
	)
	command.SetOut(&output)
	command.SetArgs([]string{"open", "feature", "--no-tmux"})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := "Branch: feature\nWorktree: " + worktree + "\nWorktree action: reused\n"
	if got := output.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestOpenCommandAtRoot(t *testing.T) {
	var output bytes.Buffer
	command := newRootCommand(
		func() (string, error) { return "/repo", nil },
		nil,
		func(context.Context, string, string, string) (string, error) {
			t.Fatal("create worktree called by open")
			return "", nil
		},
		listWorktreeOptions{},
		openWorktreeOptions{
			listBranches:  func(context.Context, string) ([]string, error) { return []string{"feature"}, nil },
			listWorktrees: openTestWorktrees,
		},
		newWorktreeOptions{},
	)
	command.SetOut(&output)
	command.SetArgs([]string{"open", "feature", "--no-tmux"})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	want := "Branch: feature\nWorktree: /repo_feature\nWorktree action: reused\n"
	if got := output.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func openTestWorktrees(context.Context, string) ([]git.ListedWorktree, error) {
	return []git.ListedWorktree{{
		Path:      "/repo_feature",
		Kind:      git.LinkedWorktree,
		Branch:    "feature",
		BranchRef: "refs/heads/feature",
	}}, nil
}

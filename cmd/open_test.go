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

func TestOpenCommandReportsBranchWithoutUsableWorktree(t *testing.T) {
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
			if !strings.Contains(err.Error(), "creating a worktree for an existing branch is not implemented yet") {
				t.Fatalf("Execute() error = %v, want next-slice guidance", err)
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

package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/AndyHolt/virga/internal/config"
	"github.com/AndyHolt/virga/internal/git"
	"github.com/AndyHolt/virga/internal/tmux"
)

func TestNewWorktreeCommandSelectsBaseBranch(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantBase string
	}{
		{
			name:     "current branch by default",
			args:     []string{"feature/login"},
			wantBase: "",
		},
		{
			name:     "named local branch",
			args:     []string{"feature/login", "--from", "release"},
			wantBase: "release",
		},
		{
			name:     "local main branch",
			args:     []string{"feature/login", "--main"},
			wantBase: "main",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			command := newWorktreeCmd(
				func() (string, error) { return "/repo/nested", nil },
				func(_ context.Context, directory, branch, base string) (string, error) {
					if directory != "/repo/nested" {
						t.Errorf("directory = %q, want /repo/nested", directory)
					}
					if branch != "feature/login" {
						t.Errorf("branch = %q, want feature/login", branch)
					}
					if base != test.wantBase {
						t.Errorf("base = %q, want %q", base, test.wantBase)
					}
					return "/worktrees/repo_feature-login", nil
				},
				newWorktreeOptions{},
			)
			command.SetOut(&output)
			command.SetArgs(test.args)

			if err := command.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if got, want := output.String(), "Branch: feature/login\nWorktree: /worktrees/repo_feature-login\n"; got != want {
				t.Errorf("output = %q, want %q", got, want)
			}
		})
	}
}

func TestNewWorktreeCommandPicksBaseBranch(t *testing.T) {
	var output, prompt bytes.Buffer
	command := newWorktreeCmd(
		func() (string, error) { return "/repo/nested", nil },
		func(_ context.Context, directory, branch, base string) (string, error) {
			if directory != "/repo/nested" || branch != "feature" || base != "release" {
				t.Errorf("create(%q, %q, %q), want create(/repo/nested, feature, release)", directory, branch, base)
			}
			return "/worktrees/repo_feature", nil
		},
		newWorktreeOptions{
			listBranches: func(_ context.Context, directory string) ([]string, error) {
				if directory != "/repo/nested" {
					t.Errorf("list directory = %q, want /repo/nested", directory)
				}
				return []string{"main", "release"}, nil
			},
			isInteractive: func() bool { return true },
			selectBranch: func(_ io.Reader, writer io.Writer, branches []string) (string, error) {
				if got, want := strings.Join(branches, ","), "main,release"; got != want {
					t.Errorf("branches = %q, want %q", got, want)
				}
				if _, err := fmt.Fprint(writer, "Select a base branch:\n"); err != nil {
					t.Fatalf("write prompt: %v", err)
				}
				return "release", nil
			},
		},
	)
	command.SetOut(&output)
	command.SetErr(&prompt)
	command.SetArgs([]string{"feature", "--pick"})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got, want := output.String(), "Branch: feature\nWorktree: /worktrees/repo_feature\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
	if got, want := prompt.String(), "Select a base branch:\n"; got != want {
		t.Errorf("prompt = %q, want %q", got, want)
	}
}

func TestNewWorktreeCommandCreatesTmuxSession(t *testing.T) {
	var output bytes.Buffer
	configuration := config.Config{Tmux: config.TmuxConfig{Windows: []config.TmuxWindow{{
		Name:  "editor",
		Panes: []config.TmuxPane{{Command: "nvim"}},
	}}}}
	command := newWorktreeCmd(
		func() (string, error) { return "/repo/nested", nil },
		func(_ context.Context, directory, branch, base string) (string, error) {
			if directory != "/repo/nested" || branch != "feature" || base != "release" {
				t.Errorf("create(%q, %q, %q), want create(/repo/nested, feature, release)", directory, branch, base)
			}
			return "/worktrees/repo_feature", nil
		},
		newWorktreeOptions{
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
			createSession: func(_ context.Context, options tmux.CreateSessionOptions) (string, error) {
				if options.RepositoryRoot != "/repo" || options.Branch != "feature" || options.WorktreeRoot != "/worktrees/repo_feature" {
					t.Errorf("tmux options = %#v, want repository, branch, and worktree", options)
				}
				if !reflect.DeepEqual(options.Tmux, configuration.Tmux) {
					t.Errorf("tmux config = %#v, want %#v", options.Tmux, configuration.Tmux)
				}
				return "repo_feature_12345678", nil
			},
			isInteractive: func() bool { return false },
			attachSession: func(context.Context, string) error {
				t.Fatal("attach called in non-interactive mode")
				return nil
			},
		},
	)
	command.SetOut(&output)
	command.SetArgs([]string{"feature", "--from", "release", "--config", "local.yaml"})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got, want := output.String(), "Branch: feature\nWorktree: /worktrees/repo_feature\nTmux session: repo_feature_12345678\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

func TestNewWorktreeCommandAttachesInteractiveTmuxSession(t *testing.T) {
	var attached string
	command := newWorktreeCmd(
		func() (string, error) { return "/repo", nil },
		func(context.Context, string, string, string) (string, error) { return "/repo_feature", nil },
		newWorktreeOptions{
			inspect: func(context.Context, string) (git.WorktreeInfo, error) {
				return git.WorktreeInfo{Kind: git.MainWorktree, WorktreeRoot: "/repo", MainWorktreeRoot: "/repo"}, nil
			},
			loadConfiguration: func(context.Context, string, string) (config.Config, error) { return config.Config{}, nil },
			createSession:     func(context.Context, tmux.CreateSessionOptions) (string, error) { return "repo_feature", nil },
			isInteractive:     func() bool { return true },
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

func TestNewWorktreeCommandNoTmuxSkipsTmuxSetup(t *testing.T) {
	var output bytes.Buffer
	command := newWorktreeCmd(
		func() (string, error) { return "/repo", nil },
		func(context.Context, string, string, string) (string, error) { return "/repo_feature", nil },
		newWorktreeOptions{
			inspect: func(context.Context, string) (git.WorktreeInfo, error) {
				t.Fatal("inspect called with --no-tmux")
				return git.WorktreeInfo{}, nil
			},
			loadConfiguration: func(context.Context, string, string) (config.Config, error) {
				t.Fatal("loadConfiguration called with --no-tmux")
				return config.Config{}, nil
			},
			createSession: func(context.Context, tmux.CreateSessionOptions) (string, error) {
				t.Fatal("createSession called with --no-tmux")
				return "", nil
			},
		},
	)
	command.SetOut(&output)
	command.SetArgs([]string{"feature", "--no-tmux"})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got, want := output.String(), "Branch: feature\nWorktree: /repo_feature\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

func TestNewWorktreeCommandNoAttachLeavesSessionRunning(t *testing.T) {
	command := newWorktreeCmd(
		func() (string, error) { return "/repo", nil },
		func(context.Context, string, string, string) (string, error) { return "/repo_feature", nil },
		newWorktreeOptions{
			inspect: func(context.Context, string) (git.WorktreeInfo, error) {
				return git.WorktreeInfo{Kind: git.MainWorktree, WorktreeRoot: "/repo", MainWorktreeRoot: "/repo"}, nil
			},
			loadConfiguration: func(context.Context, string, string) (config.Config, error) { return config.Config{}, nil },
			createSession:     func(context.Context, tmux.CreateSessionOptions) (string, error) { return "repo_feature", nil },
			isInteractive:     func() bool { return true },
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

func TestNewWorktreeCommandReportsAttachFailureAfterCreation(t *testing.T) {
	attachErr := errors.New("attach failed")
	command := newWorktreeCmd(
		func() (string, error) { return "/repo", nil },
		func(context.Context, string, string, string) (string, error) { return "/repo_feature", nil },
		newWorktreeOptions{
			inspect: func(context.Context, string) (git.WorktreeInfo, error) {
				return git.WorktreeInfo{Kind: git.MainWorktree, WorktreeRoot: "/repo", MainWorktreeRoot: "/repo"}, nil
			},
			loadConfiguration: func(context.Context, string, string) (config.Config, error) { return config.Config{}, nil },
			createSession:     func(context.Context, tmux.CreateSessionOptions) (string, error) { return "repo_feature", nil },
			isInteractive:     func() bool { return true },
			attachSession:     func(context.Context, string) error { return attachErr },
		},
	)
	command.SetArgs([]string{"feature"})

	err := command.Execute()
	if !errors.Is(err, attachErr) {
		t.Fatalf("Execute() error = %v, want wrapped %v", err, attachErr)
	}
	if !strings.Contains(err.Error(), "created branch \"feature\", worktree \"/repo_feature\", and tmux session \"repo_feature\"") {
		t.Fatalf("Execute() error = %v, want created resources", err)
	}
}

func TestNewWorktreeCommandReportsTmuxFailureAfterCreation(t *testing.T) {
	sessionErr := errors.New("tmux failed")
	command := newWorktreeCmd(
		func() (string, error) { return "/repo", nil },
		func(context.Context, string, string, string) (string, error) { return "/repo_feature", nil },
		newWorktreeOptions{
			inspect: func(context.Context, string) (git.WorktreeInfo, error) {
				return git.WorktreeInfo{Kind: git.MainWorktree, WorktreeRoot: "/repo", MainWorktreeRoot: "/repo"}, nil
			},
			loadConfiguration: func(context.Context, string, string) (config.Config, error) { return config.Config{}, nil },
			createSession:     func(context.Context, tmux.CreateSessionOptions) (string, error) { return "", sessionErr },
		},
	)
	command.SetArgs([]string{"feature"})

	err := command.Execute()
	if !errors.Is(err, sessionErr) {
		t.Fatalf("Execute() error = %v, want wrapped %v", err, sessionErr)
	}
	if !strings.Contains(err.Error(), "created branch \"feature\" and worktree \"/repo_feature\"") {
		t.Fatalf("Execute() error = %v, want created resources", err)
	}
}

func TestNewWorktreeCommandReturnsPickErrors(t *testing.T) {
	listErr := errors.New("list branches failed")
	selectErr := errors.New("selection failed")
	tests := []struct {
		name    string
		options newWorktreeOptions
		wantErr error
	}{
		{
			name: "list branches",
			options: newWorktreeOptions{
				listBranches:  func(context.Context, string) ([]string, error) { return nil, listErr },
				isInteractive: func() bool { return true },
				selectBranch: func(io.Reader, io.Writer, []string) (string, error) {
					t.Fatal("selector called after branch listing failure")
					return "", nil
				},
			},
			wantErr: listErr,
		},
		{
			name: "select branch",
			options: newWorktreeOptions{
				listBranches:  func(context.Context, string) ([]string, error) { return []string{"main"}, nil },
				isInteractive: func() bool { return true },
				selectBranch: func(io.Reader, io.Writer, []string) (string, error) {
					return "", selectErr
				},
			},
			wantErr: selectErr,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command := newWorktreeCmd(
				func() (string, error) { return "/repo", nil },
				func(context.Context, string, string, string) (string, error) {
					t.Fatal("creator called after pick failure")
					return "", nil
				},
				test.options,
			)
			command.SetArgs([]string{"feature", "--pick"})

			if err := command.Execute(); !errors.Is(err, test.wantErr) {
				t.Fatalf("Execute() error = %v, want wrapped %v", err, test.wantErr)
			}
		})
	}
}

func TestNewWorktreeCommandRejectsNonInteractivePick(t *testing.T) {
	command := newWorktreeCmd(
		func() (string, error) { return "/repo", nil },
		func(context.Context, string, string, string) (string, error) {
			t.Fatal("creator called without an interactive terminal")
			return "", nil
		},
		newWorktreeOptions{
			listBranches: func(_ context.Context, directory string) ([]string, error) {
				if directory != "/repo" {
					t.Errorf("directory = %q, want /repo", directory)
				}
				return []string{"main"}, nil
			},
			isInteractive: func() bool { return false },
			selectBranch: func(io.Reader, io.Writer, []string) (string, error) {
				t.Fatal("selector called without an interactive terminal")
				return "", nil
			},
		},
	)
	command.SetArgs([]string{"feature", "--pick"})

	if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "--pick requires an interactive terminal") {
		t.Fatalf("Execute() error = %v, want non-interactive terminal error", err)
	}
}

func TestNewWorktreeCommandRejectsConflictingBaseFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "main and from",
			args: []string{"feature", "--main", "--from", "release"},
		},
		{
			name: "main and pick",
			args: []string{"feature", "--main", "--pick"},
		},
		{
			name: "from and pick",
			args: []string{"feature", "--from", "release", "--pick"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command := newWorktreeCmd(
				func() (string, error) {
					t.Fatal("getwd called with conflicting base flags")
					return "", nil
				},
				func(context.Context, string, string, string) (string, error) {
					t.Fatal("creator called with conflicting base flags")
					return "", nil
				},
				newWorktreeOptions{},
			)
			command.SetArgs(test.args)

			if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "--main, --from, and --pick are mutually exclusive") {
				t.Fatalf("Execute() error = %v, want mutually exclusive flag error", err)
			}
		})
	}
}

func TestNewWorktreeCommandErrors(t *testing.T) {
	getwdErr := errors.New("getwd failed")
	command := newWorktreeCmd(
		func() (string, error) { return "", getwdErr },
		func(context.Context, string, string, string) (string, error) {
			t.Fatal("creator called after getwd failure")
			return "", nil
		},
		newWorktreeOptions{},
	)
	command.SetArgs([]string{"feature"})
	if err := command.Execute(); !errors.Is(err, getwdErr) {
		t.Fatalf("Execute() error = %v, want wrapped getwd error", err)
	}

	createErr := errors.New("creation failed")
	command = newWorktreeCmd(
		func() (string, error) { return "/repo", nil },
		func(context.Context, string, string, string) (string, error) { return "", createErr },
		newWorktreeOptions{},
	)
	command.SetArgs([]string{"feature"})
	if err := command.Execute(); !errors.Is(err, createErr) {
		t.Fatalf("Execute() error = %v, want wrapped creation error", err)
	}
}

func TestNewWorktreeCommandRejectsInvalidArgumentCounts(t *testing.T) {
	for _, args := range [][]string{nil, {"feature", "unexpected"}} {
		command := newWorktreeCmd(
			func() (string, error) { return "/repo", nil },
			func(context.Context, string, string, string) (string, error) {
				t.Fatal("creator called with invalid arguments")
				return "", nil
			},
			newWorktreeOptions{},
		)
		command.SetArgs(args)

		if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "accepts 1 arg(s), received") {
			t.Errorf("Execute(%q) error = %v, want argument error", args, err)
		}
	}
}

func TestNewCommandAtRoot(t *testing.T) {
	var output bytes.Buffer
	command := newRootCommand(
		func() (string, error) { return "/repo", nil },
		nil,
		func(_ context.Context, directory, branch, base string) (string, error) {
			if directory != "/repo" || branch != "feature" || base != "" {
				t.Errorf("create(%q, %q, %q), want create(/repo, feature, empty)", directory, branch, base)
			}
			return "/repo_feature", nil
		},
		newWorktreeOptions{},
	)
	command.SetOut(&output)
	command.SetArgs([]string{"new", "feature"})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got, want := output.String(), "Branch: feature\nWorktree: /repo_feature\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

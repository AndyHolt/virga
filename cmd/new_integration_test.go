package cmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/AndyHolt/virga/internal/config"
	"github.com/AndyHolt/virga/internal/git"
	"github.com/AndyHolt/virga/internal/tmux"
)

func TestNewWorktreeCommandCreatesFromSelectedBaseBranch(t *testing.T) {
	root := newCLITestRepository(t)
	releaseCommit := cliGitOutput(t, "-C", root, "rev-parse", "HEAD")
	cliRunGit(t, "-C", root, "branch", "release", releaseCommit)
	cliRunGit(t, "-C", root, "commit", "--allow-empty", "-m", "main commit")
	mainCommit := cliGitOutput(t, "-C", root, "rev-parse", "HEAD")

	tests := []struct {
		name       string
		args       []string
		branch     string
		baseCommit string
	}{
		{
			name:       "current branch by default",
			args:       []string{"new", "from-current"},
			branch:     "from-current",
			baseCommit: mainCommit,
		},
		{
			name:       "named local branch",
			args:       []string{"new", "from-release", "--from", "release"},
			branch:     "from-release",
			baseCommit: releaseCommit,
		},
		{
			name:       "local main branch",
			args:       []string{"new", "from-main", "--main"},
			branch:     "from-main",
			baseCommit: mainCommit,
		},
		{
			name:       "interactive selection",
			args:       []string{"new", "from-pick", "--pick"},
			branch:     "from-pick",
			baseCommit: releaseCommit,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			command := newRootCommand(
				func() (string, error) { return root, nil },
				git.InspectWorktree,
				git.CreateWorktree,
				newWorktreeOptions{
					listBranches:  git.ListLocalBranches,
					isInteractive: func() bool { return true },
					selectBranch: func(_ io.Reader, _ io.Writer, branches []string) (string, error) {
						if !containsBranch(branches, "release") {
							t.Errorf("branches = %q, want release", branches)
						}
						return "release", nil
					},
				},
			)
			command.SetOut(&output)
			command.SetArgs(test.args)

			if err := command.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}

			worktree := filepath.Join(filepath.Dir(root), "repository_"+test.branch)
			if got, want := output.String(), "Branch: "+test.branch+"\nWorktree: "+worktree+"\n"; got != want {
				t.Errorf("stdout = %q, want %q", got, want)
			}
			if got := cliGitOutput(t, "-C", root, "rev-parse", "refs/heads/"+test.branch); got != test.baseCommit {
				t.Errorf("branch commit = %q, want %q", got, test.baseCommit)
			}
			if got := cliGitOutput(t, "-C", worktree, "rev-parse", "HEAD"); got != test.baseCommit {
				t.Errorf("worktree commit = %q, want %q", got, test.baseCommit)
			}
		})
	}
}

func TestNewWorktreeCommandCreatesTmuxSessionFromRepositoryConfig(t *testing.T) {
	root := newCLITestRepository(t)
	if err := os.WriteFile(filepath.Join(root, ".virga.yaml"), []byte(`tmux:
  windows:
    - name: editor
      panes:
        - command: nvim
`), 0o644); err != nil {
		t.Fatalf("write repository config: %v", err)
	}

	configurationLoader := config.NewLoader(config.LoaderDependencies{})
	var output bytes.Buffer
	var sessionOptions tmux.CreateSessionOptions
	command := newRootCommand(
		func() (string, error) { return root, nil },
		git.InspectWorktree,
		git.CreateWorktree,
		newWorktreeOptions{
			inspect:           git.InspectWorktree,
			loadConfiguration: configurationLoader.Load,
			createSession: func(_ context.Context, options tmux.CreateSessionOptions) (string, error) {
				sessionOptions = options
				return "repository_configured_12345678", nil
			},
			isInteractive: func() bool { return true },
			attachSession: func(context.Context, string) error {
				t.Fatal("attach called with --no-attach")
				return nil
			},
		},
	)
	command.SetOut(&output)
	command.SetArgs([]string{"new", "configured", "--no-attach"})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	worktree := filepath.Join(filepath.Dir(root), "repository_configured")
	if got, want := output.String(), "Branch: configured\nWorktree: "+worktree+"\nTmux session: repository_configured_12345678\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
	if sessionOptions.RepositoryRoot != root || sessionOptions.Branch != "configured" || sessionOptions.WorktreeRoot != worktree {
		t.Errorf("session options = %#v, want root, branch, and worktree", sessionOptions)
	}
	wantTmux := config.TmuxConfig{Windows: []config.TmuxWindow{{Name: "editor", Panes: []config.TmuxPane{{Command: "nvim"}}}}}
	if !reflect.DeepEqual(sessionOptions.Tmux, wantTmux) {
		t.Errorf("tmux config = %#v, want %#v", sessionOptions.Tmux, wantTmux)
	}
}

func TestNewWorktreeCommandReportsNonGitRepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}

	for _, test := range []struct {
		name          string
		args          []string
		isInteractive bool
	}{
		{name: "default base branch", args: []string{"new", "feature"}},
		{name: "selected base branch", args: []string{"new", "feature", "--pick"}, isInteractive: true},
		{name: "selected base branch in a non-interactive terminal", args: []string{"new", "feature", "--pick"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stderr bytes.Buffer
			command := newRootCommand(
				func() (string, error) { return t.TempDir(), nil },
				git.InspectWorktree,
				git.CreateWorktree,
				newWorktreeOptions{
					listBranches:  git.ListLocalBranches,
					isInteractive: func() bool { return test.isInteractive },
					selectBranch: func(io.Reader, io.Writer, []string) (string, error) {
						t.Fatal("selector called outside a Git repository")
						return "", nil
					},
				},
			)
			command.SetErr(&stderr)
			command.SetArgs(test.args)

			err := command.Execute()
			if !errors.Is(err, git.ErrNotGitRepository) {
				t.Fatalf("Execute() error = %v, want %v", err, git.ErrNotGitRepository)
			}
			if got, want := stderr.String(), "Error: not in a Git repository\n"; got != want {
				t.Errorf("stderr = %q, want %q", got, want)
			}
		})
	}
}

func TestNewWorktreeCommandReportsMissingBaseBranch(t *testing.T) {
	root := newCLITestRepository(t)
	var stderr bytes.Buffer
	command := newRootCommand(
		func() (string, error) { return root, nil },
		git.InspectWorktree,
		git.CreateWorktree,
		newWorktreeOptions{},
	)
	command.SetOut(&stderr)
	command.SetErr(&stderr)
	command.SetArgs([]string{"new", "feature", "--from", "not-a-branch"})

	err := command.Execute()
	var missingBaseBranch *git.LocalBaseBranchNotFoundError
	if !errors.As(err, &missingBaseBranch) {
		t.Fatalf("Execute() error = %v, want missing base branch error", err)
	}
	if got, want := missingBaseBranch.Branch, "not-a-branch"; got != want {
		t.Errorf("missing base branch = %q, want %q", got, want)
	}

	output := stderr.String()
	if want := "Error: local base branch \"not-a-branch\" does not exist\n"; !strings.HasPrefix(output, want) {
		t.Errorf("stderr = %q, want prefix %q", output, want)
	}
	if !strings.Contains(output, "Usage:") {
		t.Errorf("stderr = %q, want command usage", output)
	}
	if strings.Contains(output, "create worktree") {
		t.Errorf("stderr = %q, contains implementation detail", output)
	}
}

func TestNewWorktreeCommandReportsExistingBranch(t *testing.T) {
	root := newCLITestRepository(t)
	cliRunGit(t, "-C", root, "branch", "existing")

	var output bytes.Buffer
	command := newRootCommand(
		func() (string, error) { return root, nil },
		git.InspectWorktree,
		git.CreateWorktree,
		newWorktreeOptions{},
	)
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs([]string{"new", "existing"})

	err := command.Execute()
	var existingBranch *git.LocalBranchExistsError
	if !errors.As(err, &existingBranch) {
		t.Fatalf("Execute() error = %v, want existing branch error", err)
	}
	if got, want := existingBranch.Branch, "existing"; got != want {
		t.Errorf("existing branch = %q, want %q", got, want)
	}

	got := output.String()
	if want := "Error: local branch \"existing\" already exists\n"; !strings.HasPrefix(got, want) {
		t.Errorf("output = %q, want prefix %q", got, want)
	}
	if !strings.Contains(got, "Usage:") {
		t.Errorf("output = %q, want command usage", got)
	}
	if strings.Contains(got, "create worktree") {
		t.Errorf("output = %q, contains implementation detail", got)
	}
}

func newCLITestRepository(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, "global.gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	root := filepath.Join(t.TempDir(), "repository")
	cliRunGit(t, "init", "--initial-branch=main", root)
	cliRunGit(t, "-C", root, "config", "user.name", "Virga Test")
	cliRunGit(t, "-C", root, "config", "user.email", "virga@example.invalid")
	cliRunGit(t, "-C", root, "commit", "--allow-empty", "-m", "initial commit")

	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("resolve repository path: %v", err)
	}
	return canonicalRoot
}

func cliRunGit(t *testing.T, arguments ...string) {
	t.Helper()
	if output, err := exec.Command("git", arguments...).CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", arguments, err, output)
	}
}

func cliGitOutput(t *testing.T, arguments ...string) string {
	t.Helper()
	output, err := exec.Command("git", arguments...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", arguments, err, output)
	}
	return strings.TrimSpace(string(output))
}

func containsBranch(branches []string, want string) bool {
	for _, branch := range branches {
		if branch == want {
			return true
		}
	}
	return false
}

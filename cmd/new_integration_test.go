package cmd

import (
	"bytes"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AndyHolt/virga/internal/git"
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

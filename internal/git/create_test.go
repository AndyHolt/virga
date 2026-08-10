package git

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateWorktree(t *testing.T) {
	mainRoot := newTestRepository(t, "repository")
	baseCommit := gitOutput(t, "-C", mainRoot, "rev-parse", "HEAD")

	destination, err := CreateWorktree(context.Background(), mainRoot, "feature", "")
	if err != nil {
		t.Fatalf("CreateWorktree() error = %v", err)
	}

	wantDestination := worktreeDestination(mainRoot, "feature")
	assertWorktreeCreated(t, mainRoot, destination, wantDestination, "feature", baseCommit)
}

func TestCreateWorktreeFromLocalBranch(t *testing.T) {
	mainRoot := newTestRepository(t, "repository")
	releaseCommit := gitOutput(t, "-C", mainRoot, "rev-parse", "HEAD")
	runGit(t, "-C", mainRoot, "commit", "--allow-empty", "-m", "main commit")
	mainCommit := gitOutput(t, "-C", mainRoot, "rev-parse", "HEAD")
	runGit(t, "-C", mainRoot, "branch", "release", releaseCommit)
	runGit(t, "-C", mainRoot, "tag", "release", mainCommit)

	tests := []struct {
		name       string
		branch     string
		baseBranch string
		baseCommit string
	}{
		{
			name:       "named branch",
			branch:     "from-release",
			baseBranch: "release",
			baseCommit: releaseCommit,
		},
		{
			name:       "main branch",
			branch:     "from-main",
			baseBranch: "main",
			baseCommit: mainCommit,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			destination, err := CreateWorktree(context.Background(), mainRoot, test.branch, test.baseBranch)
			if err != nil {
				t.Fatalf("CreateWorktree() error = %v", err)
			}

			assertWorktreeCreated(
				t,
				mainRoot,
				destination,
				worktreeDestination(mainRoot, test.branch),
				test.branch,
				test.baseCommit,
			)
		})
	}
}

func TestCreateWorktreeFromRejectsMissingLocalBranch(t *testing.T) {
	mainRoot := newTestRepository(t, "repository")

	_, err := CreateWorktree(context.Background(), mainRoot, "feature", "does-not-exist")
	var missingBaseBranch *LocalBaseBranchNotFoundError
	if !errors.As(err, &missingBaseBranch) {
		t.Fatalf("CreateWorktree() error = %v, want missing base branch error", err)
	}
	if got, want := missingBaseBranch.Branch, "does-not-exist"; got != want {
		t.Errorf("missing base branch = %q, want %q", got, want)
	}
	if got, want := err.Error(), `local base branch "does-not-exist" does not exist`; got != want {
		t.Errorf("error = %q, want %q", got, want)
	}
	if localBranchExists(t, mainRoot, "feature") {
		t.Error("branch was created despite missing base branch")
	}
	if _, err := os.Lstat(worktreeDestination(mainRoot, "feature")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("destination error = %v, want not exist", err)
	}
}

func TestCreateWorktreeFromNestedDirectoryWithSpaces(t *testing.T) {
	mainRoot := newTestRepository(t, "main repository")
	nested := filepath.Join(mainRoot, "nested directory", "deeper")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("create nested directory: %v", err)
	}
	baseCommit := gitOutput(t, "-C", mainRoot, "rev-parse", "HEAD")

	destination, err := CreateWorktree(context.Background(), nested, "feature/login", "")
	if err != nil {
		t.Fatalf("CreateWorktree() error = %v", err)
	}

	wantDestination := filepath.Join(filepath.Dir(mainRoot), "main repository_feature-login")
	assertWorktreeCreated(t, mainRoot, destination, wantDestination, "feature/login", baseCommit)
}

func TestCreateWorktreeFromLinkedWorktree(t *testing.T) {
	mainRoot := newTestRepository(t, "primary")
	linkedRoot := filepath.Join(t.TempDir(), "existing linked worktree")
	runGit(t, "-C", mainRoot, "worktree", "add", "-b", "source", linkedRoot)
	runGit(t, "-C", linkedRoot, "commit", "--allow-empty", "-m", "source commit")
	baseCommit := gitOutput(t, "-C", linkedRoot, "rev-parse", "HEAD")

	destination, err := CreateWorktree(context.Background(), linkedRoot, "from-linked", "")
	if err != nil {
		t.Fatalf("CreateWorktree() error = %v", err)
	}

	wantDestination := worktreeDestination(mainRoot, "from-linked")
	assertWorktreeCreated(t, mainRoot, destination, wantDestination, "from-linked", baseCommit)
}

func TestCreateWorktreeRejectsConflicts(t *testing.T) {
	t.Run("existing branch", func(t *testing.T) {
		mainRoot := newTestRepository(t, "repository")
		runGit(t, "-C", mainRoot, "branch", "existing")

		_, err := CreateWorktree(context.Background(), mainRoot, "existing", "")
		var existingBranch *LocalBranchExistsError
		if !errors.As(err, &existingBranch) {
			t.Fatalf("CreateWorktree() error = %v, want existing branch error", err)
		}
		if got, want := existingBranch.Branch, "existing"; got != want {
			t.Errorf("existing branch = %q, want %q", got, want)
		}
		if got, want := err.Error(), `local branch "existing" already exists`; got != want {
			t.Errorf("error = %q, want %q", got, want)
		}
	})

	t.Run("existing destination", func(t *testing.T) {
		mainRoot := newTestRepository(t, "repository")
		destination := worktreeDestination(mainRoot, "blocked")
		if err := os.Mkdir(destination, 0o755); err != nil {
			t.Fatalf("create conflicting destination: %v", err)
		}

		_, err := CreateWorktree(context.Background(), mainRoot, "blocked", "")
		if err == nil || !strings.Contains(err.Error(), "destination") || !strings.Contains(err.Error(), "already exists") {
			t.Fatalf("CreateWorktree() error = %v, want existing destination error", err)
		}
		if localBranchExists(t, mainRoot, "blocked") {
			t.Error("branch was created despite destination conflict")
		}
	})
}

func TestCreateWorktreeRejectsInvalidBranch(t *testing.T) {
	mainRoot := newTestRepository(t, "repository")

	_, err := CreateWorktree(context.Background(), mainRoot, "invalid branch", "")
	if err == nil || !strings.Contains(err.Error(), `validate branch "invalid branch"`) {
		t.Fatalf("CreateWorktree() error = %v, want invalid branch error", err)
	}
}

func TestCreateWorktreeRejectsDetachedHEAD(t *testing.T) {
	mainRoot := newTestRepository(t, "repository")
	runGit(t, "-C", mainRoot, "checkout", "--detach")

	_, err := CreateWorktree(context.Background(), mainRoot, "feature", "")
	if err == nil || !strings.Contains(err.Error(), "HEAD is detached") {
		t.Fatalf("CreateWorktree() error = %v, want detached HEAD error", err)
	}
}

func TestCreateWorktreeRejectsNonGitRepository(t *testing.T) {
	isolateGitConfiguration(t)

	_, err := CreateWorktree(context.Background(), t.TempDir(), "feature", "")
	if !errors.Is(err, ErrNotGitRepository) {
		t.Fatalf("CreateWorktree() error = %v, want %v", err, ErrNotGitRepository)
	}
	if got, want := err.Error(), ErrNotGitRepository.Error(); got != want {
		t.Errorf("error = %q, want %q", got, want)
	}
}

func TestCreateWorktreeRejectsBareRepository(t *testing.T) {
	isolateGitConfiguration(t)
	bareRoot := filepath.Join(t.TempDir(), "bare repository")
	runGit(t, "init", "--bare", bareRoot)

	_, err := CreateWorktree(context.Background(), bareRoot, "feature", "")
	if err == nil || !strings.Contains(err.Error(), "repository") || !strings.Contains(err.Error(), "is bare") {
		t.Fatalf("CreateWorktree() error = %v, want bare repository error", err)
	}
}

func TestCreateWorktreeReturnsGitAddFailure(t *testing.T) {
	mainRoot := newTestRepository(t, "repository")
	gitErr := errors.New("worktree add failed")
	run := func(ctx context.Context, dir string, arguments ...string) ([]byte, error) {
		if len(arguments) >= 2 && arguments[0] == "worktree" && arguments[1] == "add" {
			return nil, gitErr
		}
		return output(ctx, dir, arguments...)
	}

	destination, err := createWorktree(context.Background(), mainRoot, "feature", "", run)
	if destination != "" {
		t.Errorf("destination = %q, want empty", destination)
	}
	if !errors.Is(err, gitErr) {
		t.Fatalf("CreateWorktree() error = %v, want wrapped Git error", err)
	}
	for _, want := range []string{"feature", worktreeDestination(mainRoot, "feature")} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}

func newTestRepository(t *testing.T, name string) string {
	t.Helper()
	isolateGitConfiguration(t)

	root := filepath.Join(t.TempDir(), name)
	runGit(t, "init", "--initial-branch=main", root)
	runGit(t, "-C", root, "config", "user.name", "Virga Test")
	runGit(t, "-C", root, "config", "user.email", "virga@example.invalid")
	runGit(t, "-C", root, "commit", "--allow-empty", "-m", "initial commit")

	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("resolve repository path: %v", err)
	}
	return canonicalRoot
}

func isolateGitConfiguration(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, "global.gitconfig"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
}

func assertWorktreeCreated(
	t *testing.T,
	mainRoot string,
	destination string,
	wantDestination string,
	branch string,
	baseCommit string,
) {
	t.Helper()
	if destination != wantDestination {
		t.Errorf("destination = %q, want %q", destination, wantDestination)
	}
	if info, err := os.Stat(destination); err != nil {
		t.Fatalf("stat destination: %v", err)
	} else if !info.IsDir() {
		t.Fatalf("destination %q is not a directory", destination)
	}

	branchCommit := gitOutput(t, "-C", mainRoot, "rev-parse", "refs/heads/"+branch)
	if branchCommit != baseCommit {
		t.Errorf("branch commit = %q, want base commit %q", branchCommit, baseCommit)
	}
	worktreeCommit := gitOutput(t, "-C", destination, "rev-parse", "HEAD")
	if worktreeCommit != baseCommit {
		t.Errorf("worktree commit = %q, want base commit %q", worktreeCommit, baseCommit)
	}
	checkedOutBranch := gitOutput(t, "-C", destination, "branch", "--show-current")
	if checkedOutBranch != branch {
		t.Errorf("checked-out branch = %q, want %q", checkedOutBranch, branch)
	}
}

func gitOutput(t *testing.T, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", arguments...)
	commandOutput, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", arguments, err, commandOutput)
	}
	return strings.TrimSpace(string(commandOutput))
}

func localBranchExists(t *testing.T, root, branch string) bool {
	t.Helper()
	command := exec.Command("git", "-C", root, "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	err := command.Run()
	if err == nil {
		return true
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return false
	}
	t.Fatalf("check branch %q: %v", branch, err)
	return false
}

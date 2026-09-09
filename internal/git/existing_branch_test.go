package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestAddExistingBranchWorktree(t *testing.T) {
	mainRoot := newTestRepository(t, "main repository")
	featureCommit := gitOutput(t, "-C", mainRoot, "rev-parse", "HEAD")
	runGit(t, "-C", mainRoot, "commit", "--allow-empty", "-m", "main commit")
	runGit(t, "-C", mainRoot, "branch", "feature/login", featureCommit)
	runGit(t, "-C", mainRoot, "tag", "feature/login")

	destination, err := AddExistingBranchWorktree(context.Background(), mainRoot, "feature/login")
	if err != nil {
		t.Fatalf("AddExistingBranchWorktree() error = %v", err)
	}

	wantDestination := worktreeDestination(mainRoot, "feature/login")
	assertWorktreeCreated(t, mainRoot, destination, wantDestination, "feature/login", featureCommit)
}

func TestAddExistingBranchWorktreeFromNestedLinkedWorktreeWithSpaces(t *testing.T) {
	mainRoot := newTestRepository(t, "primary repository")
	releaseCommit := gitOutput(t, "-C", mainRoot, "rev-parse", "HEAD")
	runGit(t, "-C", mainRoot, "commit", "--allow-empty", "-m", "main commit")
	runGit(t, "-C", mainRoot, "branch", "release", releaseCommit)
	linkedRoot := filepath.Join(t.TempDir(), "linked worktree")
	runGit(t, "-C", mainRoot, "worktree", "add", "-b", "source", linkedRoot)
	linkedRoot = canonicalPath(t, linkedRoot)
	nested := filepath.Join(linkedRoot, "nested directory", "deeper")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("create nested directory: %v", err)
	}

	destination, err := AddExistingBranchWorktree(context.Background(), nested, "release")
	if err != nil {
		t.Fatalf("AddExistingBranchWorktree() error = %v", err)
	}

	wantDestination := worktreeDestination(mainRoot, "release")
	assertWorktreeCreated(t, mainRoot, destination, wantDestination, "release", releaseCommit)
}

func TestAddExistingBranchWorktreeRejectsMissingBranch(t *testing.T) {
	mainRoot := newTestRepository(t, "repository")

	destination, err := AddExistingBranchWorktree(context.Background(), mainRoot, "missing")
	if destination != "" {
		t.Errorf("destination = %q, want empty", destination)
	}
	var missingBranch *LocalBranchNotFoundError
	if !errors.As(err, &missingBranch) {
		t.Fatalf("AddExistingBranchWorktree() error = %v, want missing branch error", err)
	}
	if got, want := missingBranch.Branch, "missing"; got != want {
		t.Errorf("missing branch = %q, want %q", got, want)
	}
	if got, want := err.Error(), `local branch "missing" does not exist`; got != want {
		t.Errorf("error = %q, want %q", got, want)
	}
}

func TestAddExistingBranchWorktreeRejectsInvalidBranch(t *testing.T) {
	mainRoot := newTestRepository(t, "repository")

	_, err := AddExistingBranchWorktree(context.Background(), mainRoot, "invalid branch")
	if err == nil || !strings.Contains(err.Error(), `validate branch "invalid branch"`) {
		t.Fatalf("AddExistingBranchWorktree() error = %v, want invalid branch error", err)
	}
}

func TestAddExistingBranchWorktreeRejectsDestinationConflict(t *testing.T) {
	mainRoot := newTestRepository(t, "repository")
	runGit(t, "-C", mainRoot, "branch", "blocked")
	destination := worktreeDestination(mainRoot, "blocked")
	if err := os.Mkdir(destination, 0o755); err != nil {
		t.Fatalf("create conflicting destination: %v", err)
	}

	_, err := AddExistingBranchWorktree(context.Background(), mainRoot, "blocked")
	if err == nil || !strings.Contains(err.Error(), "destination") || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("AddExistingBranchWorktree() error = %v, want existing destination error", err)
	}
	worktrees, listErr := ListWorktrees(context.Background(), mainRoot)
	if listErr != nil {
		t.Fatalf("ListWorktrees() error = %v", listErr)
	}
	if _, found := checkedOutWorktreeForBranch(worktrees, "refs/heads/blocked"); found {
		t.Fatal("blocked branch was checked out despite destination conflict")
	}
}

func TestAddExistingBranchWorktreeRejectsAlreadyCheckedOutBranch(t *testing.T) {
	mainRoot := newTestRepository(t, "repository")
	linkedRoot := filepath.Join(t.TempDir(), "feature worktree")
	runGit(t, "-C", mainRoot, "worktree", "add", "-b", "feature", linkedRoot)
	linkedRoot = canonicalPath(t, linkedRoot)

	destination, err := AddExistingBranchWorktree(context.Background(), mainRoot, "feature")
	if destination != "" {
		t.Errorf("destination = %q, want empty", destination)
	}
	var checkedOut *BranchAlreadyCheckedOutError
	if !errors.As(err, &checkedOut) {
		t.Fatalf("AddExistingBranchWorktree() error = %v, want checked-out branch error", err)
	}
	if got, want := checkedOut.Branch, "feature"; got != want {
		t.Errorf("checked-out branch = %q, want %q", got, want)
	}
	if got, want := checkedOut.Path, linkedRoot; got != want {
		t.Errorf("checked-out path = %q, want %q", got, want)
	}
}

func TestAddExistingBranchWorktreeRejectsMainBranchAlreadyCheckedOut(t *testing.T) {
	mainRoot := newTestRepository(t, "repository")

	_, err := AddExistingBranchWorktree(context.Background(), mainRoot, "main")
	var checkedOut *BranchAlreadyCheckedOutError
	if !errors.As(err, &checkedOut) {
		t.Fatalf("AddExistingBranchWorktree() error = %v, want checked-out branch error", err)
	}
	if got, want := checkedOut.Path, mainRoot; got != want {
		t.Errorf("checked-out path = %q, want %q", got, want)
	}
}

func TestAddExistingBranchWorktreeRejectsNonGitRepository(t *testing.T) {
	isolateGitConfiguration(t)

	_, err := AddExistingBranchWorktree(context.Background(), t.TempDir(), "feature")
	if !errors.Is(err, ErrNotGitRepository) {
		t.Fatalf("AddExistingBranchWorktree() error = %v, want %v", err, ErrNotGitRepository)
	}
	if got, want := err.Error(), ErrNotGitRepository.Error(); got != want {
		t.Errorf("error = %q, want %q", got, want)
	}
}

func TestAddExistingBranchWorktreeRejectsBareRepository(t *testing.T) {
	isolateGitConfiguration(t)
	bareRoot := filepath.Join(t.TempDir(), "bare repository")
	runGit(t, "init", "--bare", bareRoot)

	_, err := AddExistingBranchWorktree(context.Background(), bareRoot, "feature")
	if err == nil || !strings.Contains(err.Error(), "repository") || !strings.Contains(err.Error(), "is bare") {
		t.Fatalf("AddExistingBranchWorktree() error = %v, want bare repository error", err)
	}
}

func TestAddExistingBranchWorktreeReturnsGitAddFailure(t *testing.T) {
	mainRoot := newTestRepository(t, "repository")
	runGit(t, "-C", mainRoot, "branch", "feature")
	gitErr := errors.New("worktree add failed")
	var gotAddDir string
	var gotAddArgs []string
	run := func(ctx context.Context, dir string, arguments ...string) ([]byte, error) {
		if len(arguments) >= 2 && arguments[0] == "worktree" && arguments[1] == "add" {
			gotAddDir = dir
			gotAddArgs = append([]string(nil), arguments...)
			return nil, gitErr
		}
		return output(ctx, dir, arguments...)
	}

	destination, err := addExistingBranchWorktree(context.Background(), mainRoot, "feature", run)
	if destination != "" {
		t.Errorf("destination = %q, want empty", destination)
	}
	if !errors.Is(err, gitErr) {
		t.Fatalf("AddExistingBranchWorktree() error = %v, want wrapped Git error", err)
	}
	if got, want := gotAddDir, mainRoot; got != want {
		t.Errorf("worktree add directory = %q, want %q", got, want)
	}
	wantAddArgs := []string{"worktree", "add", worktreeDestination(mainRoot, "feature"), "feature"}
	if !reflect.DeepEqual(gotAddArgs, wantAddArgs) {
		t.Errorf("worktree add arguments = %#v, want %#v", gotAddArgs, wantAddArgs)
	}
	for _, want := range []string{"feature", worktreeDestination(mainRoot, "feature")} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err, want)
		}
	}
}

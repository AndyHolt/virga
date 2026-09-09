package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
)

// LocalBranchNotFoundError reports a requested local branch that does not exist.
type LocalBranchNotFoundError struct {
	Branch string
}

func (err *LocalBranchNotFoundError) Error() string {
	return fmt.Sprintf("local branch %q does not exist", err.Branch)
}

// BranchAlreadyCheckedOutError reports a branch that already has a usable
// worktree checkout.
type BranchAlreadyCheckedOutError struct {
	Branch string
	Path   string
}

func (err *BranchAlreadyCheckedOutError) Error() string {
	return fmt.Sprintf("local branch %q is already checked out at %q", err.Branch, err.Path)
}

// AddExistingBranchWorktree adds a linked worktree for an existing local branch
// beside the primary repository. It returns the absolute path to the new
// worktree.
func AddExistingBranchWorktree(ctx context.Context, dir, branch string) (string, error) {
	return addExistingBranchWorktree(ctx, dir, branch, output)
}

func addExistingBranchWorktree(ctx context.Context, dir, branch string, run outputRunner) (string, error) {
	bareOutput, err := run(ctx, dir, "rev-parse", "--is-bare-repository")
	if err != nil {
		if isNotGitRepository(err) {
			return "", ErrNotGitRepository
		}
		return "", fmt.Errorf("check whether %q is a bare Git repository: %w", dir, err)
	}
	switch strings.TrimSpace(string(bareOutput)) {
	case "true":
		return "", fmt.Errorf("add existing branch worktree: repository at %q is bare", dir)
	case "false":
	default:
		return "", fmt.Errorf("check whether %q is a bare Git repository: unexpected Git output %q", dir, strings.TrimSpace(string(bareOutput)))
	}

	info, err := inspectWorktree(ctx, run, dir)
	if err != nil {
		return "", fmt.Errorf("discover repository roots: %w", err)
	}
	if info.Kind == NotWorktree {
		return "", fmt.Errorf("add existing branch worktree: %q is not inside a Git worktree", dir)
	}

	if _, err := run(ctx, info.WorktreeRoot, "check-ref-format", "--branch", branch); err != nil {
		return "", fmt.Errorf("validate branch %q: %w", branch, err)
	}

	branchRef := "refs/heads/" + branch
	if _, err := run(ctx, info.WorktreeRoot, "show-ref", "--verify", "--quiet", branchRef); err != nil {
		if hasExitCode(err, 1) {
			return "", &LocalBranchNotFoundError{Branch: branch}
		}
		return "", fmt.Errorf("check whether branch %q exists: %w", branch, err)
	}

	worktrees, err := listWorktrees(ctx, run, info.WorktreeRoot)
	if err != nil {
		return "", err
	}
	if worktree, found := checkedOutWorktreeForBranch(worktrees, branchRef); found {
		return "", &BranchAlreadyCheckedOutError{Branch: branch, Path: worktree.Path}
	}

	destination := worktreeDestination(info.MainWorktreeRoot, branch)
	if _, err := os.Lstat(destination); err == nil {
		return "", fmt.Errorf("add existing branch worktree: destination %q already exists", destination)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("check worktree destination %q: %w", destination, err)
	}

	// git worktree add checks out an existing branch only when given the
	// branch shorthand; a fully-qualified ref creates a detached worktree.
	// The show-ref preflight above still restricts the operation to refs/heads.
	if _, err := run(ctx, info.WorktreeRoot, "worktree", "add", destination, branch); err != nil {
		return "", fmt.Errorf("add worktree for existing branch %q at %q: %w", branch, destination, err)
	}

	return destination, nil
}

func checkedOutWorktreeForBranch(worktrees []ListedWorktree, branchRef string) (ListedWorktree, bool) {
	for _, worktree := range worktrees {
		if worktree.BranchRef == branchRef && !worktree.Detached && !worktree.Bare && !worktree.Prunable {
			return worktree, true
		}
	}
	return ListedWorktree{}, false
}

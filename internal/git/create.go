package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// CreateWorktree creates branch from baseBranch and adds a linked worktree
// beside the primary repository. An empty baseBranch uses the currently
// checked-out branch. It returns the absolute path to the new worktree.
func CreateWorktree(ctx context.Context, dir, branch, baseBranch string) (string, error) {
	return createWorktree(ctx, dir, branch, baseBranch, output)
}

func createWorktree(ctx context.Context, dir, branch, baseBranch string, run outputRunner) (string, error) {
	bareOutput, err := run(ctx, dir, "rev-parse", "--is-bare-repository")
	if err != nil {
		return "", fmt.Errorf("check whether %q is a bare Git repository: %w", dir, err)
	}
	switch strings.TrimSpace(string(bareOutput)) {
	case "true":
		return "", fmt.Errorf("create worktree: repository at %q is bare", dir)
	case "false":
	default:
		return "", fmt.Errorf("check whether %q is a bare Git repository: unexpected Git output %q", dir, strings.TrimSpace(string(bareOutput)))
	}

	info, err := inspectWorktree(ctx, run, dir)
	if err != nil {
		return "", fmt.Errorf("discover repository roots: %w", err)
	}
	if info.Kind == NotWorktree {
		return "", fmt.Errorf("create worktree: %q is not inside a Git worktree", dir)
	}

	baseRef, err := localBaseRef(ctx, run, info.WorktreeRoot, branch, baseBranch)
	if err != nil {
		return "", err
	}

	if _, err := run(ctx, info.WorktreeRoot, "check-ref-format", "--branch", branch); err != nil {
		return "", fmt.Errorf("validate branch %q: %w", branch, err)
	}

	branchRef := "refs/heads/" + branch
	if _, err := run(ctx, info.WorktreeRoot, "show-ref", "--verify", "--quiet", branchRef); err == nil {
		return "", fmt.Errorf("create worktree: local branch %q already exists", branch)
	} else if !hasExitCode(err, 1) {
		return "", fmt.Errorf("check whether branch %q exists: %w", branch, err)
	}

	destination := worktreeDestination(info.MainWorktreeRoot, branch)
	if _, err := os.Lstat(destination); err == nil {
		return "", fmt.Errorf("create worktree: destination %q already exists", destination)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("check worktree destination %q: %w", destination, err)
	}

	if _, err := run(
		ctx,
		info.WorktreeRoot,
		"worktree", "add", "-b", branch, destination, baseRef,
	); err != nil {
		return "", fmt.Errorf("create branch %q and worktree at %q: %w", branch, destination, err)
	}

	return destination, nil
}

func localBaseRef(ctx context.Context, run outputRunner, directory, newBranch, baseBranch string) (string, error) {
	if baseBranch != "" {
		baseRef := "refs/heads/" + baseBranch
		if _, err := run(ctx, directory, "show-ref", "--verify", "--quiet", baseRef); err != nil {
			if hasExitCode(err, 1) {
				return "", fmt.Errorf("create worktree: local base branch %q does not exist", baseBranch)
			}
			return "", fmt.Errorf("check whether base branch %q exists: %w", baseBranch, err)
		}
		return baseRef, nil
	}

	branchOutput, err := run(ctx, directory, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil {
		if hasExitCode(err, 1) {
			return "", fmt.Errorf("create worktree: HEAD is detached; check out a branch before creating %q", newBranch)
		}
		return "", fmt.Errorf("read currently checked-out branch: %w", err)
	}
	currentBranch := strings.TrimSpace(string(branchOutput))
	if currentBranch == "" {
		return "", fmt.Errorf("create worktree: HEAD is detached; check out a branch before creating %q", newBranch)
	}
	return "refs/heads/" + currentBranch, nil
}

func worktreeDestination(primaryRoot, branch string) string {
	repositoryName := filepath.Base(filepath.Clean(primaryRoot))
	directoryBranch := strings.ReplaceAll(branch, "/", "-")
	return filepath.Join(filepath.Dir(primaryRoot), repositoryName+"_"+directoryBranch)
}

func hasExitCode(err error, code int) bool {
	var exitErr *exec.ExitError
	return errors.As(err, &exitErr) && exitErr.ExitCode() == code
}

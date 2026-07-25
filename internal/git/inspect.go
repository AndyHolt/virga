// Package git provides operations on Git repositories and worktrees.
package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// WorktreeKind identifies how a directory relates to a Git worktree.
type WorktreeKind string

const (
	// NotWorktree means the directory is not inside a Git worktree.
	NotWorktree WorktreeKind = "not-worktree"
	// MainWorktree means the directory is inside a repository's main worktree.
	MainWorktree WorktreeKind = "main-worktree"
	// LinkedWorktree means the directory is inside a worktree linked to another repository.
	LinkedWorktree WorktreeKind = "linked-worktree"
)

// WorktreeInfo describes the Git worktree containing a directory.
type WorktreeInfo struct {
	Directory        string
	Kind             WorktreeKind
	WorktreeRoot     string
	MainWorktreeRoot string
}

// InspectWorktree determines whether dir is in a main worktree, a linked
// worktree, or outside a Git worktree. Git must be installed and available on PATH.
func InspectWorktree(ctx context.Context, dir string) (WorktreeInfo, error) {
	absoluteDir, err := filepath.Abs(dir)
	if err != nil {
		return WorktreeInfo{}, fmt.Errorf("resolve directory %q: %w", dir, err)
	}

	info := WorktreeInfo{Directory: absoluteDir, Kind: NotWorktree}
	inside, err := output(ctx, absoluteDir, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && bytes.Contains(exitErr.Stderr, []byte("not a git repository")) {
			return info, nil
		}
		return WorktreeInfo{}, fmt.Errorf("check Git worktree: %w", err)
	}
	if strings.TrimSpace(string(inside)) != "true" {
		return info, nil
	}

	info.WorktreeRoot, err = gitPath(ctx, absoluteDir, "--show-toplevel")
	if err != nil {
		return WorktreeInfo{}, fmt.Errorf("find worktree root: %w", err)
	}
	gitDir, err := gitPath(ctx, absoluteDir, "--git-dir")
	if err != nil {
		return WorktreeInfo{}, fmt.Errorf("find Git directory: %w", err)
	}
	commonDir, err := gitPath(ctx, absoluteDir, "--git-common-dir")
	if err != nil {
		return WorktreeInfo{}, fmt.Errorf("find common Git directory: %w", err)
	}

	if filepath.Clean(gitDir) == filepath.Clean(commonDir) {
		info.Kind = MainWorktree
		info.MainWorktreeRoot = info.WorktreeRoot
		return info, nil
	}

	worktrees, err := output(ctx, absoluteDir, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return WorktreeInfo{}, fmt.Errorf("list Git worktrees: %w", err)
	}
	mainRoot, err := firstWorktreePath(worktrees)
	if err != nil {
		return WorktreeInfo{}, err
	}

	info.Kind = LinkedWorktree
	info.MainWorktreeRoot = mainRoot
	return info, nil
}

func gitPath(ctx context.Context, dir, argument string) (string, error) {
	commandOutput, err := output(ctx, dir, "rev-parse", "--path-format=absolute", argument)
	if err != nil {
		return "", err
	}
	return string(bytes.TrimSuffix(commandOutput, []byte{'\n'})), nil
}

func firstWorktreePath(output []byte) (string, error) {
	field, _, found := bytes.Cut(output, []byte{0})
	if !found || !bytes.HasPrefix(field, []byte("worktree ")) {
		return "", fmt.Errorf("parse Git worktree list: missing main worktree")
	}
	path := string(bytes.TrimPrefix(field, []byte("worktree ")))
	if path == "" {
		return "", fmt.Errorf("parse Git worktree list: empty main worktree path")
	}
	return path, nil
}

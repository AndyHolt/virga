package git

import (
	"bytes"
	"context"
	"fmt"
	"strings"
)

// ListedWorktree describes one entry from git worktree list.
type ListedWorktree struct {
	Path        string
	Kind        WorktreeKind
	HEAD        string
	Branch      string
	BranchRef   string
	Detached    bool
	Bare        bool
	Locked      bool
	LockReason  string
	Prunable    bool
	PruneReason string
}

// ListWorktrees returns the Git worktrees associated with the repository that
// contains dir. Git returns the primary worktree first, followed by linked
// worktrees.
func ListWorktrees(ctx context.Context, dir string) ([]ListedWorktree, error) {
	return listWorktrees(ctx, output, dir)
}

func listWorktrees(ctx context.Context, run outputRunner, dir string) ([]ListedWorktree, error) {
	worktreeOutput, err := run(ctx, dir, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		if isNotGitRepository(err) {
			return nil, ErrNotGitRepository
		}
		return nil, fmt.Errorf("list Git worktrees: %w", err)
	}

	worktrees, err := parseWorktreeList(worktreeOutput)
	if err != nil {
		return nil, err
	}
	return worktrees, nil
}

func parseWorktreeList(output []byte) ([]ListedWorktree, error) {
	var worktrees []ListedWorktree
	var current *ListedWorktree

	finishCurrent := func() {
		if current == nil {
			return
		}
		if len(worktrees) == 0 {
			current.Kind = MainWorktree
		} else {
			current.Kind = LinkedWorktree
		}
		worktrees = append(worktrees, *current)
		current = nil
	}

	for _, rawField := range bytes.Split(output, []byte{0}) {
		if len(rawField) == 0 {
			continue
		}
		field := string(rawField)
		if strings.HasPrefix(field, "worktree ") {
			finishCurrent()
			path := strings.TrimPrefix(field, "worktree ")
			if path == "" {
				return nil, fmt.Errorf("parse Git worktree list: empty worktree path")
			}
			current = &ListedWorktree{Path: path}
			continue
		}
		if current == nil {
			return nil, fmt.Errorf("parse Git worktree list: field %q before worktree path", field)
		}

		switch {
		case strings.HasPrefix(field, "HEAD "):
			current.HEAD = strings.TrimPrefix(field, "HEAD ")
			if current.HEAD == "" {
				return nil, fmt.Errorf("parse Git worktree list: empty HEAD for worktree %q", current.Path)
			}
		case strings.HasPrefix(field, "branch "):
			current.BranchRef = strings.TrimPrefix(field, "branch ")
			if current.BranchRef == "" {
				return nil, fmt.Errorf("parse Git worktree list: empty branch for worktree %q", current.Path)
			}
			current.Branch = localBranchName(current.BranchRef)
		case field == "detached":
			current.Detached = true
		case field == "bare":
			current.Bare = true
		case field == "locked":
			current.Locked = true
		case strings.HasPrefix(field, "locked "):
			current.Locked = true
			current.LockReason = strings.TrimPrefix(field, "locked ")
		case field == "prunable":
			current.Prunable = true
		case strings.HasPrefix(field, "prunable "):
			current.Prunable = true
			current.PruneReason = strings.TrimPrefix(field, "prunable ")
		default:
			return nil, fmt.Errorf("parse Git worktree list: unrecognized field %q", field)
		}
	}
	finishCurrent()

	if len(worktrees) == 0 {
		return nil, fmt.Errorf("parse Git worktree list: no worktrees")
	}
	return worktrees, nil
}

func localBranchName(ref string) string {
	return strings.TrimPrefix(ref, "refs/heads/")
}

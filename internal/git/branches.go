package git

import (
	"bytes"
	"context"
	"fmt"
)

// ListLocalBranches returns local branch names in refname order.
// TODO consider whether --sort=-commiterdate recency order would be better than refname?
func ListLocalBranches(ctx context.Context, dir string) ([]string, error) {
	return listLocalBranches(ctx, output, dir)
}

func listLocalBranches(ctx context.Context, run outputRunner, dir string) ([]string, error) {
	if _, err := run(ctx, dir, "rev-parse", "--git-dir"); err != nil {
		if isNotGitRepository(err) {
			return nil, ErrNotGitRepository
		}
		return nil, fmt.Errorf("check Git repository: %w", err)
	}

	branchOutput, err := run(
		ctx,
		dir,
		"for-each-ref",
		"--sort=refname",
		"--format=%(refname:short)%00",
		"refs/heads/",
	)
	if err != nil {
		if isNotGitRepository(err) {
			return nil, ErrNotGitRepository
		}
		return nil, fmt.Errorf("list local branches: %w", err)
	}

	return parseLocalBranches(branchOutput), nil
}

func parseLocalBranches(output []byte) []string {
	var branches []string
	for _, record := range bytes.Split(output, []byte{0}) {
		record = bytes.TrimPrefix(record, []byte{'\n'})
		if len(record) > 0 {
			branches = append(branches, string(record))
		}
	}
	return branches
}

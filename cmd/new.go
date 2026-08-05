package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

type worktreeCreator func(context.Context, string, string) (string, error)

func newWorktreeCmd(getwd func() (string, error), create worktreeCreator) *cobra.Command {
	return &cobra.Command{
		Use:     "new <branch>",
		Short:   "Create a branch in a new Git worktree",
		Example: "  virga new feature/login",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			directory, err := getwd()
			if err != nil {
				return fmt.Errorf("get current directory: %w", err)
			}

			worktree, err := create(cmd.Context(), directory, args[0])
			if err != nil {
				return fmt.Errorf("create worktree: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Branch: %s\nWorktree: %s\n", args[0], worktree); err != nil {
				return fmt.Errorf("write worktree result: %w", err)
			}
			return nil
		},
	}
}

package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
)

type worktreeCreator func(context.Context, string, string, string) (string, error)

func newWorktreeCmd(getwd func() (string, error), create worktreeCreator) *cobra.Command {
	var baseBranch string
	var useMain bool

	command := &cobra.Command{
		Use:     "new <branch>",
		Short:   "Create a branch in a new Git worktree",
		Example: "  virga new feature/login\n  virga new feature/login --from release\n  virga new feature/login --main",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if useMain && cmd.Flags().Changed("from") {
				return fmt.Errorf("--main and --from are mutually exclusive")
			}
			if cmd.Flags().Changed("from") && baseBranch == "" {
				return fmt.Errorf("--from requires a local branch")
			}
			selectedBase := baseBranch
			if useMain {
				selectedBase = "main"
			}

			directory, err := getwd()
			if err != nil {
				return fmt.Errorf("get current directory: %w", err)
			}

			worktree, err := create(cmd.Context(), directory, args[0], selectedBase)
			if err != nil {
				return fmt.Errorf("create worktree: %w", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Branch: %s\nWorktree: %s\n", args[0], worktree); err != nil {
				return fmt.Errorf("write worktree result: %w", err)
			}
			return nil
		},
	}
	command.Flags().StringVar(&baseBranch, "from", "", "Create from a named local branch")
	command.Flags().BoolVar(&useMain, "main", false, "Create from the local main branch")
	return command
}

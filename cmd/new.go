package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/AndyHolt/virga/internal/git"
	"github.com/spf13/cobra"
)

type worktreeCreator func(context.Context, string, string, string) (string, error)
type localBranchLister func(context.Context, string) ([]string, error)
type terminalDetector func() bool

type newWorktreeOptions struct {
	listBranches  localBranchLister
	isInteractive terminalDetector
	selectBranch  branchSelector
}

func newWorktreeCmd(getwd func() (string, error), create worktreeCreator, options newWorktreeOptions) *cobra.Command {
	var baseBranch string
	var useMain bool
	var pickBase bool

	command := &cobra.Command{
		Use:     "new <branch>",
		Short:   "Create a branch in a new Git worktree",
		Example: "  virga new feature/login\n  virga new feature/login --from release\n  virga new feature/login --main\n  virga new feature/login --pick",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fromSet := cmd.Flags().Changed("from")
			if (fromSet && useMain) || (fromSet && pickBase) || (useMain && pickBase) {
				return fmt.Errorf("--main, --from, and --pick are mutually exclusive")
			}
			if fromSet && baseBranch == "" {
				return fmt.Errorf("--from requires a local branch")
			}
			if pickBase && (options.listBranches == nil || options.selectBranch == nil) {
				return fmt.Errorf("interactive branch selection is unavailable")
			}

			directory, err := getwd()
			if err != nil {
				return fmt.Errorf("get current directory: %w", err)
			}

			selectedBase := baseBranch
			switch {
			case useMain:
				selectedBase = "main"
			case pickBase:
				branches, err := options.listBranches(cmd.Context(), directory)
				if err != nil {
					return newWorktreeCommandError(cmd, "list local branches", err)
				}
				if options.isInteractive == nil || !options.isInteractive() {
					return fmt.Errorf("--pick requires an interactive terminal")
				}
				selectedBase, err = options.selectBranch(cmd.InOrStdin(), cmd.ErrOrStderr(), branches)
				if err != nil {
					return fmt.Errorf("select base branch: %w", err)
				}
			}

			worktree, err := create(cmd.Context(), directory, args[0], selectedBase)
			if err != nil {
				return newWorktreeCommandError(cmd, "create worktree", err)
			}
			if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Branch: %s\nWorktree: %s\n", args[0], worktree); err != nil {
				return fmt.Errorf("write worktree result: %w", err)
			}
			return nil
		},
	}
	command.Flags().StringVar(&baseBranch, "from", "", "Create from a named local branch")
	command.Flags().BoolVar(&useMain, "main", false, "Create from the local main branch")
	command.Flags().BoolVar(&pickBase, "pick", false, "Interactively select a local base branch")
	return command
}

func newWorktreeCommandError(cmd *cobra.Command, operation string, err error) error {
	if errors.Is(err, git.ErrNotGitRepository) {
		cmd.SilenceUsage = true
		return git.ErrNotGitRepository
	}

	var missingBaseBranch *git.LocalBaseBranchNotFoundError
	if errors.As(err, &missingBaseBranch) {
		return missingBaseBranch
	}

	var existingBranch *git.LocalBranchExistsError
	if errors.As(err, &existingBranch) {
		return existingBranch
	}
	return fmt.Errorf("%s: %w", operation, err)
}

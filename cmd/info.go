package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/AndyHolt/virga/internal/git"
	"github.com/spf13/cobra"
)

type directoryInspector func(context.Context, string) (git.WorktreeInfo, error)

func newInfoCmd(getwd func() (string, error), inspect directoryInspector) *cobra.Command {
	return &cobra.Command{
		Use:     "info",
		Short:   "Describe the current Git worktree",
		Example: "  virga info",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			directory, err := getwd()
			if err != nil {
				return fmt.Errorf("get current directory: %w", err)
			}

			info, err := inspect(cmd.Context(), directory)
			if err != nil {
				return fmt.Errorf("inspect current directory: %w", err)
			}

			output := fmt.Sprintf("Directory: %s\n", info.Directory)
			switch info.Kind {
			case git.MainWorktree:
				output += fmt.Sprintf("Type: main Git worktree\nWorktree root: %s\n", info.WorktreeRoot)
			case git.LinkedWorktree:
				output += fmt.Sprintf(
					"Type: linked Git worktree\nWorktree root: %s\nMain worktree: %s\n",
					info.WorktreeRoot,
					info.MainWorktreeRoot,
				)
			case git.NotWorktree:
				output += "Type: not a Git worktree\n"
			default:
				return fmt.Errorf("inspect current directory: unknown worktree type %q", info.Kind)
			}
			if _, err := fmt.Fprint(cmd.OutOrStdout(), output); err != nil {
				return fmt.Errorf("write info: %w", err)
			}
			return nil
		},
	}
}

func init() {
	rootCmd.AddCommand(newInfoCmd(os.Getwd, git.InspectWorktree))
}

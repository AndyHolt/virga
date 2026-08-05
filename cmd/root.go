/*
Copyright © 2026 Andy Holt <andrew.holt@hotmail.co.uk>
*/
package cmd

import (
	"os"

	"github.com/AndyHolt/virga/internal/git"
	"github.com/spf13/cobra"
)

// NewRootCommand constructs the root Virga command.
func NewRootCommand() *cobra.Command {
	return newRootCommand(os.Getwd, git.InspectWorktree, git.CreateWorktree)
}

func newRootCommand(getwd func() (string, error), inspect directoryInspector, create worktreeCreator) *cobra.Command {
	command := &cobra.Command{
		Use:   "virga",
		Short: "Multi-branch development manager",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}
	command.AddCommand(newInfoCmd(getwd, inspect), newWorktreeCmd(getwd, create))

	return command
}

// Execute runs the Virga command.
func Execute() error {
	return NewRootCommand().Execute()
}

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
	command := &cobra.Command{
		Use:   "virga",
		Short: "Multi-branch development manager",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}
	command.AddCommand(newInfoCmd(os.Getwd, git.InspectWorktree))

	return command
}

// Execute runs the Virga command.
func Execute() error {
	return NewRootCommand().Execute()
}

/*
Copyright © 2026 Andy Holt <andrew.holt@hotmail.co.uk>
*/
package cmd

import (
	"os"

	"github.com/AndyHolt/virga/internal/config"
	"github.com/AndyHolt/virga/internal/git"
	"github.com/AndyHolt/virga/internal/tmux"
	"github.com/spf13/cobra"
)

// NewRootCommand constructs the root Virga command.
func NewRootCommand() *cobra.Command {
	configurationLoader := config.NewLoader(config.LoaderDependencies{})
	return newRootCommand(
		os.Getwd,
		git.InspectWorktree,
		git.CreateWorktree,
		newWorktreeOptions{
			inspect:           git.InspectWorktree,
			listBranches:      git.ListLocalBranches,
			isInteractive:     isInteractiveTerminal,
			selectBranch:      selectHuhBranch,
			loadConfiguration: configurationLoader.Load,
			createSession:     tmux.CreateSession,
			attachSession:     tmux.AttachSession,
		},
	)
}

func newRootCommand(
	getwd func() (string, error),
	inspect directoryInspector,
	create worktreeCreator,
	worktreeOptions newWorktreeOptions,
) *cobra.Command {
	command := &cobra.Command{
		Use:   "virga",
		Short: "Multi-branch development manager",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}
	command.AddCommand(newInfoCmd(getwd, inspect), newWorktreeCmd(getwd, create, worktreeOptions))

	return command
}

// Execute runs the Virga command.
func Execute() error {
	return NewRootCommand().Execute()
}

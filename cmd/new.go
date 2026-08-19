package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/AndyHolt/virga/internal/config"
	"github.com/AndyHolt/virga/internal/git"
	"github.com/AndyHolt/virga/internal/tmux"
	"github.com/spf13/cobra"
)

type worktreeCreator func(context.Context, string, string, string) (string, error)
type configurationLoader func(context.Context, string, string) (config.Config, error)
type localBranchLister func(context.Context, string) ([]string, error)
type terminalDetector func() bool
type tmuxSessionCreator func(context.Context, tmux.CreateSessionOptions) (string, error)
type tmuxSessionAttacher func(context.Context, string) error

type newWorktreeOptions struct {
	inspect           directoryInspector
	listBranches      localBranchLister
	isInteractive     terminalDetector
	selectBranch      branchSelector
	loadConfiguration configurationLoader
	createSession     tmuxSessionCreator
	attachSession     tmuxSessionAttacher
}

func newWorktreeCmd(getwd func() (string, error), create worktreeCreator, options newWorktreeOptions) *cobra.Command {
	var baseBranch string
	var configPath string
	var useMain bool
	var pickBase bool
	var noTmux bool
	var noAttach bool

	command := &cobra.Command{
		Use:   "new <branch>",
		Short: "Create a branch in a new Git worktree",
		Example: "  virga new feature/login\n" +
			"  virga new feature/login --from release\n" +
			"  virga new feature/login --main\n" +
			"  virga new feature/login --pick\n" +
			"  virga new feature/login --no-tmux",
		Args: cobra.ExactArgs(1),
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

			setupTmux := !noTmux && options.createSession != nil
			var configuration config.Config
			var repositoryRoot string
			if setupTmux {
				if options.loadConfiguration == nil || options.inspect == nil {
					return fmt.Errorf("tmux setup is unavailable")
				}

				configuration, err = options.loadConfiguration(cmd.Context(), directory, configPath)
				if err != nil {
					return newWorktreeCommandError(cmd, "load configuration", err)
				}

				info, err := options.inspect(cmd.Context(), directory)
				if err != nil {
					return fmt.Errorf("inspect current worktree: %w", err)
				}
				if info.Kind == git.NotWorktree {
					return newWorktreeCommandError(cmd, "inspect current worktree", git.ErrNotGitRepository)
				}
				repositoryRoot = info.MainWorktreeRoot
				if repositoryRoot == "" {
					return fmt.Errorf("inspect current worktree: Git returned an empty primary worktree root")
				}
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

			output := fmt.Sprintf("Branch: %s\nWorktree: %s\n", args[0], worktree)
			var sessionName string
			if setupTmux {
				sessionName, err = options.createSession(cmd.Context(), tmux.CreateSessionOptions{
					RepositoryRoot: repositoryRoot,
					Branch:         args[0],
					WorktreeRoot:   worktree,
					Tmux:           configuration.Tmux,
				})
				if err != nil {
					return fmt.Errorf("created branch %q and worktree %q, but create tmux session: %w", args[0], worktree, err)
				}
				output += fmt.Sprintf("Tmux session: %s\n", sessionName)
			}

			if _, err := fmt.Fprint(cmd.OutOrStdout(), output); err != nil {
				return fmt.Errorf("write worktree result: %w", err)
			}

			if setupTmux && !noAttach && options.attachSession != nil && options.isInteractive != nil && options.isInteractive() {
				if err := options.attachSession(cmd.Context(), sessionName); err != nil {
					return fmt.Errorf("created branch %q, worktree %q, and tmux session %q, but attach tmux session: %w", args[0], worktree, sessionName, err)
				}
			}
			return nil
		},
	}
	command.Flags().StringVar(&baseBranch, "from", "", "Create from a named local branch")
	command.Flags().StringVar(&configPath, "config", "", "Load configuration from a file")
	command.Flags().BoolVar(&useMain, "main", false, "Create from the local main branch")
	command.Flags().BoolVar(&pickBase, "pick", false, "Interactively select a local base branch")
	command.Flags().BoolVar(&noTmux, "no-tmux", false, "Do not create a tmux session")
	command.Flags().BoolVar(&noAttach, "no-attach", false, "Create tmux session without attaching")
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

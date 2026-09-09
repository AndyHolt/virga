package cmd

import (
	"context"
	"errors"
	"fmt"

	"github.com/AndyHolt/virga/internal/config"
	"github.com/AndyHolt/virga/internal/files"
	"github.com/AndyHolt/virga/internal/git"
	"github.com/AndyHolt/virga/internal/tmux"
	"github.com/spf13/cobra"
)

type tmuxSessionEnsurer func(context.Context, tmux.CreateSessionOptions) (tmux.EnsureSessionResult, error)
type existingBranchWorktreeAdder func(context.Context, string, string) (string, error)

type openWorktreeOptions struct {
	inspect           directoryInspector
	listBranches      localBranchLister
	listWorktrees     worktreeLister
	addWorktree       existingBranchWorktreeAdder
	isInteractive     terminalDetector
	loadConfiguration configurationLoader
	materializeFiles  fileMaterializer
	ensureSession     tmuxSessionEnsurer
	attachSession     tmuxSessionAttacher
}

func newOpenCmd(getwd func() (string, error), options openWorktreeOptions) *cobra.Command {
	var configPath string
	var noTmux bool
	var noAttach bool

	command := &cobra.Command{
		Use:   "open <branch>",
		Short: "Open an existing branch worktree",
		Example: "  virga open feature/login\n" +
			"  virga open feature/login --no-attach\n" +
			"  virga open feature/login --no-tmux",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			branch := args[0]
			directory, err := getwd()
			if err != nil {
				return fmt.Errorf("get current directory: %w", err)
			}
			if options.listBranches == nil {
				return fmt.Errorf("open worktree: local branch listing is unavailable")
			}

			branches, err := options.listBranches(cmd.Context(), directory)
			if err != nil {
				return openWorktreeCommandError(cmd, "list local branches", err)
			}
			if !branchExists(branches, branch) {
				return fmt.Errorf("local branch %q does not exist", branch)
			}

			if options.listWorktrees == nil {
				return fmt.Errorf("open worktree: worktree listing is unavailable")
			}

			worktrees, err := options.listWorktrees(cmd.Context(), directory)
			if err != nil {
				return openWorktreeCommandError(cmd, "list worktrees", err)
			}
			worktree, found := reusableWorktreeForBranch(worktrees, branch)
			worktreeAction := "reused"
			setupTmux := !noTmux
			createdWorktree := !found
			materializeFiles := createdWorktree && options.materializeFiles != nil
			needsConfiguration := setupTmux || materializeFiles

			if createdWorktree {
				if options.addWorktree == nil {
					return fmt.Errorf("local branch %q exists but has no usable worktree and worktree creation is unavailable", branch)
				}
			}
			if setupTmux && options.ensureSession == nil {
				return fmt.Errorf("open worktree: tmux setup is unavailable")
			}

			var configuration config.Config
			var repositoryRoot string
			if needsConfiguration {
				if options.loadConfiguration == nil || options.inspect == nil {
					return fmt.Errorf("configuration setup is unavailable")
				}

				configuration, err = options.loadConfiguration(cmd.Context(), directory, configPath)
				if err != nil {
					return openWorktreeCommandError(cmd, "load configuration", err)
				}
				info, err := options.inspect(cmd.Context(), directory)
				if err != nil {
					return fmt.Errorf("inspect current worktree: %w", err)
				}
				if info.Kind == git.NotWorktree {
					return openWorktreeCommandError(cmd, "inspect current worktree", git.ErrNotGitRepository)
				}
				repositoryRoot = info.MainWorktreeRoot
				if repositoryRoot == "" {
					return fmt.Errorf("inspect current worktree: Git returned an empty primary worktree root")
				}
			}

			if createdWorktree {
				path, err := options.addWorktree(cmd.Context(), directory, branch)
				if err != nil {
					return openWorktreeCommandError(cmd, "create worktree", err)
				}
				worktree = git.ListedWorktree{Path: path, Kind: git.LinkedWorktree, Branch: branch, BranchRef: "refs/heads/" + branch}
				worktreeAction = "created"
			}

			output := fmt.Sprintf("Branch: %s\nWorktree: %s\nWorktree action: %s\n", branch, worktree.Path, worktreeAction)
			if materializeFiles && len(configuration.Files) > 0 {
				if err := options.materializeFiles(cmd.Context(), files.Options{
					RepositoryRoot: repositoryRoot,
					WorktreeRoot:   worktree.Path,
					Entries:        configuration.Files,
				}); err != nil {
					return fmt.Errorf("created worktree for branch %q at %q, but materialize files: %w", branch, worktree.Path, err)
				}
			}

			var session tmux.EnsureSessionResult
			if setupTmux {
				session, err = options.ensureSession(cmd.Context(), tmux.CreateSessionOptions{
					RepositoryRoot: repositoryRoot,
					Branch:         branch,
					WorktreeRoot:   worktree.Path,
					Tmux:           configuration.Tmux,
				})
				if err != nil {
					if createdWorktree {
						return fmt.Errorf("created worktree for branch %q at %q, but ensure tmux session: %w", branch, worktree.Path, err)
					}
					return fmt.Errorf("open branch %q in worktree %q, but ensure tmux session: %w", branch, worktree.Path, err)
				}
				output += fmt.Sprintf("Tmux session: %s\nTmux action: %s\n", session.Name, session.Action)
			}

			if _, err := fmt.Fprint(cmd.OutOrStdout(), output); err != nil {
				return fmt.Errorf("write open result: %w", err)
			}

			if setupTmux && !noAttach && options.attachSession != nil && options.isInteractive != nil && options.isInteractive() {
				if err := options.attachSession(cmd.Context(), session.Name); err != nil {
					return fmt.Errorf("opened branch %q in worktree %q and tmux session %q, but attach tmux session: %w", branch, worktree.Path, session.Name, err)
				}
			}
			return nil
		},
	}
	command.Flags().StringVar(&configPath, "config", "", "Load configuration from a file")
	command.Flags().BoolVar(&noTmux, "no-tmux", false, "Do not create or reuse a tmux session")
	command.Flags().BoolVar(&noAttach, "no-attach", false, "Create or reuse tmux session without attaching")
	return command
}

func branchExists(branches []string, branch string) bool {
	for _, candidate := range branches {
		if candidate == branch {
			return true
		}
	}
	return false
}

func reusableWorktreeForBranch(worktrees []git.ListedWorktree, branch string) (git.ListedWorktree, bool) {
	branchRef := "refs/heads/" + branch
	for _, worktree := range worktrees {
		if worktree.BranchRef == branchRef && !worktree.Detached && !worktree.Bare && !worktree.Prunable {
			return worktree, true
		}
	}
	return git.ListedWorktree{}, false
}

func openWorktreeCommandError(cmd *cobra.Command, operation string, err error) error {
	if errors.Is(err, git.ErrNotGitRepository) {
		cmd.SilenceUsage = true
		return git.ErrNotGitRepository
	}
	return fmt.Errorf("%s: %w", operation, err)
}

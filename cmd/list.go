package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/AndyHolt/virga/internal/git"
	"github.com/AndyHolt/virga/internal/tmux"
	"github.com/spf13/cobra"
)

type worktreeLister func(context.Context, string) ([]git.ListedWorktree, error)
type tmuxSessionChecker func(context.Context, string) (bool, error)

type listWorktreeOptions struct {
	listWorktrees worktreeLister
	hasSession    tmuxSessionChecker
}

type listOutput struct {
	Worktrees []listedWorktreeOutput `json:"worktrees"`
}

type listedWorktreeOutput struct {
	Path        string           `json:"path"`
	Type        git.WorktreeKind `json:"type"`
	HEAD        string           `json:"head"`
	Branch      string           `json:"branch"`
	BranchRef   string           `json:"branch_ref"`
	State       []string         `json:"state"`
	Detached    bool             `json:"detached"`
	Bare        bool             `json:"bare"`
	Locked      bool             `json:"locked"`
	LockReason  string           `json:"lock_reason"`
	Prunable    bool             `json:"prunable"`
	PruneReason string           `json:"prune_reason"`
	Tmux        listTmuxOutput   `json:"tmux"`
}

type listTmuxOutput struct {
	Session string `json:"session"`
	Status  string `json:"status"`
}

func newListCmd(getwd func() (string, error), options listWorktreeOptions) *cobra.Command {
	var jsonOutput bool

	command := &cobra.Command{
		Use:     "list",
		Short:   "List Git worktrees and Virga tmux sessions",
		Example: "  virga list\n  virga list --json",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			directory, err := getwd()
			if err != nil {
				return fmt.Errorf("get current directory: %w", err)
			}
			if options.listWorktrees == nil {
				return fmt.Errorf("list worktrees: worktree listing is unavailable")
			}

			worktrees, err := options.listWorktrees(cmd.Context(), directory)
			if err != nil {
				if errors.Is(err, git.ErrNotGitRepository) {
					cmd.SilenceUsage = true
					return git.ErrNotGitRepository
				}
				return fmt.Errorf("list worktrees: %w", err)
			}

			output := buildListOutput(cmd.Context(), worktrees, options.hasSession)
			if jsonOutput {
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")
				if err := encoder.Encode(output); err != nil {
					return fmt.Errorf("write worktree list JSON: %w", err)
				}
				return nil
			}

			if _, err := fmt.Fprint(cmd.OutOrStdout(), formatListOutput(output)); err != nil {
				return fmt.Errorf("write worktree list: %w", err)
			}
			return nil
		},
	}
	command.Flags().BoolVar(&jsonOutput, "json", false, "Print worktrees as JSON")
	return command
}

func buildListOutput(ctx context.Context, worktrees []git.ListedWorktree, hasSession tmuxSessionChecker) listOutput {
	output := listOutput{Worktrees: make([]listedWorktreeOutput, len(worktrees))}
	for index, worktree := range worktrees {
		entry := listedWorktreeOutput{
			Path:        worktree.Path,
			Type:        worktree.Kind,
			HEAD:        worktree.HEAD,
			Branch:      worktree.Branch,
			BranchRef:   worktree.BranchRef,
			State:       worktreeState(worktree),
			Detached:    worktree.Detached,
			Bare:        worktree.Bare,
			Locked:      worktree.Locked,
			LockReason:  worktree.LockReason,
			Prunable:    worktree.Prunable,
			PruneReason: worktree.PruneReason,
			Tmux:        tmuxStatus(ctx, worktree, hasSession),
		}
		output.Worktrees[index] = entry
	}
	return output
}

func worktreeState(worktree git.ListedWorktree) []string {
	var states []string
	if worktree.Detached {
		states = append(states, "detached")
	}
	if worktree.Bare {
		states = append(states, "bare")
	}
	if worktree.Locked {
		state := "locked"
		if worktree.LockReason != "" {
			state += ": " + worktree.LockReason
		}
		states = append(states, state)
	}
	if worktree.Prunable {
		state := "prunable"
		if worktree.PruneReason != "" {
			state += ": " + worktree.PruneReason
		}
		states = append(states, state)
	}
	if len(states) == 0 {
		states = append(states, "normal")
	}
	return states
}

func tmuxStatus(ctx context.Context, worktree git.ListedWorktree, hasSession tmuxSessionChecker) listTmuxOutput {
	if worktree.Branch == "" {
		return listTmuxOutput{Status: "none"}
	}

	session := tmux.SessionName(worktree.Path)
	if hasSession == nil {
		return listTmuxOutput{Session: session, Status: "unknown"}
	}

	exists, err := hasSession(ctx, session)
	if err != nil {
		if errors.Is(err, tmux.ErrNotInstalled) {
			return listTmuxOutput{Session: session, Status: "unavailable"}
		}
		return listTmuxOutput{Session: session, Status: "unknown"}
	}
	if exists {
		return listTmuxOutput{Session: session, Status: "running"}
	}
	return listTmuxOutput{Session: session, Status: "missing"}
}

func formatListOutput(output listOutput) string {
	var builder strings.Builder
	for index, worktree := range output.Worktrees {
		if index > 0 {
			builder.WriteByte('\n')
		}
		builder.WriteString("Branch: ")
		builder.WriteString(displayBranch(worktree))
		builder.WriteByte('\n')
		builder.WriteString("Worktree: ")
		builder.WriteString(worktree.Path)
		builder.WriteByte('\n')
		builder.WriteString("Type: ")
		builder.WriteString(displayWorktreeKind(worktree.Type))
		builder.WriteByte('\n')
		builder.WriteString("State: ")
		builder.WriteString(strings.Join(worktree.State, ", "))
		builder.WriteByte('\n')
		builder.WriteString("Tmux session: ")
		builder.WriteString(displayTmux(worktree.Tmux))
		builder.WriteByte('\n')
	}
	return builder.String()
}

func displayBranch(worktree listedWorktreeOutput) string {
	if worktree.Branch != "" {
		return worktree.Branch
	}
	if worktree.Detached {
		return "(detached)"
	}
	return "(none)"
}

func displayWorktreeKind(kind git.WorktreeKind) string {
	switch kind {
	case git.MainWorktree:
		return "main Git worktree"
	case git.LinkedWorktree:
		return "linked Git worktree"
	default:
		return string(kind)
	}
}

func displayTmux(info listTmuxOutput) string {
	if info.Status == "none" {
		return "(none)"
	}
	if info.Session == "" {
		return "(" + info.Status + ")"
	}
	return fmt.Sprintf("%s (%s)", info.Session, info.Status)
}

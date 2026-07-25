package cmd

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/AndyHolt/virga/internal/git"
)

func TestInfoCommand(t *testing.T) {
	tests := []struct {
		name string
		info git.WorktreeInfo
		want string
	}{
		{
			name: "main worktree",
			info: git.WorktreeInfo{
				Directory:        "/repo/nested",
				Kind:             git.MainWorktree,
				WorktreeRoot:     "/repo",
				MainWorktreeRoot: "/repo",
			},
			want: "Directory: /repo/nested\nType: main Git worktree\nWorktree root: /repo\n",
		},
		{
			name: "linked worktree",
			info: git.WorktreeInfo{
				Directory:        "/worktrees/feature",
				Kind:             git.LinkedWorktree,
				WorktreeRoot:     "/worktrees/feature",
				MainWorktreeRoot: "/repo",
			},
			want: "Directory: /worktrees/feature\nType: linked Git worktree\nWorktree root: /worktrees/feature\nMain worktree: /repo\n",
		},
		{
			name: "not a worktree",
			info: git.WorktreeInfo{Directory: "/tmp", Kind: git.NotWorktree},
			want: "Directory: /tmp\nType: not a Git worktree\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			command := newInfoCmd(
				func() (string, error) { return "/invocation", nil },
				func(_ context.Context, directory string) (git.WorktreeInfo, error) {
					if directory != "/invocation" {
						t.Errorf("inspect directory = %q, want /invocation", directory)
					}
					return test.info, nil
				},
			)
			command.SetOut(&output)

			if err := command.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if got := output.String(); got != test.want {
				t.Errorf("output = %q, want %q", got, test.want)
			}
		})
	}
}

func TestInfoCommandErrors(t *testing.T) {
	getwdErr := errors.New("getwd failed")
	command := newInfoCmd(
		func() (string, error) { return "", getwdErr },
		func(context.Context, string) (git.WorktreeInfo, error) {
			t.Fatal("inspector called after getwd failure")
			return git.WorktreeInfo{}, nil
		},
	)
	if err := command.Execute(); !errors.Is(err, getwdErr) {
		t.Fatalf("Execute() error = %v, want wrapped getwd error", err)
	}

	inspectErr := errors.New("inspection failed")
	command = newInfoCmd(
		func() (string, error) { return "/repo", nil },
		func(context.Context, string) (git.WorktreeInfo, error) { return git.WorktreeInfo{}, inspectErr },
	)
	if err := command.Execute(); !errors.Is(err, inspectErr) {
		t.Fatalf("Execute() error = %v, want wrapped inspection error", err)
	}
}

func TestInfoCommandRejectsArguments(t *testing.T) {
	command := newInfoCmd(
		func() (string, error) { return "/repo", nil },
		func(context.Context, string) (git.WorktreeInfo, error) { return git.WorktreeInfo{}, nil },
	)
	command.SetArgs([]string{"unexpected"})

	if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "unknown command") && !strings.Contains(err.Error(), "accepts 0 arg") {
		t.Fatalf("Execute() error = %v, want argument error", err)
	}
}

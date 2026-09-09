package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/AndyHolt/virga/internal/git"
	"github.com/AndyHolt/virga/internal/tmux"
)

func TestListCommandPrintsWorktreesAndTmuxSessions(t *testing.T) {
	var output bytes.Buffer
	var checkedSessions []string
	command := newListCmd(
		func() (string, error) { return "/repo/nested", nil },
		listWorktreeOptions{
			listWorktrees: func(_ context.Context, directory string) ([]git.ListedWorktree, error) {
				if directory != "/repo/nested" {
					t.Errorf("directory = %q, want /repo/nested", directory)
				}
				return []git.ListedWorktree{
					{
						Path:      "/repo",
						Kind:      git.MainWorktree,
						HEAD:      "1111111111111111111111111111111111111111",
						Branch:    "main",
						BranchRef: "refs/heads/main",
					},
					{
						Path:        "/repo_feature",
						Kind:        git.LinkedWorktree,
						HEAD:        "2222222222222222222222222222222222222222",
						Branch:      "feature/login",
						BranchRef:   "refs/heads/feature/login",
						Locked:      true,
						LockReason:  "working on review",
						Prunable:    true,
						PruneReason: "gitdir file points to non-existent location",
					},
					{
						Path:     "/repo_detached",
						Kind:     git.LinkedWorktree,
						HEAD:     "3333333333333333333333333333333333333333",
						Detached: true,
					},
				}, nil
			},
			hasSession: func(_ context.Context, session string) (bool, error) {
				checkedSessions = append(checkedSessions, session)
				return session == "repo", nil
			},
		},
	)
	command.SetOut(&output)

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	want := "Branch: main\n" +
		"Worktree: /repo\n" +
		"Type: main Git worktree\n" +
		"State: normal\n" +
		"Tmux session: repo (running)\n" +
		"\n" +
		"Branch: feature/login\n" +
		"Worktree: /repo_feature\n" +
		"Type: linked Git worktree\n" +
		"State: locked: working on review, prunable: gitdir file points to non-existent location\n" +
		"Tmux session: repo_feature (missing)\n" +
		"\n" +
		"Branch: (detached)\n" +
		"Worktree: /repo_detached\n" +
		"Type: linked Git worktree\n" +
		"State: detached\n" +
		"Tmux session: (none)\n"
	if got := output.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if want := []string{"repo", "repo_feature"}; !reflect.DeepEqual(checkedSessions, want) {
		t.Fatalf("checked sessions = %#v, want %#v", checkedSessions, want)
	}
}

func TestListCommandPrintsJSON(t *testing.T) {
	var output bytes.Buffer
	command := newListCmd(
		func() (string, error) { return "/repo", nil },
		listWorktreeOptions{
			listWorktrees: func(context.Context, string) ([]git.ListedWorktree, error) {
				return []git.ListedWorktree{{
					Path:      "/repo",
					Kind:      git.MainWorktree,
					HEAD:      "1111111111111111111111111111111111111111",
					Branch:    "main",
					BranchRef: "refs/heads/main",
				}}, nil
			},
			hasSession: func(context.Context, string) (bool, error) { return false, nil },
		},
	)
	command.SetOut(&output)
	command.SetArgs([]string{"--json"})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var got listOutput
	if err := json.Unmarshal(output.Bytes(), &got); err != nil {
		t.Fatalf("decode JSON %q: %v", output.String(), err)
	}
	want := listOutput{Worktrees: []listedWorktreeOutput{{
		Path:      "/repo",
		Type:      git.MainWorktree,
		HEAD:      "1111111111111111111111111111111111111111",
		Branch:    "main",
		BranchRef: "refs/heads/main",
		State:     []string{"normal"},
		Tmux: listTmuxOutput{
			Session: "repo",
			Status:  "missing",
		},
	}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("JSON = %#v, want %#v", got, want)
	}
	if !strings.Contains(output.String(), "\n  \"worktrees\":") {
		t.Fatalf("JSON output is not indented: %q", output.String())
	}
}

func TestListCommandReportsTmuxUnavailableWithoutFailing(t *testing.T) {
	var output bytes.Buffer
	command := newListCmd(
		func() (string, error) { return "/repo", nil },
		listWorktreeOptions{
			listWorktrees: func(context.Context, string) ([]git.ListedWorktree, error) {
				return []git.ListedWorktree{{Path: "/repo", Kind: git.MainWorktree, Branch: "main", BranchRef: "refs/heads/main"}}, nil
			},
			hasSession: func(context.Context, string) (bool, error) {
				return false, tmux.ErrNotInstalled
			},
		},
	)
	command.SetOut(&output)

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := output.String(); !strings.Contains(got, "Tmux session: repo (unavailable)") {
		t.Fatalf("output = %q, want unavailable tmux status", got)
	}
}

func TestListCommandReportsTmuxUnknownWithoutFailing(t *testing.T) {
	var output bytes.Buffer
	command := newListCmd(
		func() (string, error) { return "/repo", nil },
		listWorktreeOptions{
			listWorktrees: func(context.Context, string) ([]git.ListedWorktree, error) {
				return []git.ListedWorktree{{Path: "/repo", Kind: git.MainWorktree, Branch: "main", BranchRef: "refs/heads/main"}}, nil
			},
			hasSession: func(context.Context, string) (bool, error) {
				return false, errors.New("tmux server failed")
			},
		},
	)
	command.SetOut(&output)

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := output.String(); !strings.Contains(got, "Tmux session: repo (unknown)") {
		t.Fatalf("output = %q, want unknown tmux status", got)
	}
}

func TestListCommandErrors(t *testing.T) {
	getwdErr := errors.New("getwd failed")
	command := newListCmd(
		func() (string, error) { return "", getwdErr },
		listWorktreeOptions{
			listWorktrees: func(context.Context, string) ([]git.ListedWorktree, error) {
				t.Fatal("lister called after getwd failure")
				return nil, nil
			},
		},
	)
	if err := command.Execute(); !errors.Is(err, getwdErr) {
		t.Fatalf("Execute() error = %v, want wrapped getwd error", err)
	}

	listErr := errors.New("list failed")
	command = newListCmd(
		func() (string, error) { return "/repo", nil },
		listWorktreeOptions{
			listWorktrees: func(context.Context, string) ([]git.ListedWorktree, error) { return nil, listErr },
		},
	)
	if err := command.Execute(); !errors.Is(err, listErr) {
		t.Fatalf("Execute() error = %v, want wrapped list error", err)
	}
}

func TestListCommandReportsNonGitRepository(t *testing.T) {
	command := newListCmd(
		func() (string, error) { return "/tmp", nil },
		listWorktreeOptions{
			listWorktrees: func(context.Context, string) ([]git.ListedWorktree, error) {
				return nil, git.ErrNotGitRepository
			},
		},
	)

	err := command.Execute()
	if !errors.Is(err, git.ErrNotGitRepository) {
		t.Fatalf("Execute() error = %v, want %v", err, git.ErrNotGitRepository)
	}
	if !command.SilenceUsage {
		t.Fatal("SilenceUsage = false, want true for not-a-repository error")
	}
}

func TestListCommandRejectsArguments(t *testing.T) {
	command := newListCmd(
		func() (string, error) { return "/repo", nil },
		listWorktreeOptions{
			listWorktrees: func(context.Context, string) ([]git.ListedWorktree, error) {
				t.Fatal("lister called with invalid arguments")
				return nil, nil
			},
		},
	)
	command.SetArgs([]string{"unexpected"})

	if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "unknown command") && !strings.Contains(err.Error(), "accepts 0 arg") {
		t.Fatalf("Execute() error = %v, want argument error", err)
	}
}

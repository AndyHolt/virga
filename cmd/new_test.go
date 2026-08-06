package cmd

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func TestNewWorktreeCommandSelectsBaseBranch(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantBase string
	}{
		{
			name:     "current branch by default",
			args:     []string{"feature/login"},
			wantBase: "",
		},
		{
			name:     "named local branch",
			args:     []string{"feature/login", "--from", "release"},
			wantBase: "release",
		},
		{
			name:     "local main branch",
			args:     []string{"feature/login", "--main"},
			wantBase: "main",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			command := newWorktreeCmd(
				func() (string, error) { return "/repo/nested", nil },
				func(_ context.Context, directory, branch, base string) (string, error) {
					if directory != "/repo/nested" {
						t.Errorf("directory = %q, want /repo/nested", directory)
					}
					if branch != "feature/login" {
						t.Errorf("branch = %q, want feature/login", branch)
					}
					if base != test.wantBase {
						t.Errorf("base = %q, want %q", base, test.wantBase)
					}
					return "/worktrees/repo_feature-login", nil
				},
			)
			command.SetOut(&output)
			command.SetArgs(test.args)

			if err := command.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if got, want := output.String(), "Branch: feature/login\nWorktree: /worktrees/repo_feature-login\n"; got != want {
				t.Errorf("output = %q, want %q", got, want)
			}
		})
	}
}

func TestNewWorktreeCommandRejectsConflictingBaseFlags(t *testing.T) {
	command := newWorktreeCmd(
		func() (string, error) {
			t.Fatal("getwd called with conflicting base flags")
			return "", nil
		},
		func(context.Context, string, string, string) (string, error) {
			t.Fatal("creator called with conflicting base flags")
			return "", nil
		},
	)
	command.SetArgs([]string{"feature", "--main", "--from", "release"})

	if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "--main and --from are mutually exclusive") {
		t.Fatalf("Execute() error = %v, want mutually exclusive flag error", err)
	}
}

func TestNewWorktreeCommandErrors(t *testing.T) {
	getwdErr := errors.New("getwd failed")
	command := newWorktreeCmd(
		func() (string, error) { return "", getwdErr },
		func(context.Context, string, string, string) (string, error) {
			t.Fatal("creator called after getwd failure")
			return "", nil
		},
	)
	command.SetArgs([]string{"feature"})
	if err := command.Execute(); !errors.Is(err, getwdErr) {
		t.Fatalf("Execute() error = %v, want wrapped getwd error", err)
	}

	createErr := errors.New("creation failed")
	command = newWorktreeCmd(
		func() (string, error) { return "/repo", nil },
		func(context.Context, string, string, string) (string, error) { return "", createErr },
	)
	command.SetArgs([]string{"feature"})
	if err := command.Execute(); !errors.Is(err, createErr) {
		t.Fatalf("Execute() error = %v, want wrapped creation error", err)
	}
}

func TestNewWorktreeCommandRejectsInvalidArgumentCounts(t *testing.T) {
	for _, args := range [][]string{nil, {"feature", "unexpected"}} {
		command := newWorktreeCmd(
			func() (string, error) { return "/repo", nil },
			func(context.Context, string, string, string) (string, error) {
				t.Fatal("creator called with invalid arguments")
				return "", nil
			},
		)
		command.SetArgs(args)

		if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "accepts 1 arg(s), received") {
			t.Errorf("Execute(%q) error = %v, want argument error", args, err)
		}
	}
}

func TestNewCommandAtRoot(t *testing.T) {
	var output bytes.Buffer
	command := newRootCommand(
		func() (string, error) { return "/repo", nil },
		nil,
		func(_ context.Context, directory, branch, base string) (string, error) {
			if directory != "/repo" || branch != "feature" || base != "" {
				t.Errorf("create(%q, %q, %q), want create(/repo, feature, empty)", directory, branch, base)
			}
			return "/repo_feature", nil
		},
	)
	command.SetOut(&output)
	command.SetArgs([]string{"new", "feature"})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got, want := output.String(), "Branch: feature\nWorktree: /repo_feature\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

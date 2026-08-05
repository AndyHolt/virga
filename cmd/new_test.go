package cmd

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func TestNewWorktreeCommand(t *testing.T) {
	var output bytes.Buffer
	command := newWorktreeCmd(
		func() (string, error) { return "/repo/nested", nil },
		func(_ context.Context, directory, branch string) (string, error) {
			if directory != "/repo/nested" {
				t.Errorf("directory = %q, want /repo/nested", directory)
			}
			if branch != "feature/login" {
				t.Errorf("branch = %q, want feature/login", branch)
			}
			return "/worktrees/repo_feature-login", nil
		},
	)
	command.SetOut(&output)
	command.SetArgs([]string{"feature/login"})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got, want := output.String(), "Branch: feature/login\nWorktree: /worktrees/repo_feature-login\n"; got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

func TestNewWorktreeCommandErrors(t *testing.T) {
	getwdErr := errors.New("getwd failed")
	command := newWorktreeCmd(
		func() (string, error) { return "", getwdErr },
		func(context.Context, string, string) (string, error) {
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
		func(context.Context, string, string) (string, error) { return "", createErr },
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
			func(context.Context, string, string) (string, error) {
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
		func(_ context.Context, directory, branch string) (string, error) {
			if directory != "/repo" || branch != "feature" {
				t.Errorf("create(%q, %q), want create(/repo, feature)", directory, branch)
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

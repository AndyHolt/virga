package cmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
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
				newWorktreeOptions{},
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

func TestNewWorktreeCommandPicksBaseBranch(t *testing.T) {
	var output, prompt bytes.Buffer
	command := newWorktreeCmd(
		func() (string, error) { return "/repo/nested", nil },
		func(_ context.Context, directory, branch, base string) (string, error) {
			if directory != "/repo/nested" || branch != "feature" || base != "release" {
				t.Errorf("create(%q, %q, %q), want create(/repo/nested, feature, release)", directory, branch, base)
			}
			return "/worktrees/repo_feature", nil
		},
		newWorktreeOptions{
			listBranches: func(_ context.Context, directory string) ([]string, error) {
				if directory != "/repo/nested" {
					t.Errorf("list directory = %q, want /repo/nested", directory)
				}
				return []string{"main", "release"}, nil
			},
			isInteractive: func() bool { return true },
			selectBranch: func(_ io.Reader, writer io.Writer, branches []string) (string, error) {
				if got, want := strings.Join(branches, ","), "main,release"; got != want {
					t.Errorf("branches = %q, want %q", got, want)
				}
				if _, err := fmt.Fprint(writer, "Select a base branch:\n"); err != nil {
					t.Fatalf("write prompt: %v", err)
				}
				return "release", nil
			},
		},
	)
	command.SetOut(&output)
	command.SetErr(&prompt)
	command.SetArgs([]string{"feature", "--pick"})

	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got, want := output.String(), "Branch: feature\nWorktree: /worktrees/repo_feature\n"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
	if got, want := prompt.String(), "Select a base branch:\n"; got != want {
		t.Errorf("prompt = %q, want %q", got, want)
	}
}

func TestNewWorktreeCommandReturnsPickErrors(t *testing.T) {
	listErr := errors.New("list branches failed")
	selectErr := errors.New("selection failed")
	tests := []struct {
		name    string
		options newWorktreeOptions
		wantErr error
	}{
		{
			name: "list branches",
			options: newWorktreeOptions{
				listBranches:  func(context.Context, string) ([]string, error) { return nil, listErr },
				isInteractive: func() bool { return true },
				selectBranch: func(io.Reader, io.Writer, []string) (string, error) {
					t.Fatal("selector called after branch listing failure")
					return "", nil
				},
			},
			wantErr: listErr,
		},
		{
			name: "select branch",
			options: newWorktreeOptions{
				listBranches:  func(context.Context, string) ([]string, error) { return []string{"main"}, nil },
				isInteractive: func() bool { return true },
				selectBranch: func(io.Reader, io.Writer, []string) (string, error) {
					return "", selectErr
				},
			},
			wantErr: selectErr,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command := newWorktreeCmd(
				func() (string, error) { return "/repo", nil },
				func(context.Context, string, string, string) (string, error) {
					t.Fatal("creator called after pick failure")
					return "", nil
				},
				test.options,
			)
			command.SetArgs([]string{"feature", "--pick"})

			if err := command.Execute(); !errors.Is(err, test.wantErr) {
				t.Fatalf("Execute() error = %v, want wrapped %v", err, test.wantErr)
			}
		})
	}
}

func TestNewWorktreeCommandRejectsNonInteractivePick(t *testing.T) {
	command := newWorktreeCmd(
		func() (string, error) {
			t.Fatal("getwd called without an interactive terminal")
			return "", nil
		},
		func(context.Context, string, string, string) (string, error) {
			t.Fatal("creator called without an interactive terminal")
			return "", nil
		},
		newWorktreeOptions{
			listBranches: func(context.Context, string) ([]string, error) {
				t.Fatal("branch lister called without an interactive terminal")
				return nil, nil
			},
			isInteractive: func() bool { return false },
			selectBranch: func(io.Reader, io.Writer, []string) (string, error) {
				t.Fatal("selector called without an interactive terminal")
				return "", nil
			},
		},
	)
	command.SetArgs([]string{"feature", "--pick"})

	if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "--pick requires an interactive terminal") {
		t.Fatalf("Execute() error = %v, want non-interactive terminal error", err)
	}
}

func TestNewWorktreeCommandRejectsConflictingBaseFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{
			name: "main and from",
			args: []string{"feature", "--main", "--from", "release"},
		},
		{
			name: "main and pick",
			args: []string{"feature", "--main", "--pick"},
		},
		{
			name: "from and pick",
			args: []string{"feature", "--from", "release", "--pick"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command := newWorktreeCmd(
				func() (string, error) {
					t.Fatal("getwd called with conflicting base flags")
					return "", nil
				},
				func(context.Context, string, string, string) (string, error) {
					t.Fatal("creator called with conflicting base flags")
					return "", nil
				},
				newWorktreeOptions{},
			)
			command.SetArgs(test.args)

			if err := command.Execute(); err == nil || !strings.Contains(err.Error(), "--main, --from, and --pick are mutually exclusive") {
				t.Fatalf("Execute() error = %v, want mutually exclusive flag error", err)
			}
		})
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
		newWorktreeOptions{},
	)
	command.SetArgs([]string{"feature"})
	if err := command.Execute(); !errors.Is(err, getwdErr) {
		t.Fatalf("Execute() error = %v, want wrapped getwd error", err)
	}

	createErr := errors.New("creation failed")
	command = newWorktreeCmd(
		func() (string, error) { return "/repo", nil },
		func(context.Context, string, string, string) (string, error) { return "", createErr },
		newWorktreeOptions{},
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
			newWorktreeOptions{},
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
		newWorktreeOptions{},
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

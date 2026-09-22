package setup

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestRunExecutesCommandsInOrder(t *testing.T) {
	var calls []string
	var stdout, stderr bytes.Buffer
	err := run(context.Background(), Options{
		WorktreeRoot: "/worktree",
		Commands:     []string{"uv sync", "make test"},
		Stdout:       &stdout,
		Stderr:       &stderr,
	}, func(_ context.Context, directory, command string, gotStdout, gotStderr io.Writer) error {
		if directory != "/worktree" {
			t.Errorf("directory = %q, want /worktree", directory)
		}
		if gotStdout != &stdout || gotStderr != &stderr {
			t.Errorf("writers = (%T, %T), want supplied writers", gotStdout, gotStderr)
		}
		calls = append(calls, command)
		return nil
	})
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if want := []string{"uv sync", "make test"}; !reflect.DeepEqual(calls, want) {
		t.Errorf("commands = %#v, want %#v", calls, want)
	}
}

func TestRunExecutesShellCommandInWorktree(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses POSIX shell syntax")
	}

	worktree := t.TempDir()
	var stdout, stderr bytes.Buffer
	err := Run(context.Background(), Options{
		WorktreeRoot: worktree,
		Commands:     []string{"printf stdout; printf stderr >&2; printf complete > setup-result"},
		Stdout:       &stdout,
		Stderr:       &stderr,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := stdout.String(), "stdout"; got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
	if got, want := stderr.String(), "stderr"; got != want {
		t.Errorf("stderr = %q, want %q", got, want)
	}
	contents, err := os.ReadFile(filepath.Join(worktree, "setup-result"))
	if err != nil {
		t.Fatalf("read setup result: %v", err)
	}
	if got, want := string(contents), "complete"; got != want {
		t.Errorf("setup result = %q, want %q", got, want)
	}
}

func TestRunStopsAtFirstFailure(t *testing.T) {
	runErr := errors.New("command failed")
	var calls []string
	err := run(context.Background(), Options{
		WorktreeRoot: "/worktree",
		Commands:     []string{"first", "second"},
	}, func(_ context.Context, _ string, command string, _, _ io.Writer) error {
		calls = append(calls, command)
		return runErr
	})
	if !errors.Is(err, runErr) {
		t.Fatalf("run() error = %v, want wrapped %v", err, runErr)
	}
	if want := []string{"first"}; !reflect.DeepEqual(calls, want) {
		t.Errorf("commands = %#v, want %#v", calls, want)
	}
}

func TestRunRejectsEmptyWorktreeRoot(t *testing.T) {
	err := run(context.Background(), Options{}, func(context.Context, string, string, io.Writer, io.Writer) error {
		t.Fatal("executor called with an empty worktree root")
		return nil
	})
	if err == nil || err.Error() != "worktree root is required" {
		t.Fatalf("run() error = %v, want missing worktree root error", err)
	}
}

func TestRunHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := run(ctx, Options{WorktreeRoot: "/worktree", Commands: []string{"uv sync"}}, func(context.Context, string, string, io.Writer, io.Writer) error {
		t.Fatal("executor called with canceled context")
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("run() error = %v, want context cancellation", err)
	}
}

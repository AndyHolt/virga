package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestInspectWorktree(t *testing.T) {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not installed")
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("PATH", filepath.Dir(gitPath)+string(os.PathListSeparator)+os.Getenv("PATH"))

	mainRoot := filepath.Join(t.TempDir(), "main repo")
	runGit(t, "init", "--initial-branch=main", mainRoot)
	runGit(t, "-C", mainRoot, "config", "user.name", "Virga Test")
	runGit(t, "-C", mainRoot, "config", "user.email", "virga@example.invalid")
	runGit(t, "-C", mainRoot, "commit", "--allow-empty", "-m", "initial commit")

	linkedRoot := filepath.Join(t.TempDir(), "linked worktree")
	runGit(t, "-C", mainRoot, "worktree", "add", "-b", "feature", linkedRoot)

	mainRootCanonical, err := filepath.EvalSymlinks(mainRoot)
	if err != nil {
		t.Fatalf("resolve main worktree path: %v", err)
	}
	linkedRootCanonical, err := filepath.EvalSymlinks(linkedRoot)
	if err != nil {
		t.Fatalf("resolve linked worktree path: %v", err)
	}

	mainNested := filepath.Join(mainRoot, "nested", "directory")
	if err := os.MkdirAll(mainNested, 0o755); err != nil {
		t.Fatalf("create nested directory: %v", err)
	}

	tests := []struct {
		name     string
		dir      string
		wantKind WorktreeKind
		wantRoot string
		wantMain string
	}{
		{
			name:     "main worktree from nested directory",
			dir:      mainNested,
			wantKind: MainWorktree,
			wantRoot: mainRootCanonical,
			wantMain: mainRootCanonical,
		},
		{
			name:     "linked worktree",
			dir:      linkedRoot,
			wantKind: LinkedWorktree,
			wantRoot: linkedRootCanonical,
			wantMain: mainRootCanonical,
		},
		{
			name:     "outside worktree",
			dir:      home,
			wantKind: NotWorktree,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := InspectWorktree(context.Background(), test.dir)
			if err != nil {
				t.Fatalf("InspectWorktree() error = %v", err)
			}
			if got.Directory != test.dir {
				t.Errorf("Directory = %q, want %q", got.Directory, test.dir)
			}
			if got.Kind != test.wantKind {
				t.Errorf("Kind = %q, want %q", got.Kind, test.wantKind)
			}
			if got.WorktreeRoot != test.wantRoot {
				t.Errorf("WorktreeRoot = %q, want %q", got.WorktreeRoot, test.wantRoot)
			}
			if got.MainWorktreeRoot != test.wantMain {
				t.Errorf("MainWorktreeRoot = %q, want %q", got.MainWorktreeRoot, test.wantMain)
			}
		})
	}
}

func TestInspectWorktreeReturnsGitErrors(t *testing.T) {
	t.Run("git unavailable", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		if _, err := InspectWorktree(context.Background(), t.TempDir()); err == nil {
			t.Fatal("InspectWorktree() returned nil error without git on PATH")
		}
	})

	t.Run("directory missing", func(t *testing.T) {
		if _, err := InspectWorktree(context.Background(), filepath.Join(t.TempDir(), "missing")); err == nil {
			t.Fatal("InspectWorktree() returned nil error for a missing directory")
		}
	})
}

func TestFirstWorktreePathRejectsMalformedOutput(t *testing.T) {
	for _, output := range [][]byte{nil, []byte("worktree /repo"), []byte("HEAD abc\x00"), []byte("worktree \x00")} {
		if _, err := firstWorktreePath(output); err == nil {
			t.Errorf("firstWorktreePath(%q) returned nil error", output)
		}
	}
}

func runGit(t *testing.T, arguments ...string) {
	t.Helper()
	command := exec.Command("git", arguments...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", arguments, err, output)
	}
}

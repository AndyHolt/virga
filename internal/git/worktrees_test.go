package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestListWorktrees(t *testing.T) {
	mainRoot := newTestRepository(t, "main repository")
	mainCommit := gitOutput(t, "-C", mainRoot, "rev-parse", "HEAD")
	linkedRoot := filepath.Join(t.TempDir(), "feature worktree")
	runGit(t, "-C", mainRoot, "worktree", "add", "-b", "feature/login", linkedRoot)
	linkedRoot = canonicalPath(t, linkedRoot)
	linkedCommit := gitOutput(t, "-C", linkedRoot, "rev-parse", "HEAD")
	detachedRoot := filepath.Join(t.TempDir(), "detached worktree")
	runGit(t, "-C", mainRoot, "worktree", "add", "--detach", detachedRoot, "HEAD")
	detachedRoot = canonicalPath(t, detachedRoot)
	detachedCommit := gitOutput(t, "-C", detachedRoot, "rev-parse", "HEAD")

	worktrees, err := ListWorktrees(context.Background(), mainRoot)
	if err != nil {
		t.Fatalf("ListWorktrees() error = %v", err)
	}

	if got, want := len(worktrees), 3; got != want {
		t.Fatalf("worktree count = %d, want %d: %#v", got, want, worktrees)
	}
	assertListedWorktree(t, worktrees[0], ListedWorktree{
		Path:      mainRoot,
		Kind:      MainWorktree,
		HEAD:      mainCommit,
		Branch:    "main",
		BranchRef: "refs/heads/main",
	})

	byPath := worktreesByPath(worktrees[1:])
	assertListedWorktree(t, byPath[linkedRoot], ListedWorktree{
		Path:      linkedRoot,
		Kind:      LinkedWorktree,
		HEAD:      linkedCommit,
		Branch:    "feature/login",
		BranchRef: "refs/heads/feature/login",
	})
	assertListedWorktree(t, byPath[detachedRoot], ListedWorktree{
		Path:     detachedRoot,
		Kind:     LinkedWorktree,
		HEAD:     detachedCommit,
		Detached: true,
	})
}

func TestListWorktreesFromNestedLinkedWorktree(t *testing.T) {
	mainRoot := newTestRepository(t, "repository")
	linkedRoot := filepath.Join(t.TempDir(), "linked worktree")
	runGit(t, "-C", mainRoot, "worktree", "add", "-b", "feature", linkedRoot)
	linkedRoot = canonicalPath(t, linkedRoot)
	nested := filepath.Join(linkedRoot, "nested", "directory")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("create nested directory: %v", err)
	}

	worktrees, err := ListWorktrees(context.Background(), nested)
	if err != nil {
		t.Fatalf("ListWorktrees() error = %v", err)
	}
	if len(worktrees) != 2 {
		t.Fatalf("worktrees = %#v, want main and linked worktrees", worktrees)
	}
	if worktrees[0].Path != mainRoot || worktrees[0].Kind != MainWorktree {
		t.Errorf("main worktree = %#v, want path %q and main kind", worktrees[0], mainRoot)
	}
	if got := worktreesByPath(worktrees)[linkedRoot]; got.Branch != "feature" || got.Kind != LinkedWorktree {
		t.Errorf("linked worktree = %#v, want feature linked worktree", got)
	}
}

func TestListWorktreesRejectsNonGitRepository(t *testing.T) {
	isolateGitConfiguration(t)

	worktrees, err := ListWorktrees(context.Background(), t.TempDir())
	if worktrees != nil {
		t.Errorf("worktrees = %#v, want nil", worktrees)
	}
	if !errors.Is(err, ErrNotGitRepository) {
		t.Fatalf("ListWorktrees() error = %v, want %v", err, ErrNotGitRepository)
	}
	if got, want := err.Error(), ErrNotGitRepository.Error(); got != want {
		t.Errorf("error = %q, want %q", got, want)
	}
}

func TestListWorktreesReturnsGitError(t *testing.T) {
	gitErr := errors.New("worktree list failed")
	worktrees, err := listWorktrees(
		context.Background(),
		func(_ context.Context, directory string, arguments ...string) ([]byte, error) {
			if directory != "/repository" {
				t.Errorf("directory = %q, want /repository", directory)
			}
			if !reflect.DeepEqual(arguments, []string{"worktree", "list", "--porcelain", "-z"}) {
				t.Errorf("arguments = %q, want worktree list command", arguments)
			}
			return nil, gitErr
		},
		"/repository",
	)
	if worktrees != nil {
		t.Errorf("worktrees = %#v, want nil", worktrees)
	}
	if !errors.Is(err, gitErr) {
		t.Fatalf("listWorktrees() error = %v, want wrapped Git error", err)
	}
}

func TestParseWorktreeList(t *testing.T) {
	output := strings.Join([]string{
		"worktree /repo",
		"HEAD 1111111111111111111111111111111111111111",
		"branch refs/heads/main",
		"worktree /repo_feature",
		"HEAD 2222222222222222222222222222222222222222",
		"branch refs/heads/feature/login",
		"locked working on review",
		"prunable gitdir file points to non-existent location",
		"worktree /repo_detached",
		"HEAD 3333333333333333333333333333333333333333",
		"detached",
		"bare",
		"locked",
		"prunable",
		"",
	}, "\x00")

	worktrees, err := parseWorktreeList([]byte(output))
	if err != nil {
		t.Fatalf("parseWorktreeList() error = %v", err)
	}
	want := []ListedWorktree{
		{Path: "/repo", Kind: MainWorktree, HEAD: "1111111111111111111111111111111111111111", Branch: "main", BranchRef: "refs/heads/main"},
		{Path: "/repo_feature", Kind: LinkedWorktree, HEAD: "2222222222222222222222222222222222222222", Branch: "feature/login", BranchRef: "refs/heads/feature/login", Locked: true, LockReason: "working on review", Prunable: true, PruneReason: "gitdir file points to non-existent location"},
		{Path: "/repo_detached", Kind: LinkedWorktree, HEAD: "3333333333333333333333333333333333333333", Detached: true, Bare: true, Locked: true, Prunable: true},
	}
	if !reflect.DeepEqual(worktrees, want) {
		t.Fatalf("worktrees = %#v, want %#v", worktrees, want)
	}
}

func TestParseWorktreeListRejectsMalformedOutput(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		wantErr string
	}{
		{name: "empty", output: "", wantErr: "no worktrees"},
		{name: "field before worktree", output: "HEAD abc\x00", wantErr: "before worktree path"},
		{name: "empty worktree path", output: "worktree \x00", wantErr: "empty worktree path"},
		{name: "empty head", output: "worktree /repo\x00HEAD \x00", wantErr: "empty HEAD"},
		{name: "empty branch", output: "worktree /repo\x00branch \x00", wantErr: "empty branch"},
		{name: "unrecognized field", output: "worktree /repo\x00unknown value\x00", wantErr: "unrecognized field"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			worktrees, err := parseWorktreeList([]byte(test.output))
			if worktrees != nil {
				t.Errorf("worktrees = %#v, want nil", worktrees)
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("parseWorktreeList() error = %v, want containing %q", err, test.wantErr)
			}
		})
	}
}

func assertListedWorktree(t *testing.T, got, want ListedWorktree) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("worktree = %#v, want %#v", got, want)
	}
}

func worktreesByPath(worktrees []ListedWorktree) map[string]ListedWorktree {
	byPath := make(map[string]ListedWorktree, len(worktrees))
	for _, worktree := range worktrees {
		byPath[worktree.Path] = worktree
	}
	return byPath
}

func canonicalPath(t *testing.T, path string) string {
	t.Helper()
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("resolve path %q: %v", path, err)
	}
	return canonical
}

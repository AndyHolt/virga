package git

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestListLocalBranches(t *testing.T) {
	root := newTestRepository(t, "repository")
	runGit(t, "-C", root, "branch", "release")
	runGit(t, "-C", root, "branch", "feature/login")
	runGit(t, "-C", root, "tag", "v1.0.0")

	branches, err := ListLocalBranches(context.Background(), root)
	if err != nil {
		t.Fatalf("ListLocalBranches() error = %v", err)
	}
	want := []string{"feature/login", "main", "release"}
	if !reflect.DeepEqual(branches, want) {
		t.Errorf("branches = %q, want %q", branches, want)
	}
}

func TestListLocalBranchesRejectsNonGitRepository(t *testing.T) {
	isolateGitConfiguration(t)

	branches, err := ListLocalBranches(context.Background(), t.TempDir())
	if branches != nil {
		t.Errorf("branches = %q, want nil", branches)
	}
	if !errors.Is(err, ErrNotGitRepository) {
		t.Fatalf("ListLocalBranches() error = %v, want %v", err, ErrNotGitRepository)
	}
	if got, want := err.Error(), ErrNotGitRepository.Error(); got != want {
		t.Errorf("error = %q, want %q", got, want)
	}
}

func TestListLocalBranchesReturnsGitError(t *testing.T) {
	gitErr := errors.New("for-each-ref failed")
	branches, err := listLocalBranches(
		context.Background(),
		func(_ context.Context, directory string, arguments ...string) ([]byte, error) {
			if directory != "/repository" {
				t.Errorf("directory = %q, want /repository", directory)
			}
			switch {
			case reflect.DeepEqual(arguments, []string{"rev-parse", "--git-dir"}):
				return []byte(".git\n"), nil
			case reflect.DeepEqual(arguments, []string{
				"for-each-ref",
				"--sort=refname",
				"--format=%(refname:short)%00",
				"refs/heads/",
			}):
				return nil, gitErr
			default:
				t.Errorf("arguments = %q, want a repository or branch-listing command", arguments)
				return nil, errors.New("unexpected Git command")
			}
		},
		"/repository",
	)
	if branches != nil {
		t.Errorf("branches = %q, want nil", branches)
	}
	if !errors.Is(err, gitErr) {
		t.Fatalf("listLocalBranches() error = %v, want wrapped Git error", err)
	}
}

func TestParseLocalBranches(t *testing.T) {
	output := []byte("feature/login\x00\nmain\x00\nrelease\x00\n")
	want := []string{"feature/login", "main", "release"}
	if got := parseLocalBranches(output); !reflect.DeepEqual(got, want) {
		t.Errorf("parseLocalBranches(%q) = %q, want %q", output, got, want)
	}
}

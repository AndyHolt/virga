package files

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMaterialiseCopiesFilesWithSpacesAndUnicode(t *testing.T) {
	repository := t.TempDir()
	worktree := t.TempDir()
	source := filepath.Join("config space", "local-å.yaml")
	writeFile(t, filepath.Join(repository, source), "secret: true\n", 0o640)

	err := Materialise(context.Background(), Options{
		RepositoryRoot: repository,
		WorktreeRoot:   worktree,
		Entries:        []Entry{{Source: source, Mode: ModeCopy}},
	})
	if err != nil {
		t.Fatalf("Materialise() error = %v", err)
	}

	destination := filepath.Join(worktree, source)
	contents, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("read destination: %v", err)
	}
	if got, want := string(contents), "secret: true\n"; got != want {
		t.Errorf("destination contents = %q, want %q", got, want)
	}
	info, err := os.Stat(destination)
	if err != nil {
		t.Fatalf("stat destination: %v", err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o640); got != want {
		t.Errorf("destination mode = %v, want %v", got, want)
	}
}

func TestMaterialiseCreatesRelativeSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks on Windows requires privileges")
	}
	repository := t.TempDir()
	worktree := t.TempDir()
	writeFile(t, filepath.Join(repository, ".env"), "TOKEN=value\n", 0o600)

	err := Materialise(context.Background(), Options{
		RepositoryRoot: repository,
		WorktreeRoot:   worktree,
		Entries:        []Entry{{Source: ".env", Mode: ModeSymlink}},
	})
	if err != nil {
		t.Fatalf("Materialise() error = %v", err)
	}

	destination := filepath.Join(worktree, ".env")
	target, err := os.Readlink(destination)
	if err != nil {
		t.Fatalf("read symlink: %v", err)
	}
	if filepath.IsAbs(target) {
		t.Errorf("symlink target = %q, want relative target", target)
	}
	wantTarget, err := filepath.Rel(filepath.Dir(destination), filepath.Join(repository, ".env"))
	if err != nil {
		t.Fatalf("calculate relative target: %v", err)
	}
	if target != wantTarget {
		t.Errorf("symlink target = %q, want %q", target, wantTarget)
	}
	contents, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("read symlink destination: %v", err)
	}
	if got, want := string(contents), "TOKEN=value\n"; got != want {
		t.Errorf("symlink contents = %q, want %q", got, want)
	}
}

func TestMaterialiseCreatesRelativeDirectorySymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks on Windows requires privileges")
	}
	repository := t.TempDir()
	worktree := t.TempDir()
	source := "cache"
	if err := os.Mkdir(filepath.Join(repository, source), 0o755); err != nil {
		t.Fatalf("create source directory: %v", err)
	}

	err := Materialise(context.Background(), Options{
		RepositoryRoot: repository,
		WorktreeRoot:   worktree,
		Entries:        []Entry{{Source: source, Mode: ModeSymlink}},
	})
	if err != nil {
		t.Fatalf("Materialise() error = %v", err)
	}

	destination := filepath.Join(worktree, source)
	target, err := os.Readlink(destination)
	if err != nil {
		t.Fatalf("read directory symlink: %v", err)
	}
	wantTarget, err := filepath.Rel(filepath.Dir(destination), filepath.Join(repository, source))
	if err != nil {
		t.Fatalf("calculate relative target: %v", err)
	}
	if target != wantTarget {
		t.Errorf("symlink target = %q, want %q", target, wantTarget)
	}
	info, err := os.Stat(destination)
	if err != nil {
		t.Fatalf("stat directory symlink: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("symlink target is not a directory")
	}

	file := filepath.Join(destination, "dataset.bin")
	if err := os.WriteFile(file, []byte("shared data"), 0o600); err != nil {
		t.Fatalf("write through directory symlink: %v", err)
	}
	contents, err := os.ReadFile(filepath.Join(repository, source, "dataset.bin"))
	if err != nil {
		t.Fatalf("read source directory after write: %v", err)
	}
	if got, want := string(contents), "shared data"; got != want {
		t.Errorf("source contents = %q, want %q", got, want)
	}
}

func TestMaterialiseRejectsDirectoryCopy(t *testing.T) {
	repository := t.TempDir()
	worktree := t.TempDir()
	if err := os.Mkdir(filepath.Join(repository, "cache"), 0o755); err != nil {
		t.Fatalf("create source directory: %v", err)
	}

	err := Materialise(context.Background(), Options{
		RepositoryRoot: repository,
		WorktreeRoot:   worktree,
		Entries:        []Entry{{Source: "cache", Mode: ModeCopy}},
	})
	if err == nil || !strings.Contains(err.Error(), "regular file for copy mode") {
		t.Fatalf("Materialise() error = %v, want directory copy rejection", err)
	}
}

func TestMaterialisePreflightsBeforeChangingFiles(t *testing.T) {
	repository := t.TempDir()
	worktree := t.TempDir()
	writeFile(t, filepath.Join(repository, "present.env"), "present\n", 0o600)

	err := Materialise(context.Background(), Options{
		RepositoryRoot: repository,
		WorktreeRoot:   worktree,
		Entries: []Entry{
			{Source: "present.env", Mode: ModeCopy},
			{Source: "missing.env", Mode: ModeCopy},
		},
	})
	if err == nil || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Materialise() error = %v, want missing source error", err)
	}
	if _, err := os.Lstat(filepath.Join(worktree, "present.env")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("destination for valid entry exists after failed preflight: %v", err)
	}
}

func TestMaterialiseRejectsPathTraversal(t *testing.T) {
	repository := t.TempDir()
	worktree := t.TempDir()
	writeFile(t, filepath.Join(repository, "safe.env"), "safe\n", 0o600)

	err := Materialise(context.Background(), Options{
		RepositoryRoot: repository,
		WorktreeRoot:   worktree,
		Entries:        []Entry{{Source: "../safe.env", Mode: ModeCopy}},
	})
	if err == nil || !strings.Contains(err.Error(), "within the repository") {
		t.Fatalf("Materialise() error = %v, want path traversal error", err)
	}
}

func TestMaterialiseRejectsExistingDestination(t *testing.T) {
	repository := t.TempDir()
	worktree := t.TempDir()
	writeFile(t, filepath.Join(repository, ".env"), "repository\n", 0o600)
	writeFile(t, filepath.Join(worktree, ".env"), "worktree\n", 0o600)

	err := Materialise(context.Background(), Options{
		RepositoryRoot: repository,
		WorktreeRoot:   worktree,
		Entries:        []Entry{{Source: ".env", Mode: ModeCopy}},
	})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("Materialise() error = %v, want existing destination error", err)
	}
	contents, err := os.ReadFile(filepath.Join(worktree, ".env"))
	if err != nil {
		t.Fatalf("read existing destination: %v", err)
	}
	if got, want := string(contents), "worktree\n"; got != want {
		t.Errorf("existing destination contents = %q, want %q", got, want)
	}
}

func TestMaterialiseRejectsDuplicateDestinations(t *testing.T) {
	repository := t.TempDir()
	worktree := t.TempDir()
	writeFile(t, filepath.Join(repository, "app.env"), "app\n", 0o600)

	err := Materialise(context.Background(), Options{
		RepositoryRoot: repository,
		WorktreeRoot:   worktree,
		Entries: []Entry{
			{Source: "app.env", Mode: ModeCopy},
			{Source: "app.env", Mode: ModeSymlink},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "also configured") {
		t.Fatalf("Materialise() error = %v, want duplicate destination error", err)
	}
}

func TestMaterialiseRejectsFileDestinationParent(t *testing.T) {
	repository := t.TempDir()
	worktree := t.TempDir()
	writeFile(t, filepath.Join(repository, "config", "local.yaml"), "value\n", 0o600)
	writeFile(t, filepath.Join(worktree, "config"), "not a directory\n", 0o600)

	err := Materialise(context.Background(), Options{
		RepositoryRoot: repository,
		WorktreeRoot:   worktree,
		Entries:        []Entry{{Source: filepath.Join("config", "local.yaml"), Mode: ModeCopy}},
	})
	if err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("Materialise() error = %v, want destination parent error", err)
	}
}

func TestMaterialiseRejectsSymlinkDestinationParent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks on Windows requires privileges")
	}
	repository := t.TempDir()
	worktree := t.TempDir()
	outside := t.TempDir()
	writeFile(t, filepath.Join(repository, "config", "local.yaml"), "value\n", 0o600)
	if err := os.Symlink(outside, filepath.Join(worktree, "config")); err != nil {
		t.Fatalf("create destination parent symlink: %v", err)
	}

	err := Materialise(context.Background(), Options{
		RepositoryRoot: repository,
		WorktreeRoot:   worktree,
		Entries:        []Entry{{Source: filepath.Join("config", "local.yaml"), Mode: ModeCopy}},
	})
	if err == nil || !strings.Contains(err.Error(), "is a symlink") {
		t.Fatalf("Materialise() error = %v, want symlink parent error", err)
	}
	if _, err := os.Lstat(filepath.Join(outside, "local.yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("destination was created outside worktree: %v", err)
	}
}

func writeFile(t *testing.T, path, contents string, permission os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create parent directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), permission); err != nil {
		t.Fatalf("write %q: %v", path, err)
	}
}

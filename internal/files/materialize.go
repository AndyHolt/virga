// Package files materializes repository-managed files into Virga worktrees.
package files

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Mode describes how a repository file should appear in a new worktree.
type Mode string

const (
	// ModeCopy copies file contents into the worktree.
	ModeCopy Mode = "copy"
	// ModeSymlink creates a symlink in the worktree that points at the repository file.
	ModeSymlink Mode = "symlink"
)

// Entry describes one repository file to materialize into a worktree. Source is
// relative to the primary repository root and is also used as the worktree
// destination path.
type Entry struct {
	Source string
	Mode   Mode
}

// Options describes a file materialization operation.
type Options struct {
	RepositoryRoot string
	WorktreeRoot   string
	Entries        []Entry
}

type plannedEntry struct {
	index       int
	source      string
	destination string
	relative    string
	mode        Mode
	permission  os.FileMode
}

// Materialize copies or links configured files from the primary repository into
// a worktree. All entries are validated before any filesystem changes are made.
func Materialize(ctx context.Context, options Options) error {
	planned, err := preflight(options)
	if err != nil {
		return err
	}

	for _, entry := range planned {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(entry.destination), 0o755); err != nil {
			return fmt.Errorf("create destination directory for files[%d] %q: %w", entry.index, entry.relative, err)
		}

		switch entry.mode {
		case ModeCopy:
			if err := copyFile(ctx, entry.source, entry.destination, entry.permission); err != nil {
				return fmt.Errorf("copy files[%d] %q: %w", entry.index, entry.relative, err)
			}
		case ModeSymlink:
			target := symlinkTarget(entry.source, entry.destination)
			if err := os.Symlink(target, entry.destination); err != nil {
				return fmt.Errorf("symlink files[%d] %q: %w", entry.index, entry.relative, err)
			}
		default:
			panic("preflight accepted unsupported file materialization mode")
		}
	}
	return nil
}

func preflight(options Options) ([]plannedEntry, error) {
	repositoryRoot, err := cleanRoot(options.RepositoryRoot, "repository root")
	if err != nil {
		return nil, err
	}
	worktreeRoot, err := cleanRoot(options.WorktreeRoot, "worktree root")
	if err != nil {
		return nil, err
	}
	if err := requireDirectory(repositoryRoot, "repository root"); err != nil {
		return nil, err
	}
	if err := requireDirectory(worktreeRoot, "worktree root"); err != nil {
		return nil, err
	}

	planned := make([]plannedEntry, 0, len(options.Entries))
	seen := make(map[string]int, len(options.Entries))
	for index, entry := range options.Entries {
		relative, err := cleanRelativePath(entry.Source)
		if err != nil {
			return nil, fmt.Errorf("validate files[%d]: %w", index, err)
		}
		if entry.Mode != ModeCopy && entry.Mode != ModeSymlink {
			return nil, fmt.Errorf("validate files[%d] %q: mode must be %q or %q", index, entry.Source, ModeCopy, ModeSymlink)
		}
		if previous, ok := seen[relative]; ok {
			return nil, fmt.Errorf("validate files[%d] %q: destination also configured by files[%d]", index, entry.Source, previous)
		}
		seen[relative] = index

		source := filepath.Join(repositoryRoot, relative)
		destination := filepath.Join(worktreeRoot, relative)
		if !pathWithin(repositoryRoot, source) {
			return nil, fmt.Errorf("validate files[%d] %q: source escapes repository root", index, entry.Source)
		}
		if !pathWithin(worktreeRoot, destination) {
			return nil, fmt.Errorf("validate files[%d] %q: destination escapes worktree root", index, entry.Source)
		}

		sourceInfo, err := os.Stat(source)
		if err != nil {
			return nil, fmt.Errorf("stat source for files[%d] %q: %w", index, entry.Source, err)
		}
		if !sourceInfo.Mode().IsRegular() {
			return nil, fmt.Errorf("validate files[%d] %q: source must be a regular file", index, entry.Source)
		}
		if _, err := os.Lstat(destination); err == nil {
			return nil, fmt.Errorf("validate files[%d] %q: destination %q already exists", index, entry.Source, destination)
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("check destination for files[%d] %q: %w", index, entry.Source, err)
		}
		if err := checkDestinationParents(worktreeRoot, destination); err != nil {
			return nil, fmt.Errorf("validate files[%d] %q: %w", index, entry.Source, err)
		}

		planned = append(planned, plannedEntry{
			index:       index,
			source:      source,
			destination: destination,
			relative:    relative,
			mode:        entry.Mode,
			permission:  sourceInfo.Mode().Perm(),
		})
	}

	if err := rejectDestinationAncestorConflicts(planned); err != nil {
		return nil, err
	}
	return planned, nil
}

func cleanRoot(path, name string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve %s %q: %w", name, path, err)
	}
	return filepath.Clean(absolute), nil
}

func requireDirectory(path, name string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat %s %q: %w", name, path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s %q is not a directory", name, path)
	}
	return nil
}

func cleanRelativePath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("source is required")
	}
	converted := filepath.FromSlash(path)
	if filepath.IsAbs(converted) {
		return "", fmt.Errorf("source %q must be a relative path within the repository", path)
	}
	for _, component := range strings.Split(converted, string(filepath.Separator)) {
		if component == ".." {
			return "", fmt.Errorf("source %q must be a relative path within the repository", path)
		}
	}
	relative := filepath.Clean(converted)
	if relative == "." || !filepath.IsLocal(relative) {
		return "", fmt.Errorf("source %q must be a relative path within the repository", path)
	}
	return relative, nil
}

func pathWithin(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && filepath.IsLocal(relative)
}

func checkDestinationParents(worktreeRoot, destination string) error {
	parent := filepath.Dir(destination)
	relative, err := filepath.Rel(worktreeRoot, parent)
	if err != nil {
		return fmt.Errorf("destination parent %q is outside worktree root: %w", parent, err)
	}
	if relative == "." {
		return nil
	}

	current := worktreeRoot
	for _, element := range splitPath(relative) {
		current = filepath.Join(current, element)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("check destination parent %q: %w", current, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("destination parent %q is a symlink", current)
		}
		if !info.IsDir() {
			return fmt.Errorf("destination parent %q is not a directory", current)
		}
	}
	return nil
}

func splitPath(path string) []string {
	return strings.Split(path, string(filepath.Separator))
}

func rejectDestinationAncestorConflicts(planned []plannedEntry) error {
	for outerIndex, outer := range planned {
		for innerIndex := outerIndex + 1; innerIndex < len(planned); innerIndex++ {
			inner := planned[innerIndex]
			switch {
			case isAncestorPath(outer.relative, inner.relative):
				return fmt.Errorf("validate files[%d] %q: destination conflicts with files[%d] %q", inner.index, inner.relative, outer.index, outer.relative)
			case isAncestorPath(inner.relative, outer.relative):
				return fmt.Errorf("validate files[%d] %q: destination conflicts with files[%d] %q", outer.index, outer.relative, inner.index, inner.relative)
			}
		}
	}
	return nil
}

func isAncestorPath(parent, child string) bool {
	return child != parent && len(child) > len(parent) && child[:len(parent)] == parent && child[len(parent)] == filepath.Separator
}

func copyFile(ctx context.Context, source, destination string, permission os.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer func() {
		_ = input.Close()
	}()

	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, permission)
	if err != nil {
		return err
	}
	removeDestination := true
	defer func() {
		if removeDestination {
			_ = os.Remove(destination)
		}
	}()

	buffer := make([]byte, 32*1024)
	for {
		if err := ctx.Err(); err != nil {
			_ = output.Close()
			return err
		}
		read, readErr := input.Read(buffer)
		if read > 0 {
			if _, writeErr := output.Write(buffer[:read]); writeErr != nil {
				_ = output.Close()
				return writeErr
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			_ = output.Close()
			return readErr
		}
	}
	if err := output.Close(); err != nil {
		return err
	}
	if err := os.Chmod(destination, permission); err != nil {
		return err
	}
	removeDestination = false
	return nil
}

func symlinkTarget(source, destination string) string {
	target, err := filepath.Rel(filepath.Dir(destination), source)
	if err == nil && target != "" && !filepath.IsAbs(target) {
		return target
	}
	return source
}

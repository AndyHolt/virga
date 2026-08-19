// Package config loads Virga configuration from user, repository, and explicit sources.
package config

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/AndyHolt/virga/internal/files"
	"github.com/AndyHolt/virga/internal/git"
	"gopkg.in/yaml.v3"
)

const environmentConfigPath = "VIRGA_CONFIG"

// Config is Virga's configuration.
type Config struct {
	Tmux  TmuxConfig
	Files []files.Entry
}

// Inspector finds the Git worktree containing a directory.
type Inspector func(context.Context, string) (git.WorktreeInfo, error)

// LoaderDependencies contains the external operations used by Loader. Nil
// fields use the standard Git, environment, filesystem, and runtime operations.
type LoaderDependencies struct {
	InspectWorktree Inspector
	LookupEnv       func(string) (string, bool)
	UserHomeDir     func() (string, error)
	UserConfigDir   func() (string, error)
	ReadFile        func(string) ([]byte, error)
	GOOS            string
}

// Loader loads configuration without depending on process-global state.
type Loader struct {
	inspectWorktree Inspector
	lookupEnv       func(string) (string, bool)
	userHomeDir     func() (string, error)
	userConfigDir   func() (string, error)
	readFile        func(string) ([]byte, error)
	goos            string
}

// NewLoader constructs a configuration loader. Dependencies can be overridden
// to isolate tests from Git, the environment, and the user's configuration.
func NewLoader(dependencies LoaderDependencies) Loader {
	loader := Loader{
		inspectWorktree: dependencies.InspectWorktree,
		lookupEnv:       dependencies.LookupEnv,
		userHomeDir:     dependencies.UserHomeDir,
		userConfigDir:   dependencies.UserConfigDir,
		readFile:        dependencies.ReadFile,
		goos:            dependencies.GOOS,
	}
	if loader.inspectWorktree == nil {
		loader.inspectWorktree = git.InspectWorktree
	}
	if loader.lookupEnv == nil {
		loader.lookupEnv = os.LookupEnv
	}
	if loader.userHomeDir == nil {
		loader.userHomeDir = os.UserHomeDir
	}
	if loader.userConfigDir == nil {
		loader.userConfigDir = os.UserConfigDir
	}
	if loader.readFile == nil {
		loader.readFile = os.ReadFile
	}
	if loader.goos == "" {
		loader.goos = runtime.GOOS
	}
	return loader
}

type configSource struct {
	path     string
	optional bool
	name     string
}

// Load reads configuration for dir. User configuration is loaded first,
// followed by .virga.yaml at the primary worktree root, a VIRGA_CONFIG file,
// and explicitPath. Later sources replace earlier tmux and files
// configuration. User and repository configuration files are optional;
// VIRGA_CONFIG and explicitPath must name files that exist.
func (loader Loader) Load(ctx context.Context, dir, explicitPath string) (Config, error) {
	userPath, err := loader.userConfigPath()
	if err != nil {
		return Config{}, err
	}

	var result rawConfig
	userConfiguration, found, err := loader.readConfig(configSource{
		path: userPath, optional: true, name: "user configuration",
	})
	if err != nil {
		return Config{}, err
	}
	if found {
		mergeRawConfig(&result, userConfiguration)
	}

	info, err := loader.inspectWorktree(ctx, dir)
	if err != nil {
		return Config{}, fmt.Errorf("discover primary repository: %w", err)
	}
	if info.Kind == git.NotWorktree {
		return Config{}, git.ErrNotGitRepository
	}
	if info.MainWorktreeRoot == "" {
		return Config{}, fmt.Errorf("discover primary repository: Git returned an empty primary worktree root")
	}

	sources := []configSource{{
		path: filepath.Join(info.MainWorktreeRoot, ".virga.yaml"), optional: true, name: "repository configuration",
	}}
	if path, ok := loader.lookupEnv(environmentConfigPath); ok && path != "" {
		sources = append(sources, configSource{
			path: loader.resolvePath(dir, path),
			name: environmentConfigPath,
		})
	}
	if explicitPath != "" {
		sources = append(sources, configSource{
			path: loader.resolvePath(dir, explicitPath),
			name: "explicit configuration",
		})
	}

	for _, source := range sources {
		overlay, found, err := loader.readConfig(source)
		if err != nil {
			return Config{}, err
		}
		if found {
			mergeRawConfig(&result, overlay)
		}
	}
	return validate(result)
}

func (loader Loader) readConfig(source configSource) (rawConfig, bool, error) {
	contents, err := loader.readFile(source.path)
	if err != nil {
		if source.optional && errors.Is(err, os.ErrNotExist) {
			return rawConfig{}, false, nil
		}
		return rawConfig{}, false, fmt.Errorf("read %s %q: %w", source.name, source.path, err)
	}

	var configuration rawConfig
	decoder := yaml.NewDecoder(bytes.NewReader(contents))
	decoder.KnownFields(true)
	if err := decoder.Decode(&configuration); err != nil && !errors.Is(err, io.EOF) {
		return rawConfig{}, false, fmt.Errorf("parse %s %q: %w", source.name, source.path, err)
	}
	if err := ensureSingleYAMLDocument(decoder); err != nil {
		return rawConfig{}, false, fmt.Errorf("parse %s %q: %w", source.name, source.path, err)
	}
	return configuration, true, nil
}

func ensureSingleYAMLDocument(decoder *yaml.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return fmt.Errorf("configuration contains more than one YAML document")
	}
	return err
}

func (loader Loader) userConfigPath() (string, error) {
	if loader.goos == "windows" {
		directory, err := loader.userConfigDir()
		if err != nil {
			return "", fmt.Errorf("find user configuration directory: %w", err)
		}
		return filepath.Join(directory, "virga", "config.yaml"), nil
	}

	if directory, ok := loader.lookupEnv("XDG_CONFIG_HOME"); ok && filepath.IsAbs(directory) {
		return filepath.Join(directory, "virga", "config.yaml"), nil
	}
	home, err := loader.userHomeDir()
	if err != nil {
		return "", fmt.Errorf("find user home directory: %w", err)
	}
	return filepath.Join(home, ".config", "virga", "config.yaml"), nil
}

func (loader Loader) resolvePath(directory, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(directory, path)
}

func mergeRawConfig(base *rawConfig, overlay rawConfig) {
	if overlay.Tmux != nil {
		base.Tmux = overlay.Tmux
	}
	if overlay.Files != nil {
		base.Files = overlay.Files
	}
}

func validate(raw rawConfig) (Config, error) {
	var configuration Config
	if raw.Files != nil {
		configuration.Files = make([]files.Entry, len(*raw.Files))
		for index, entry := range *raw.Files {
			if entry.Source == "" {
				return Config{}, fmt.Errorf("validate files[%d]: source is required", index)
			}
			mode := files.Mode(strings.TrimSpace(entry.Mode))
			if mode != files.ModeCopy && mode != files.ModeSymlink {
				return Config{}, fmt.Errorf("validate files[%d] %q: mode must be %q or %q", index, entry.Source, files.ModeCopy, files.ModeSymlink)
			}
			configuration.Files[index] = files.Entry{Source: entry.Source, Mode: mode}
		}
	}

	if raw.Tmux == nil {
		return configuration, nil
	}

	configuration.Tmux.Windows = make([]TmuxWindow, len(raw.Tmux.Windows))
	for index, window := range raw.Tmux.Windows {
		name := strings.TrimSpace(window.Name)
		if name == "" {
			return Config{}, fmt.Errorf("validate tmux.windows[%d]: name is required", index)
		}

		if len(window.Panes) == 0 {
			return Config{}, fmt.Errorf("validate tmux.windows[%d]: at least one pane is required", index)
		}
		panes := make([]TmuxPane, len(window.Panes))
		for paneIndex, pane := range window.Panes {
			panes[paneIndex] = TmuxPane(pane)
		}
		configuration.Tmux.Windows[index] = TmuxWindow{Name: name, Panes: panes}
	}
	return configuration, nil
}

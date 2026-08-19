package config

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/AndyHolt/virga/internal/files"
	"github.com/AndyHolt/virga/internal/git"
)

func TestLoaderLoadsConfigurationInPrecedenceOrder(t *testing.T) {
	root := t.TempDir()
	userHome := t.TempDir()
	environmentPath := filepath.Join(t.TempDir(), "environment.yaml")
	explicitPath := filepath.Join(t.TempDir(), "explicit.yaml")
	writeTestConfig(t, filepath.Join(userHome, ".config", "virga", "config.yaml"), testConfigYAML("user"))
	writeTestConfig(t, filepath.Join(root, ".virga.yaml"), testConfigYAML("repository"))
	writeTestConfig(t, environmentPath, testConfigYAML("environment"))
	writeTestConfig(t, explicitPath, testConfigYAML("explicit"))

	tests := []struct {
		name         string
		environment  map[string]string
		explicitPath string
		wantWindow   string
	}{
		{name: "user configuration", wantWindow: "user"},
		{name: "repository configuration", environment: map[string]string{}, wantWindow: "repository"},
		{name: "environment configuration", environment: map[string]string{environmentConfigPath: environmentPath}, wantWindow: "environment"},
		{name: "explicit configuration", environment: map[string]string{environmentConfigPath: environmentPath}, explicitPath: explicitPath, wantWindow: "explicit"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.name == "user configuration" {
				removeTestConfig(t, filepath.Join(root, ".virga.yaml"))
			} else {
				writeTestConfig(t, filepath.Join(root, ".virga.yaml"), testConfigYAML("repository"))
			}
			loader := testLoader(root, userHome, test.environment, "linux")
			configuration, err := loader.Load(context.Background(), filepath.Join(root, "nested"), test.explicitPath)
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if got := configuration.Tmux.Windows[0].Name; got != test.wantWindow {
				t.Errorf("window name = %q, want %q", got, test.wantWindow)
			}
		})
	}
}

func TestLoaderUsesPrimaryWorktreeConfiguration(t *testing.T) {
	root := t.TempDir()
	linked := t.TempDir()
	writeTestConfig(t, filepath.Join(root, ".virga.yaml"), testConfigYAML("repository"))

	loader := NewLoader(LoaderDependencies{
		InspectWorktree: func(context.Context, string) (git.WorktreeInfo, error) {
			return git.WorktreeInfo{Kind: git.LinkedWorktree, MainWorktreeRoot: root}, nil
		},
		LookupEnv:   func(string) (string, bool) { return "", false },
		UserHomeDir: func() (string, error) { return t.TempDir(), nil },
		GOOS:        "linux",
	})
	configuration, err := loader.Load(context.Background(), linked, "")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got, want := configuration.Tmux.Windows[0].Name, "repository"; got != want {
		t.Errorf("window name = %q, want %q", got, want)
	}
}

func TestLoaderAllowsEmptyOptionalConfiguration(t *testing.T) {
	root := t.TempDir()
	loader := testLoader(root, t.TempDir(), nil, "linux")

	for _, name := range []string{"absent", "empty"} {
		t.Run(name, func(t *testing.T) {
			if name == "empty" {
				writeTestConfig(t, filepath.Join(root, ".virga.yaml"), "")
			}
			configuration, err := loader.Load(context.Background(), root, "")
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if !reflect.DeepEqual(configuration, Config{}) {
				t.Errorf("configuration = %#v, want empty configuration", configuration)
			}
		})
	}
}

func TestLoaderRejectsConfigurationErrors(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{name: "malformed YAML", content: "tmux: [", want: "parse repository configuration"},
		{name: "unknown field", content: "tmux:\n  unknown: value\n", want: "field unknown not found"},
		{name: "multiple documents", content: testConfigYAML("one") + "---\n" + testConfigYAML("two"), want: "more than one YAML document"},
		{name: "missing window name", content: "tmux:\n  windows:\n    - panes:\n        - command: nvim\n", want: "name is required"},
		{name: "window without panes", content: "tmux:\n  windows:\n    - name: editor\n", want: "at least one pane is required"},
		{name: "missing file source", content: "files:\n  - mode: copy\n", want: "source is required"},
		{name: "invalid file mode", content: "files:\n  - source: .env\n    mode: move\n", want: "mode must be"},
		{name: "unknown file field", content: "files:\n  - source: .env\n    mode: copy\n    destination: .env\n", want: "field destination not found"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeTestConfig(t, filepath.Join(root, ".virga.yaml"), test.content)
			loader := testLoader(root, t.TempDir(), nil, "linux")

			_, err := loader.Load(context.Background(), root, "")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Load() error = %v, want error containing %q", err, test.want)
			}
		})
	}
}

func TestLoaderLoadsFilesConfiguration(t *testing.T) {
	root := t.TempDir()
	writeTestConfig(t, filepath.Join(root, ".virga.yaml"), "files:\n  - source: .env\n    mode: symlink\n  - source: config/local.yaml\n    mode: copy\n")
	loader := testLoader(root, t.TempDir(), nil, "linux")

	configuration, err := loader.Load(context.Background(), root, "")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	want := []files.Entry{
		{Source: ".env", Mode: files.ModeSymlink},
		{Source: "config/local.yaml", Mode: files.ModeCopy},
	}
	if !reflect.DeepEqual(configuration.Files, want) {
		t.Errorf("files = %#v, want %#v", configuration.Files, want)
	}
}

func TestLoaderAllowsDuplicateWindowNames(t *testing.T) {
	root := t.TempDir()
	writeTestConfig(t, filepath.Join(root, ".virga.yaml"), "tmux:\n  windows:\n    - name: editor\n      panes:\n        - command: nvim\n    - name: editor\n      panes:\n        - command: make test\n")
	loader := testLoader(root, t.TempDir(), nil, "linux")

	configuration, err := loader.Load(context.Background(), root, "")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got, want := len(configuration.Tmux.Windows), 2; got != want {
		t.Fatalf("window count = %d, want %d", got, want)
	}
	for index, window := range configuration.Tmux.Windows {
		if got, want := window.Name, "editor"; got != want {
			t.Errorf("windows[%d].Name = %q, want %q", index, got, want)
		}
	}
}

func TestLoaderUsesXDGConfigurationDirectory(t *testing.T) {
	tests := []struct {
		name      string
		xdgConfig string
		wantPath  string
	}{
		{name: "absolute", xdgConfig: "/configuration", wantPath: filepath.Join("/configuration", "virga", "config.yaml")},
		{name: "unset", wantPath: filepath.Join("/home", ".config", "virga", "config.yaml")},
		{name: "relative", xdgConfig: "relative", wantPath: filepath.Join("/home", ".config", "virga", "config.yaml")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var readPaths []string
			loader := NewLoader(LoaderDependencies{
				InspectWorktree: func(context.Context, string) (git.WorktreeInfo, error) {
					return git.WorktreeInfo{Kind: git.MainWorktree, MainWorktreeRoot: "/repository"}, nil
				},
				LookupEnv: func(key string) (string, bool) {
					if key == "XDG_CONFIG_HOME" && test.xdgConfig != "" {
						return test.xdgConfig, true
					}
					return "", false
				},
				UserHomeDir: func() (string, error) { return "/home", nil },
				ReadFile: func(path string) ([]byte, error) {
					readPaths = append(readPaths, path)
					return nil, os.ErrNotExist
				},
				GOOS: "linux",
			})
			if _, err := loader.Load(context.Background(), "/repository", ""); err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if len(readPaths) == 0 || readPaths[0] != test.wantPath {
				t.Errorf("first read path = %q, want %q", readPaths, test.wantPath)
			}
		})
	}
}

func TestLoaderUsesWindowsUserConfigurationDirectory(t *testing.T) {
	var readPaths []string
	loader := NewLoader(LoaderDependencies{
		InspectWorktree: func(context.Context, string) (git.WorktreeInfo, error) {
			return git.WorktreeInfo{Kind: git.MainWorktree, MainWorktreeRoot: `C:\repository`}, nil
		},
		LookupEnv:     func(string) (string, bool) { return "", false },
		UserConfigDir: func() (string, error) { return `C:\AppData`, nil },
		ReadFile: func(path string) ([]byte, error) {
			readPaths = append(readPaths, path)
			return nil, os.ErrNotExist
		},
		GOOS: "windows",
	})
	if _, err := loader.Load(context.Background(), `C:\repository`, ""); err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got, want := readPaths[0], filepath.Join(`C:\AppData`, "virga", "config.yaml"); got != want {
		t.Errorf("user configuration path = %q, want %q", got, want)
	}
}

func TestLoaderReturnsLookupAndRequiredFileErrors(t *testing.T) {
	t.Run("unavailable home directory", func(t *testing.T) {
		homeErr := errors.New("home unavailable")
		loader := NewLoader(LoaderDependencies{
			InspectWorktree: testInspector(t.TempDir()),
			LookupEnv:       func(string) (string, bool) { return "", false },
			UserHomeDir:     func() (string, error) { return "", homeErr },
			GOOS:            "linux",
		})
		_, err := loader.Load(context.Background(), "/repository", "")
		if !errors.Is(err, homeErr) {
			t.Fatalf("Load() error = %v, want wrapped %v", err, homeErr)
		}
	})

	t.Run("unavailable Windows configuration directory", func(t *testing.T) {
		directoryErr := errors.New("directory unavailable")
		loader := NewLoader(LoaderDependencies{
			InspectWorktree: testInspector(t.TempDir()),
			UserConfigDir:   func() (string, error) { return "", directoryErr },
			GOOS:            "windows",
		})
		_, err := loader.Load(context.Background(), "/repository", "")
		if !errors.Is(err, directoryErr) {
			t.Fatalf("Load() error = %v, want wrapped %v", err, directoryErr)
		}
	})

	t.Run("missing explicit configuration", func(t *testing.T) {
		root := t.TempDir()
		loader := testLoader(root, t.TempDir(), nil, "linux")
		_, err := loader.Load(context.Background(), root, filepath.Join(root, "missing.yaml"))
		if err == nil || !strings.Contains(err.Error(), "read explicit configuration") || !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("Load() error = %v, want missing explicit configuration error", err)
		}
	})
}

func TestLoaderReturnsRepositoryErrors(t *testing.T) {
	inspectErr := errors.New("Git unavailable")
	loader := NewLoader(LoaderDependencies{
		InspectWorktree: func(context.Context, string) (git.WorktreeInfo, error) { return git.WorktreeInfo{}, inspectErr },
		LookupEnv:       func(string) (string, bool) { return "", false },
		UserHomeDir:     func() (string, error) { return t.TempDir(), nil },
		GOOS:            "linux",
	})
	if _, err := loader.Load(context.Background(), "/repository", ""); !errors.Is(err, inspectErr) {
		t.Fatalf("Load() error = %v, want wrapped %v", err, inspectErr)
	}

	loader = NewLoader(LoaderDependencies{
		InspectWorktree: func(context.Context, string) (git.WorktreeInfo, error) {
			return git.WorktreeInfo{Kind: git.NotWorktree}, nil
		},
		LookupEnv:   func(string) (string, bool) { return "", false },
		UserHomeDir: func() (string, error) { return t.TempDir(), nil },
		GOOS:        "linux",
	})
	if _, err := loader.Load(context.Background(), "/repository", ""); !errors.Is(err, git.ErrNotGitRepository) {
		t.Fatalf("Load() error = %v, want %v", err, git.ErrNotGitRepository)
	}
}

func testLoader(root, home string, environment map[string]string, goos string) Loader {
	return NewLoader(LoaderDependencies{
		InspectWorktree: testInspector(root),
		LookupEnv: func(key string) (string, bool) {
			value, ok := environment[key]
			return value, ok
		},
		UserHomeDir: func() (string, error) { return home, nil },
		GOOS:        goos,
	})
}

func testInspector(root string) Inspector {
	return func(context.Context, string) (git.WorktreeInfo, error) {
		return git.WorktreeInfo{Kind: git.MainWorktree, MainWorktreeRoot: root}, nil
	}
}

func testConfigYAML(window string) string {
	return "tmux:\n  windows:\n    - name: " + window + "\n      panes:\n        - command: nvim\n"
}

func writeTestConfig(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create configuration directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write configuration: %v", err)
	}
}

func removeTestConfig(t *testing.T, path string) {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove configuration: %v", err)
	}
}

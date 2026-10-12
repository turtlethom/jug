package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/turtlethom/jug/internal/config"
	"github.com/turtlethom/jug/internal/shell"
)

// rootEnv points jug at a fresh $HOME and sets up a cd file the way the
// shell wrapper does (an empty temp file).
type rootEnv struct {
	home, configPath, legacyPath, cdFile string
}

func newRootEnv(t *testing.T) rootEnv {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	configPath, err := config.DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	legacyPath, err := config.LegacyPath()
	if err != nil {
		t.Fatal(err)
	}
	cdFile := filepath.Join(t.TempDir(), "cd")
	if err := os.WriteFile(cdFile, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(shell.CDFileEnv, cdFile)
	return rootEnv{home, configPath, legacyPath, cdFile}
}

func (e rootEnv) run(t *testing.T, menu menuFunc) (stdout, stderr string, err error) {
	t.Helper()
	root := newRootCmd(menu)
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(nil)
	err = root.Execute()
	return out.String(), errOut.String(), err
}

func (e rootEnv) cdTarget(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(e.cdFile)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRootChoosingWritesCDTarget(t *testing.T) {
	e := newRootEnv(t)
	_, _, err := e.run(t, func(*config.Config, string) (string, error) {
		return "/chosen/dir", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := e.cdTarget(t); got != "/chosen/dir" {
		t.Errorf("cd file: got %q, want /chosen/dir", got)
	}
}

// v1 bug: quitting with q or ctrl+c still cd'd to the highlighted item.
func TestRootQuitWritesNothing(t *testing.T) {
	e := newRootEnv(t)
	stdout, _, err := e.run(t, func(*config.Config, string) (string, error) {
		return "", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := e.cdTarget(t); got != "" {
		t.Errorf("quitting wrote %q to the cd file", got)
	}
	if stdout != "" {
		t.Errorf("quitting printed %q", stdout)
	}
}

func TestRootMigratesOnFirstRun(t *testing.T) {
	e := newRootEnv(t)
	writeFile(t, e.legacyPath, `{"shortcuts":[{"label":"A","path":"/a"},{"label":"B","path":"/b"}]}`)

	var got *config.Config
	var gotPath string
	_, stderr, err := e.run(t, func(cfg *config.Config, path string) (string, error) {
		got, gotPath = cfg, path
		return "", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got.Bookmarks) != 2 {
		t.Errorf("menu should get the 2 migrated bookmarks, got %+v", got)
	}
	if gotPath != e.configPath {
		t.Errorf("menu got config path %q, want %q", gotPath, e.configPath)
	}
	if !strings.Contains(stderr, "imported 2 bookmarks") {
		t.Errorf("want the migration notice on stderr, got %q", stderr)
	}
	if _, err := os.Stat(e.configPath); err != nil {
		t.Errorf("migrated config was not saved: %v", err)
	}
}

// v1 bug: a broken config was silently reset, wiping every bookmark.
func TestRootMalformedConfigStopsBeforeMenu(t *testing.T) {
	e := newRootEnv(t)
	broken := "[[bookmark]]\nlabel = \"NVIM\"\npath = \n"
	writeFile(t, e.configPath, broken)

	_, _, err := e.run(t, func(*config.Config, string) (string, error) {
		t.Error("the menu must not open with a broken config")
		return "", nil
	})
	if err == nil || !strings.Contains(err.Error(), e.configPath) {
		t.Errorf("want an error naming the config file, got %v", err)
	}
	b, _ := os.ReadFile(e.configPath)
	if string(b) != broken {
		t.Error("the broken config was modified")
	}
}

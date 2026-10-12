package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// setupHome points $HOME at a fresh temp dir and returns it.
func setupHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	return home
}

func mkdir(t *testing.T, parts ...string) string {
	t.Helper()
	dir := filepath.Join(parts...)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	mkdir(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestDefaultPath(t *testing.T) {
	home := setupHome(t)

	got, err := DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".config", "jug", "config.toml"); got != want {
		t.Errorf("without XDG_CONFIG_HOME: got %q, want %q", got, want)
	}

	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	got, err = DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(xdg, "jug", "config.toml"); got != want {
		t.Errorf("with XDG_CONFIG_HOME: got %q, want %q", got, want)
	}

	// The XDG spec says to ignore relative paths. Honoring one would move
	// the config depending on where jug was launched from.
	t.Setenv("XDG_CONFIG_HOME", "relative/config")
	got, err = DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".config", "jug", "config.toml"); got != want {
		t.Errorf("with relative XDG_CONFIG_HOME: got %q, want %q", got, want)
	}
}

func TestLegacyPath(t *testing.T) {
	setupHome(t)
	dir, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	got, err := LegacyPath()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "fcd", "fcd_config.json"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestLoadMissingFile(t *testing.T) {
	home := setupHome(t)
	path := filepath.Join(home, ".config", "jug", "config.toml")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("missing file should not be an error: %v", err)
	}
	if cfg == nil || len(cfg.Bookmarks) != 0 {
		t.Errorf("want empty config, got %+v", cfg)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Error("Load must not create the file")
	}
}

// The v1 bug: a broken config was replaced with an empty one, losing every bookmark.
func TestLoadMalformedKeepsFile(t *testing.T) {
	home := setupHome(t)
	path := filepath.Join(home, ".config", "jug", "config.toml")
	broken := "[[bookmark]]\nlabel = \"NVIM\"\npath = \n"
	writeFile(t, path, broken)

	if _, err := Load(path); err == nil {
		t.Error("malformed config should return an error")
	}
	if got := readFile(t, path); got != broken {
		t.Errorf("malformed config was modified:\n%s", got)
	}
}

func TestLoadExpandsHome(t *testing.T) {
	home := setupHome(t)
	path := filepath.Join(home, ".config", "jug", "config.toml")
	writeFile(t, path, `
[[bookmark]]
label = "HOME"
path = "~"

[[bookmark]]
label = "APP"
path = "~/projects/app/"
`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []Bookmark{
		{Label: "HOME", Path: home},
		{Label: "APP", Path: filepath.Join(home, "projects", "app")},
	}
	if !reflect.DeepEqual(cfg.Bookmarks, want) {
		t.Errorf("got %+v\nwant %+v", cfg.Bookmarks, want)
	}
}

// Save only writes known fields, so ignoring a typo like "lable" would
// silently delete it on the next save.
func TestLoadRejectsUnknownKeys(t *testing.T) {
	home := setupHome(t)
	path := filepath.Join(home, ".config", "jug", "config.toml")
	writeFile(t, path, "[[bookmark]]\nlable = \"NVIM\"\npath = \"/n\"\n")

	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), "lable") {
		t.Errorf("want an error naming the unknown key, got %v", err)
	}
}

// A hand-edited file can hold what Add would reject. Missing folders and
// duplicate paths are allowed; the menu shows them.
func TestLoadValidates(t *testing.T) {
	home := setupHome(t)
	path := filepath.Join(home, ".config", "jug", "config.toml")
	entry := func(label, p string) string {
		return fmt.Sprintf("[[bookmark]]\nlabel = %q\npath = %q\n", label, p)
	}

	bad := []struct{ name, toml string }{
		{"empty label", entry("  ", "/a")},
		{"duplicate label", entry("NVIM", "/a") + entry("nvim", "/b")},
		{"relative path", entry("REL", "projects/app")},
	}
	for _, tc := range bad {
		writeFile(t, path, tc.toml)
		if _, err := Load(path); err == nil {
			t.Errorf("%s: Load should fail", tc.name)
		}
	}

	writeFile(t, path, entry("GONE", "/does/not/exist")+entry("SAME", "/does/not/exist"))
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("missing folders and duplicate paths should load: %v", err)
	}
	if len(cfg.Bookmarks) != 2 {
		t.Errorf("want 2 bookmarks, got %+v", cfg.Bookmarks)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	home := setupHome(t)
	path := filepath.Join(home, ".config", "jug", "config.toml")
	outside := t.TempDir() // not under $HOME

	want := &Config{
		Bookmarks: []Bookmark{
			{Label: "APP", Path: filepath.Join(home, "projects", "app"), Group: "Projects"},
			{Label: "TMP", Path: outside},
		},
		Colors: Colors{Primary: "#56B6C2"},
	}
	snapshot := *want
	snapshot.Bookmarks = append([]Bookmark(nil), want.Bookmarks...)

	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(*want, snapshot) {
		t.Error("Save modified the config it was given")
	}

	disk := readFile(t, path)
	if !strings.Contains(disk, `"~/projects/app"`) {
		t.Errorf("paths inside $HOME should be written as ~/...; file:\n%s", disk)
	}
	if !strings.Contains(disk, outside) {
		t.Errorf("paths outside $HOME should be written as-is; file:\n%s", disk)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("round trip mismatch:\n got %+v\nwant %+v", got, want)
	}
}

func TestSaveCreatesDirsAndLeavesNoTempFiles(t *testing.T) {
	home := setupHome(t)
	dir := filepath.Join(home, "a", "b", "c")
	path := filepath.Join(dir, "config.toml")

	if err := Save(path, &Config{}); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "config.toml" {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("want only config.toml in %s, got %v", dir, names)
	}
}

// Dotfiles users symlink their config into a repo. Saving must not replace the link.
func TestSaveFollowsSymlink(t *testing.T) {
	home := setupHome(t)
	real := filepath.Join(home, "dotfiles", "jug.toml")
	writeFile(t, real, "")
	link := filepath.Join(mkdir(t, home, ".config", "jug"), "config.toml")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}

	cfg := &Config{Bookmarks: []Bookmark{{Label: "HOME", Path: home}}}
	if err := Save(link, cfg); err != nil {
		t.Fatal(err)
	}

	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Error("Save replaced the symlink with a regular file")
	}
	if !strings.Contains(readFile(t, real), "HOME") {
		t.Error("Save did not write through the symlink to the real file")
	}

	// The backup goes next to the link, not into the dotfiles repo.
	if fi, err := os.Lstat(link + ".bak"); err != nil || !fi.Mode().IsRegular() {
		t.Errorf("want a regular backup file at %s (err: %v)", link+".bak", err)
	}
	if _, err := os.Stat(real + ".bak"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Save left a backup in the dotfiles repo (stat err: %v)", err)
	}
}

func TestSaveContractsOnlyInsideHome(t *testing.T) {
	home := setupHome(t)
	path := filepath.Join(home, ".config", "jug", "config.toml")
	cfg := &Config{Bookmarks: []Bookmark{
		{Label: "HOME", Path: home},
		{Label: "SIBLING", Path: home + "2"}, // /home/tom2 is not inside /home/tom
	}}
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	disk := readFile(t, path)
	if !strings.Contains(disk, `path = "~"`) {
		t.Errorf("$HOME itself should be written as ~; file:\n%s", disk)
	}
	if !strings.Contains(disk, fmt.Sprintf("%q", home+"2")) {
		t.Errorf("a sibling of $HOME should be written as-is; file:\n%s", disk)
	}
}

func TestSaveKeepsPermissions(t *testing.T) {
	home := setupHome(t)
	path := filepath.Join(home, ".config", "jug", "config.toml")

	if err := Save(path, &Config{}); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o644 {
		t.Errorf("new config: want 0644, got %v", fi.Mode().Perm())
	}

	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, &Config{}); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Errorf("existing 0600 config: want 0600 kept, got %v", fi.Mode().Perm())
	}
}

func TestSaveDanglingSymlink(t *testing.T) {
	home := setupHome(t)
	link := filepath.Join(mkdir(t, home, ".config", "jug"), "config.toml")
	missing := filepath.Join(home, "dotfiles", "jug.toml")
	if err := os.Symlink(missing, link); err != nil {
		t.Fatal(err)
	}

	if err := Save(link, &Config{}); err == nil {
		t.Error("saving through a symlink to a missing file should fail")
	}
	if _, err := os.Stat(missing); !errors.Is(err, os.ErrNotExist) {
		t.Error("Save created the symlink's missing target")
	}
}

// A mistaken clear in the menu must be recoverable by hand.
func TestSaveKeepsBackup(t *testing.T) {
	home := setupHome(t)
	path := filepath.Join(home, ".config", "jug", "config.toml")

	first := &Config{Bookmarks: []Bookmark{{Label: "FIRST", Path: home}}}
	if err := Save(path, first); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path + ".bak"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("first save of a new config should make no backup (stat err: %v)", err)
	}

	before := readFile(t, path)
	if err := Save(path, &Config{}); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path+".bak"); got != before {
		t.Errorf("backup should hold the previous config:\n got %q\nwant %q", got, before)
	}

	// Only one level of backup: the next save replaces it.
	before = readFile(t, path)
	if err := Save(path, first); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, path+".bak"); got != before {
		t.Errorf("second backup should replace the first:\n got %q\nwant %q", got, before)
	}
}

const fcdJSON = `{
  "shortcuts": [
    {"label": "FCD", "path": "/home/someone/linux_tools/fcd"},
    {"label": "GONE", "path": "/this/directory/does/not/exist"}
  ],
  "userColors": {"primary": "#56B6C2", "secondary": "", "tertiary": ""}
}`

func TestMigrateFCD(t *testing.T) {
	setupHome(t)
	path := filepath.Join(t.TempDir(), "fcd_config.json")
	writeFile(t, path, fcdJSON)

	got, err := MigrateFCD(path)
	if err != nil {
		t.Fatal(err)
	}
	want := &Config{
		Bookmarks: []Bookmark{
			{Label: "FCD", Path: "/home/someone/linux_tools/fcd"},
			{Label: "GONE", Path: "/this/directory/does/not/exist"},
		},
		Colors: Colors{Primary: "#56B6C2"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
	if readFile(t, path) != fcdJSON {
		t.Error("MigrateFCD modified the v1 JSON file")
	}
}

func TestLoadOrMigrate(t *testing.T) {
	t.Run("new config wins", func(t *testing.T) {
		home := setupHome(t)
		path := filepath.Join(home, ".config", "jug", "config.toml")
		legacy := filepath.Join(home, ".config", "fcd", "fcd_config.json")
		if err := Save(path, &Config{Bookmarks: []Bookmark{{Label: "NEW", Path: home}}}); err != nil {
			t.Fatal(err)
		}
		writeFile(t, legacy, fcdJSON)

		cfg, migrated, err := LoadOrMigrate(path, legacy)
		if err != nil {
			t.Fatal(err)
		}
		if migrated || len(cfg.Bookmarks) != 1 || cfg.Bookmarks[0].Label != "NEW" {
			t.Errorf("want the existing jug config, got migrated=%v %+v", migrated, cfg)
		}
	})

	t.Run("migrates fcd once", func(t *testing.T) {
		home := setupHome(t)
		path := filepath.Join(home, ".config", "jug", "config.toml")
		legacy := filepath.Join(home, ".config", "fcd", "fcd_config.json")
		writeFile(t, legacy, fcdJSON)

		cfg, migrated, err := LoadOrMigrate(path, legacy)
		if err != nil {
			t.Fatal(err)
		}
		if !migrated || len(cfg.Bookmarks) != 2 {
			t.Errorf("want 2 migrated bookmarks, got migrated=%v %+v", migrated, cfg)
		}
		if _, err := os.Stat(path); err != nil {
			t.Errorf("migrated config was not saved: %v", err)
		}
		if readFile(t, legacy) != fcdJSON {
			t.Error("legacy file must be left untouched")
		}

		_, migrated, err = LoadOrMigrate(path, legacy)
		if err != nil || migrated {
			t.Errorf("second run should load the new config, got migrated=%v err=%v", migrated, err)
		}
	})

	t.Run("fresh install", func(t *testing.T) {
		home := setupHome(t)
		path := filepath.Join(home, ".config", "jug", "config.toml")
		legacy := filepath.Join(home, ".config", "fcd", "fcd_config.json")

		cfg, migrated, err := LoadOrMigrate(path, legacy)
		if err != nil {
			t.Fatal(err)
		}
		if migrated || cfg == nil || len(cfg.Bookmarks) != 0 {
			t.Errorf("want empty config, got migrated=%v %+v", migrated, cfg)
		}
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Error("a fresh install should not create a config file until something is saved")
		}
	})

	// Saving it would leave a jug config that the next startup refuses.
	t.Run("bad fcd config is not saved", func(t *testing.T) {
		home := setupHome(t)
		path := filepath.Join(home, ".config", "jug", "config.toml")
		legacy := filepath.Join(home, ".config", "fcd", "fcd_config.json")
		bad := `{"shortcuts":[{"label":"NVIM","path":"/a"},{"label":"nvim","path":"/b"}]}`
		writeFile(t, legacy, bad)

		_, _, err := LoadOrMigrate(path, legacy)
		if err == nil || !strings.Contains(err.Error(), legacy) {
			t.Errorf("want an error naming the fcd file, got %v", err)
		}
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Error("a rejected migration must not create a jug config")
		}
		if readFile(t, legacy) != bad {
			t.Error("legacy file must be left untouched")
		}
	})
}

func TestAdd(t *testing.T) {
	home := setupHome(t)
	projects := mkdir(t, home, "projects")
	nvim := mkdir(t, home, ".config", "nvim")
	outside := t.TempDir()
	file := filepath.Join(home, "notes.txt")
	writeFile(t, file, "")

	cfg := &Config{}
	adds := []struct{ label, dir string }{
		{"  PROJ  ", projects},
		{"NVIM", "~/.config/nvim"},
		{"OUT", outside},
	}
	for _, tc := range adds {
		if err := cfg.Add(tc.label, tc.dir); err != nil {
			t.Fatalf("Add(%q, %q): %v", tc.label, tc.dir, err)
		}
	}
	want := []Bookmark{
		{Label: "PROJ", Path: projects},
		{Label: "NVIM", Path: nvim},
		{Label: "OUT", Path: outside},
	}
	if !reflect.DeepEqual(cfg.Bookmarks, want) {
		t.Errorf("bookmarks:\n got %+v\nwant %+v", cfg.Bookmarks, want)
	}

	if err := cfg.Add("CLEAN", projects+"/../projects/./"); !errors.Is(err, ErrDuplicate) {
		t.Errorf("same path written differently: want ErrDuplicate, got %v", err)
	}
	if err := cfg.Add("nvim", outside); !errors.Is(err, ErrDuplicate) {
		t.Errorf("case-insensitive label clash: want ErrDuplicate, got %v", err)
	}

	bad := []struct{ name, label, dir string }{
		{"empty label", "   ", projects},
		{"missing dir", "GONE", filepath.Join(home, "nope")},
		{"not a dir", "FILE", file},
	}
	for _, tc := range bad {
		if err := cfg.Add(tc.label, tc.dir); err == nil {
			t.Errorf("%s: Add(%q, %q) should fail", tc.name, tc.label, tc.dir)
		}
	}
	if len(cfg.Bookmarks) != 3 {
		t.Errorf("failed Adds must not change the config; have %d bookmarks", len(cfg.Bookmarks))
	}
}

func TestAddRelativePath(t *testing.T) {
	home := setupHome(t)
	mkdir(t, home, "sub")
	t.Chdir(home)
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	cfg := &Config{}
	if err := cfg.Add("SUB", "sub"); err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(wd, "sub"); cfg.Bookmarks[0].Path != want {
		t.Errorf("got %q, want %q", cfg.Bookmarks[0].Path, want)
	}
}

func TestRemoveAndFind(t *testing.T) {
	cfg := &Config{Bookmarks: []Bookmark{
		{Label: "A", Path: "/a"},
		{Label: "NVIM", Path: "/n"},
		{Label: "C", Path: "/c"},
	}}

	if b, ok := cfg.Find("nvim"); !ok || b.Path != "/n" {
		t.Errorf("Find(nvim) = %+v, %v", b, ok)
	}
	if _, ok := cfg.Find("missing"); ok {
		t.Error("Find(missing) should be false")
	}

	if err := cfg.Remove("Nvim"); err != nil {
		t.Fatal(err)
	}
	want := []Bookmark{{Label: "A", Path: "/a"}, {Label: "C", Path: "/c"}}
	if !reflect.DeepEqual(cfg.Bookmarks, want) {
		t.Errorf("after Remove: %+v", cfg.Bookmarks)
	}
	if err := cfg.Remove("NVIM"); !errors.Is(err, ErrNotFound) {
		t.Errorf("removing twice: want ErrNotFound, got %v", err)
	}
}

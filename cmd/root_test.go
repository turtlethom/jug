package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/turtlethom/pathpin/internal/config"
	"github.com/turtlethom/pathpin/internal/shell"
	"github.com/turtlethom/pathpin/internal/ui"
)

// rootEnv points pathpin at a fresh $HOME and sets up a cd file the way the
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
	t.Setenv(shell.CmdEnv, "") // the default name, unless a test picks one
	return rootEnv{home, configPath, legacyPath, cdFile}
}

func (e rootEnv) run(t *testing.T, menu menuFunc, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	root := newRootCmd(menu)
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(args)
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

// noMenu fails the test if the menu opens.
func noMenu(t *testing.T) menuFunc {
	return func(*config.Config, string) (string, error) {
		t.Error("the menu should not open")
		return "", nil
	}
}

// withBookmarks saves a pathpin config with these labels, each pointing at a
// folder under $HOME. Paths starting with "!" are left missing on disk.
func (e rootEnv) withBookmarks(t *testing.T, labelsAndPaths ...string) {
	t.Helper()
	cfg := &config.Config{}
	for i := 0; i < len(labelsAndPaths); i += 2 {
		rel := labelsAndPaths[i+1]
		missing := strings.HasPrefix(rel, "!")
		dir := filepath.Join(e.home, strings.TrimPrefix(rel, "!"))
		if !missing {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		cfg.Bookmarks = append(cfg.Bookmarks, config.Bookmark{Label: labelsAndPaths[i], Path: dir})
	}
	if err := config.Save(e.configPath, cfg); err != nil {
		t.Fatal(err)
	}
}

func (e rootEnv) saved(t *testing.T) []config.Bookmark {
	t.Helper()
	cfg, err := config.Load(e.configPath)
	if err != nil {
		t.Fatal(err)
	}
	return cfg.Bookmarks
}

func TestJumpWritesCDTarget(t *testing.T) {
	e := newRootEnv(t)
	e.withBookmarks(t, "NVIM", ".config/nvim", "Day Job", "work")

	if _, _, err := e.run(t, noMenu(t), "nvim"); err != nil {
		t.Fatal(err)
	}
	if got, want := e.cdTarget(t), filepath.Join(e.home, ".config", "nvim"); got != want {
		t.Errorf("pathpin nvim: cd file %q, want %q", got, want)
	}

	// Words are joined, so labels with spaces need no quotes.
	if _, _, err := e.run(t, noMenu(t), "day", "job"); err != nil {
		t.Fatal(err)
	}
	if got, want := e.cdTarget(t), filepath.Join(e.home, "work"); got != want {
		t.Errorf("pathpin day job: cd file %q, want %q", got, want)
	}
}

func TestJumpUnknownLabel(t *testing.T) {
	e := newRootEnv(t)
	e.withBookmarks(t, "NVIM", ".config/nvim")
	_, _, err := e.run(t, noMenu(t), "nvm")
	if err == nil || !strings.Contains(err.Error(), `no bookmark "nvm" (see 'pathpin ls')`) {
		t.Errorf("want a no-bookmark error, got %v", err)
	}
	if e.cdTarget(t) != "" {
		t.Error("an unknown label must not cd")
	}
}

func TestJumpMissingFolder(t *testing.T) {
	e := newRootEnv(t)
	e.withBookmarks(t, "OLD", "!old/project")
	_, _, err := e.run(t, noMenu(t), "old")
	if err == nil || !strings.Contains(err.Error(), "~/old/project no longer exists") {
		t.Errorf("want a missing-folder error, got %v", err)
	}
	if e.cdTarget(t) != "" {
		t.Error("a missing folder must not cd")
	}
}

func TestAddSuggestsLabel(t *testing.T) {
	e := newRootEnv(t)
	dir := filepath.Join(e.home, "projects", "app")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	_, stderr, err := e.run(t, noMenu(t), "add")
	if err != nil {
		t.Fatal(err)
	}
	if got := e.saved(t); len(got) != 1 || got[0].Label != "APP" {
		t.Errorf("want APP saved, got %+v", got)
	}
	if !strings.Contains(stderr, "pathpin: added APP → ~/projects/app") {
		t.Errorf("want a confirmation on stderr, got %q", stderr)
	}
}

func TestAddTypedLabelAndDuplicate(t *testing.T) {
	e := newRootEnv(t)
	dir := filepath.Join(e.home, "work")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	if _, _, err := e.run(t, noMenu(t), "add", "Day", "Job"); err != nil {
		t.Fatal(err)
	}
	if got := e.saved(t); len(got) != 1 || got[0].Label != "Day Job" {
		t.Errorf("want the typed label as-is, got %+v", got)
	}
	_, _, err := e.run(t, noMenu(t), "add", "OTHER")
	if err == nil || !strings.Contains(err.Error(), `already saved as "Day Job"`) {
		t.Errorf("adding the same folder twice should fail, got %v", err)
	}
}

func TestAddReservedLabelWarnsButSaves(t *testing.T) {
	e := newRootEnv(t)
	t.Chdir(e.home)
	_, stderr, err := e.run(t, noMenu(t), "add", "init")
	if err != nil {
		t.Fatal(err)
	}
	if got := e.saved(t); len(got) != 1 || got[0].Label != "init" {
		t.Errorf("the bookmark should still be saved, got %+v", got)
	}
	if !strings.Contains(stderr, `"init" is also a command`) || !strings.Contains(stderr, "'pathpin INIT'") {
		t.Errorf("want the reserved-label note, got %q", stderr)
	}
}

// In tests stdout is a buffer, like a pipe: tab-separated absolute paths.
func TestLsPiped(t *testing.T) {
	e := newRootEnv(t)
	e.withBookmarks(t, "NVIM", ".config/nvim", "GONE", "!old")
	stdout, _, err := e.run(t, noMenu(t), "ls")
	if err != nil {
		t.Fatal(err)
	}
	// Always three columns; the group is empty for ungrouped bookmarks.
	want := "NVIM\t" + filepath.Join(e.home, ".config", "nvim") + "\t\nGONE\t" + filepath.Join(e.home, "old") + "\t\n"
	if stdout != want {
		t.Errorf("got %q\nwant %q", stdout, want)
	}
}

func TestLsTerminalFormat(t *testing.T) {
	e := newRootEnv(t)
	e.withBookmarks(t, "NVIM", ".config/nvim", "OLDPROJ", "!old/project")
	got := formatList(e.saved(t))
	want := "NVIM     ~/.config/nvim\nOLDPROJ  ~/old/project  (missing)\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// Labels complete in lowercase, so `pathpin no<TAB>` gives `pathpin notes`.
// Shells drop candidates whose case doesn't match what was typed, so once a
// capital letter is typed (`pathpin NO<TAB>`), they complete in uppercase.
func TestCompletesLabels(t *testing.T) {
	e := newRootEnv(t)
	e.withBookmarks(t, "NVIM", ".config/nvim", "Notes", "notes")
	for _, c := range []struct {
		typed     string
		want, not []string
	}{
		{"", []string{"nvim", "notes"}, []string{"NVIM", "Notes"}},
		{"n", []string{"nvim", "notes"}, []string{"NVIM", "Notes"}},
		{"N", []string{"NVIM", "NOTES"}, []string{"nvim", "Notes"}},
		{"nO", []string{"NVIM", "NOTES"}, []string{"nvim", "Notes"}},
	} {
		stdout, _, err := e.run(t, noMenu(t), "__complete", c.typed)
		if err != nil {
			t.Fatal(err)
		}
		for _, label := range c.want {
			if !strings.Contains(stdout, label+"\n") {
				t.Errorf("typed %q: completions should include %s, got %q", c.typed, label, stdout)
			}
		}
		for _, label := range c.not {
			if strings.Contains(stdout, label+"\n") {
				t.Errorf("typed %q: completions should not include %s, got %q", c.typed, label, stdout)
			}
		}
	}
}

// A label named like a subcommand completes in uppercase: `pathpin ls` would
// run the ls command, while `pathpin LS` jumps.
func TestCompletesCommandNamedLabelInUppercase(t *testing.T) {
	e := newRootEnv(t)
	e.withBookmarks(t, "Ls", "listings", "NOTES", "notes")
	stdout, _, err := e.run(t, noMenu(t), "__complete", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "LS\n") || strings.Contains(stdout, "ls\n") {
		t.Errorf("want LS, not ls, got %q", stdout)
	}
	if !strings.Contains(stdout, "notes\n") {
		t.Errorf("other labels should stay lowercase, got %q", stdout)
	}
}

// `pathpin add <TAB>` is typing a new label, so it offers no file names.
func TestAddCompletesNoFiles(t *testing.T) {
	e := newRootEnv(t)
	stdout, _, err := e.run(t, noMenu(t), "__complete", "add", "")
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf(":%d\n", cobra.ShellCompDirectiveNoFileComp)
	if !strings.HasSuffix(stdout, want) {
		t.Errorf("want directive %q (no file completion), got %q", want, stdout)
	}
}

// With --cmd pp, the wrapper sets PATHPIN_CMD=pp and messages use it.
func TestMessagesUseChosenName(t *testing.T) {
	e := newRootEnv(t)
	e.withBookmarks(t, "NVIM", ".config/nvim")
	t.Setenv(shell.CmdEnv, "pp")
	_, _, err := e.run(t, noMenu(t), "nvm")
	if err == nil || !strings.Contains(err.Error(), "(see 'pp ls')") {
		t.Errorf("want the error to name pp, got %v", err)
	}
	stdout, _, err := e.run(t, noMenu(t), "--help")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, "pp [label]") || !strings.Contains(stdout, "pathpin init --help") {
		t.Errorf("help should use pp, but setup instructions the binary name:\n%s", stdout)
	}
}

// withTheme writes a config with one bookmark (NOTES → ~/notes) and this theme.
func (e rootEnv) withTheme(t *testing.T, theme string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(e.home, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, e.configPath, fmt.Sprintf(
		"[[bookmark]]\nlabel = \"NOTES\"\npath = \"~/notes\"\n\n[colors]\ntheme = %q\n", theme))
}

// A misspelled theme still opens the menu (in terminal colors) and says why
// the colors look wrong, listing the themes there are.
func TestUnknownThemeWarns(t *testing.T) {
	e := newRootEnv(t)
	e.withTheme(t, "nrod")
	opened := false
	_, stderr, err := e.run(t, func(*config.Config, string) (string, error) {
		opened = true
		return "", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !opened {
		t.Error("an unknown theme should not stop the menu")
	}
	if !strings.Contains(stderr, `unknown theme "nrod"`) || strings.Count(stderr, "\n") != 1 {
		t.Errorf("want one line naming the theme, got %q", stderr)
	}
	for _, name := range ui.ThemeNames() {
		if !strings.Contains(stderr, name) {
			t.Errorf("the warning should list %s, got %q", name, stderr)
		}
	}
}

// Only the menu uses colors, so jumping doesn't warn, and a known theme
// (in any case) never does.
func TestThemeWarningOnlyWhenNeeded(t *testing.T) {
	e := newRootEnv(t)
	e.withTheme(t, "nrod")
	if _, stderr, err := e.run(t, noMenu(t), "notes"); err != nil || stderr != "" {
		t.Errorf("pathpin notes: want no warning, got err %v, stderr %q", err, stderr)
	}

	e.withTheme(t, "Nord")
	_, stderr, err := e.run(t, func(*config.Config, string) (string, error) { return "", nil })
	if err != nil || stderr != "" {
		t.Errorf("a known theme should not warn, got err %v, stderr %q", err, stderr)
	}
}

// withColors writes a config with one bookmark (NOTES → ~/notes) and these
// [colors] lines.
func (e rootEnv) withColors(t *testing.T, lines string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(e.home, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, e.configPath, "[[bookmark]]\nlabel = \"NOTES\"\npath = \"~/notes\"\n\n[colors]\n"+lines)
}

// An unknown [menu] selection gets one warning line when the menu opens, and
// the menu still opens (in block mode). Jumps don't warn, and the known
// values, in any case, never do.
func TestUnknownSelectionWarns(t *testing.T) {
	e := newRootEnv(t)
	e.withColors(t, "[menu]\nselection = \"bars\"\n")
	opened := false
	_, stderr, err := e.run(t, func(*config.Config, string) (string, error) {
		opened = true
		return "", nil
	})
	if err != nil || !opened {
		t.Fatalf("an unknown selection should not stop the menu (err %v, opened %v)", err, opened)
	}
	want := `[menu] selection = "bars" is not block or bar, using block`
	if !strings.Contains(stderr, want) || strings.Count(stderr, "\n") != 1 {
		t.Errorf("want one line %q, got %q", want, stderr)
	}
	if _, stderr, err := e.run(t, noMenu(t), "notes"); err != nil || stderr != "" {
		t.Errorf("pathpin notes: want no warning, got err %v, stderr %q", err, stderr)
	}

	for _, ok := range []string{"bar", "Block", ""} {
		e.withColors(t, fmt.Sprintf("[menu]\nselection = %q\n", ok))
		_, stderr, err := e.run(t, func(*config.Config, string) (string, error) { return "", nil })
		if err != nil || stderr != "" {
			t.Errorf("selection %q should not warn, got err %v, stderr %q", ok, err, stderr)
		}
	}
}

// A [colors] value that isn't a color gets one warning line each when the
// menu opens. A theme name in a color key suggests the theme key instead.
func TestInvalidColorWarns(t *testing.T) {
	e := newRootEnv(t)
	e.withColors(t, "primary = \"monokai-pro-ristretto\"\ntertiary = \"grey\"\nsecondary = \"#56B6C2\"\n")
	opened := false
	_, stderr, err := e.run(t, func(*config.Config, string) (string, error) {
		opened = true
		return "", nil
	})
	if err != nil || !opened {
		t.Fatalf("invalid colors should not stop the menu: err %v, opened %v", err, opened)
	}
	lines := strings.Split(strings.TrimSuffix(stderr, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("want one line per invalid color, got %q", stderr)
	}
	if !strings.Contains(lines[0], `primary = "monokai-pro-ristretto" is not a color`) ||
		!strings.Contains(lines[0], `theme = "monokai-pro-ristretto"`) {
		t.Errorf("want the bad primary and a theme suggestion, got %q", lines[0])
	}
	if !strings.Contains(lines[1], `tertiary = "grey" is not a color`) || !strings.Contains(lines[1], "#RRGGBB") {
		t.Errorf("want the bad tertiary and the accepted formats, got %q", lines[1])
	}

	// Jumping doesn't use colors, so it doesn't warn.
	if _, stderr, err := e.run(t, noMenu(t), "notes"); err != nil || stderr != "" {
		t.Errorf("pathpin notes: want no warning, got err %v, stderr %q", err, stderr)
	}
}

// withGroups sets the group of each labeled bookmark straight in the file,
// bypassing SetGroup, so tests can also write what only a hand edit could.
func (e rootEnv) withGroups(t *testing.T, labelsAndGroups ...string) {
	t.Helper()
	cfg, err := config.Load(e.configPath)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < len(labelsAndGroups); i += 2 {
		for j := range cfg.Bookmarks {
			if cfg.Bookmarks[j].Label == labelsAndGroups[i] {
				cfg.Bookmarks[j].Group = labelsAndGroups[i+1]
			}
		}
	}
	if err := config.Save(e.configPath, cfg); err != nil {
		t.Fatal(err)
	}
}

// groupOf is the saved group of the bookmark with this label.
func (e rootEnv) groupOf(t *testing.T, label string) string {
	t.Helper()
	for _, b := range e.saved(t) {
		if b.Label == label {
			return b.Group
		}
	}
	t.Fatalf("%s isn't saved", label)
	return ""
}

// pathpin add --group (or -g) saves into a group: an existing group in any
// case keeps its spelling, and the menu's tab names are refused with nothing
// saved.
func TestAddGroupFlag(t *testing.T) {
	e := newRootEnv(t)
	e.withBookmarks(t, "NOTES", "notes")
	e.withGroups(t, "NOTES", "School")

	app := filepath.Join(e.home, "projects", "app")
	if err := os.MkdirAll(app, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(app)
	_, stderr, err := e.run(t, noMenu(t), "add", "--group", "school")
	if err != nil {
		t.Fatal(err)
	}
	if got := e.groupOf(t, "APP"); got != "School" {
		t.Errorf("--group school: got %q, want School", got)
	}
	if !strings.Contains(stderr, "added APP → ~/projects/app (School)") {
		t.Errorf("want the group in the confirmation, got %q", stderr)
	}

	web := filepath.Join(e.home, "projects", "web")
	if err := os.MkdirAll(web, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(web)
	if _, _, err := e.run(t, noMenu(t), "add", "-g", "Tools", "site"); err != nil {
		t.Fatal(err)
	}
	if got := e.groupOf(t, "site"); got != "Tools" {
		t.Errorf("-g Tools: got %q, want Tools", got)
	}

	other := filepath.Join(e.home, "projects", "other")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(other)
	_, _, err = e.run(t, noMenu(t), "add", "--group", "Other")
	if err == nil || !strings.Contains(err.Error(), "tab names") {
		t.Errorf("--group Other should fail naming the tab names, got %v", err)
	}
	if got := len(e.saved(t)); got != 3 {
		t.Errorf("a refused group must save nothing: want 3 bookmarks, got %d", got)
	}
}

// pathpin add --group <TAB> completes group names (not the tab names).
func TestAddGroupCompletion(t *testing.T) {
	e := newRootEnv(t)
	e.withBookmarks(t, "NOTES", "notes", "BASH", "bash", "OLD", "old")
	e.withGroups(t, "NOTES", "School", "BASH", "Config", "OLD", "other")
	stdout, _, err := e.run(t, noMenu(t), "__complete", "add", "--group", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range []string{"School", "Config"} {
		if !strings.Contains(stdout, g+"\n") {
			t.Errorf("want %s offered, got %q", g, stdout)
		}
	}
	if strings.Contains(stdout, "other\n") {
		t.Errorf("tab names aren't groups to offer, got %q", stdout)
	}
	if want := fmt.Sprintf(":%d\n", cobra.ShellCompDirectiveNoFileComp); !strings.HasSuffix(stdout, want) {
		t.Errorf("want no file completion (%q), got %q", want, stdout)
	}
}

// In a terminal, ls lists bookmarks under group headings in tab order (groups
// as they first appear, then Other), indented, with labels aligned across
// groups. Hand-edited groups named like a tab count as Other.
func TestLsTerminalGroups(t *testing.T) {
	e := newRootEnv(t)
	e.withBookmarks(t, "BASH", "bash", "CS310", "cs310", "PATHPIN", "pathpin", "GHOSTTY", "ghostty", "OLD", "!old")
	e.withGroups(t, "BASH", "Config", "CS310", "School", "GHOSTTY", "config", "OLD", "other")
	got := formatList(e.saved(t))
	want := `Config
  BASH     ~/bash
  GHOSTTY  ~/ghostty

School
  CS310    ~/cs310

Other
  PATHPIN  ~/pathpin
  OLD      ~/old  (missing)
`
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// Piped, ls keeps config order and adds each bookmark's group (its first
// spelling; empty for ungrouped, including hand-edited tab names) as a third
// column, so cut -f1,2 still works.
func TestLsPipedGroups(t *testing.T) {
	e := newRootEnv(t)
	e.withBookmarks(t, "BASH", "bash", "PATHPIN", "pathpin", "GHOSTTY", "ghostty", "OLD", "old")
	e.withGroups(t, "BASH", "Config", "GHOSTTY", "config", "OLD", "ALL")
	stdout, _, err := e.run(t, noMenu(t), "ls")
	if err != nil {
		t.Fatal(err)
	}
	abs := func(rel string) string { return filepath.Join(e.home, rel) }
	want := "BASH\t" + abs("bash") + "\tConfig\n" +
		"PATHPIN\t" + abs("pathpin") + "\t\n" +
		"GHOSTTY\t" + abs("ghostty") + "\tConfig\n" +
		"OLD\t" + abs("old") + "\t\n"
	if stdout != want {
		t.Errorf("got %q\nwant %q", stdout, want)
	}
}

// ls --help shows the read example with a literal \t, and no control
// characters that would print as blank space.
func TestLsHelpText(t *testing.T) {
	e := newRootEnv(t)
	stdout, _, err := e.run(t, noMenu(t), "ls", "--help")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range stdout {
		if r < 0x20 && r != '\n' {
			t.Fatalf("help has control character %q:\n%s", r, stdout)
		}
	}
	if !strings.Contains(stdout, `while IFS=$'\t' read -r label path group`) {
		t.Errorf("want the read example:\n%s", stdout)
	}
}

// --group completion keeps what was typed and completes the rest, since
// shells drop candidates whose case doesn't match (groups match in any case).
func TestAddGroupCompletionKeepsTypedCase(t *testing.T) {
	e := newRootEnv(t)
	e.withBookmarks(t, "NOTES", "notes", "BASH", "bash")
	e.withGroups(t, "NOTES", "School", "BASH", "Config")
	for typed, want := range map[string]string{"sc": "school", "Sc": "School", "CO": "COnfig"} {
		stdout, _, err := e.run(t, noMenu(t), "__complete", "add", "--group", typed)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(stdout, want+"\n") {
			t.Errorf("typed %q: want %q offered, got %q", typed, want, stdout)
		}
	}
	stdout, _, _ := e.run(t, noMenu(t), "__complete", "add", "--group", "zz")
	if strings.Contains(stdout, "School") || strings.Contains(stdout, "Config") {
		t.Errorf("typed zz: want nothing offered, got %q", stdout)
	}
}

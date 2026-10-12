package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/turtlethom/jug/internal/config"
)

// These tests call Update and View directly: a Bubble Tea model is just a
// value, and Update is just a function from (model, message) to (model, command).

// newTestModel builds a sized menu over bookmarks under a fresh $HOME. Paths
// starting with "!" are left missing on disk; the rest are created.
func newTestModel(t *testing.T, bookmarks ...config.Bookmark) Model {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	for i, b := range bookmarks {
		missing := strings.HasPrefix(b.Path, "!")
		bookmarks[i].Path = filepath.Join(home, strings.TrimPrefix(b.Path, "!"))
		if !missing {
			if err := os.MkdirAll(bookmarks[i].Path, 0o755); err != nil {
				t.Fatal(err)
			}
		}
	}
	cfg := &config.Config{Bookmarks: bookmarks}
	configPath := filepath.Join(home, ".config", "jug", "config.toml")
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	m := New(cfg, configPath)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return next.(Model)
}

// onDisk loads the config the menu saved to.
func onDisk(t *testing.T, m Model) []config.Bookmark {
	t.Helper()
	cfg, err := config.Load(m.configPath)
	if err != nil {
		t.Fatal(err)
	}
	return cfg.Bookmarks
}

func twoBookmarks(t *testing.T) Model {
	return newTestModel(t,
		config.Bookmark{Label: "NVIM", Path: ".config/nvim"},
		config.Bookmark{Label: "NOTES", Path: "notes"},
	)
}

func press(m Model, key tea.KeyMsg) (Model, tea.Cmd) {
	next, cmd := m.Update(key)
	return next.(Model), cmd
}

func runes(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

// isQuit runs cmd, so only call it when cmd can't be a timer.
func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func TestEnterChoosesHighlighted(t *testing.T) {
	m, cmd := press(twoBookmarks(t), tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.HasSuffix(m.chosen, filepath.Join(".config", "nvim")) {
		t.Errorf("chosen: got %q", m.chosen)
	}
	if !isQuit(cmd) {
		t.Error("enter should quit")
	}
}

// v1 bug: quitting still cd'd to the highlighted item.
func TestQuitKeysChooseNothing(t *testing.T) {
	keys := map[string]tea.KeyMsg{
		"q":      runes("q"),
		"esc":    {Type: tea.KeyEsc},
		"ctrl+c": {Type: tea.KeyCtrlC},
	}
	for name, key := range keys {
		m, cmd := press(twoBookmarks(t), key)
		if m.chosen != "" {
			t.Errorf("%s: chose %q", name, m.chosen)
		}
		if !isQuit(cmd) {
			t.Errorf("%s should quit", name)
		}
	}
}

// v1 bug: q quit even while typing in the filter.
func TestQWhileFilteringIsTyped(t *testing.T) {
	m, _ := press(twoBookmarks(t), runes("/"))
	if m.list.FilterState() != list.Filtering {
		t.Fatalf("/ should start filtering, state is %v", m.list.FilterState())
	}
	// Don't run the returned command: while filtering it includes the
	// cursor-blink timer, which sleeps.
	m, _ = press(m, runes("q"))
	if m.list.FilterState() != list.Filtering || m.list.FilterValue() != "q" {
		t.Errorf("q should be typed into the filter; state %v, value %q",
			m.list.FilterState(), m.list.FilterValue())
	}
	if m.chosen != "" {
		t.Errorf("chose %q while filtering", m.chosen)
	}
}

func TestRowsShowHomeAsTilde(t *testing.T) {
	view := twoBookmarks(t).View()
	if !strings.Contains(view, "~/.config/nvim") {
		t.Errorf("want ~/.config/nvim in the view:\n%s", view)
	}
}

func TestMissingRowRefusesEnter(t *testing.T) {
	m := newTestModel(t, config.Bookmark{Label: "GONE", Path: "!old/project"})
	if !strings.Contains(m.View(), "GONE (missing)") {
		t.Errorf("want a (missing) tag:\n%s", m.View())
	}

	// Don't run the command: it's the status message's timer.
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.chosen != "" {
		t.Errorf("enter on a missing folder chose %q", m.chosen)
	}
	if !strings.Contains(m.View(), "~/old/project no longer exists") {
		t.Errorf("want a status message:\n%s", m.View())
	}
}

// The folder may come back after the menu opened.
func TestEnterRechecksMissingRow(t *testing.T) {
	m := newTestModel(t, config.Bookmark{Label: "BACK", Path: "!back"})
	dir := m.list.SelectedItem().(item).Path
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	m, cmd := press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.chosen != dir || !isQuit(cmd) {
		t.Errorf("want to cd into the restored folder, chose %q", m.chosen)
	}
}

func TestEmptyState(t *testing.T) {
	m := newTestModel(t)
	if !strings.Contains(m.View(), emptyMessage) {
		t.Errorf("want the empty-state message:\n%s", m.View())
	}
	if _, cmd := press(m, tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil {
		t.Error("enter on an empty menu should do nothing")
	}
}

func TestNewStyles(t *testing.T) {
	def := NewStyles(config.Colors{})
	if got := def.item.SelectedTitle.GetForeground(); got != lipgloss.Color("6") {
		t.Errorf("default primary: got %v, want ANSI 6", got)
	}
	if got := def.item.NormalDesc.GetForeground(); got != lipgloss.Color("8") {
		t.Errorf("default tertiary: got %v, want ANSI 8", got)
	}

	set := NewStyles(config.Colors{Primary: "#56B6C2", Tertiary: "#777777"})
	if got := set.item.SelectedTitle.GetForeground(); got != lipgloss.Color("#56B6C2") {
		t.Errorf("configured primary: got %v", got)
	}
	if got := set.missing.NormalTitle.GetForeground(); got != lipgloss.Color("#777777") {
		t.Errorf("missing rows should use tertiary: got %v", got)
	}
}

func TestRemoveConfirmed(t *testing.T) {
	m, _ := press(twoBookmarks(t), runes("r"))
	if m.mode != modeConfirm || !strings.Contains(m.View(), "Remove NVIM? y/N") {
		t.Fatalf("r should prompt:\n%s", m.View())
	}
	// Don't run the command: it's the status message's timer.
	m, _ = press(m, runes("y"))
	if m.mode != modeBrowse {
		t.Errorf("y should return to browse mode")
	}
	if got := onDisk(t, m); len(got) != 1 || got[0].Label != "NOTES" {
		t.Errorf("file should hold only NOTES, got %+v", got)
	}
	if n := len(m.list.Items()); n != 1 {
		t.Errorf("list should have 1 row, has %d", n)
	}
	if !strings.Contains(m.View(), "Removed NVIM") {
		t.Errorf("want a Removed status:\n%s", m.View())
	}
}

// Anything but y cancels, and q in a prompt cancels instead of quitting.
func TestConfirmCancelKeys(t *testing.T) {
	keys := map[string]tea.KeyMsg{
		"n":     runes("n"),
		"q":     runes("q"),
		"enter": {Type: tea.KeyEnter},
		"esc":   {Type: tea.KeyEsc},
	}
	for name, k := range keys {
		for _, start := range []string{"r", "c"} {
			m, _ := press(twoBookmarks(t), runes(start))
			m, cmd := press(m, k)
			if m.mode != modeBrowse || cmd != nil {
				t.Errorf("%s then %s: want browse mode and no command", start, name)
			}
			if len(onDisk(t, m)) != 2 || len(m.list.Items()) != 2 {
				t.Errorf("%s then %s changed the bookmarks", start, name)
			}
		}
	}
}

func TestClearConfirmed(t *testing.T) {
	m, _ := press(twoBookmarks(t), runes("c"))
	if !strings.Contains(m.View(), "Delete all 2 bookmarks? y/N") {
		t.Fatalf("c should prompt with the count:\n%s", m.View())
	}
	m, _ = press(m, runes("Y"))
	if len(onDisk(t, m)) != 0 {
		t.Error("the file should be empty")
	}
	view := m.View()
	if !strings.Contains(view, emptyMessage) || !strings.Contains(view, "Cleared 2 bookmarks. Backup: ~/.config/jug/config.toml.bak") {
		t.Errorf("want the empty state and the backup path:\n%s", view)
	}
	bak, err := config.Load(m.configPath + ".bak")
	if err != nil || len(bak.Bookmarks) != 2 {
		t.Errorf("the backup should hold both bookmarks, got %+v, %v", bak, err)
	}
}

// With a filter applied, r acts on the highlighted row and c still means all.
func TestRemoveWithFilterApplied(t *testing.T) {
	m := twoBookmarks(t)
	m.list.SetFilterText("NOTES")
	m, _ = press(m, runes("r"))
	if !strings.Contains(m.View(), "Remove NOTES? y/N") {
		t.Fatalf("r should target the filtered row:\n%s", m.View())
	}
	m, _ = press(m, runes("y"))
	if got := onDisk(t, m); len(got) != 1 || got[0].Label != "NVIM" {
		t.Errorf("file should hold only NVIM, got %+v", got)
	}

	m = twoBookmarks(t)
	m.list.SetFilterText("NOTES")
	m, _ = press(m, runes("c"))
	if !strings.Contains(m.View(), "Delete all 2 bookmarks? y/N") {
		t.Errorf("c should count every bookmark, not just filtered ones:\n%s", m.View())
	}
}

func TestRAndCWhileFilteringAreTyped(t *testing.T) {
	m, _ := press(twoBookmarks(t), runes("/"))
	m, _ = press(m, runes("r"))
	m, _ = press(m, runes("c"))
	if m.mode != modeBrowse || m.list.FilterValue() != "rc" {
		t.Errorf("want r and c typed into the filter, got mode %v, filter %q", m.mode, m.list.FilterValue())
	}
}

func TestRAndCOnEmptyList(t *testing.T) {
	for _, k := range []string{"r", "c"} {
		m, _ := press(newTestModel(t), runes(k))
		if m.mode != modeBrowse {
			t.Errorf("%s on an empty list should not prompt", k)
		}
	}
}

func TestRemoveMissingRow(t *testing.T) {
	m := newTestModel(t, config.Bookmark{Label: "GONE", Path: "!old/project"})
	m, _ = press(m, runes("r"))
	m, _ = press(m, runes("y"))
	if len(onDisk(t, m)) != 0 {
		t.Error("r should remove a bookmark whose folder is missing")
	}
}

// The screen must match the file: a failed save changes nothing.
func TestFailedSaveKeepsBookmark(t *testing.T) {
	m := twoBookmarks(t)
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	m.configPath = filepath.Join(blocker, "config.toml")

	m, _ = press(m, runes("r"))
	m, _ = press(m, runes("y"))
	if len(m.list.Items()) != 2 || len(m.cfg.Bookmarks) != 2 {
		t.Error("a failed save should keep the bookmark")
	}
	if !strings.Contains(m.View(), "couldn't save") {
		t.Errorf("want the save error on screen:\n%s", m.View())
	}
	// The error stays until the next key.
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyDown})
	if strings.Contains(m.View(), "couldn't save") {
		t.Error("the error should clear on the next key")
	}
}

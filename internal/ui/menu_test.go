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
	m := New(&config.Config{Bookmarks: bookmarks}, "")
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return next.(Model)
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

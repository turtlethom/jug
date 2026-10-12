package ui

import (
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/turtlethom/jug/internal/config"
	"github.com/turtlethom/jug/internal/styles"
)

// These tests call Update directly: a Bubble Tea model is just a value, and
// Update is just a function from (model, message) to (model, command).

func newTestModel() MenuModel {
	cfg := &config.Config{Bookmarks: []config.Bookmark{
		{Label: "NVIM", Path: "/home/me/.config/nvim"},
		{Label: "NOTES", Path: "/home/me/notes"},
	}}
	items := make([]list.Item, len(cfg.Bookmarks))
	for i, b := range cfg.Bookmarks {
		items[i] = item{b}
	}
	m := NewMenuModel(styles.CreateFCDMenuComponent(items, styles.NewFCDMenuStyles(cfg)))
	m.List.SetSize(80, 24)
	return m
}

func press(m MenuModel, key tea.KeyMsg) (MenuModel, tea.Cmd) {
	next, cmd := m.Update(key)
	return next.(MenuModel), cmd
}

func runes(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func TestEnterChoosesHighlighted(t *testing.T) {
	m, cmd := press(newTestModel(), tea.KeyMsg{Type: tea.KeyEnter})
	if m.chosen != "/home/me/.config/nvim" {
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
		m, cmd := press(newTestModel(), key)
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
	m, _ := press(newTestModel(), runes("/"))
	if m.List.FilterState() != list.Filtering {
		t.Fatalf("/ should start filtering, state is %v", m.List.FilterState())
	}
	// Don't run the returned command: while filtering it includes the
	// cursor-blink timer, which sleeps.
	m, _ = press(m, runes("q"))
	if m.List.FilterState() != list.Filtering || m.List.FilterValue() != "q" {
		t.Errorf("q should be typed into the filter; state %v, value %q",
			m.List.FilterState(), m.List.FilterValue())
	}
	if m.chosen != "" {
		t.Errorf("chose %q while filtering", m.chosen)
	}
}

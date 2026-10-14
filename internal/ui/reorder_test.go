package ui

import (
	"os"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/turtlethom/pathpin/internal/config"
)

// fileOrder lists the saved labels in file order.
func fileOrder(t *testing.T, m Model) []string {
	t.Helper()
	var labels []string
	for _, b := range onDisk(t, m) {
		labels = append(labels, b.Label)
	}
	return labels
}

func selected(m Model) string {
	it, _ := m.list.SelectedItem().(item)
	return it.Label
}

var (
	keyJ         = runes("J")
	keyK         = runes("K")
	keyShiftDown = tea.KeyMsg{Type: tea.KeyShiftDown}
	keyShiftUp   = tea.KeyMsg{Type: tea.KeyShiftUp}
)

// On All, J/K (and shift+↓/↑) move the highlighted bookmark one place down or
// up, saving each time, and the cursor follows it. The full help lists them.
func TestReorderOnAll(t *testing.T) {
	all := []string{"BASH", "CS310", "PATHPIN", "GHOSTTY", "NOTES", "CSSLAB"}
	if help, _ := press(groupModel(t), runes("?")); !helpShows(help, "J/K", "reorder") {
		t.Errorf("the full help (?) should mention J/K:\n%s", plain(help.View()))
	}
	for name, keys := range map[string][2]tea.KeyMsg{"J/K": {keyJ, keyK}, "shift+arrows": {keyShiftDown, keyShiftUp}} {
		m := selectLabel(t, groupModel(t), "CS310")
		m, _ = press(m, keys[0])
		want := []string{"BASH", "PATHPIN", "CS310", "GHOSTTY", "NOTES", "CSSLAB"}
		if got := fileOrder(t, m); !slices.Equal(got, want) {
			t.Errorf("%s down: file %q, want %q", name, got, want)
		}
		if got := shown(m); !slices.Equal(got, want) {
			t.Errorf("%s down: rows %q, want %q", name, got, want)
		}
		if got := selected(m); got != "CS310" {
			t.Errorf("%s down: the cursor should follow CS310, got %q", name, got)
		}
		m, _ = press(m, keys[1])
		if got := fileOrder(t, m); !slices.Equal(got, all) {
			t.Errorf("%s back up: file %q, want %q", name, got, all)
		}
	}
}

// On a group's tab, the bookmark swaps with its neighbour on that tab, which
// is far away in the file; everything else, and the tabs, stay put.
func TestReorderOnGroupTab(t *testing.T) {
	m := selectLabel(t, schoolTab(t), "CS310") // School: CS310 NOTES
	m, _ = press(m, keyJ)
	if got, want := shown(m), []string{"NOTES", "CS310"}; !slices.Equal(got, want) {
		t.Errorf("School rows: got %q, want %q", got, want)
	}
	if got, want := fileOrder(t, m), []string{"BASH", "NOTES", "PATHPIN", "GHOSTTY", "CS310", "CSSLAB"}; !slices.Equal(got, want) {
		t.Errorf("file: got %q, want %q (only the two swapped)", got, want)
	}
	if got, want := tabBar(t, m), []string{"All", "Config", "School", "Other"}; !slices.Equal(got, want) {
		t.Errorf("tabs: got %q, want %q", got, want)
	}
	if got := activeTab(t, m); got != "School" {
		t.Errorf("want School still open, got %s", got)
	}
	if got := selected(m); got != "CS310" {
		t.Errorf("the cursor should follow CS310, got %q", got)
	}
}

// At the top or bottom of the tab, J/K do nothing and save nothing (every
// save replaces config.toml.bak).
func TestReorderAtEdges(t *testing.T) {
	for _, tc := range []struct {
		label string
		key   tea.KeyMsg
	}{{"BASH", keyK}, {"CSSLAB", keyJ}} {
		m := selectLabel(t, groupModel(t), tc.label)
		m, _ = press(m, tc.key)
		if _, err := os.Stat(m.configPath + ".bak"); err == nil {
			t.Errorf("%s at the edge: nothing should be saved (a backup was written)", tc.label)
		}
		if got := selected(m); got != tc.label {
			t.Errorf("%s at the edge: cursor moved to %q", tc.label, got)
		}
	}
	if m, _ := press(newTestModel(t), keyJ); m.mode != modeBrowse {
		t.Error("J on an empty menu should do nothing")
	}
}

// With a filter applied, J/K say to clear it instead of moving; while typing
// the filter, J and K are letters.
func TestReorderAndFilter(t *testing.T) {
	m := groupModel(t)
	m.list.SetFilterText("cs")
	m, _ = press(m, keyJ)
	if !strings.Contains(plain(m.View()), "Clear the filter to reorder") {
		t.Errorf("want the clear-the-filter note:\n%s", plain(m.View()))
	}
	if _, err := os.Stat(m.configPath + ".bak"); err == nil {
		t.Error("nothing should be saved while filtered")
	}

	m, _ = press(groupModel(t), runes("/"))
	m, _ = press(m, keyJ)
	m, _ = press(m, keyK)
	if got := m.list.FilterValue(); got != "JK" {
		t.Errorf("want J and K typed into the filter, got %q", got)
	}
}

// Reordering keeps every other setting in the file.
func TestReorderKeepsSettings(t *testing.T) {
	m := selectLabel(t, groupModel(t), "CS310")
	m.cfg.Menu.Icons = true
	m, _ = press(m, keyJ)
	cfg, err := config.Load(m.configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Menu.Icons || cfg.Colors.Theme != "nord" {
		t.Errorf("reordering lost settings: menu %+v, colors %+v", cfg.Menu, cfg.Colors)
	}
}

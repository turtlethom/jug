package ui

import (
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/turtlethom/pathpin/internal/config"
)

// The icons, from a Nerd Font, plus ✗ (in most fonts) for a missing folder.
const (
	folderIcon  = "\uf07b"
	homeIcon    = "\uf015"
	missingIcon = "✗"
)

// iconModel builds a menu with [menu] icons = true.
func iconModel(t *testing.T, bookmarks ...config.Bookmark) Model {
	t.Helper()
	m := newTestModel(t, bookmarks...) // creates the folders and the config file
	cfg := &config.Config{Menu: config.Menu{Icons: true}, Bookmarks: m.cfg.Bookmarks}
	return started(New(cfg, m.configPath)).sized(80, 24)
}

// Icons are off unless turned on, since they need a Nerd Font.
func TestIconsOffByDefault(t *testing.T) {
	m := newTestModel(t,
		config.Bookmark{Label: "NOTES", Path: "notes"},
		config.Bookmark{Label: "HOME", Path: ""},
		config.Bookmark{Label: "GONE", Path: "!old"},
	)
	view := m.View()
	for _, icon := range []string{folderIcon, homeIcon, missingIcon} {
		if strings.Contains(view, icon) {
			t.Errorf("icons are off by default, but the view has %q:\n%s", icon, view)
		}
	}
}

// With icons on, each label has an icon in front: a folder, a house for
// $HOME, and ✗ (next to the (missing) tag) for a folder that's gone. Paths
// have no icon.
func TestIconsBeforeLabels(t *testing.T) {
	m := iconModel(t,
		config.Bookmark{Label: "NOTES", Path: "notes"},
		config.Bookmark{Label: "HOME", Path: ""},
		config.Bookmark{Label: "GONE", Path: "!old"},
	)
	view := m.View()
	for _, want := range []string{folderIcon + " NOTES", homeIcon + " HOME", missingIcon + " GONE (missing)"} {
		if !strings.Contains(view, want) {
			t.Errorf("want %q in the view:\n%s", want, view)
		}
	}
	if line := plain(rawLine(t, view, "~/notes")); strings.Contains(line, folderIcon) {
		t.Errorf("the path line should have no icon: %q", line)
	}

	// Long labels still fit the window with the icon.
	long := iconModel(t, config.Bookmark{Label: strings.Repeat("LONG", 20), Path: "a"}).sized(30, 24)
	long.list.SetShowHelp(false) // see TestLongLabelFitsWidth
	if w := lipgloss.Width(long.View()); w > 30 {
		t.Errorf("view is %d wide, window is 30:\n%s", w, long.View())
	}
}

// The filter underlines the letters it matched, by position in the label
// line, so the icon must not shift them.
func TestIconsKeepFilterMatchesAligned(t *testing.T) {
	m := iconModel(t,
		config.Bookmark{Label: "NOTES", Path: "notes"},
		config.Bookmark{Label: "NVIM", Path: "nvim"},
	)
	for _, query := range []string{"no", "vim"} {
		m.list.SetFilterText(query)
		visible := m.list.VisibleItems()
		if len(visible) != 1 {
			t.Fatalf("filter %q: want 1 row, got %d", query, len(visible))
		}
		title := []rune(visible[0].(item).Title())
		var got []rune
		for _, i := range m.list.MatchesForItem(0) {
			if i >= len(title) {
				t.Fatalf("filter %q: match position %d is past the end of %q", query, i, string(title))
			}
			got = append(got, title[i])
		}
		if !strings.EqualFold(string(got), query) {
			t.Errorf("filter %q underlines %q in %q", query, string(got), string(title))
		}
	}

	// The same goes for a non-ASCII letter before the match, icons or not:
	// the fuzzy matcher counts bytes, and Ö is two.
	plainModel := newTestModel(t, config.Bookmark{Label: "wörk", Path: "work"})
	plainModel.list.SetFilterText("rk")
	title := []rune(plainModel.list.VisibleItems()[0].(item).Title())
	var got []rune
	for _, i := range plainModel.list.MatchesForItem(0) {
		if i >= len(title) {
			t.Fatalf("match position %d is past the end of %q", i, string(title))
		}
		got = append(got, title[i])
	}
	if string(got) != "RK" {
		t.Errorf("filter \"rk\" underlines %q in %q", string(got), string(title))
	}
}

// A folder that vanished after the menu opened gets ✗ when enter finds it gone.
func TestIconTurnsMissingOnEnter(t *testing.T) {
	m := iconModel(t, config.Bookmark{Label: "NOTES", Path: "notes"})
	if err := os.Remove(m.cfg.Bookmarks[0].Path); err != nil {
		t.Fatal(err)
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.Contains(m.View(), missingIcon+" NOTES (missing)") {
		t.Errorf("want ✗ on the row enter found missing:\n%s", m.View())
	}
}

// A bookmark added from the menu gets its icon too.
func TestAddedRowHasIcon(t *testing.T) {
	m, _ := launchFrom(t, iconModel(t, config.Bookmark{Label: "NOTES", Path: "notes"}), "projects/app")
	m, _ = press(m, runes("a"))
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.Contains(m.View(), folderIcon+" APP") {
		t.Errorf("want the new row with a folder icon:\n%s", m.View())
	}
}

// Adding and removing from the menu save the whole config, so settings like
// [menu] icons survive, not just the bookmarks and colors.
func TestMenuChangesKeepSettings(t *testing.T) {
	m, _ := launchFrom(t, iconModel(t, config.Bookmark{Label: "NOTES", Path: "notes"}), "projects/app")
	m.cfg.Colors.Theme = "nord"
	settingsOnDisk := func(after string) {
		t.Helper()
		cfg, err := config.Load(m.configPath)
		if err != nil {
			t.Fatal(err)
		}
		if !cfg.Menu.Icons || cfg.Colors.Theme != "nord" {
			t.Errorf("after %s, the file lost settings: menu %+v, colors %+v", after, cfg.Menu, cfg.Colors)
		}
	}

	m, _ = press(m, runes("a"))
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	settingsOnDisk("adding")

	m, _ = press(m, runes("r"))
	m, _ = press(m, runes("y"))
	settingsOnDisk("removing")
}

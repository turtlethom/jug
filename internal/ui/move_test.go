package ui

import (
	"os"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/turtlethom/pathpin/internal/config"
)

// selectLabel moves the cursor to the row with this label on the open tab.
func selectLabel(t *testing.T, m Model, label string) Model {
	t.Helper()
	i := slices.Index(shown(m), label)
	if i < 0 {
		t.Fatalf("%s isn't on this tab: %q", label, shown(m))
	}
	m.list.Select(i)
	return m
}

// groupOnDisk is the saved group of the bookmark with this label.
func groupOnDisk(t *testing.T, m Model, label string) string {
	t.Helper()
	for _, b := range onDisk(t, m) {
		if b.Label == label {
			return b.Group
		}
	}
	t.Fatalf("%s isn't in the file", label)
	return ""
}

func clearField(m Model) Model {
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyCtrlU}) // delete to the start of the field
	return m
}

func enter(m Model) Model {
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	return m
}

// m opens a Group field on the bottom line, filled with the bookmark's group
// (empty if it has none). The help mentions m.
func TestMoveOpensGroupField(t *testing.T) {
	m := selectLabel(t, groupModel(t), "CS310")
	if help, _ := press(m, runes("?")); !helpShows(help, "m", "move") {
		t.Errorf("the full help (?) should mention m:\n%s", plain(help.View()))
	}
	m, _ = press(m, runes("m"))
	if m.mode == modeBrowse || !strings.Contains(plain(m.View()), "Group: School") {
		t.Fatalf("m should open the field with School in it:\n%s", plain(m.View()))
	}

	m, _ = press(selectLabel(t, groupModel(t), "PATHPIN"), runes("m"))
	if m.mode == modeBrowse || !strings.Contains(plain(m.View()), "Group:") || m.group.Value() != "" {
		t.Errorf("an ungrouped bookmark's field should start empty, got %q", m.group.Value())
	}

	// No bookmarks, nothing to move.
	if m, _ := press(newTestModel(t), runes("m")); m.mode != modeBrowse {
		t.Error("m on an empty menu should do nothing")
	}
}

// enter saves: a new group gets its tab, and the bookmark stays selected on All.
func TestMoveToNewGroup(t *testing.T) {
	m := selectLabel(t, groupModel(t), "PATHPIN")
	m, _ = press(m, runes("m"))
	m, _ = press(m, runes("Tools"))
	m = enter(m)
	if m.mode != modeBrowse {
		t.Fatalf("enter should close the field:\n%s", plain(m.View()))
	}
	if got := groupOnDisk(t, m, "PATHPIN"); got != "Tools" {
		t.Errorf("saved group: got %q, want Tools", got)
	}
	if got, want := tabBar(t, m), []string{"All", "Config", "School", "Tools", "Other"}; !slices.Equal(got, want) {
		t.Errorf("tab bar: got %q, want %q", got, want)
	}
	if sel, _ := m.list.SelectedItem().(item); sel.Label != "PATHPIN" {
		t.Errorf("PATHPIN should stay selected, got %q", sel.Label)
	}
	if !strings.Contains(plain(m.View()), "Moved PATHPIN to Tools") {
		t.Errorf("want a Moved status:\n%s", plain(m.View()))
	}
}

// Typing an existing group's name in another case joins it with its spelling,
// and tab completes an existing name. (bubbles keeps the case you typed when
// completing, so the field says "config"; the saved group is still "Config".)
func TestMoveToExistingGroup(t *testing.T) {
	m := selectLabel(t, groupModel(t), "PATHPIN")
	m, _ = press(m, runes("m"))
	m, _ = press(m, runes("school"))
	m = enter(m)
	if got := groupOnDisk(t, m, "PATHPIN"); got != "School" {
		t.Errorf("typed school: got %q, want School", got)
	}

	m = selectLabel(t, groupModel(t), "CSSLAB")
	m, _ = press(m, runes("m"))
	m, _ = press(m, runes("co"))
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyTab})
	if got := m.group.Value(); !strings.EqualFold(got, "Config") {
		t.Errorf("tab after co: got %q, want config", got)
	}
	m = enter(m)
	if got := groupOnDisk(t, m, "CSSLAB"); got != "Config" {
		t.Errorf("saved group: got %q, want Config", got)
	}
}

// An empty field removes the group.
func TestMoveToNoGroup(t *testing.T) {
	m := selectLabel(t, groupModel(t), "BASH")
	m, _ = press(m, runes("m"))
	m = enter(clearField(m))
	if got := groupOnDisk(t, m, "BASH"); got != "" {
		t.Errorf("want BASH ungrouped, got %q", got)
	}
	if !strings.Contains(plain(m.View()), "BASH has no group now") {
		t.Errorf("want the no-group status:\n%s", plain(m.View()))
	}
}

// All and Other are refused, and the field stays open to fix. esc cancels
// without saving.
func TestMoveErrorsAndCancel(t *testing.T) {
	m := selectLabel(t, groupModel(t), "BASH")
	m, _ = press(m, runes("m"))
	m, _ = press(clearField(m), runes("other"))
	m = enter(m)
	if m.mode == modeBrowse || !strings.Contains(plain(m.View()), "tab names") {
		t.Errorf("Other should be refused with the field kept open:\n%s", plain(m.View()))
	}
	if got := groupOnDisk(t, m, "BASH"); got != "Config" {
		t.Errorf("a refused name must not be saved, got %q", got)
	}

	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != modeBrowse || groupOnDisk(t, m, "BASH") != "Config" {
		t.Error("esc should close the field without saving")
	}

	// In the field, keys are letters: r, c and q don't act.
	m, _ = press(selectLabel(t, groupModel(t), "BASH"), runes("m"))
	m, _ = press(clearField(m), runes("rcq"))
	if m.mode == modeBrowse || m.group.Value() != "rcq" {
		t.Errorf("want r, c, q typed into the field, got mode %v, value %q", m.mode, m.group.Value())
	}
}

// On a group's tab, a bookmark moved to another group leaves the tab; if the
// tab empties, the menu shows All.
func TestMoveOffGroupTab(t *testing.T) {
	m := pressTab(pressTab(groupModel(t))) // School: CS310 NOTES
	m = selectLabel(t, m, "CS310")
	m, _ = press(m, runes("m"))
	m, _ = press(clearField(m), runes("Config"))
	m = enter(m)
	if got := shown(m); !slices.Equal(got, []string{"NOTES"}) {
		t.Errorf("School after moving CS310 away: got %q", got)
	}

	m, _ = press(m, runes("m")) // NOTES, the last in School
	m = enter(clearField(m))
	if got := activeTab(t, m); got != "All" {
		t.Errorf("want All once School is empty, got %s", got)
	}
	if got, want := tabBar(t, m), []string{"All", "Config", "Other"}; !slices.Equal(got, want) {
		t.Errorf("tab bar: got %q, want %q", got, want)
	}
}

// Moving keeps every other setting in the file, like add and remove do.
func TestMoveKeepsSettings(t *testing.T) {
	m := selectLabel(t, groupModel(t), "BASH")
	m.cfg.Menu.Icons = true
	m, _ = press(m, runes("m"))
	m, _ = press(clearField(m), runes("Shell"))
	m = enter(m)
	cfg, err := config.Load(m.configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Menu.Icons || cfg.Colors.Theme != "nord" {
		t.Errorf("moving lost settings: menu %+v, colors %+v", cfg.Menu, cfg.Colors)
	}
}

// enter without changing the group (in any case) saves nothing: every save
// replaces config.toml.bak, and a save that changes nothing would throw away
// the last real backup.
func TestMoveUnchangedDoesNotSave(t *testing.T) {
	for _, typed := range []string{"", "config"} { // as it was; another case
		m := selectLabel(t, groupModel(t), "BASH") // Config
		m, _ = press(m, runes("m"))
		if typed != "" {
			m, _ = press(clearField(m), runes(typed))
		}
		m = enter(m)
		if m.mode != modeBrowse {
			t.Fatalf("enter should close the field:\n%s", plain(m.View()))
		}
		if _, err := os.Stat(m.configPath + ".bak"); err == nil {
			t.Errorf("typed %q: an unchanged group should not save (a backup was written)", typed)
		}
		if !strings.Contains(plain(m.View()), "BASH stays in Config") {
			t.Errorf("typed %q: want a stays-in status:\n%s", typed, plain(m.View()))
		}
	}
}

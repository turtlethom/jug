package ui

import (
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/turtlethom/pathpin/internal/config"
)

// labelOnDisk reports whether a bookmark with exactly this label is saved,
// and its group.
func labelOnDisk(t *testing.T, m Model, label string) (string, bool) {
	t.Helper()
	for _, b := range onDisk(t, m) {
		if b.Label == label {
			return b.Group, true
		}
	}
	return "", false
}

// n opens the Label field filled with the label as stored (not as shown),
// and the help mentions it.
func TestRenameOpensLabelField(t *testing.T) {
	m := newTestModel(t, config.Bookmark{Label: "Day Job", Path: "work"})
	if help, _ := press(m, runes("?")); !helpShows(help, "n", "rename") {
		t.Errorf("the full help (?) should mention n:\n%s", plain(help.View()))
	}
	m, _ = press(m, runes("n"))
	if m.mode == modeBrowse || !strings.Contains(plain(m.View()), "Label: Day Job") {
		t.Errorf("n should open the field with Day Job in it:\n%s", plain(m.View()))
	}

	if m, _ := press(newTestModel(t), runes("n")); m.mode != modeBrowse {
		t.Error("n on an empty menu should do nothing")
	}
}

// enter renames: the bookmark keeps its path and group, stays selected on the
// open tab, and the status says so.
func TestRenameSaves(t *testing.T) {
	m := pressTab(pressTab(groupModel(t))) // School: CS310 NOTES
	m = selectLabel(t, m, "CS310")
	m, _ = press(m, runes("n"))
	m, _ = press(clearField(m), runes("OS"))
	m = enter(m)
	if m.mode != modeBrowse {
		t.Fatalf("enter should close the field:\n%s", plain(m.View()))
	}
	if group, ok := labelOnDisk(t, m, "OS"); !ok || group != "School" {
		t.Errorf("want OS saved in School, got ok=%v group=%q", ok, group)
	}
	if _, ok := labelOnDisk(t, m, "CS310"); ok {
		t.Error("CS310 should be gone from the file")
	}
	if sel, _ := m.list.SelectedItem().(item); sel.Label != "OS" {
		t.Errorf("the renamed row should be selected, got %q", sel.Label)
	}
	if got := shown(m); len(got) != 2 || got[0] != "OS" {
		t.Errorf("School after renaming: got %q", got)
	}
	if !strings.Contains(plain(m.View()), "Renamed CS310 to OS") {
		t.Errorf("want a Renamed status:\n%s", plain(m.View()))
	}
}

// A label in use (any case) or an empty one is refused with the field kept
// open; esc cancels. Nothing is saved.
func TestRenameErrorsAndCancel(t *testing.T) {
	m := selectLabel(t, groupModel(t), "CS310")
	m, _ = press(m, runes("n"))
	m, _ = press(clearField(m), runes("notes"))
	m = enter(m)
	if m.mode == modeBrowse || !strings.Contains(plain(m.View()), "already exists") {
		t.Errorf("a label in use should be refused with the field open:\n%s", plain(m.View()))
	}
	m = enter(clearField(m))
	if m.mode == modeBrowse || !strings.Contains(plain(m.View()), "label is empty") {
		t.Errorf("an empty label should be refused with the field open:\n%s", plain(m.View()))
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	if _, ok := labelOnDisk(t, m, "CS310"); m.mode != modeBrowse || !ok {
		t.Error("esc should close the field with CS310 unchanged")
	}

	// In the field, keys are letters: r, c and q don't act.
	m, _ = press(selectLabel(t, groupModel(t), "BASH"), runes("n"))
	m, _ = press(clearField(m), runes("rcq"))
	if m.mode == modeBrowse || m.input.Value() != "rcq" {
		t.Errorf("want r, c, q typed into the field, got mode %v, value %q", m.mode, m.input.Value())
	}
}

// enter on the same label saves nothing (every save replaces the backup); a
// case-only change is a change, since labels are stored as typed.
func TestRenameUnchangedDoesNotSave(t *testing.T) {
	m := selectLabel(t, groupModel(t), "CS310")
	m, _ = press(m, runes("n"))
	m = enter(m)
	if m.mode != modeBrowse {
		t.Fatalf("enter should close the field:\n%s", plain(m.View()))
	}
	if _, err := os.Stat(m.configPath + ".bak"); err == nil {
		t.Error("an unchanged label should not save (a backup was written)")
	}

	m, _ = press(m, runes("n"))
	m, _ = press(clearField(m), runes("cs310"))
	m = enter(m)
	if _, ok := labelOnDisk(t, m, "cs310"); !ok {
		t.Errorf("a case-only change should be saved as typed, got %+v", onDisk(t, m))
	}
	// The menu shows labels in uppercase, so say what changed.
	if !strings.Contains(plain(m.View()), `CS310 is now saved as "cs310"`) {
		t.Errorf("want a saved-as status for a case-only change:\n%s", plain(m.View()))
	}
}

// With a filter applied, the filter is cleared so the renamed row shows.
func TestRenameWithFilterApplied(t *testing.T) {
	m := groupModel(t)
	m.list.SetFilterText("cs310")
	m, _ = press(m, runes("n"))
	m, _ = press(clearField(m), runes("OS"))
	m = enter(m)
	if sel, _ := m.list.SelectedItem().(item); sel.Label != "OS" || len(shown(m)) != 6 {
		t.Errorf("want the filter cleared and OS selected, got %q selected among %q", sel.Label, shown(m))
	}
}

// Renaming keeps every other setting in the file, like add and remove do.
func TestRenameKeepsSettings(t *testing.T) {
	m := selectLabel(t, groupModel(t), "BASH")
	m.cfg.Menu.Icons = true
	m, _ = press(m, runes("n"))
	m, _ = press(clearField(m), runes("SHELL"))
	m = enter(m)
	cfg, err := config.Load(m.configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Menu.Icons || cfg.Colors.Theme != "nord" {
		t.Errorf("renaming lost settings: menu %+v, colors %+v", cfg.Menu, cfg.Colors)
	}
}

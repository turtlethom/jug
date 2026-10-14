package ui

import (
	"os"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/turtlethom/pathpin/internal/config"
)

// schoolTab is groupModel with the School tab open (CS310, NOTES).
func schoolTab(t *testing.T) Model {
	t.Helper()
	return pressTab(pressTab(groupModel(t)))
}

func pressN(m Model) Model {
	m, _ = press(m, runes("N"))
	return m
}

// N on a group's tab opens a field filled with the group's name. The full
// help lists it.
func TestRenameGroupOpensField(t *testing.T) {
	m := schoolTab(t)
	if help, _ := press(m, runes("?")); !helpShows(help, "N", "rename group") {
		t.Errorf("the full help (?) should mention N:\n%s", plain(help.View()))
	}
	m = pressN(m)
	if m.mode == modeBrowse || !strings.Contains(plain(m.View()), "Rename group: School") {
		t.Errorf("N should open the field with School in it:\n%s", plain(m.View()))
	}
}

// On All or Other, or with no groups, N says to open a group's tab.
func TestRenameGroupNeedsGroupTab(t *testing.T) {
	for name, m := range map[string]Model{
		"All":       groupModel(t),
		"Other":     pressShiftTab(groupModel(t)),
		"no groups": twoBookmarks(t),
	} {
		m = pressN(m)
		if m.mode != modeBrowse || !strings.Contains(plain(m.View()), "Open a group's tab to rename it") {
			t.Errorf("%s: want a note and no field:\n%s", name, plain(m.View()))
		}
	}
}

// enter renames the group for all its bookmarks; the tab follows the new name
// and stays open. A case-only change re-cases it the same way.
func TestRenameGroupSaves(t *testing.T) {
	for _, newName := range []string{"Classes", "SCHOOL"} {
		m := pressN(schoolTab(t))
		m, _ = press(clearField(m), runes(newName))
		m = enter(m)
		if m.mode != modeBrowse {
			t.Fatalf("%s: enter should close the field:\n%s", newName, plain(m.View()))
		}
		for _, label := range []string{"CS310", "NOTES"} {
			if got := groupOnDisk(t, m, label); got != newName {
				t.Errorf("%s: %s's group is %q", newName, label, got)
			}
		}
		if got, want := tabBar(t, m), []string{"All", "Config", newName, "Other"}; !slices.Equal(got, want) {
			t.Errorf("%s: tab bar %q, want %q", newName, got, want)
		}
		if got := activeTab(t, m); got != newName {
			t.Errorf("%s: want the renamed tab open, got %s", newName, got)
		}
		if got := shown(m); !slices.Equal(got, []string{"CS310", "NOTES"}) {
			t.Errorf("%s: rows %q", newName, got)
		}
		if !strings.Contains(plain(m.View()), "Renamed group School to "+newName) {
			t.Errorf("%s: want a Renamed group status:\n%s", newName, plain(m.View()))
		}
	}
}

// A name matching another group asks before merging. y merges and opens the
// merged-into tab; any other key cancels, saving nothing.
func TestRenameGroupMerge(t *testing.T) {
	m := pressN(schoolTab(t))
	m, _ = press(clearField(m), runes("config"))
	m = enter(m)
	if m.mode != modeConfirm || !strings.Contains(plain(m.View()), "Merge School into Config? y/N") {
		t.Fatalf("want the merge prompt:\n%s", plain(m.View()))
	}
	cancelled, _ := press(m, runes("n"))
	if cancelled.mode != modeBrowse || groupOnDisk(t, cancelled, "CS310") != "School" {
		t.Error("n should cancel the merge without saving")
	}
	if got := activeTab(t, cancelled); got != "School" {
		t.Errorf("after cancelling, want School still open, got %s", got)
	}

	m, _ = press(m, runes("y"))
	for _, label := range []string{"CS310", "NOTES"} {
		if got := groupOnDisk(t, m, label); got != "Config" {
			t.Errorf("after merging, %s's group is %q, want Config", label, got)
		}
	}
	if got, want := tabBar(t, m), []string{"All", "Config", "Other"}; !slices.Equal(got, want) {
		t.Errorf("tab bar %q, want %q", got, want)
	}
	if got := activeTab(t, m); got != "Config" {
		t.Errorf("want the Config tab open after merging, got %s", got)
	}
	if !strings.Contains(plain(m.View()), "Merged School into Config") {
		t.Errorf("want a Merged status:\n%s", plain(m.View()))
	}
}

// Empty names and tab names are refused with the field kept open; esc
// cancels; an unchanged name saves nothing.
func TestRenameGroupErrorsAndUnchanged(t *testing.T) {
	m := pressN(schoolTab(t))
	m = enter(clearField(m))
	if m.mode == modeBrowse || !strings.Contains(plain(m.View()), "group name is empty") {
		t.Errorf("an empty name should be refused with the field open:\n%s", plain(m.View()))
	}
	m, _ = press(m, runes("Other"))
	m = enter(m)
	if m.mode == modeBrowse || !strings.Contains(plain(m.View()), "tab names") {
		t.Errorf("Other should be refused with the field open:\n%s", plain(m.View()))
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != modeBrowse || groupOnDisk(t, m, "CS310") != "School" {
		t.Error("esc should close the field without saving")
	}

	m = enter(pressN(schoolTab(t)))
	if m.mode != modeBrowse {
		t.Fatalf("enter should close the field:\n%s", plain(m.View()))
	}
	if _, err := os.Stat(m.configPath + ".bak"); err == nil {
		t.Error("an unchanged name should not save (a backup was written)")
	}
}

// Renaming a group keeps every other setting in the file.
func TestRenameGroupKeepsSettings(t *testing.T) {
	m := schoolTab(t)
	m.cfg.Menu.Icons = true
	m = pressN(m)
	m, _ = press(clearField(m), runes("Classes"))
	m = enter(m)
	cfg, err := config.Load(m.configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Menu.Icons || cfg.Colors.Theme != "nord" {
		t.Errorf("renaming a group lost settings: menu %+v, colors %+v", cfg.Menu, cfg.Colors)
	}
}

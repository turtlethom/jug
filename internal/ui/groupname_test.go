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

// Tab names are refused with the field kept open; esc cancels; an unchanged
// name saves nothing. (An empty name ungroups: see TestUngroupPrompt.)
func TestRenameGroupErrorsAndUnchanged(t *testing.T) {
	m := pressN(schoolTab(t))
	m, _ = press(clearField(m), runes("Other"))
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

// Clearing the N field shows a muted hint that an empty name ungroups. The m
// field, which N shares, never shows it.
func TestUngroupHint(t *testing.T) {
	m := clearField(pressN(schoolTab(t)))
	if !strings.Contains(plain(m.View()), "empty: ungroup all") {
		t.Errorf("an empty N field should hint that it ungroups:\n%s", plain(m.View()))
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	m, _ = press(m, runes("m"))
	m = clearField(m)
	if m.mode != modeGroup || strings.Contains(plain(m.View()), "ungroup all") {
		t.Errorf("the m field should open without N's hint:\n%s", plain(m.View()))
	}
}

// soloTab is a menu with one group of one bookmark (SOLO in Solo) and one
// ungrouped bookmark, with the Solo tab open.
func soloTab(t *testing.T) Model {
	t.Helper()
	return pressTab(groupModelOf(t,
		config.Bookmark{Label: "SOLO", Path: "solo", Group: "Solo"},
		config.Bookmark{Label: "NOTES", Path: "notes"},
	))
}

// An empty (or blank) name asks before ungrouping, counting the group's
// bookmarks in every spelling. Any key but y cancels, saving nothing.
func TestUngroupPrompt(t *testing.T) {
	for name, tc := range map[string]struct {
		m          Model
		tab, typed string
		prompt     string
	}{
		"School":        {schoolTab(t), "School", "", "Ungroup all 2 bookmarks in School? y/N"},
		"two spellings": {pressTab(groupModel(t)), "Config", "  ", "Ungroup all 2 bookmarks in Config? y/N"},
		"one bookmark":  {soloTab(t), "Solo", "", "Ungroup 1 bookmark in Solo? y/N"},
	} {
		m := pressN(tc.m)
		m, _ = press(clearField(m), runes(tc.typed))
		m = enter(m)
		if m.mode != modeConfirm || !strings.Contains(plain(m.View()), tc.prompt) {
			t.Errorf("%s: want the prompt %q:\n%s", name, tc.prompt, plain(m.View()))
			continue
		}
		m, _ = press(m, runes("n"))
		if m.mode != modeBrowse {
			t.Errorf("%s: n should cancel", name)
		}
		if _, err := os.Stat(m.configPath + ".bak"); err == nil {
			t.Errorf("%s: cancelling should not save (a backup was written)", name)
		}
		if got := activeTab(t, m); got != tc.tab {
			t.Errorf("%s: after cancelling, want %s still open, got %s", name, tc.tab, got)
		}
	}
}

// y ungroups: the bookmarks keep their places in the file with no group, the
// other groups are untouched, the tab goes away, and All opens with the cursor
// still on the highlighted bookmark.
func TestUngroupSaves(t *testing.T) {
	m := selectLabel(t, schoolTab(t), "NOTES")
	m = enter(clearField(pressN(m)))
	m, _ = press(m, runes("y"))
	if m.mode != modeBrowse {
		t.Fatalf("y should close the prompt:\n%s", plain(m.View()))
	}
	var labels, groups []string
	for _, b := range onDisk(t, m) {
		labels, groups = append(labels, b.Label), append(groups, b.Group)
	}
	if want := []string{"BASH", "CS310", "PATHPIN", "GHOSTTY", "NOTES", "CSSLAB"}; !slices.Equal(labels, want) {
		t.Errorf("order on disk %q, want %q", labels, want)
	}
	if want := []string{"Config", "", "", "config", "", ""}; !slices.Equal(groups, want) {
		t.Errorf("groups on disk %q, want %q", groups, want)
	}
	if got, want := tabBar(t, m), []string{"All", "Config", "Other"}; !slices.Equal(got, want) {
		t.Errorf("tab bar %q, want %q", got, want)
	}
	if got := activeTab(t, m); got != "All" {
		t.Errorf("want All open after ungrouping, got %s", got)
	}
	if got := selected(m); got != "NOTES" {
		t.Errorf("want the cursor still on NOTES, got %s", got)
	}
	if !strings.Contains(plain(m.View()), "Ungrouped 2 bookmarks from School") {
		t.Errorf("want an Ungrouped status:\n%s", plain(m.View()))
	}
}

// Ungrouping the only group leaves no groups, so the tab bar goes away and
// the menu looks as it does without groups.
func TestUngroupLastGroup(t *testing.T) {
	m := enter(clearField(pressN(soloTab(t))))
	m, _ = press(m, runes("y"))
	if bar := tabBar(t, m); bar != nil {
		t.Errorf("want no tab bar without groups, got %q", bar)
	}
	if got := shown(m); !slices.Equal(got, []string{"SOLO", "NOTES"}) {
		t.Errorf("rows %q, want SOLO NOTES", got)
	}
	if !strings.Contains(plain(m.View()), "Ungrouped 1 bookmark from Solo") {
		t.Errorf("want an Ungrouped status:\n%s", plain(m.View()))
	}
}

// With a filter applied, ungrouping clears it, so All shows every bookmark,
// with the cursor on the one that was highlighted.
func TestUngroupWithFilterApplied(t *testing.T) {
	m := schoolTab(t)
	m.list.SetFilterText("cs310")
	m = enter(clearField(pressN(m)))
	m, _ = press(m, runes("y"))
	if got := selected(m); got != "CS310" || len(shown(m)) != 6 {
		t.Errorf("want the filter cleared and CS310 selected, got %q selected among %q", got, shown(m))
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

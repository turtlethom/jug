package ui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/turtlethom/pathpin/internal/config"
)

// rawRow is the rendered label line of the row with this label (the last line
// starting with it, after the bar, so the title doesn't count for PATHPIN).
func rawRow(t *testing.T, m Model, label string) string {
	t.Helper()
	found := ""
	for line := range strings.SplitSeq(m.View(), "\n") {
		if strings.HasPrefix(strings.TrimLeft(plain(line), " │"), label) {
			found = line
		}
	}
	if found == "" {
		t.Fatalf("no row for %s:\n%s", label, plain(m.View()))
	}
	return found
}

// row is the visible text of a row's label line, without the bar and the
// spaces around it.
func row(t *testing.T, m Model, label string) string {
	t.Helper()
	return strings.TrimRight(strings.TrimLeft(plain(rawRow(t, m, label)), " │"), " ")
}

// On All, a grouped row shows its group after the label, in the group's first
// spelling (GHOSTTY is saved in "config"). Ungrouped rows have no tag, and
// neither does any row on a group's tab or in a menu without groups.
func TestGroupTagsOnAll(t *testing.T) {
	m := groupModel(t)
	for label, want := range map[string]string{
		"CS310":   "CS310 · School",
		"GHOSTTY": "GHOSTTY · Config",
		"NOTES":   "NOTES · School",
		"PATHPIN": "PATHPIN",
		"CSSLAB":  "CSSLAB",
	} {
		if got := row(t, m, label); got != want {
			t.Errorf("All: want %q, got %q", want, got)
		}
	}
	if got := row(t, m, "BASH"); !strings.HasPrefix(got, "BASH") || !strings.HasSuffix(got, " · Config") {
		t.Errorf("All: want the selected BASH tagged Config, got %q", got)
	}

	school := pressTab(pressTab(groupModel(t)))
	for _, label := range []string{"CS310", "NOTES"} {
		if got := row(t, school, label); strings.Contains(got, "·") {
			t.Errorf("School tab: want no tag, got %q", got)
		}
	}
	if view := plain(twoBookmarks(t).View()); strings.Contains(view, "·") {
		t.Errorf("no groups: want no tags:\n%s", view)
	}
}

// The tag is muted with no background, in both selection modes, and a plain
// space separates it from the label. On the selected row in block mode, that
// space comes right after the block.
func TestGroupTagColors(t *testing.T) {
	withColor(t)
	for _, sel := range []string{"block", "bar"} {
		m := groupLook(t, sel)
		for _, label := range []string{"BASH", "CS310"} { // BASH is selected
			before, tag, _ := find(t, cells(rawRow(t, m, label)), "·")
			if len(before) == 0 || before[len(before)-1].r != ' ' || before[len(before)-1].bg != "" {
				t.Errorf("%s, %s: want a plain space before the tag", sel, label)
			}
			for _, c := range tag {
				if c.fg != fg(nordMuted) || c.bg != "" {
					t.Errorf("%s, %s: the · is drawn in fg %q bg %q, want muted with no background", sel, label, c.fg, c.bg)
				}
			}
			_, name, _ := find(t, cells(rawRow(t, m, label)), map[string]string{"BASH": "Config", "CS310": "School"}[label])
			for _, c := range name {
				if c.fg != fg(nordMuted) || c.bg != "" {
					t.Errorf("%s, %s: the group name is drawn in fg %q bg %q, want muted with no background", sel, label, c.fg, c.bg)
					break
				}
			}
		}
		if sel == "block" {
			before, _, _ := find(t, cells(rawRow(t, m, "BASH")), "·")
			if len(before) < 2 || before[len(before)-2].bg != bg(nordAccent) {
				t.Error("block: want the selected block, then one plain space, then the tag")
			}
		}
	}
}

// The tag shows only if it fits the window whole: a label that fits keeps its
// line without the tag, and a label cut with … has none.
func TestGroupTagFits(t *testing.T) {
	model := func(w int) Model {
		return groupModelOf(t,
			config.Bookmark{Label: "NOTES", Path: "notes"},
			config.Bookmark{Label: "SEMESTERPROJECT", Path: "project", Group: "Config"},
		).sized(w, 24)
	}
	if got := row(t, model(80), "SEMESTERPROJECT"); got != "SEMESTERPROJECT · Config" {
		t.Errorf("80 columns: want the tag, got %q", got)
	}
	if got := row(t, model(24), "SEMESTERPROJECT"); got != "SEMESTERPROJECT" {
		t.Errorf("24 columns: want the label without the tag, got %q", got)
	}
	if got := row(t, model(14), "SEMESTER"); !strings.Contains(got, "…") || strings.Contains(got, "·") {
		t.Errorf("14 columns: want the cut label without the tag, got %q", got)
	}
}

// A missing row keeps its tag, after the (missing) note.
func TestGroupTagOnMissingRow(t *testing.T) {
	m := groupModelOf(t,
		config.Bookmark{Label: "NOTES", Path: "notes"},
		config.Bookmark{Label: "GONE", Path: "!gone", Group: "Config"},
	)
	if got := row(t, m, "GONE"); got != "GONE (missing) · Config" {
		t.Errorf("want the missing row tagged, got %q", got)
	}
}

// Filtering All by a group's name shows why each row matched: its tag. Right
// after /, with nothing typed, the dimmed rows keep the space before the tag.
func TestGroupTagWhileFiltering(t *testing.T) {
	dimmed, _ := press(groupModel(t), runes("/"))
	if got := row(t, dimmed, "CS310"); got != "CS310 · School" {
		t.Errorf("after /: want %q, got %q", "CS310 · School", got)
	}

	m := groupModel(t)
	m.list.SetFilterText("school")
	for _, label := range []string{"CS310", "NOTES"} {
		if got := row(t, m, label); !strings.HasSuffix(got, " · School") {
			t.Errorf("want %s tagged School while filtering by school, got %q", label, got)
		}
	}
}

// Moving a bookmark to a group with m tags its row on All right away.
func TestGroupTagAfterMove(t *testing.T) {
	m := selectLabel(t, groupModel(t), "PATHPIN")
	m, _ = press(m, runes("m"))
	m, _ = press(m, runes("School"))
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if got := row(t, m, "PATHPIN"); !strings.HasSuffix(got, " · School") {
		t.Errorf("want PATHPIN tagged School after the move, got %q", got)
	}
}

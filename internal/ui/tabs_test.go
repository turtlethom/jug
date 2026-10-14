package ui

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/turtlethom/pathpin/internal/config"
)

// groupModel builds a menu (in nord, so colors are hex) over six bookmarks in
// two groups plus two ungrouped ones. Groups are interleaved and one is
// spelled two ways, so tabs must collect them in config order:
//
//	All:    BASH CS310 PATHPIN GHOSTTY NOTES CSSLAB
//	Config: BASH GHOSTTY
//	School: CS310 NOTES
//	Other:  PATHPIN CSSLAB
func groupModel(t *testing.T) Model {
	t.Helper()
	return groupModelOf(t,
		config.Bookmark{Label: "BASH", Path: "bash", Group: "Config"},
		config.Bookmark{Label: "CS310", Path: "cs310", Group: "School"},
		config.Bookmark{Label: "PATHPIN", Path: "pathpin"},
		config.Bookmark{Label: "GHOSTTY", Path: "ghostty", Group: "config"},
		config.Bookmark{Label: "NOTES", Path: "notes", Group: "School"},
		config.Bookmark{Label: "CSSLAB", Path: "csslab"},
	)
}

func groupModelOf(t *testing.T, bookmarks ...config.Bookmark) Model {
	t.Helper()
	m := newTestModel(t, bookmarks...) // creates the folders and the config file
	cfg := &config.Config{Colors: config.Colors{Theme: "nord"}, Bookmarks: m.cfg.Bookmarks}
	return New(cfg, m.configPath).sized(80, 40)
}

// shown lists the labels of the rows the menu shows now.
func shown(m Model) []string {
	var labels []string
	for _, li := range m.list.VisibleItems() {
		labels = append(labels, li.(item).Label)
	}
	return labels
}

// tabBar returns the tab names on the tab bar, or nil if there's none. The
// bar sits between the title and the status line.
func tabBar(t *testing.T, m Model) []string {
	t.Helper()
	lines := strings.Split(plain(m.View()), "\n")
	title := slices.IndexFunc(lines, func(l string) bool { return strings.Contains(l, "pathpin") })
	status := slices.IndexFunc(lines, func(l string) bool { return strings.Contains(l, "bookmark") })
	if title < 0 || status < 0 {
		t.Fatalf("no title or status line:\n%s", plain(m.View()))
	}
	for _, l := range lines[title+1 : status] {
		if f := strings.Fields(l); len(f) > 0 && f[0] == config.TabAll {
			return f
		}
	}
	return nil
}

// activeTab is the one tab drawn in the accent color (as text or block).
func activeTab(t *testing.T, m Model) string {
	t.Helper()
	withColor(t)
	view := m.View()
	line := rawLine(t, view, config.TabAll+" ")
	var active []string
	for _, name := range tabBar(t, m) {
		_, cs, _ := find(t, cells(line), name)
		inAccent := true
		for _, c := range cs {
			inAccent = inAccent && (c.fg == fg(nordAccent) || c.bg == bg(nordAccent))
		}
		if inAccent {
			active = append(active, name)
		}
	}
	if len(active) != 1 {
		t.Fatalf("want exactly one tab in the accent color, got %q", active)
	}
	return active[0]
}

func pressTab(m Model) Model {
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyTab})
	return m
}

func pressShiftTab(m Model) Model {
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyShiftTab})
	return m
}

// With no groups, there's no tab bar: the menu looks as it did before groups.
func TestNoTabsWithoutGroups(t *testing.T) {
	if bar := tabBar(t, twoBookmarks(t)); bar != nil {
		t.Errorf("want no tab bar without groups, got %q", bar)
	}
	m := pressTab(twoBookmarks(t))
	if got := shown(m); !slices.Equal(got, []string{"NVIM", "NOTES"}) {
		t.Errorf("tab without groups should change nothing, got %q", got)
	}
}

// The tab bar lists All, the groups in the order they first appear (first
// spelling), then Other. All is open at start, and the help mentions tab.
func TestTabBar(t *testing.T) {
	m := groupModel(t)
	if got, want := tabBar(t, m), []string{"All", "Config", "School", "Other"}; !slices.Equal(got, want) {
		t.Errorf("tab bar: got %q, want %q", got, want)
	}
	if got := activeTab(t, m); got != "All" {
		t.Errorf("open at start: got %s, want All", got)
	}
	if got, want := shown(m), []string{"BASH", "CS310", "PATHPIN", "GHOSTTY", "NOTES", "CSSLAB"}; !slices.Equal(got, want) {
		t.Errorf("All: got %q, want %q", got, want)
	}
	if !strings.Contains(plain(rawLine(t, m.View(), "quit")), "tab") {
		t.Errorf("the help line should mention tab:\n%s", plain(m.View()))
	}

	// Other only shows when some bookmarks are grouped and some aren't.
	allGrouped := groupModelOf(t,
		config.Bookmark{Label: "BASH", Path: "bash", Group: "Config"},
		config.Bookmark{Label: "NOTES", Path: "notes", Group: "School"},
	)
	if got, want := tabBar(t, allGrouped), []string{"All", "Config", "School"}; !slices.Equal(got, want) {
		t.Errorf("no ungrouped bookmarks: got %q, want %q", got, want)
	}
}

// tab and shift+tab move through the tabs, wrapping around. Each tab shows
// its bookmarks in config order, and the status line counts them.
func TestTabSwitching(t *testing.T) {
	m := groupModel(t)
	steps := []struct {
		key    func(Model) Model
		tab    string
		labels []string
	}{
		{pressTab, "Config", []string{"BASH", "GHOSTTY"}},
		{pressTab, "School", []string{"CS310", "NOTES"}},
		{pressTab, "Other", []string{"PATHPIN", "CSSLAB"}},
		{pressTab, "All", []string{"BASH", "CS310", "PATHPIN", "GHOSTTY", "NOTES", "CSSLAB"}},
		{pressShiftTab, "Other", []string{"PATHPIN", "CSSLAB"}},
		{pressShiftTab, "School", []string{"CS310", "NOTES"}},
	}
	for _, s := range steps {
		m = s.key(m)
		if got := activeTab(t, m); got != s.tab {
			t.Fatalf("want the %s tab, got %s", s.tab, got)
		}
		if got := shown(m); !slices.Equal(got, s.labels) {
			t.Errorf("%s: got %q, want %q", s.tab, got, s.labels)
		}
		if !strings.Contains(plain(m.View()), "2 bookmarks") && s.tab != "All" {
			t.Errorf("%s: want the status line to count 2 bookmarks:\n%s", s.tab, plain(m.View()))
		}
	}
}

// enter on a group tab goes to the bookmark highlighted there.
func TestEnterOnGroupTab(t *testing.T) {
	m := pressTab(pressTab(groupModel(t))) // School
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyDown})
	m, cmd := press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if filepath.Base(m.chosen) != "notes" || !isQuit(cmd) {
		t.Errorf("want to cd into notes, chose %q", m.chosen)
	}
}

// The filter searches the tab you're on, and matches group names too.
func TestFilterOnTabs(t *testing.T) {
	m := groupModel(t)
	m.list.SetFilterText("school")
	if got := shown(m); !slices.Equal(got, []string{"CS310", "NOTES"}) {
		t.Errorf("All, filter school: got %q, want the School group", got)
	}

	m = pressTab(pressTab(groupModel(t))) // School
	m.list.SetFilterText("cs")
	if got := shown(m); !slices.Equal(got, []string{"CS310"}) {
		t.Errorf("School, filter cs: got %q, want only CS310 (CSSLAB is on another tab)", got)
	}
}

// Labels and groups are matched separately: a query never takes some letters
// from the label and the rest from the group. Group names only count on All,
// since every row on a group's tab shares one.
func TestFilterKeepsLabelAndGroupApart(t *testing.T) {
	for _, q := range []string{"nh", "bo"} { // N(OTES)+(Sc)h(ool), B(ASH)+(C)o(nfig)
		m := groupModel(t)
		m.list.SetFilterText(q)
		if got := shown(m); len(got) != 0 {
			t.Errorf("All, filter %q: got %q, want nothing", q, got)
		}
	}
	m := pressTab(pressTab(groupModel(t))) // School
	m.list.SetFilterText("ol")
	if got := shown(m); len(got) != 0 {
		t.Errorf("School, filter ol: got %q, want nothing (the group name doesn't count here)", got)
	}

	// Label matches come first, then rows whose group matches. A group-only
	// match underlines nothing in the label.
	m = groupModelOf(t,
		config.Bookmark{Label: "NOTES", Path: "notes", Group: "School"},
		config.Bookmark{Label: "SCHOOLWORK", Path: "work"},
	)
	m.list.SetFilterText("school")
	if got := shown(m); !slices.Equal(got, []string{"SCHOOLWORK", "NOTES"}) {
		t.Errorf("All, filter school: got %q, want the label match first", got)
	}
	if got := m.list.MatchesForItem(1); len(got) != 0 {
		t.Errorf("NOTES matched by its group should underline nothing, got %v", got)
	}
}

// A hand-edited group named All or Other (the menu refuses those names)
// doesn't make a second tab of that name: its bookmarks count as ungrouped.
func TestReservedGroupNamesCountAsUngrouped(t *testing.T) {
	m := groupModelOf(t,
		config.Bookmark{Label: "BASH", Path: "bash", Group: "Config"},
		config.Bookmark{Label: "OLD", Path: "old", Group: "other"},
		config.Bookmark{Label: "TMP", Path: "tmp"},
		config.Bookmark{Label: "EVERY", Path: "every", Group: "ALL"},
	)
	if got, want := tabBar(t, m), []string{"All", "Config", "Other"}; !slices.Equal(got, want) {
		t.Errorf("tab bar: got %q, want %q", got, want)
	}
	m = pressShiftTab(m) // Other
	if got, want := shown(m), []string{"OLD", "TMP", "EVERY"}; !slices.Equal(got, want) {
		t.Errorf("Other: got %q, want %q", got, want)
	}
}

// While typing in the filter, tab doesn't switch tabs. Switching tabs clears
// a filter that was applied.
func TestTabKeysAndFilter(t *testing.T) {
	m, _ := press(groupModel(t), runes("/"))
	m = pressTab(m)
	if m.list.FilterState() != list.Filtering || len(shown(m)) != 6 {
		t.Errorf("tab while typing a filter should stay on All, got state %v and %q", m.list.FilterState(), shown(m))
	}

	m = groupModel(t)
	m.list.SetFilterText("school")
	m = pressTab(m)
	if m.list.FilterState() != list.Unfiltered {
		t.Errorf("switching tabs should clear the filter, got state %v", m.list.FilterState())
	}
	if got := shown(m); !slices.Equal(got, []string{"BASH", "GHOSTTY"}) {
		t.Errorf("Config after a filter: got %q", got)
	}
}

// a on a group tab adds into that group; on All it adds ungrouped.
func TestAddOnGroupTab(t *testing.T) {
	m, _ := launchFrom(t, pressTab(pressTab(groupModel(t))), "projects/os") // School
	m, _ = press(m, runes("a"))
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEnter}) // takes the suggestion, OS
	got := onDisk(t, m)
	if len(got) != 7 || got[6].Label != "OS" || got[6].Group != "School" {
		t.Fatalf("want OS saved in School, got %+v", got[len(got)-1])
	}
	if labels := shown(m); !slices.Equal(labels, []string{"CS310", "NOTES", "OS"}) {
		t.Errorf("School after adding: got %q", labels)
	}
	if sel, _ := m.list.SelectedItem().(item); sel.Label != "OS" {
		t.Errorf("the new row should be selected, got %q", sel.Label)
	}

	m, _ = launchFrom(t, groupModel(t), "projects/web") // All
	m, _ = press(m, runes("a"))
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if got := onDisk(t, m); got[len(got)-1].Group != "" {
		t.Errorf("adding on All should leave the bookmark ungrouped, got %q", got[len(got)-1].Group)
	}
}

// a for a folder that's saved on another tab switches to All and selects it.
func TestAddAlreadySavedOnAnotherTab(t *testing.T) {
	m := pressTab(pressTab(groupModel(t))) // School
	m.cwd = m.cfg.Bookmarks[0].Path        // BASH, in Config
	m, _ = press(m, runes("a"))
	if got := activeTab(t, m); got != "All" {
		t.Errorf("want the All tab, got %s", got)
	}
	if sel, _ := m.list.SelectedItem().(item); sel.Label != "BASH" {
		t.Errorf("BASH should be selected, got %q", sel.Label)
	}
	if !strings.Contains(plain(m.View()), "already saved as BASH") {
		t.Errorf("want the already-saved message:\n%s", plain(m.View()))
	}
}

// r on a group tab removes the bookmark highlighted there. When the group
// empties, its tab goes away and the menu shows All.
func TestRemoveOnGroupTab(t *testing.T) {
	m := pressTab(pressTab(groupModel(t))) // School
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyDown})
	m, _ = press(m, runes("r"))
	if !strings.Contains(m.View(), "Remove NOTES? y/N") {
		t.Fatalf("want the prompt for NOTES:\n%s", plain(m.View()))
	}
	m, _ = press(m, runes("y"))
	if got := shown(m); !slices.Equal(got, []string{"CS310"}) {
		t.Errorf("School after removing NOTES: got %q", got)
	}

	m, _ = press(m, runes("r"))
	m, _ = press(m, runes("y")) // CS310, the last in School
	if got, want := tabBar(t, m), []string{"All", "Config", "Other"}; !slices.Equal(got, want) {
		t.Errorf("School should be gone: got %q, want %q", got, want)
	}
	if got := activeTab(t, m); got != "All" {
		t.Errorf("want All after the group empties, got %s", got)
	}
	if got := len(onDisk(t, m)); got != 4 {
		t.Errorf("want 4 bookmarks left in the file, got %d", got)
	}
}

// c on a group tab still clears every bookmark, and says how many.
func TestClearOnGroupTab(t *testing.T) {
	m := pressTab(groupModel(t)) // Config
	m, _ = press(m, runes("c"))
	if !strings.Contains(m.View(), "Delete all 6 bookmarks? y/N") {
		t.Fatalf("c should count every bookmark:\n%s", plain(m.View()))
	}
	m, _ = press(m, runes("y"))
	if len(onDisk(t, m)) != 0 || !strings.Contains(m.View(), emptyMessage) {
		t.Errorf("want everything cleared and the empty state:\n%s", plain(m.View()))
	}
}

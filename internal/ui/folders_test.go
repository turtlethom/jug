package ui

import (
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/turtlethom/pathpin/internal/config"
)

// started runs Init's commands and feeds their messages to Update, as Bubble
// Tea does when the program starts.
func started(m Model) Model { return runCmd(m, m.Init()) }

// runCmd runs cmd, feeds its messages to Update (each command of a batch in
// turn), and follows the commands Update returns. Only use it where no timer
// can come up (status messages and cursor blinks sleep). Like Bubble Tea, it
// skips a command that returns no message.
func runCmd(m Model, cmd tea.Cmd) Model {
	if cmd == nil {
		return m
	}
	switch msg := cmd().(type) {
	case nil:
	case tea.BatchMsg:
		for _, c := range msg {
			m = runCmd(m, c)
		}
	default:
		next, cmd := m.Update(msg)
		m = runCmd(next.(Model), cmd)
	}
	return m
}

// checkNow runs Init's checks now and returns the messages they send, to
// deliver later.
func checkNow(m Model) []tea.Msg {
	var msgs []tea.Msg
	for _, c := range m.Init()().(tea.BatchMsg) {
		if msg := c(); msg != nil {
			msgs = append(msgs, msg)
		}
	}
	return msgs
}

// deliver feeds msgs to Update and drops the commands it returns.
func deliver(m Model, msgs ...tea.Msg) Model {
	for _, msg := range msgs {
		next, _ := m.Update(msg)
		m = next.(Model)
	}
	return m
}

// unstarted builds the menu over these bookmarks without running Init, so no
// folder has been checked yet. Paths starting with "!" are missing on disk.
func unstarted(t *testing.T, bookmarks ...config.Bookmark) Model {
	t.Helper()
	m := newTestModel(t, bookmarks...) // creates the folders and the config file
	return New(m.cfg, m.configPath).sized(80, 24)
}

// rowFor is the row for the bookmark with this label among the list's items.
func rowFor(t *testing.T, m Model, label string) item {
	t.Helper()
	for _, li := range m.list.Items() {
		if it := li.(item); it.Label == label {
			return it
		}
	}
	t.Fatalf("no %s row: %q", label, shown(m))
	return item{}
}

func nvimAndGone(t *testing.T) Model {
	return unstarted(t,
		config.Bookmark{Label: "NVIM", Path: ".config/nvim"},
		config.Bookmark{Label: "GONE", Path: "!gone"},
	)
}

// The menu opens without touching the disk: rows show as present until their
// check comes back. Init checks each folder in its own command, so a hung
// mount holds up only its own row.
func TestFoldersCheckedInBackground(t *testing.T) {
	m := nvimAndGone(t)
	if rowFor(t, m, "GONE").missing || strings.Contains(plain(m.View()), "(missing)") {
		t.Errorf("before its check, a row should show as present:\n%s", plain(m.View()))
	}
	batch, ok := m.Init()().(tea.BatchMsg)
	if !ok || len(batch) != 2 {
		t.Fatalf("Init should batch one check per bookmark, got %T (%d)", batch, len(batch))
	}
	m = started(m)
	if !rowFor(t, m, "GONE").missing || rowFor(t, m, "NVIM").missing {
		t.Errorf("after the checks, only GONE should be missing: %+v", m.list.Items())
	}
	if !strings.Contains(plain(m.View()), "(missing)") {
		t.Errorf("want GONE drawn as missing:\n%s", plain(m.View()))
	}
}

// A check reports only a folder that's gone: rows start as present, so there's
// nothing to say otherwise (and Bubble Tea skips a command's nil message).
func TestCheckReportsOnlyGone(t *testing.T) {
	m := nvimAndGone(t)
	if msg := checkFolder(rowFor(t, m, "NVIM").Path)(); msg != nil {
		t.Errorf("a folder that exists should send nothing, got %#v", msg)
	}
	gone := rowFor(t, m, "GONE").Path
	if msg := checkFolder(gone)(); msg != (folderGoneMsg{path: gone}) {
		t.Errorf("want folderGoneMsg for GONE, got %#v", msg)
	}
}

// A slow check that found the folder can't undo what enter found since.
func TestLateCheckCantUndoEnter(t *testing.T) {
	m := nvimAndGone(t)
	late := checkNow(m) // NVIM is still there
	if err := os.RemoveAll(rowFor(t, m, "NVIM").Path); err != nil {
		t.Fatal(err)
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	m = deliver(m, late...)
	if !rowFor(t, m, "NVIM").missing {
		t.Error("NVIM should stay missing after a late check")
	}
}

// Tabs and reloads use what the checks found instead of checking again. enter
// still checks before it cds.
func TestTabSwitchUsesKnownState(t *testing.T) {
	m := groupModelOf(t,
		config.Bookmark{Label: "OS", Path: "os", Group: "School"},
		config.Bookmark{Label: "LAB", Path: "!lab", Group: "School"},
		config.Bookmark{Label: "NOTES", Path: "notes"},
	)
	lab := rowFor(t, m, "LAB")
	if !lab.missing {
		t.Fatal("LAB should be missing after the checks")
	}
	if err := os.MkdirAll(lab.Path, 0o755); err != nil {
		t.Fatal(err)
	}
	m = pressTab(m) // School
	if !rowFor(t, m, "LAB").missing {
		t.Error("a tab switch shouldn't check folders again")
	}
	m = selectLabel(t, m, "LAB")
	m, cmd := press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.chosen != lab.Path || !isQuit(cmd) {
		t.Errorf("enter should check again and cd to the folder that's back, chose %q", m.chosen)
	}
}

// enter on a folder that has gone since its check records it, so the row
// stays missing when the rows are rebuilt.
func TestEnterRecordsGoneFolder(t *testing.T) {
	m := selectLabel(t, groupModel(t), "BASH")
	if err := os.RemoveAll(rowFor(t, m, "BASH").Path); err != nil {
		t.Fatal(err)
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.chosen != "" || !rowFor(t, m, "BASH").missing {
		t.Fatalf("enter on a gone folder should mark it missing, chose %q", m.chosen)
	}
	m = pressShiftTab(pressTab(m)) // to Config and back to All
	if !rowFor(t, m, "BASH").missing {
		t.Error("BASH should stay missing after the rows are rebuilt")
	}
}

// With a filter applied, a check that comes back dims the visible row right
// away, in Update: bubbles' background re-filter would read the rows while
// later keys (r, y) change them. The cursor stays put.
func TestCheckResultWithFilterApplied(t *testing.T) {
	m := nvimAndGone(t)
	m.list.SetFilterText("n") // NVIM and GONE
	m.list.Select(1)
	m = deliver(m, checkNow(m)...) // the commands Update returns are dropped
	visible := m.list.VisibleItems()
	if len(visible) != 2 || !visible[1].(item).missing {
		t.Errorf("want the filtered GONE row missing without a background filter, got %+v", visible)
	}
	if m.list.Index() != 1 || m.list.FilterState() != list.FilterApplied {
		t.Errorf("want the cursor kept on GONE with the filter applied; index %d, state %v", m.list.Index(), m.list.FilterState())
	}
}

// While a filter is being typed, the list re-filters in the background on
// every key, so the result waits: it comes back after a moment and dims the
// row once typing ends.
func TestCheckResultWhileTyping(t *testing.T) {
	m := nvimAndGone(t)
	m, _ = press(m, runes("/"))
	m, _ = press(m, runes("n"))
	next, retry := m.Update(checkNow(m)[0])
	m = next.(Model)
	if retry == nil || rowFor(t, m, "GONE").missing || m.list.FilterState() != list.Filtering {
		t.Fatalf("while typing, the row should wait and the result come back later; missing %v, state %v",
			rowFor(t, m, "GONE").missing, m.list.FilterState())
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEnter}) // apply the filter
	m = deliver(m, retry())                         // after the 100ms wait
	if !rowFor(t, m, "GONE").missing {
		t.Error("once typing ends, the result should dim the row")
	}
}

// Checks come back whatever the menu is doing: a field, a prompt or the
// picker stays open, and the row is marked underneath.
func TestCheckResultInAnyMode(t *testing.T) {
	for _, k := range []string{"m", "n", "r", "t"} {
		m := nvimAndGone(t)
		m, _ = press(m, runes(k))
		mode := m.mode
		if mode == modeBrowse {
			t.Fatalf("%s should leave browse mode", k)
		}
		m = started(m)
		if m.mode != mode || !rowFor(t, m, "GONE").missing {
			t.Errorf("%s: want mode %v kept and GONE missing, got mode %v", k, mode, m.mode)
		}
	}
}

// A check for a bookmark removed meanwhile changes nothing.
func TestCheckResultForRemovedBookmark(t *testing.T) {
	m := nvimAndGone(t)
	checks := m.Init()
	m = selectLabel(t, m, "GONE")
	m, _ = press(m, runes("r"))
	m, _ = press(m, runes("y"))
	m = runCmd(m, checks)
	if len(m.list.Items()) != 1 || rowFor(t, m, "NVIM").missing {
		t.Errorf("want NVIM alone and present, got %+v", m.list.Items())
	}
}

package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/turtlethom/pathpin/internal/config"
)

// These tests call Update and View directly: a Bubble Tea model is just a
// value, and Update is just a function from (model, message) to (model, command).

// newTestModel builds a sized menu over bookmarks under a fresh $HOME. Paths
// starting with "!" are left missing on disk; the rest are created.
func newTestModel(t *testing.T, bookmarks ...config.Bookmark) Model {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	for i, b := range bookmarks {
		missing := strings.HasPrefix(b.Path, "!")
		bookmarks[i].Path = filepath.Join(home, strings.TrimPrefix(b.Path, "!"))
		if !missing {
			if err := os.MkdirAll(bookmarks[i].Path, 0o755); err != nil {
				t.Fatal(err)
			}
		}
	}
	cfg := &config.Config{Bookmarks: bookmarks}
	configPath := filepath.Join(home, ".config", "pathpin", "config.toml")
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	m := started(New(cfg, configPath)) // checks the folders, as the program does at start
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	return next.(Model)
}

// onDisk loads the config the menu saved to.
func onDisk(t *testing.T, m Model) []config.Bookmark {
	t.Helper()
	cfg, err := config.Load(m.configPath)
	if err != nil {
		t.Fatal(err)
	}
	return cfg.Bookmarks
}

func twoBookmarks(t *testing.T) Model {
	return newTestModel(t,
		config.Bookmark{Label: "NVIM", Path: ".config/nvim"},
		config.Bookmark{Label: "NOTES", Path: "notes"},
	)
}

func press(m Model, key tea.KeyMsg) (Model, tea.Cmd) {
	next, cmd := m.Update(key)
	return next.(Model), cmd
}

func runes(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

// isQuit runs cmd, so only call it when cmd can't be a timer.
func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func TestEnterChoosesHighlighted(t *testing.T) {
	m, cmd := press(twoBookmarks(t), tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.HasSuffix(m.chosen, filepath.Join(".config", "nvim")) {
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
		m, cmd := press(twoBookmarks(t), key)
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
	m, _ := press(twoBookmarks(t), runes("/"))
	if m.list.FilterState() != list.Filtering {
		t.Fatalf("/ should start filtering, state is %v", m.list.FilterState())
	}
	// Don't run the returned command: while filtering it includes the
	// cursor-blink timer, which sleeps.
	m, _ = press(m, runes("q"))
	if m.list.FilterState() != list.Filtering || m.list.FilterValue() != "q" {
		t.Errorf("q should be typed into the filter; state %v, value %q",
			m.list.FilterState(), m.list.FilterValue())
	}
	if m.chosen != "" {
		t.Errorf("chose %q while filtering", m.chosen)
	}
}

func TestRowsShowHomeAsTilde(t *testing.T) {
	view := twoBookmarks(t).View()
	if !strings.Contains(view, "~/.config/nvim") {
		t.Errorf("want ~/.config/nvim in the view:\n%s", view)
	}
}

func TestMissingRowRefusesEnter(t *testing.T) {
	m := newTestModel(t, config.Bookmark{Label: "GONE", Path: "!old/project"})
	if !strings.Contains(m.View(), "GONE (missing)") {
		t.Errorf("want a (missing) tag:\n%s", m.View())
	}

	// Don't run the command: it's the status message's timer.
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.chosen != "" {
		t.Errorf("enter on a missing folder chose %q", m.chosen)
	}
	if !strings.Contains(m.View(), "~/old/project no longer exists") {
		t.Errorf("want a status message:\n%s", m.View())
	}
}

// The folder may come back after the menu opened.
func TestEnterRechecksMissingRow(t *testing.T) {
	m := newTestModel(t, config.Bookmark{Label: "BACK", Path: "!back"})
	dir := m.list.SelectedItem().(item).Path
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	m, cmd := press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.chosen != dir || !isQuit(cmd) {
		t.Errorf("want to cd into the restored folder, chose %q", m.chosen)
	}
}

func TestEmptyState(t *testing.T) {
	m := newTestModel(t)
	if !strings.Contains(m.View(), emptyMessage) {
		t.Errorf("want the empty-state message:\n%s", m.View())
	}
	if _, cmd := press(m, tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil {
		t.Error("enter on an empty menu should do nothing")
	}
}

func TestNewStyles(t *testing.T) {
	def := NewStyles(config.Colors{})
	if got := def.item.SelectedTitle.GetBackground(); got != lipgloss.Color("6") {
		t.Errorf("default primary: got %v, want ANSI 6", got)
	}
	if got := def.item.NormalDesc.GetForeground(); got != lipgloss.Color("8") {
		t.Errorf("default tertiary: got %v, want ANSI 8", got)
	}

	set := NewStyles(config.Colors{Primary: "#56B6C2", Tertiary: "#777777"})
	if got := set.item.SelectedTitle.GetBackground(); got != lipgloss.Color("#56B6C2") {
		t.Errorf("configured primary: got %v", got)
	}
	if got := set.missing.NormalTitle.GetForeground(); got != lipgloss.Color("#777777") {
		t.Errorf("missing rows should use tertiary: got %v", got)
	}
}

// The selected row looks like fcd's: the label is a bold, bright white block on
// the primary color, and the path below it is underlined in the tertiary
// color. Unselected rows keep the plain look.
func TestSelectedRowStyle(t *testing.T) {
	s := NewStyles(config.Colors{Primary: "#56B6C2", Tertiary: "#777777"})

	title := s.item.SelectedTitle
	if title.GetBackground() != lipgloss.Color("#56B6C2") || title.GetForeground() != lipgloss.Color("15") || !title.GetBold() {
		t.Errorf("selected label: want bold ANSI 15 on primary, got fg %v bg %v bold %v",
			title.GetForeground(), title.GetBackground(), title.GetBold())
	}
	if !title.GetBorderLeft() {
		t.Error("selected label should keep the left bar")
	}
	desc := s.item.SelectedDesc
	if desc.GetForeground() != lipgloss.Color("#777777") || !desc.GetUnderline() {
		t.Errorf("selected path: want underlined tertiary, got fg %v underline %v",
			desc.GetForeground(), desc.GetUnderline())
	}

	if _, ok := s.item.NormalTitle.GetBackground().(lipgloss.NoColor); !ok {
		t.Errorf("unselected labels should have no background, got %v", s.item.NormalTitle.GetBackground())
	}
	if s.item.NormalDesc.GetUnderline() {
		t.Error("unselected paths should not be underlined")
	}

	// The block means "enter goes here", and enter on a missing row doesn't
	// cd, so a selected missing row keeps the bar but stays dimmed.
	gone := s.missing.SelectedTitle
	if _, ok := gone.GetBackground().(lipgloss.NoColor); !ok {
		t.Errorf("selected missing label should have no background, got %v", gone.GetBackground())
	}
	if gone.GetForeground() != lipgloss.Color("#777777") || !gone.GetBorderLeft() {
		t.Errorf("selected missing label: want tertiary text and the bar, got fg %v bar %v",
			gone.GetForeground(), gone.GetBorderLeft())
	}
}

// A label too long for the window is cut off with an ellipsis, and the
// selected row's padded block must still fit (e.g. in a narrow tmux split).
func TestLongLabelFitsWidth(t *testing.T) {
	m := newTestModel(t,
		config.Bookmark{Label: strings.Repeat("LONG", 20), Path: "a"},
		config.Bookmark{Label: strings.Repeat("GONE", 20), Path: "!b"},
	)
	const width = 30
	next, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 24})
	m = next.(Model)
	// Only the rows are under test. The help line isn't cut to the window
	// either, but Bubble Tea's renderer clips it, so it's harmless on screen.
	m.list.SetShowHelp(false)
	for _, down := range []bool{false, true} { // first row selected, then the missing one
		if down {
			m, _ = press(m, tea.KeyMsg{Type: tea.KeyDown})
		}
		// lipgloss pads every line to the widest, so one check covers them all.
		if w := lipgloss.Width(m.View()); w > width {
			t.Errorf("selected row %d: view is %d wide, window is %d:\n%s", m.list.Index(), w, width, m.View())
		}
	}
}

// The menu shows every label in uppercase, but the config keeps labels as
// typed. Labels are unique ignoring case, so two rows can't look the same.
func TestLabelsShowUppercase(t *testing.T) {
	m := newTestModel(t,
		config.Bookmark{Label: "dotfiles", Path: "cfg/home"},
		config.Bookmark{Label: "Day Job", Path: "work"},
		config.Bookmark{Label: "gone", Path: "!old/project"},
	)
	view := m.View()
	for _, want := range []string{"DOTFILES", "DAY JOB", "GONE (missing)"} {
		if !strings.Contains(view, want) {
			t.Errorf("want %q in the view:\n%s", want, view)
		}
	}
	for _, typed := range []string{"dotfiles", "Day Job", "gone"} {
		if strings.Contains(view, typed) {
			t.Errorf("the view should not show %q as typed:\n%s", typed, view)
		}
	}

	// The filter matches the uppercase labels whatever case you type.
	for _, f := range []string{"dot", "DOT", "Dot"} {
		m.list.SetFilterText(f)
		if got := m.list.VisibleItems(); len(got) != 1 || got[0].(item).Path != m.cfg.Bookmarks[0].Path {
			t.Errorf("filter %q: want only DOTFILES, got %d rows", f, len(got))
		}
	}
	m.list.ResetFilter()

	// Prompts show the label the way the row does.
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyDown}) // Day Job
	m, _ = press(m, runes("r"))
	if !strings.Contains(m.View(), "Remove DAY JOB? y/N") {
		t.Fatalf("want the prompt in uppercase:\n%s", m.View())
	}
	m, _ = press(m, runes("y"))
	got := onDisk(t, m)
	if len(got) != 2 || got[0].Label != "dotfiles" || got[1].Label != "gone" {
		t.Errorf("the file should keep the other labels as typed, got %+v", got)
	}

	m.cwd = m.cfg.Bookmarks[0].Path // dotfiles
	m, _ = press(m, runes("a"))
	if !strings.Contains(m.View(), "This folder is already saved as DOTFILES") {
		t.Errorf("want the already-saved message in uppercase:\n%s", m.View())
	}
}

func TestRemoveConfirmed(t *testing.T) {
	m, _ := press(twoBookmarks(t), runes("r"))
	if m.mode != modeConfirm || !strings.Contains(m.View(), "Remove NVIM? y/N") {
		t.Fatalf("r should prompt:\n%s", m.View())
	}
	// Don't run the command: it's the status message's timer.
	m, _ = press(m, runes("y"))
	if m.mode != modeBrowse {
		t.Errorf("y should return to browse mode")
	}
	if got := onDisk(t, m); len(got) != 1 || got[0].Label != "NOTES" {
		t.Errorf("file should hold only NOTES, got %+v", got)
	}
	if n := len(m.list.Items()); n != 1 {
		t.Errorf("list should have 1 row, has %d", n)
	}
	if !strings.Contains(m.View(), "Removed NVIM") {
		t.Errorf("want a Removed status:\n%s", m.View())
	}
}

// Anything but y cancels, and q in a prompt cancels instead of quitting.
func TestConfirmCancelKeys(t *testing.T) {
	keys := map[string]tea.KeyMsg{
		"n":     runes("n"),
		"q":     runes("q"),
		"enter": {Type: tea.KeyEnter},
		"esc":   {Type: tea.KeyEsc},
	}
	for name, k := range keys {
		for _, start := range []string{"r", "c"} {
			m, _ := press(twoBookmarks(t), runes(start))
			m, cmd := press(m, k)
			if m.mode != modeBrowse || cmd != nil {
				t.Errorf("%s then %s: want browse mode and no command", start, name)
			}
			if len(onDisk(t, m)) != 2 || len(m.list.Items()) != 2 {
				t.Errorf("%s then %s changed the bookmarks", start, name)
			}
		}
	}
}

func TestClearConfirmed(t *testing.T) {
	m, _ := press(twoBookmarks(t), runes("c"))
	if !strings.Contains(m.View(), "Delete all 2 bookmarks? y/N") {
		t.Fatalf("c should prompt with the count:\n%s", m.View())
	}
	m, _ = press(m, runes("Y"))
	if len(onDisk(t, m)) != 0 {
		t.Error("the file should be empty")
	}
	view := m.View()
	if !strings.Contains(view, emptyMessage) || !strings.Contains(view, "Cleared 2 bookmarks. Backup: ~/.config/pathpin/config.toml.bak") {
		t.Errorf("want the empty state and the backup path:\n%s", view)
	}
	bak, err := config.Load(m.configPath + ".bak")
	if err != nil || len(bak.Bookmarks) != 2 {
		t.Errorf("the backup should hold both bookmarks, got %+v, %v", bak, err)
	}
}

// With a filter applied, r acts on the highlighted row and c still means all.
func TestRemoveWithFilterApplied(t *testing.T) {
	m := twoBookmarks(t)
	m.list.SetFilterText("NOTES")
	m, _ = press(m, runes("r"))
	if !strings.Contains(m.View(), "Remove NOTES? y/N") {
		t.Fatalf("r should target the filtered row:\n%s", m.View())
	}
	m, _ = press(m, runes("y"))
	if got := onDisk(t, m); len(got) != 1 || got[0].Label != "NVIM" {
		t.Errorf("file should hold only NVIM, got %+v", got)
	}

	m = twoBookmarks(t)
	m.list.SetFilterText("NOTES")
	m, _ = press(m, runes("c"))
	if !strings.Contains(m.View(), "Delete all 2 bookmarks? y/N") {
		t.Errorf("c should count every bookmark, not just filtered ones:\n%s", m.View())
	}
}

func TestRAndCWhileFilteringAreTyped(t *testing.T) {
	m, _ := press(twoBookmarks(t), runes("/"))
	m, _ = press(m, runes("r"))
	m, _ = press(m, runes("c"))
	if m.mode != modeBrowse || m.list.FilterValue() != "rc" {
		t.Errorf("want r and c typed into the filter, got mode %v, filter %q", m.mode, m.list.FilterValue())
	}
}

func TestRAndCOnEmptyList(t *testing.T) {
	for _, k := range []string{"r", "c"} {
		m, _ := press(newTestModel(t), runes(k))
		if m.mode != modeBrowse {
			t.Errorf("%s on an empty list should not prompt", k)
		}
	}
}

func TestRemoveMissingRow(t *testing.T) {
	m := newTestModel(t, config.Bookmark{Label: "GONE", Path: "!old/project"})
	m, _ = press(m, runes("r"))
	m, _ = press(m, runes("y"))
	if len(onDisk(t, m)) != 0 {
		t.Error("r should remove a bookmark whose folder is missing")
	}
}

// The screen must match the file: a failed save changes nothing.
func TestFailedSaveKeepsBookmark(t *testing.T) {
	m := twoBookmarks(t)
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	m.configPath = filepath.Join(blocker, "config.toml")

	m, _ = press(m, runes("r"))
	m, _ = press(m, runes("y"))
	if len(m.list.Items()) != 2 || len(m.cfg.Bookmarks) != 2 {
		t.Error("a failed save should keep the bookmark")
	}
	if !strings.Contains(m.View(), "couldn't save") {
		t.Errorf("want the save error on screen:\n%s", m.View())
	}
	// The error stays until the next key.
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyDown})
	if strings.Contains(m.View(), "couldn't save") {
		t.Error("the error should clear on the next key")
	}
}

// launchFrom sets the directory pathpin was "launched" from, creating it under $HOME.
func launchFrom(t *testing.T, m Model, rel string) (Model, string) {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, rel)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	m.cwd = dir
	return m, dir
}

func TestAddUsesSuggestedLabel(t *testing.T) {
	m, dir := launchFrom(t, twoBookmarks(t), "projects/app")
	m, _ = press(m, runes("a"))
	if m.mode != modeInput || !strings.Contains(m.View(), "APP") {
		t.Fatalf("a should open the field suggesting APP:\n%s", m.View())
	}
	// Don't run commands from here on: they include the status message's timer.
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	got := onDisk(t, m)
	if len(got) != 3 || got[2].Label != "APP" || got[2].Path != dir {
		t.Errorf("want APP appended for %s, got %+v", dir, got)
	}
	if sel, _ := m.list.SelectedItem().(item); sel.Label != "APP" {
		t.Errorf("the new row should be selected, got %q", sel.Label)
	}
	if m.mode != modeBrowse || !strings.Contains(m.View(), "Added APP") {
		t.Errorf("want browse mode and an Added status:\n%s", m.View())
	}
}

func TestAddTypedLabelIsKeptAsTyped(t *testing.T) {
	m, _ := launchFrom(t, twoBookmarks(t), "work")
	m, _ = press(m, runes("a"))
	m, _ = press(m, runes("Day Job"))
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if got := onDisk(t, m); len(got) != 3 || got[2].Label != "Day Job" {
		t.Errorf("want the typed label as-is, got %+v", got)
	}
	if !strings.Contains(m.View(), "Added DAY JOB") {
		t.Errorf("the status should show the label the way the row does:\n%s", m.View())
	}
}

// In the field, q, r and c are letters, and esc cancels without saving.
func TestAddFieldTypesLettersAndEscCancels(t *testing.T) {
	m, _ := launchFrom(t, twoBookmarks(t), "work")
	m, _ = press(m, runes("a"))
	for _, k := range []string{"q", "r", "c"} {
		m, _ = press(m, runes(k))
	}
	if m.mode != modeInput || m.input.Value() != "qrc" {
		t.Errorf("want q, r, c typed into the field, got mode %v, value %q", m.mode, m.input.Value())
	}
	m, cmd := press(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != modeBrowse || cmd != nil || len(onDisk(t, m)) != 2 {
		t.Error("esc should cancel without saving or quitting")
	}
}

func TestAddAlreadySavedFolder(t *testing.T) {
	m := twoBookmarks(t)
	m.cwd = m.cfg.Bookmarks[1].Path // NOTES
	m, _ = press(m, runes("a"))
	if m.mode != modeBrowse {
		t.Error("a should not open the field for a folder that's already saved")
	}
	if sel, _ := m.list.SelectedItem().(item); sel.Label != "NOTES" {
		t.Errorf("the existing bookmark should be selected, got %q", sel.Label)
	}
	if !strings.Contains(m.View(), "This folder is already saved as NOTES") {
		t.Errorf("want the already-saved message:\n%s", m.View())
	}
}

// A duplicate label can be fixed in place.
func TestAddDuplicateLabelKeepsFieldOpen(t *testing.T) {
	m, _ := launchFrom(t, twoBookmarks(t), "editor")
	m, _ = press(m, runes("a"))
	m, _ = press(m, runes("nvim"))
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if m.mode != modeInput || !strings.Contains(m.View(), "already exists") {
		t.Fatalf("want the field open with the error:\n%s", m.View())
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyCtrlU}) // clear the field
	m, _ = press(m, runes("EDITOR"))
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if got := onDisk(t, m); len(got) != 3 || got[2].Label != "EDITOR" {
		t.Errorf("want EDITOR added after fixing the label, got %+v", got)
	}
}

func TestAddFromMissingFolder(t *testing.T) {
	m := twoBookmarks(t)
	m.cwd = filepath.Join(t.TempDir(), "deleted")
	m, _ = press(m, runes("a"))
	if m.mode != modeBrowse || !strings.Contains(m.View(), "can't add: current folder no longer exists") {
		t.Errorf("want an error instead of the field:\n%s", m.View())
	}
}

func TestAddFailedSaveKeepsList(t *testing.T) {
	m, _ := launchFrom(t, twoBookmarks(t), "work")
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	m.configPath = filepath.Join(blocker, "config.toml")
	m, _ = press(m, runes("a"))
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if len(m.list.Items()) != 2 || len(m.cfg.Bookmarks) != 2 {
		t.Error("a failed save should not add the row")
	}
	if m.mode != modeInput || !strings.Contains(m.View(), "couldn't save") {
		t.Errorf("want the field open with the save error:\n%s", m.View())
	}
}

func TestAddFromEmptyMenu(t *testing.T) {
	m, _ := launchFrom(t, newTestModel(t), "first")
	m, _ = press(m, runes("a"))
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEnter})
	if got := onDisk(t, m); len(got) != 1 || got[0].Label != "FIRST" {
		t.Errorf("a should work from the empty state, got %+v", got)
	}
}

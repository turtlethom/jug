package ui

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/turtlethom/pathpin/internal/config"
)

// pickerRows are the picker's rows, top to bottom, named as in the TOML.
var pickerRows = []string{"theme", "primary", "secondary", "tertiary", "selected_text", "error", "selection", "icons"}

// colorsModel builds a menu with two bookmarks and these [colors].
func colorsModel(t *testing.T, c config.Colors) Model {
	t.Helper()
	return lookModel(t, c, config.Menu{})
}

// lookModel builds a menu with two bookmarks (NVIM, NOTES), these [colors]
// and this [menu].
func lookModel(t *testing.T, c config.Colors, menu config.Menu) Model {
	t.Helper()
	m := twoBookmarks(t) // creates the folders and the config file
	cfg := &config.Config{Colors: c, Menu: menu, Bookmarks: m.cfg.Bookmarks}
	if err := config.Save(m.configPath, cfg); err != nil {
		t.Fatal(err)
	}
	os.Remove(m.configPath + ".bak") // so tests can tell whether the picker saved
	return started(New(cfg, m.configPath)).sized(80, 24)
}

func openPicker(t *testing.T, m Model) Model {
	t.Helper()
	m, _ = press(m, runes("t"))
	if m.mode != modeTheme {
		t.Fatalf("t should open the theme picker:\n%s", plain(m.View()))
	}
	return m
}

// toRow highlights the picker row with this name, from the top.
func toRow(t *testing.T, m Model, name string) Model {
	t.Helper()
	i := slices.Index(pickerRows, name)
	if i < 0 {
		t.Fatalf("no picker row %q", name)
	}
	for range len(pickerRows) { // up to the theme row, whether ↑ stops at the top or wraps
		m, _ = press(m, tea.KeyMsg{Type: tea.KeyUp})
		if strings.Contains(pickerLine(t, m, "theme"), "›") {
			break
		}
	}
	for range i {
		m, _ = press(m, tea.KeyMsg{Type: tea.KeyDown})
	}
	if !strings.Contains(pickerLine(t, m, name), "›") {
		t.Fatalf("want %s highlighted:\n%s", name, plain(m.View()))
	}
	return m
}

// pickerLine is the visible text of the picker row with this name.
func pickerLine(t *testing.T, m Model, name string) string {
	t.Helper()
	for line := range strings.SplitSeq(plain(m.View()), "\n") {
		if f := strings.Fields(strings.ReplaceAll(line, "›", " ")); len(f) > 0 && f[0] == name {
			return line
		}
	}
	t.Fatalf("no %s row in the picker:\n%s", name, plain(m.View()))
	return ""
}

func right(m Model) Model { m, _ = press(m, tea.KeyMsg{Type: tea.KeyRight}); return m }
func left(m Model) Model  { m, _ = press(m, tea.KeyMsg{Type: tea.KeyLeft}); return m }

// saved reports whether the menu has written the config since colorsModel.
func saved(m Model) bool {
	_, err := os.Stat(m.configPath + ".bak")
	return err == nil
}

// t opens the picker with a row per [colors] key plus selection and icons,
// the theme row highlighted, and its keys on the bottom line. The help
// mentions t; the browse help line is hidden while picking.
func TestThemePickerOpens(t *testing.T) {
	m := colorsModel(t, config.Colors{})
	if help, _ := press(m, runes("?")); !helpShows(help, "t", "appearance") {
		t.Errorf("the full help (?) should mention t:\n%s", plain(help.View()))
	}
	m = openPicker(t, m)
	for _, row := range pickerRows {
		pickerLine(t, m, row)
	}
	if !strings.Contains(pickerLine(t, m, "theme"), "›") {
		t.Errorf("the theme row should be highlighted first:\n%s", plain(m.View()))
	}
	if !strings.Contains(pickerLine(t, m, "theme"), "terminal") {
		t.Errorf("no theme set should show terminal: %q", pickerLine(t, m, "theme"))
	}
	if !strings.Contains(pickerLine(t, m, "primary"), "theme's") {
		t.Errorf("an unset color should show theme's: %q", pickerLine(t, m, "primary"))
	}
	view := plain(m.View())
	for _, s := range []string{"d reset", "enter save", "esc cancel"} {
		if !strings.Contains(view, s) {
			t.Errorf("want %q on the picker's bottom line:\n%s", s, view)
		}
	}
	if strings.Contains(view, "a add") {
		t.Errorf("the browse help shouldn't show while picking:\n%s", view)
	}
	if saved(m) {
		t.Error("opening the picker shouldn't save")
	}

	// An empty menu has colors too, with one blank line above the rows as usual.
	lines := strings.Split(plain(openPicker(t, newTestModel(t)).View()), "\n")
	i := slices.IndexFunc(lines, func(l string) bool { return strings.Contains(l, "No bookmarks yet") })
	if i < 0 || i+2 >= len(lines) || strings.TrimSpace(lines[i+1]) != "" || !strings.Contains(lines[i+2], "theme") {
		t.Errorf("want one blank line between the empty message and the picker:\n%s", strings.Join(lines, "\n"))
	}
}

// While typing a filter, t is typed.
func TestTWhileFilteringIsTyped(t *testing.T) {
	m, _ := press(twoBookmarks(t), runes("/"))
	m, _ = press(m, runes("t"))
	if m.mode != modeBrowse || m.list.FilterValue() != "t" {
		t.Errorf("t should be typed into the filter; mode %v, filter %q", m.mode, m.list.FilterValue())
	}
}

// The panel fits the window: the list shrinks to make room, with or without
// the tab bar, and after a resize.
func TestThemePickerFitsWindow(t *testing.T) {
	for name, m := range map[string]Model{
		"no groups": colorsModel(t, config.Colors{}),
		"groups":    groupModel(t).sized(80, 24),
		"empty":     newTestModel(t),
	} {
		m = openPicker(t, m)
		for size, m := range map[string]Model{"opened": m, "resized": m.sized(80, 24)} {
			lines := strings.Split(m.View(), "\n")
			if len(lines) > 24 {
				t.Errorf("%s, %s: %d lines, want at most 24:\n%s", name, size, len(lines), plain(m.View()))
			}
			for _, l := range lines {
				if w := lipgloss.Width(l); w > 80 {
					t.Errorf("%s, %s: a line is %d columns: %q", name, size, w, plain(l))
				}
			}
			pickerLine(t, m, "icons") // the last row still shows
		}
	}
}

// ↑↓ move between rows, not the list's cursor; q doesn't quit.
func TestThemePickerKeysStayInPicker(t *testing.T) {
	m := openPicker(t, colorsModel(t, config.Colors{}))
	for _, k := range []tea.KeyMsg{runes("j"), {Type: tea.KeyDown}} {
		m, _ = press(m, k)
	}
	if !strings.Contains(pickerLine(t, m, "secondary"), "›") {
		t.Errorf("j and ↓ should move down two rows:\n%s", plain(m.View()))
	}
	m, _ = press(m, runes("k"))
	if !strings.Contains(pickerLine(t, m, "primary"), "›") {
		t.Errorf("k should move up a row:\n%s", plain(m.View()))
	}
	if m.list.Index() != 0 {
		t.Errorf("the list's cursor moved to %d", m.list.Index())
	}
	m, cmd := press(m, runes("q"))
	if m.mode != modeTheme || isQuit(cmd) {
		t.Error("q shouldn't quit or close the picker")
	}
}

// ←→ cycle the theme through ThemeNames, wrapping, and the menu is redrawn in
// it right away: the title badge and the selected row use the new accent.
func TestThemeRowCyclesAndPreviews(t *testing.T) {
	withColor(t)
	names := ThemeNames()
	m := openPicker(t, colorsModel(t, config.Colors{}))

	m = right(m)
	if !strings.Contains(pickerLine(t, m, "theme"), names[1]) {
		t.Errorf("→ should show %s: %q", names[1], pickerLine(t, m, "theme"))
	}
	if colorsOf(m.styles) != colorsOf(NewStyles(config.Colors{Theme: names[1]}, "")) {
		t.Errorf("→ should switch the styles to %s", names[1])
	}
	m = left(left(m))
	if !strings.Contains(pickerLine(t, m, "theme"), names[len(names)-1]) {
		t.Errorf("← from terminal should wrap to %s: %q", names[len(names)-1], pickerLine(t, m, "theme"))
	}

	for range names {
		if strings.Contains(pickerLine(t, m, "theme"), "nord") {
			break
		}
		m = right(m)
	}
	view := m.View()
	for _, s := range []string{"PATHPIN", "NVIM"} { // the badge and the selected row's block
		_, word, _ := find(t, cells(rawLine(t, view, s)), s)
		if word[0].bg != bg(nordAccent) {
			t.Errorf("%s should be drawn on nord's accent while previewing, got bg %q", s, word[0].bg)
		}
	}
	if saved(m) {
		t.Error("cycling shouldn't save")
	}
}

// esc closes the picker without saving and puts the saved colors back.
func TestThemePickerEscRestores(t *testing.T) {
	withColor(t)
	m := openPicker(t, colorsModel(t, config.Colors{Theme: "nord"}))
	m = toRow(t, right(m), "primary")
	m = right(m)
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != modeBrowse {
		t.Fatalf("esc should close the picker:\n%s", plain(m.View()))
	}
	if saved(m) || m.cfg.Colors != (config.Colors{Theme: "nord"}) {
		t.Errorf("esc shouldn't save or change the colors, got %+v", m.cfg.Colors)
	}
	if colorsOf(m.styles) != colorsOf(NewStyles(m.cfg.Colors, "")) {
		t.Error("esc should restore the saved colors")
	}
	_, word, _ := find(t, cells(rawLine(t, m.View(), "PATHPIN")), "PATHPIN")
	if word[0].bg != bg(nordAccent) {
		t.Errorf("the badge should be back on nord's accent, got bg %q", word[0].bg)
	}
}

// colorChoices are the terminal's 16 ANSI colors, then the named colors in
// hue order (reds round to pinks, then browns and neutrals light to dark).
// Every named color is one lowercase word that no other color uses, ANSI
// names included, and every value is a valid, unrepeated color.
func TestColorChoices(t *testing.T) {
	choices := colorChoices()
	ansi := []string{"black", "red", "green", "yellow", "blue", "magenta", "cyan", "white"}
	named := [][2]string{
		{"crimson", "#DC143C"}, {"blush", "#E06C75"}, {"salmon", "#F08080"},
		{"copper", "#D19A66"}, {"orange", "#FF9500"},
		{"honey", "#E5C078"}, {"mustard", "#DAA520"}, {"amber", "#FFBF00"},
		{"lime", "#A6E22E"}, {"pistachio", "#98C379"}, {"emerald", "#50C878"}, {"mint", "#3EB489"},
		{"teal", "#008080"}, {"aqua", "#56B6C2"}, {"sky", "#87CEEB"},
		{"cobalt", "#0047AB"}, {"cornflower", "#6495ED"}, {"navy", "#000080"},
		{"amethyst", "#9966CC"}, {"indigo", "#4B0082"}, {"lavender", "#B57EDC"}, {"orchid", "#C678DD"},
		{"rose", "#FF007F"},
		{"sienna", "#A0522D"},
		{"snow", "#FFFFFF"}, {"silver", "#C0C0C0"}, {"slate", "#5C6370"}, {"charcoal", "#36454F"}, {"ink", "#000000"},
	}
	if len(choices) != 16+len(named) {
		t.Fatalf("want %d choices, got %d", 16+len(named), len(choices))
	}
	for i, c := range choices[:16] {
		want := ansi[i%8]
		if i >= 8 {
			want = "bright-" + want
		}
		if c.name != want || c.value != strconv.Itoa(i) {
			t.Errorf("choice %d: want %s · %d, got %s · %s", i, want, i, c.name, c.value)
		}
	}
	for i, c := range choices[16:] {
		if c.name != named[i][0] || !strings.EqualFold(c.value, named[i][1]) {
			t.Errorf("choice %d: want %s · %s, got %s · %s", 16+i, named[i][0], named[i][1], c.name, c.value)
		}
	}

	names, values := map[string]bool{}, map[string]bool{}
	for _, c := range choices {
		if names[c.name] {
			t.Errorf("the name %q is used twice", c.name)
		}
		names[c.name] = true
		if !validColor(c.value) || values[strings.ToLower(c.value)] {
			t.Errorf("%s · %s is invalid or repeated", c.name, c.value)
		}
		values[strings.ToLower(c.value)] = true
	}
	for _, c := range choices[16:] {
		if c.name != strings.ToLower(c.name) || strings.ContainsAny(c.name, "- ") {
			t.Errorf("%q should be one lowercase word", c.name)
		}
	}
}

// ←→ on a color row cycle theme's → the choices → theme's, showing name ·
// value, and the slot's color changes with it.
func TestColorRowCycles(t *testing.T) {
	choices := colorChoices()
	m := toRow(t, openPicker(t, colorsModel(t, config.Colors{})), "primary")

	m = right(m)
	if line := pickerLine(t, m, "primary"); !strings.Contains(line, "black · 0") {
		t.Errorf("→ from theme's should show the first choice: %q", line)
	}
	if got := colorsOf(m.styles).accent; got != lipgloss.Color("0") {
		t.Errorf("primary should preview as 0, got %v", got)
	}
	m = left(m)
	if line := pickerLine(t, m, "primary"); !strings.Contains(line, "theme's") {
		t.Errorf("← should go back to theme's: %q", line)
	}
	m = left(m)
	last := choices[len(choices)-1]
	if line := pickerLine(t, m, "primary"); !strings.Contains(line, last.name+" · "+last.value) {
		t.Errorf("← from theme's should wrap to %s · %s: %q", last.name, last.value, line)
	}

	// h and l work too, on any color row.
	m = toRow(t, m, "error")
	m, _ = press(m, runes("l"))
	if got := colorsOf(m.styles).err; got != lipgloss.Color("0") {
		t.Errorf("l on error should preview 0, got %v", got)
	}
}

// enter saves the draft, keeping the bookmarks and other settings, and the
// menu keeps the new colors.
func TestThemePickerSaves(t *testing.T) {
	m := colorsModel(t, config.Colors{})
	m.cfg.Menu.Icons = true
	m = openPicker(t, m)
	m = right(m) // the first preset
	m = right(toRow(t, m, "primary"))
	m = enter(m)
	if m.mode != modeBrowse {
		t.Fatalf("enter should close the picker:\n%s", plain(m.View()))
	}
	want := config.Colors{Theme: ThemeNames()[1], Primary: "0"}
	cfg, err := config.Load(m.configPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Colors != want || !cfg.Menu.Icons || len(cfg.Bookmarks) != 2 {
		t.Errorf("want colors %+v saved with icons and 2 bookmarks, got %+v, %+v, %d", want, cfg.Colors, cfg.Menu, len(cfg.Bookmarks))
	}
	if m.cfg.Colors != want || colorsOf(m.styles) != colorsOf(NewStyles(want, "")) {
		t.Error("the menu should keep the saved colors")
	}
	if !strings.Contains(plain(m.View()), "Appearance saved") {
		t.Errorf("want a status:\n%s", plain(m.View()))
	}
}

// enter with nothing changed (or changed and changed back) saves nothing,
// since every save replaces the one backup.
func TestThemePickerUnchangedDoesNotSave(t *testing.T) {
	m := enter(openPicker(t, colorsModel(t, config.Colors{Theme: "nord"})))
	if m.mode != modeBrowse || saved(m) {
		t.Errorf("unchanged colors shouldn't save; mode %v, saved %v", m.mode, saved(m))
	}
	if !strings.Contains(plain(m.View()), "Appearance unchanged") {
		t.Errorf("want a status:\n%s", plain(m.View()))
	}
	m = enter(left(right(openPicker(t, m))))
	if saved(m) {
		t.Error("changed and changed back shouldn't save")
	}
}

// Switching the theme keeps colors set on the other rows.
func TestThemeSwitchKeepsOverrides(t *testing.T) {
	m := right(openPicker(t, colorsModel(t, config.Colors{Primary: "#ff0000"})))
	if line := pickerLine(t, m, "primary"); !strings.Contains(line, "#ff0000") {
		t.Errorf("primary should keep its override: %q", line)
	}
	if got := colorsOf(m.styles).accent; got != lipgloss.Color("#ff0000") {
		t.Errorf("the override should still win over the theme, got %v", got)
	}
}

// d resets a row: a color to the theme's, the theme to terminal, which saves
// as no theme at all.
func TestThemePickerResetRow(t *testing.T) {
	m := openPicker(t, colorsModel(t, config.Colors{Theme: "nord", Primary: "6"}))
	if line := pickerLine(t, m, "primary"); !strings.Contains(line, "cyan · 6") {
		t.Errorf("want name · value for an ANSI color: %q", line)
	}
	m, _ = press(m, runes("d"))
	if line := pickerLine(t, m, "theme"); !strings.Contains(line, "terminal") {
		t.Errorf("d on the theme row should pick terminal: %q", line)
	}
	m, _ = press(toRow(t, m, "primary"), runes("d"))
	if line := pickerLine(t, m, "primary"); !strings.Contains(line, "theme's") {
		t.Errorf("d on a color row should reset it: %q", line)
	}
	m = enter(m)
	cfg, err := config.Load(m.configPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Colors != (config.Colors{}) {
		t.Errorf("want no colors saved, got %+v", cfg.Colors)
	}
}

// # on a color row opens a field for any color: it's filled with the row's
// hex (or just #), a bad value keeps it open, enter puts the value in the draft
// (not saved yet), and esc goes back to the picker. The theme row has no field.
func TestThemePickerHexField(t *testing.T) {
	m := openPicker(t, colorsModel(t, config.Colors{}))
	if m, _ = press(m, runes("#")); m.mode != modeTheme {
		t.Errorf("# on the theme row should do nothing:\n%s", plain(m.View()))
	}

	m, _ = press(toRow(t, m, "primary"), runes("#"))
	if m.mode == modeTheme || !strings.Contains(plain(m.View()), "primary: #") {
		t.Fatalf("# should open a primary: field:\n%s", plain(m.View()))
	}
	m, _ = press(m, runes("12"))
	m = enter(m)
	if m.mode == modeTheme || m.inputErr == "" {
		t.Fatalf("#12 should be refused with the field open:\n%s", plain(m.View()))
	}
	m, _ = press(clearField(m), runes("#ABCDEF"))
	m = enter(m)
	if m.mode != modeTheme {
		t.Fatalf("a good value should go back to the picker:\n%s", plain(m.View()))
	}
	if line := pickerLine(t, m, "primary"); !strings.Contains(line, "#ABCDEF") {
		t.Errorf("want the typed value on the row: %q", line)
	}
	if got := colorsOf(m.styles).accent; got != lipgloss.Color("#ABCDEF") {
		t.Errorf("want #ABCDEF previewed, got %v", got)
	}
	if saved(m) {
		t.Error("the field shouldn't save; enter in the picker does")
	}

	m, _ = press(m, runes("#"))
	if !strings.Contains(plain(m.View()), "primary: #ABCDEF") {
		t.Errorf("the field should start with the row's value:\n%s", plain(m.View()))
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != modeTheme || !strings.Contains(pickerLine(t, m, "primary"), "#ABCDEF") {
		t.Errorf("esc should go back to the picker with the draft kept:\n%s", plain(m.View()))
	}
}

// The # field starts with an ANSI number too, and takes one typed in.
func TestThemePickerHexFieldTakesANSI(t *testing.T) {
	m := toRow(t, openPicker(t, colorsModel(t, config.Colors{Tertiary: "8"})), "tertiary")
	m, _ = press(m, runes("#"))
	if !strings.Contains(plain(m.View()), "tertiary: 8") {
		t.Errorf("the field should start with the row's 8:\n%s", plain(m.View()))
	}
	m, _ = press(clearField(m), runes("6"))
	m = enter(m)
	if m.mode != modeTheme || colorsOf(m.styles).muted != lipgloss.Color("6") {
		t.Errorf("6 should be taken; mode %v, muted %v", m.mode, colorsOf(m.styles).muted)
	}
}

// The # field doesn't leave its prompt on the label field used by a and n.
func TestHexFieldKeepsLabelPrompt(t *testing.T) {
	m := toRow(t, openPicker(t, colorsModel(t, config.Colors{})), "primary")
	m, _ = press(m, runes("#"))
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	m, _ = press(m, runes("n"))
	if view := plain(m.View()); !strings.Contains(view, "Label: NVIM") {
		t.Errorf("n should show the Label: field after using #:\n%s", view)
	}
}

// Values the menu ignores (from a hand-edited file) are shown as such.
func TestThemePickerShowsBadValues(t *testing.T) {
	m := openPicker(t, colorsModel(t, config.Colors{Theme: "dracula", Primary: "aqua"}))
	if line := pickerLine(t, m, "theme"); !strings.Contains(line, "dracula (unknown)") {
		t.Errorf("want the unknown theme marked: %q", line)
	}
	if line := pickerLine(t, m, "primary"); !strings.Contains(line, "aqua (ignored)") {
		t.Errorf("want the bad color marked: %q", line)
	}
}

// A failed save keeps the picker open with the error, and the file's colors.
func TestThemePickerFailedSave(t *testing.T) {
	m := colorsModel(t, config.Colors{})
	blocker := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	m.configPath = filepath.Join(blocker, "config.toml")

	m = enter(right(openPicker(t, m)))
	if m.mode != modeTheme || !strings.Contains(plain(m.View()), "couldn't save") {
		t.Errorf("want the picker open with the error:\n%s", plain(m.View()))
	}
	if m.cfg.Colors != (config.Colors{}) {
		t.Errorf("a failed save shouldn't change the colors, got %+v", m.cfg.Colors)
	}
}

// hasBlock reports whether the menu draws the selected row as a block (else
// with just the bar).
func hasBlock(m Model) bool {
	_, none := m.styles.item.SelectedTitle.GetBackground().(lipgloss.NoColor)
	return !none
}

// The selection row shows block (the default) or bar, with a hint. ←→ cycle
// the two (wrapping), the menu previews the change, and d resets to block.
func TestPickerSelectionRow(t *testing.T) {
	m := openPicker(t, colorsModel(t, config.Colors{}))
	if line := pickerLine(t, m, "selection"); !strings.Contains(line, "block") || !strings.Contains(line, "selected row: block or bar") {
		t.Errorf("want block and the hint: %q", line)
	}
	m = right(toRow(t, m, "selection"))
	if line := pickerLine(t, m, "selection"); !strings.Contains(line, "bar") || hasBlock(m) {
		t.Errorf("→ should show and preview bar: %q, block %v", line, hasBlock(m))
	}
	m = right(m)
	if line := pickerLine(t, m, "selection"); !strings.Contains(line, "block") || !hasBlock(m) {
		t.Errorf("→ from bar should wrap to block: %q, block %v", line, hasBlock(m))
	}
	m = left(m)
	if line := pickerLine(t, m, "selection"); !strings.Contains(line, "bar") {
		t.Errorf("← from block should wrap to bar: %q", line)
	}
	m, _ = press(m, runes("d"))
	if line := pickerLine(t, m, "selection"); !strings.Contains(line, "block") || !hasBlock(m) {
		t.Errorf("d should reset to block: %q, block %v", line, hasBlock(m))
	}
	if m, _ = press(m, runes("#")); m.mode != modeTheme {
		t.Error("# is for color rows only")
	}
}

// The icons row shows off (the default) or on, with a hint. ←→ toggle, the
// rows above show the icons right away, and d turns them off.
func TestPickerIconsRow(t *testing.T) {
	m := openPicker(t, colorsModel(t, config.Colors{}))
	if line := pickerLine(t, m, "icons"); !strings.Contains(line, "off") || !strings.Contains(line, "needs a Nerd Font") {
		t.Errorf("want off and the hint: %q", line)
	}
	m = right(toRow(t, m, "icons"))
	if line := pickerLine(t, m, "icons"); !strings.Contains(line, "on") || !strings.Contains(m.View(), folderIcon+" NVIM") {
		t.Errorf("→ should show and preview icons: %q\n%s", line, m.View())
	}
	m = right(m)
	if line := pickerLine(t, m, "icons"); !strings.Contains(line, "off") || strings.Contains(m.View(), folderIcon) {
		t.Errorf("→ from on should wrap to off: %q\n%s", line, m.View())
	}
	m, _ = press(right(m), runes("d"))
	if line := pickerLine(t, m, "icons"); !strings.Contains(line, "off") || strings.Contains(m.View(), folderIcon) {
		t.Errorf("d should turn icons off: %q\n%s", line, m.View())
	}
	if m, _ = press(m, runes("#")); m.mode != modeTheme {
		t.Error("# is for color rows only")
	}
}

// Toggling icons keeps the cursor on its bookmark, and an applied filter
// stays applied, showing the same rows (now with icons).
func TestPickerIconsKeepCursorAndFilter(t *testing.T) {
	m := selectLabel(t, colorsModel(t, config.Colors{}), "NOTES")
	m = right(toRow(t, openPicker(t, m), "icons"))
	if got := selected(m); got != "NOTES" {
		t.Errorf("want the cursor still on NOTES, got %s", got)
	}

	m = colorsModel(t, config.Colors{})
	m.list.SetFilterText("notes")
	m = right(toRow(t, openPicker(t, m), "icons"))
	if m.list.FilterState() != list.FilterApplied || !slices.Equal(shown(m), []string{"NOTES"}) || selected(m) != "NOTES" {
		t.Errorf("want the filter kept with NOTES selected, got state %v, rows %q, selected %s",
			m.list.FilterState(), shown(m), selected(m))
	}
	if !strings.Contains(m.View(), folderIcon+" NOTES") {
		t.Errorf("want the filtered row with its icon:\n%s", m.View())
	}
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.list.FilterState() != list.FilterApplied || !slices.Equal(shown(m), []string{"NOTES"}) || strings.Contains(m.View(), folderIcon) {
		t.Errorf("esc should drop the icons and keep the filter, got state %v, rows %q:\n%s",
			m.list.FilterState(), shown(m), m.View())
	}
}

// enter saves selection and icons with the colors, keeping the bookmarks; the
// menu keeps them. Going back to block and off saves the defaults, which the
// file leaves out.
func TestPickerSavesMenuOptions(t *testing.T) {
	m := openPicker(t, colorsModel(t, config.Colors{Theme: "nord"}))
	m = right(toRow(t, m, "selection"))
	m = enter(right(toRow(t, m, "icons")))
	if m.mode != modeBrowse {
		t.Fatalf("enter should close the picker:\n%s", plain(m.View()))
	}
	cfg, err := config.Load(m.configPath)
	if err != nil {
		t.Fatal(err)
	}
	if want := (config.Menu{Icons: true, Selection: "bar"}); cfg.Menu != want || m.cfg.Menu != want {
		t.Errorf("want %+v saved and kept, got file %+v, menu %+v", want, cfg.Menu, m.cfg.Menu)
	}
	if cfg.Colors.Theme != "nord" || len(cfg.Bookmarks) != 2 {
		t.Errorf("saving should keep the colors and bookmarks, got %+v, %d bookmarks", cfg.Colors, len(cfg.Bookmarks))
	}
	if hasBlock(m) || !strings.Contains(m.View(), folderIcon+" NVIM") {
		t.Errorf("after saving, the menu should keep bar and icons:\n%s", m.View())
	}
	if !strings.Contains(plain(m.View()), "Appearance saved") {
		t.Errorf("want a status:\n%s", plain(m.View()))
	}

	m = openPicker(t, m)
	m = left(toRow(t, m, "selection"))
	m = enter(left(toRow(t, m, "icons")))
	data, err := os.ReadFile(m.configPath)
	if err != nil {
		t.Fatal(err)
	}
	if m.cfg.Menu != (config.Menu{}) || strings.Contains(string(data), "selection") || strings.Contains(string(data), "icons") {
		t.Errorf("block and off are the defaults and shouldn't be written, got %+v:\n%s", m.cfg.Menu, data)
	}
}

// esc puts back the saved selection and icons, and saves nothing.
func TestPickerEscRestoresMenuOptions(t *testing.T) {
	m := openPicker(t, colorsModel(t, config.Colors{}))
	m = right(toRow(t, m, "selection"))
	m = right(toRow(t, m, "icons"))
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != modeBrowse || saved(m) {
		t.Errorf("esc should close the picker without saving; mode %v, saved %v", m.mode, saved(m))
	}
	if !hasBlock(m) || strings.Contains(m.View(), folderIcon) || m.cfg.Menu != (config.Menu{}) {
		t.Errorf("esc should restore block and no icons:\n%s", m.View())
	}
}

// A hand-edited selection the menu doesn't know shows as unknown and steps
// from block; one in another case counts as unchanged.
func TestPickerHandEditedSelection(t *testing.T) {
	m := openPicker(t, lookModel(t, config.Colors{}, config.Menu{Selection: "bars"}))
	if line := pickerLine(t, m, "selection"); !strings.Contains(line, "bars (unknown)") {
		t.Errorf("want the unknown value marked: %q", line)
	}
	if line := pickerLine(t, right(toRow(t, m, "selection")), "selection"); !strings.Contains(line, "bar") || strings.Contains(line, "unknown") {
		t.Errorf("→ from an unknown value should step from block to bar: %q", line)
	}

	m = openPicker(t, lookModel(t, config.Colors{}, config.Menu{Selection: "Bar"}))
	if line := pickerLine(t, m, "selection"); !strings.Contains(line, "bar") || strings.Contains(line, "unknown") {
		t.Errorf("Bar should show as bar: %q", line)
	}
	m = enter(m)
	if saved(m) || !strings.Contains(plain(m.View()), "Appearance unchanged") {
		t.Errorf("Bar is unchanged and shouldn't save; saved %v:\n%s", saved(m), plain(m.View()))
	}
}

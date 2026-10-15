package ui

import (
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/turtlethom/pathpin/internal/config"
)

const nordErr = "#bf616a" // nord's error color, from themes.go

// helpLine is the visible text of the short help line, trimmed.
func helpLine(t *testing.T, m Model) string {
	t.Helper()
	return strings.TrimSpace(plain(rawLine(t, m.View(), "↑/k up")))
}

// The help line is never wider than the window. When it doesn't fit, it
// stops before the first key that doesn't, so every key shown is whole, and
// ends with …. At 80 columns nothing is cut, even with groups: the help may
// use the window's last column, while the list keeps a column of margin on
// each side.
func TestHelpLineFits(t *testing.T) {
	full := helpLine(t, groupModel(t).sized(200, 24))
	for _, w := range []int{80, 50, 30, 20} {
		m := groupModel(t).sized(w, 24)
		if m.list.Width() != w-2 {
			t.Errorf("%d columns: the list is %d wide, want %d (a column of margin on each side)", w, m.list.Width(), w-2)
		}
		for l := range strings.SplitSeq(m.View(), "\n") {
			if got := lipgloss.Width(l); got > w {
				t.Errorf("%d columns: a line is %d wide: %q", w, got, plain(l))
			}
		}
		line := helpLine(t, m)
		if w == 80 {
			if line != full {
				t.Errorf("80 columns: want the whole help line %q, got %q", full, line)
			}
			continue
		}
		shown, cut := strings.CutSuffix(line, " …")
		if !cut || !strings.HasPrefix(full, shown) || !strings.HasPrefix(full[len(shown):], " • ") {
			t.Errorf("%d columns: want whole keys and then …, got %q (full: %q)", w, line, full)
		}
	}
}

// With ? the full help shows, fitting the window's width and height (the list
// shrinks for it), and ? again brings back the one line. At 80 columns every
// key shows; narrower, the columns are cut at the window's edge.
func TestFullHelpFits(t *testing.T) {
	for _, w := range []int{80, 40} {
		m := groupModel(t).sized(w, 24)
		m, _ = press(m, runes("?"))
		if !helpShows(m, "a", "add") || (w == 80 && !helpShows(m, "J/K", "reorder")) {
			t.Errorf("%d columns: ? should show the full help:\n%s", w, plain(m.View()))
		}
		lines := strings.Split(m.View(), "\n")
		if len(lines) > 24 {
			t.Errorf("%d columns: %d lines with ?, want at most 24:\n%s", w, len(lines), plain(m.View()))
		}
		for _, l := range lines {
			if got := lipgloss.Width(l); got > w {
				t.Errorf("%d columns: a full-help line is %d wide: %q", w, got, plain(l))
			}
		}
		m, _ = press(m, runes("?"))
		if len(strings.Split(m.View(), "\n")) > 24 || !strings.Contains(helpLine(t, m), "↓/j down") {
			t.Errorf("%d columns: ? again should bring back the help line:\n%s", w, plain(m.View()))
		}
	}
}

// The full help is one row taller while a filter is applied (esc clear
// filter). Switching tabs clears the filter, so the list grows back: the menu
// still fills the window exactly, with no spare line.
func TestFullHelpAfterTabSwitch(t *testing.T) {
	m := groupModel(t).sized(80, 24)
	for _, k := range []tea.KeyMsg{runes("/"), runes("cs"), {Type: tea.KeyEnter}, runes("?")} {
		m, _ = press(m, k)
	}
	if lines := strings.Split(m.View(), "\n"); len(lines) != 24 {
		t.Fatalf("filter applied with ?: %d lines, want 24", len(lines))
	}
	m = pressTab(m)
	if lines := strings.Split(m.View(), "\n"); len(lines) != 24 {
		t.Errorf("after tab: %d lines, want 24:\n%s", len(lines), plain(m.View()))
	}
}

// pickerKeysLine is the picker's bottom line, every key in it.
const pickerKeysLine = "↑↓/jk row • ←→/hl change • # hex • d reset • enter save • esc cancel"

// The picker's bottom line names every key, with the letter keys next to the
// arrows (h and l were missing). In a narrow window it's cut like the help
// line: whole keys, then ….
func TestPickerKeysLine(t *testing.T) {
	for _, w := range []int{80, 40} {
		m := openPicker(t, colorsModel(t, config.Colors{}).sized(w, 24))
		line := strings.TrimSpace(plain(rawLine(t, m.View(), "↑↓/jk")))
		if w == 80 {
			if line != pickerKeysLine {
				t.Errorf("80 columns: want %q, got %q", pickerKeysLine, line)
			}
			continue
		}
		if got := lipgloss.Width(rawLine(t, m.View(), "↑↓/jk")); got > w {
			t.Errorf("%d columns: the keys line is %d wide", w, got)
		}
		shown, cut := strings.CutSuffix(line, " …")
		if !cut || !strings.HasPrefix(pickerKeysLine, shown) || !strings.HasPrefix(pickerKeysLine[len(shown):], " • ") {
			t.Errorf("%d columns: want whole keys and then …, got %q", w, line)
		}
	}
}

// ? says that shift+tab goes back a tab. The short help line has no room for
// it and keeps "tab group".
func TestTabKeysInHelp(t *testing.T) {
	m := groupModel(t)
	if line := helpLine(t, m); !strings.Contains(line, "tab group") || strings.Contains(line, "⇧") {
		t.Errorf("short help: want tab group as before, got %q", line)
	}
	m, _ = press(m, runes("?"))
	if !helpShows(m, "tab/⇧tab", "group") {
		t.Errorf("? should list tab/⇧tab group:\n%s", plain(m.View()))
	}
}

// checkHint checks one "key desc" hint on a rendered line: the key in the
// accent, the description muted.
func checkHint(t *testing.T, name, line, key, desc string) {
	t.Helper()
	_, k, _ := find(t, cells(line), key+" "+desc)
	for i, c := range k {
		want := fg(nordMuted)
		if i < len([]rune(key)) {
			want = fg(nordAccent)
		} else if c.r == ' ' {
			continue
		}
		if c.fg != want {
			t.Errorf("%s: in %q, %q is drawn in %q, want %q", name, key+" "+desc, string(c.r), c.fg, want)
			return
		}
	}
}

// Every key hint looks the same: keys in the accent, descriptions and the •
// between hints muted. The picker and the fields use • like the help line.
func TestHintColors(t *testing.T) {
	withColor(t)
	m := nordModel(t, config.Bookmark{Label: "NOTES", Path: "notes"}, config.Bookmark{Label: "WORK", Path: "work"})

	help := rawLine(t, m.View(), "↑/k up")
	checkHint(t, "help line", help, "q", "quit")
	checkHint(t, "help line", help, "/", "filter")
	for _, c := range cells(help) {
		if c.r == '•' && c.fg != fg(nordMuted) {
			t.Errorf("help line: • is drawn in %q, want muted", c.fg)
		}
	}

	picker, _ := press(m, runes("t"))
	line := rawLine(t, picker.View(), "reset")
	if want := pickerKeysLine; !strings.Contains(plain(line), want) {
		t.Errorf("picker: want %q, got %q", want, plain(line))
	}
	checkHint(t, "picker", line, "d", "reset")
	checkHint(t, "picker", line, "esc", "cancel")

	field, _ := press(m, runes("n"))
	line = rawLine(t, field.View(), "Label:")
	if !strings.Contains(plain(line), "enter save • esc cancel") {
		t.Errorf("rename field: want enter save • esc cancel, got %q", plain(line))
	}
	checkHint(t, "rename field", line, "enter", "save")

	group, _ := press(m, runes("m"))
	if line := plain(rawLine(t, group.View(), "Group:")); !strings.Contains(line, "tab complete • empty = no group") {
		t.Errorf("group field: want tab complete • empty = no group, got %q", line)
	}
}

// Nothing is drawn in bubbles' own colors: in every state of the menu, each
// colored cell uses one of the theme's five colors. (The picker's swatches
// show other colors on purpose, so they're skipped.)
func TestOnlyThemeColors(t *testing.T) {
	withColor(t)
	allowed := map[string]bool{"": true}
	for _, hex := range []string{nordAccent, nordOnAccent, nordText, nordMuted, nordErr} {
		allowed[fg(hex)], allowed[bg(hex)] = true, true
	}
	keys := func(m Model, ks ...tea.KeyMsg) Model {
		for _, k := range ks {
			m, _ = press(m, k)
		}
		return m
	}
	enterKey := tea.KeyMsg{Type: tea.KeyEnter}
	groups := groupModel(t).sized(80, 16) // several pages, so the page dots show
	gone := nordModel(t, config.Bookmark{Label: "GONE", Path: "!gone"}, config.Bookmark{Label: "NOTES", Path: "notes"})

	states := map[string]Model{
		"browsing":        groups,
		"next page":       keys(groups, tea.KeyMsg{Type: tea.KeyRight}),
		"typing a filter": keys(groups, runes("/"), runes("c")),
		"no matches":      keys(groups, runes("/"), runes("zzz")),
		"filter applied":  keys(groups, runes("/"), runes("cs"), enterKey),
		"full help":       keys(groups, runes("?")),
		"confirm":         keys(groups, runes("r")),
		"rename field":    keys(groups, runes("n")),
		"group field":     keys(groups, runes("m")),
		"picker":          keys(groups, runes("t")),
		"hex field":       keys(groups, runes("t"), tea.KeyMsg{Type: tea.KeyDown}, runes("#")),
		"missing + enter": keys(gone, enterKey),
		"empty":           nordModel(t),
	}
	for name, m := range states {
		for line := range strings.SplitSeq(m.View(), "\n") {
			for _, c := range cells(line) {
				if c.r == '█' {
					continue // a picker swatch
				}
				if !allowed[c.fg] || !allowed[c.bg] {
					t.Errorf("%s: %q is drawn in fg %q bg %q, not a theme color, in %q", name, string(c.r), c.fg, c.bg, plain(line))
					break
				}
			}
		}
		if slices.Contains([]string{"browsing", "next page"}, name) && !strings.Contains(plain(m.View()), "•") {
			t.Errorf("%s: want the page dots to show:\n%s", name, plain(m.View()))
		}
	}
}

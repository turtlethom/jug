package ui

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/turtlethom/pathpin/internal/config"
)

// withColor makes lipgloss draw 24-bit color for the rest of the test. Tests
// run without a terminal, so lipgloss normally draws no color at all. The
// setting is global, so tests in this package must not use t.Parallel.
func withColor(t *testing.T) {
	t.Helper()
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(0) // termenv.TrueColor; untyped, so termenv stays an indirect dependency
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
}

// nordModel builds a menu in the nord theme, whose colors are all hex.
func nordModel(t *testing.T, bookmarks ...config.Bookmark) Model {
	t.Helper()
	m := newTestModel(t, bookmarks...) // creates the folders and the config file
	cfg := &config.Config{Colors: config.Colors{Theme: "nord"}, Bookmarks: m.cfg.Bookmarks}
	return New(cfg, m.configPath).sized(80, 24)
}

// sized sends a window size, as Bubble Tea does when the program starts.
func (m Model) sized(w, h int) Model {
	next, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
	return next.(Model)
}

// fg and bg are the escape codes lipgloss writes for a hex color.
func fg(hex string) string { return "38;2;" + rgb(hex) }
func bg(hex string) string { return "48;2;" + rgb(hex) }

func rgb(hex string) string {
	v, _ := strconv.ParseUint(strings.TrimPrefix(hex, "#"), 16, 32)
	return fmt.Sprintf("%d;%d;%d", v>>16, v>>8&0xFF, v&0xFF)
}

var escape = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func plain(s string) string { return escape.ReplaceAllString(s, "") }

// rawLine is the rendered line (escape codes included) whose visible text
// contains s.
func rawLine(t *testing.T, view, s string) string {
	t.Helper()
	for line := range strings.SplitSeq(view, "\n") {
		if strings.Contains(plain(line), s) {
			return line
		}
	}
	t.Fatalf("no line shows %q:\n%s", s, plain(view))
	return ""
}

// cell is one visible character of a rendered line and the colors it's drawn
// in ("" for the terminal's default).
type cell struct {
	r      rune
	fg, bg string
}

// cells reads the escape codes in a rendered line and returns each visible
// character with its colors. lipgloss sometimes styles one character at a time
// (underlined text) and sometimes whole runs, so tests compare characters.
func cells(raw string) []cell {
	var out []cell
	var fgc, bgc string
	for len(raw) > 0 {
		if loc := escape.FindStringIndex(raw); loc != nil && loc[0] == 0 {
			params := strings.Split(strings.TrimSuffix(strings.TrimPrefix(raw[:loc[1]], "\x1b["), "m"), ";")
			for i := 0; i < len(params); i++ {
				switch p := params[i]; {
				case p == "" || p == "0":
					fgc, bgc = "", ""
				case p == "39":
					fgc = ""
				case p == "49":
					bgc = ""
				case (p == "38" || p == "48") && i+4 < len(params) && params[i+1] == "2":
					c := p + ";2;" + strings.Join(params[i+2:i+5], ";")
					if p == "38" {
						fgc = c
					} else {
						bgc = c
					}
					i += 4
				case (p == "38" || p == "48") && i+2 < len(params) && params[i+1] == "5":
					i += 2
				}
			}
			raw = raw[loc[1]:]
			continue
		}
		r, size := utf8.DecodeRuneInString(raw)
		out = append(out, cell{r, fgc, bgc})
		raw = raw[size:]
	}
	return out
}

// find returns the cells of the first place s appears in the visible text.
func find(t *testing.T, cs []cell, s string) (before, match, after []cell) {
	t.Helper()
	text := make([]rune, len(cs))
	for i, c := range cs {
		text[i] = c.r
	}
	i := strings.Index(string(text), s)
	if i < 0 {
		t.Fatalf("%q not in %q", s, string(text))
	}
	start := utf8.RuneCountInString(string(text)[:i])
	end := start + utf8.RuneCountInString(s)
	return cs[:start], cs[start:end], cs[end:]
}

// allFg reports whether every cell has foreground color c.
func allFg(cs []cell, c string) bool {
	for _, x := range cs {
		if x.fg != c {
			return false
		}
	}
	return len(cs) > 0
}

// nord's colors, from themes.go.
const (
	nordAccent   = "#88c0d0"
	nordOnAccent = "#2e3440"
	nordText     = "#d8dee9"
	nordMuted    = "#616e88"
)

// The title is a badge, PATHPIN in uppercase like the labels: the accent block
// with the theme's text on it, like the selected row. The empty menu draws the
// same badge.
func TestTitleBadge(t *testing.T) {
	withColor(t)
	for name, m := range map[string]Model{
		"bookmarks": nordModel(t, config.Bookmark{Label: "NOTES", Path: "notes"}),
		"empty":     nordModel(t),
	} {
		line := rawLine(t, m.View(), "PATHPIN")
		if strings.Contains(plain(line), "pathpin") {
			t.Errorf("%s: the title should be uppercase, got %q", name, plain(line))
		}
		before, word, after := find(t, cells(line), "PATHPIN")
		for _, c := range word {
			if c.bg != bg(nordAccent) || c.fg != fg(nordOnAccent) {
				t.Fatalf("%s: want the title in %s on the accent (%s), got fg %q bg %q", name, nordOnAccent, nordAccent, c.fg, c.bg)
			}
		}
		// The badge has one space of padding on each side.
		if len(before) == 0 || len(after) == 0 || before[len(before)-1].bg != bg(nordAccent) || after[0].bg != bg(nordAccent) {
			t.Errorf("%s: want one space of the badge on each side of the title", name)
		}
	}
}

// A path's last folder is in the label color; the rest stays muted. This holds
// on the selected row too, but not on a missing row, which stays dimmed.
func TestPathEmphasis(t *testing.T) {
	withColor(t)
	m := nordModel(t,
		config.Bookmark{Label: "APP", Path: "projects/app"},  // selected
		config.Bookmark{Label: "API", Path: "projects/api"},  // not selected
		config.Bookmark{Label: "OLD", Path: "!projects/old"}, // missing
	)
	view := m.View()
	for _, last := range []string{"app", "api"} {
		_, path, _ := find(t, cells(rawLine(t, view, "~/projects/"+last)), "~/projects/"+last)
		prefix, folder := path[:len("~/projects/")], path[len("~/projects/"):]
		if !allFg(prefix, fg(nordMuted)) {
			t.Errorf("%s: ~/projects/ should be muted (%s), got %+v", last, nordMuted, prefix)
		}
		if !allFg(folder, fg(nordText)) {
			t.Errorf("%s: the last folder should use the label color (%s), got %+v", last, nordText, folder)
		}
	}
	if _, path, _ := find(t, cells(rawLine(t, view, "~/projects/old")), "~/projects/old"); !allFg(path, fg(nordMuted)) {
		t.Errorf("a missing row's path should stay muted, got %+v", path)
	}

	// A bookmark of $HOME itself is all "last folder".
	home := nordModel(t, config.Bookmark{Label: "HOME", Path: ""})
	if _, tilde, _ := find(t, cells(rawLine(t, home.View(), "│ ~")), "~"); !allFg(tilde, fg(nordText)) {
		t.Errorf("~ should use the label color, got %+v", tilde)
	}
}

// A path too long for the window is cut in the middle, so the start and the
// last folder stay visible: ~/Desktop/…/pathpin.
func TestLongPathCutInMiddle(t *testing.T) {
	m := newTestModel(t,
		config.Bookmark{Label: "DEEP", Path: "Desktop/wörkspace/TURTLETHOM/linux_tools/pathpin"},
		config.Bookmark{Label: "WIDE", Path: "!a/" + strings.Repeat("x", 60)},
	).sized(30, 24)
	m.list.SetShowHelp(false) // the help line isn't cut to the window either; see TestLongLabelFitsWidth
	view := m.View()

	path := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(plain(rawLine(t, view, "/pathpin"))), "│"))
	if !strings.HasPrefix(path, "~/") || !strings.HasSuffix(path, "/pathpin") || !strings.Contains(path, "…") {
		t.Errorf("want ~/…/pathpin with the middle cut, got %q", path)
	}

	// A last folder that alone is too long is cut from the right instead.
	wide := strings.TrimSpace(plain(rawLine(t, view, "xxx")))
	if !strings.HasSuffix(wide, "…") {
		t.Errorf("an overlong last folder should end in …, got %q", wide)
	}
	// lipgloss pads every line to the widest, so one check covers them all.
	if w := lipgloss.Width(view); w > 30 {
		t.Errorf("view is %d wide, window is 30:\n%s", w, view)
	}
	if !utf8.ValidString(view) {
		t.Error("cutting a path must not split a character (ö is two bytes)")
	}
}

// With no room at all, a path draws nothing rather than overflowing.
func TestCutAtTinyWidths(t *testing.T) {
	for w := -1; w <= 3; w++ {
		got := cutMiddle("~/Desktop/notes", w)
		if lipgloss.Width(got) > max(w, 0) {
			t.Errorf("width %d: got %q, %d columns wide", w, got, lipgloss.Width(got))
		}
	}
}

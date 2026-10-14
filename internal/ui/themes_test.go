package ui

import (
	"math"
	"slices"
	"strconv"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/turtlethom/pathpin/internal/config"
)

// The colors a theme decides, read back from the styles built for it.
type themeColors struct {
	accent, onAccent, text, muted, err lipgloss.TerminalColor
}

func colorsOf(s Styles) themeColors {
	return themeColors{
		accent:   s.item.SelectedTitle.GetBackground(),
		onAccent: s.item.SelectedTitle.GetForeground(),
		text:     s.item.NormalTitle.GetForeground(),
		muted:    s.item.NormalDesc.GetForeground(),
		err:      s.errText.GetForeground(),
	}
}

// terminal (the ANSI defaults) comes first, then the presets by name.
func TestThemeNames(t *testing.T) {
	want := []string{"terminal", "catppuccin-latte", "catppuccin-mocha", "gruvbox-dark",
		"monokai-pro-ristretto", "nord", "one-dark", "tokyo-night"}
	if got := ThemeNames(); !slices.Equal(got, want) {
		t.Errorf("ThemeNames:\n got %v\nwant %v", got, want)
	}
}

// terminal uses the terminal's own palette, so it follows the terminal's theme.
func TestTerminalTheme(t *testing.T) {
	for _, c := range []config.Colors{{}, {Theme: "terminal"}, {Theme: "Terminal"}} {
		got := colorsOf(NewStyles(c))
		want := themeColors{
			accent:   lipgloss.Color("6"),
			onAccent: lipgloss.Color("15"),
			text:     lipgloss.NoColor{},
			muted:    lipgloss.Color("8"),
			err:      lipgloss.Color("1"),
		}
		if got != want {
			t.Errorf("%+v:\n got %+v\nwant %+v", c, got, want)
		}
	}
}

// Every preset sets all five colors in hex, and the text on the selected
// block (the theme's background) is readable against the accent.
func TestPresetThemes(t *testing.T) {
	seen := map[themeColors]string{}
	for _, name := range ThemeNames()[1:] {
		got := colorsOf(NewStyles(config.Colors{Theme: name}))
		for slot, c := range map[string]lipgloss.TerminalColor{
			"accent": got.accent, "on accent": got.onAccent, "text": got.text, "muted": got.muted, "error": got.err,
		} {
			if !isHex(c) {
				t.Errorf("%s: %s should be a hex color, got %#v", name, slot, c)
			}
		}
		if isHex(got.accent) && isHex(got.onAccent) {
			if r := contrast(got.accent.(lipgloss.Color), got.onAccent.(lipgloss.Color)); r < 4.5 {
				t.Errorf("%s: selected label contrast is %.1f:1, want at least 4.5:1", name, r)
			}
		}
		if other, dup := seen[got]; dup {
			t.Errorf("%s has the same colors as %s", name, other)
		}
		seen[got] = name
	}

	// Names match in any case, like labels.
	if colorsOf(NewStyles(config.Colors{Theme: "NORD"})) != colorsOf(NewStyles(config.Colors{Theme: "nord"})) {
		t.Error("theme names should match in any case")
	}
}

// Colors set in [colors] win over the theme's, one slot at a time.
func TestColorsOverrideTheme(t *testing.T) {
	nord := colorsOf(NewStyles(config.Colors{Theme: "nord"}))
	got := colorsOf(NewStyles(config.Colors{
		Theme:        "nord",
		Primary:      "#123456",
		SelectedText: "#ABCDEF",
		Error:        "#FF0000",
	}))
	want := nord
	want.accent, want.onAccent, want.err = lipgloss.Color("#123456"), lipgloss.Color("#ABCDEF"), lipgloss.Color("#FF0000")
	if got != want {
		t.Errorf("overrides:\n got %+v\nwant %+v", got, want)
	}

	got = colorsOf(NewStyles(config.Colors{Secondary: "#EEEEEE", Tertiary: "#777777"}))
	if got.text != lipgloss.Color("#EEEEEE") || got.muted != lipgloss.Color("#777777") {
		t.Errorf("secondary/tertiary should override terminal's, got %+v", got)
	}
}

// A misspelled theme falls back to terminal (cmd prints the warning).
func TestUnknownThemeFallsBack(t *testing.T) {
	if colorsOf(NewStyles(config.Colors{Theme: "nrod"})) != colorsOf(NewStyles(config.Colors{})) {
		t.Error("an unknown theme should look like terminal")
	}
}

func isHex(c lipgloss.TerminalColor) bool {
	s, ok := c.(lipgloss.Color)
	if !ok || len(s) != 7 || s[0] != '#' {
		return false
	}
	_, err := strconv.ParseUint(string(s[1:]), 16, 32)
	return err == nil
}

// contrast is the WCAG contrast ratio between two "#RRGGBB" colors.
func contrast(a, b lipgloss.Color) float64 {
	la, lb := luminance(a), luminance(b)
	return (math.Max(la, lb) + 0.05) / (math.Min(la, lb) + 0.05)
}

func luminance(c lipgloss.Color) float64 {
	v, _ := strconv.ParseUint(string(c[1:]), 16, 32)
	channel := func(shift uint) float64 {
		x := float64((v>>shift)&0xFF) / 255
		if x <= 0.03928 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	return 0.2126*channel(16) + 0.7152*channel(8) + 0.0722*channel(0)
}

// A value that isn't a color is ignored, so the theme's color shows instead of
// none (cmd prints the warning). Hex is #RGB or #RRGGBB; ANSI is 0-255.
func TestInvalidColorsAreIgnored(t *testing.T) {
	nord := colorsOf(NewStyles(config.Colors{Theme: "nord"}))
	got := colorsOf(NewStyles(config.Colors{
		Theme:        "nord",
		Primary:      "monokai-pro-ristretto",
		Secondary:    "#GGGGGG",
		Tertiary:     "256",
		SelectedText: "#12345",
		Error:        "red",
	}))
	if got != nord {
		t.Errorf("invalid colors should leave the theme's:\n got %+v\nwant %+v", got, nord)
	}

	for _, ok := range []string{"#56B6C2", "#56b6c2", "#abc", "0", "6", "255"} {
		if got := colorsOf(NewStyles(config.Colors{Primary: ok})); got.accent != lipgloss.Color(ok) {
			t.Errorf("%q is a color, got accent %v", ok, got.accent)
		}
	}
	// ANSI numbers are digits only: strconv.Atoi alone would take a sign.
	for _, bad := range []string{"+5", "-0", " 5", "5 ", "#+bc"} {
		if validColor(bad) {
			t.Errorf("%q shouldn't be a color", bad)
		}
	}
}

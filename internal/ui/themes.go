package ui

import (
	"slices"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/turtlethom/pathpin/internal/config"
)

// palette is the five colors a theme decides. lipgloss.TerminalColor is an
// interface: lipgloss.Color("6") is ANSI color 6, which the terminal's own
// palette decides; lipgloss.Color("#56b6c2") is an exact color, which lipgloss
// rounds to the nearest one on terminals with fewer colors; NoColor{} keeps
// the terminal's default.
type palette struct {
	accent   lipgloss.TerminalColor // selected row's block, title, filter
	onAccent lipgloss.TerminalColor // text on the accent
	text     lipgloss.TerminalColor // labels
	muted    lipgloss.TerminalColor // paths, missing rows, status bar
	err      lipgloss.TerminalColor // errors on the bottom line
}

// terminalTheme uses the terminal's own palette, so it follows whatever theme
// the terminal has. It's the default.
const terminalTheme = "terminal"

// themes returns every theme by name. It's a function rather than a package
// variable so there's no package-level mutable state.
//
// The presets take their colors from each theme's own palette: the text on the
// accent is the theme's background, and paths use its comment color.
func themes() map[string]palette {
	return map[string]palette{
		terminalTheme: {
			accent:   lipgloss.Color("6"),
			onAccent: lipgloss.Color("15"),
			text:     lipgloss.NoColor{},
			muted:    lipgloss.Color("8"),
			err:      lipgloss.Color("1"),
		},
		"catppuccin-latte":      hex("#8839ef", "#eff1f5", "#4c4f69", "#7c7f93", "#d20f39"),
		"catppuccin-mocha":      hex("#cba6f7", "#1e1e2e", "#cdd6f4", "#9399b2", "#f38ba8"),
		"gruvbox-dark":          hex("#fabd2f", "#282828", "#ebdbb2", "#928374", "#fb4934"),
		"monokai-pro-ristretto": hex("#85dacc", "#2c2525", "#fff1f3", "#72696a", "#fd6883"),
		"nord":                  hex("#88c0d0", "#2e3440", "#d8dee9", "#616e88", "#bf616a"),
		"one-dark":              hex("#56b6c2", "#282c34", "#abb2bf", "#5c6370", "#e06c75"),
		"tokyo-night":           hex("#7aa2f7", "#1a1b26", "#c0caf5", "#565f89", "#f7768e"),
	}
}

// hex builds a palette from five "#rrggbb" colors, so each preset fits on one
// line.
func hex(accent, onAccent, text, muted, err string) palette {
	return palette{
		accent:   lipgloss.Color(accent),
		onAccent: lipgloss.Color(onAccent),
		text:     lipgloss.Color(text),
		muted:    lipgloss.Color(muted),
		err:      lipgloss.Color(err),
	}
}

// ThemeNames lists the themes for messages and docs: terminal first, then the
// presets by name.
func ThemeNames() []string {
	var names []string
	for name := range themes() {
		if name != terminalTheme {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return append([]string{terminalTheme}, names...)
}

// KnownTheme reports whether name is a theme, in any case. Empty means the
// default, so it counts as known.
func KnownTheme(name string) bool {
	_, ok := lookupTheme(name)
	return ok
}

// lookupTheme finds a theme in any case. Empty or unknown names give
// terminal; ok is false only for an unknown name.
func lookupTheme(name string) (p palette, ok bool) {
	all := themes()
	if name == "" {
		return all[terminalTheme], true
	}
	// The map's keys are lowercase, so lowercasing name matches any case.
	if p, ok := all[strings.ToLower(name)]; ok {
		return p, true
	}
	return all[terminalTheme], false
}

// validColor reports whether lipgloss can draw s: "#RGB", "#RRGGBB", or an
// ANSI color number 0-255. lipgloss draws anything else as no color at all.
func validColor(s string) bool {
	if h, ok := strings.CutPrefix(s, "#"); ok {
		if len(h) != 3 && len(h) != 6 {
			return false
		}
		_, err := strconv.ParseUint(h, 16, 32) // base 16: fails on anything but hex digits
		return err == nil
	}
	// Digits only: Atoi alone would also take a sign, like "+5".
	if s == "" || strings.Trim(s, "0123456789") != "" {
		return false
	}
	n, err := strconv.Atoi(s)
	return err == nil && n <= 255
}

// BadColor is a [colors] value that isn't a color, which the menu ignores.
type BadColor struct {
	Key, Value string // as written in config.toml
}

// InvalidColors lists the [colors] values that aren't colors, in the order
// primary, secondary, tertiary, selected_text, error.
func InvalidColors(c config.Colors) []BadColor {
	var bad []BadColor
	for _, kv := range []BadColor{
		{"primary", c.Primary},
		{"secondary", c.Secondary},
		{"tertiary", c.Tertiary},
		{"selected_text", c.SelectedText},
		{"error", c.Error},
	} {
		if kv.Value != "" && !validColor(kv.Value) {
			bad = append(bad, kv)
		}
	}
	return bad
}

// namedColor is a color the theme picker offers for a [colors] slot.
type namedColor struct {
	name  string
	value string // as saved in config.toml: an ANSI number or "#RRGGBB"
}

// colorChoices lists the picker's colors: the terminal's 16 ANSI colors
// first, which follow the terminal's own palette, then named colors in hue
// order (reds round to pinks, then browns, then neutrals light to dark), so
// ←→ changes gradually. Each named color is one word no other color uses.
// The picker shows name and value, since the value is what's saved.
func colorChoices() []namedColor {
	ansi := []string{"black", "red", "green", "yellow", "blue", "magenta", "cyan", "white"}
	var choices []namedColor
	for i := range 16 {
		name := ansi[i%8]
		if i >= 8 {
			name = "bright-" + name
		}
		choices = append(choices, namedColor{name, strconv.Itoa(i)})
	}
	return append(choices, []namedColor{
		// reds
		{"crimson", "#DC143C"}, {"blush", "#E06C75"}, {"salmon", "#F08080"},
		// oranges
		{"copper", "#D19A66"}, {"orange", "#FF9500"},
		// yellows
		{"honey", "#E5C078"}, {"mustard", "#DAA520"}, {"amber", "#FFBF00"},
		// greens
		{"lime", "#A6E22E"}, {"pistachio", "#98C379"}, {"emerald", "#50C878"}, {"mint", "#3EB489"},
		// cyans
		{"teal", "#008080"}, {"aqua", "#56B6C2"}, {"sky", "#87CEEB"},
		// blues
		{"cobalt", "#0047AB"}, {"cornflower", "#6495ED"}, {"navy", "#000080"},
		// purples
		{"amethyst", "#9966CC"}, {"indigo", "#4B0082"}, {"lavender", "#B57EDC"}, {"orchid", "#C678DD"},
		// pink, brown
		{"rose", "#FF007F"}, {"sienna", "#A0522D"},
		// neutrals, light to dark
		{"snow", "#FFFFFF"}, {"silver", "#C0C0C0"}, {"slate", "#5C6370"}, {"charcoal", "#36454F"}, {"ink", "#000000"},
	}...)
}

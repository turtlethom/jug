package ui

import (
	"slices"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// palette is the five colors a theme decides.
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
	if p, ok := all[strings.ToLower(name)]; ok {
		return p, true
	}
	return all[terminalTheme], false
}

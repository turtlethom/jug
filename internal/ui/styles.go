package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/lipgloss"

	"github.com/turtlethom/pathpin/internal/config"
)

// Styles holds every style the menu uses. Build it with NewStyles and pass it
// in; there are no package-level styles.
type Styles struct {
	app     lipgloss.Style // frame around the whole menu
	title   lipgloss.Style
	status  lipgloss.Style
	empty   lipgloss.Style
	filter  lipgloss.Style // filter prompt and cursor
	prompt  lipgloss.Style // y/N prompts on the bottom line
	note    lipgloss.Style // messages on the bottom line
	errText lipgloss.Style // errors on the bottom line
	muted   lipgloss.Style // hints and placeholders
	item    list.DefaultItemStyles
	missing list.DefaultItemStyles // rows whose folder is gone
	pathEnd lipgloss.TerminalColor // a path's last folder: the label color
	tabBar  lipgloss.Style         // the line of group tabs
	tab     lipgloss.Style         // a tab
	tabOpen lipgloss.Style         // the open tab
	picker  lipgloss.Style         // the theme picker's panel
	picked  lipgloss.Style         // the picker's highlighted row
}

// The ways to show the selected row, set with [menu] selection. Empty means
// block.
const (
	selectBlock = "block" // the label is a block of selected_text on primary (fcd's look)
	selectBar   = "bar"   // just the bar; the label is bold in primary
)

// KnownSelection reports whether s is a [menu] selection the menu knows, in
// any case. Empty counts, since it means block; anything else unknown gives
// block too, and cmd warns about it.
func KnownSelection(s string) bool {
	return s == "" || strings.EqualFold(s, selectBlock) || strings.EqualFold(s, selectBar)
}

// NewStyles builds the menu's styles from the user's theme (default:
// terminal, which follows the terminal's own palette), then applies any colors
// set in [colors] on top, one at a time:
//
//   - primary: accent (selected row's background, title, filter).
//   - secondary: normal text.
//   - tertiary: muted (descriptions, missing rows, status bar).
//   - selected_text: text on the accent.
//   - error: errors on the bottom line.
//
// An unknown theme gives terminal, and a value that isn't a color is ignored;
// cmd warns about both. selection is [menu] selection: bar shows the selected
// row with just the bar, anything else as a block.
func NewStyles(c config.Colors, selection string) Styles {
	theme, _ := lookupTheme(c.Theme)
	primary := pick(c.Primary, theme.accent)
	onPrimary := pick(c.SelectedText, theme.onAccent)
	text := pick(c.Secondary, theme.text)
	muted := pick(c.Tertiary, theme.muted)
	errColor := pick(c.Error, theme.err)

	bar := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(primary).
		Padding(0, 0, 0, 1)
	plain := lipgloss.NewStyle().Padding(0, 0, 0, 2)

	// The selected label is a block on the accent color (padding included, so
	// one space shows on each side), and its path is underlined. The list cuts
	// titles to fit NormalTitle's padding, so it gets the same right padding
	// (invisible without a background) or the block would overflow the window
	// by a column. In bar mode the label is just in the accent color.
	selected := bar.Foreground(onPrimary).Background(primary).Bold(true).PaddingRight(1)
	if strings.EqualFold(selection, selectBar) {
		selected = bar.Foreground(primary).Bold(true).PaddingRight(1)
	}
	item := list.DefaultItemStyles{
		NormalTitle:   plain.Foreground(text).PaddingRight(1),
		NormalDesc:    plain.Foreground(muted),
		SelectedTitle: selected,
		SelectedDesc:  bar.Foreground(muted).Underline(true),
		DimmedTitle:   plain.Foreground(muted),
		DimmedDesc:    plain.Foreground(muted),
		FilterMatch:   lipgloss.NewStyle().Underline(true),
	}
	// The block means "enter goes here", so a missing row never gets it. In
	// bar mode there's no block to remove, so missing rows look the same.
	missing := item
	missing.NormalTitle = item.NormalTitle.Foreground(muted)
	missing.SelectedTitle = item.SelectedTitle.UnsetBackground().Foreground(muted)

	return Styles{
		app:     lipgloss.NewStyle().Padding(1, 1, 0, 1),
		title:   lipgloss.NewStyle().Foreground(onPrimary).Background(primary).Bold(true).Padding(0, 1), // a badge, like the selected row
		status:  lipgloss.NewStyle().Foreground(muted).Padding(0, 0, 1, 2),
		empty:   lipgloss.NewStyle().Foreground(muted).Padding(0, 0, 0, 2),
		filter:  lipgloss.NewStyle().Foreground(primary),
		prompt:  lipgloss.NewStyle().Foreground(primary).Bold(true).Padding(0, 0, 0, 2),
		note:    lipgloss.NewStyle().Foreground(text).Padding(0, 0, 0, 2),
		errText: lipgloss.NewStyle().Foreground(errColor).Padding(0, 0, 0, 2),
		muted:   lipgloss.NewStyle().Foreground(muted),
		item:    item,
		missing: missing,
		pathEnd: text,
		tabBar:  lipgloss.NewStyle().Padding(0, 0, 0, 2), // lines up with the title's text
		tab:     lipgloss.NewStyle().Foreground(muted),
		tabOpen: lipgloss.NewStyle().Foreground(primary).Bold(true),
		picker:  lipgloss.NewStyle().MarginTop(1), // its › sits where the selected row's bar is
		picked:  lipgloss.NewStyle().Foreground(primary).Bold(true),
	}
}

// pick is the color set in [colors], or fallback if it's unset or not a color.
func pick(set string, fallback lipgloss.TerminalColor) lipgloss.TerminalColor {
	if set != "" && validColor(set) {
		return lipgloss.Color(set)
	}
	return fallback
}

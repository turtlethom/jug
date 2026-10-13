package ui

import (
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
// An unknown theme gives terminal; cmd warns about it.
func NewStyles(c config.Colors) Styles {
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
	// by a column.
	item := list.DefaultItemStyles{
		NormalTitle:   plain.Foreground(text).PaddingRight(1),
		NormalDesc:    plain.Foreground(muted),
		SelectedTitle: bar.Foreground(onPrimary).Background(primary).Bold(true).PaddingRight(1),
		SelectedDesc:  bar.Foreground(muted).Underline(true),
		DimmedTitle:   plain.Foreground(muted),
		DimmedDesc:    plain.Foreground(muted),
		FilterMatch:   lipgloss.NewStyle().Underline(true),
	}
	// The block means "enter goes here", so a missing row never gets it.
	missing := item
	missing.NormalTitle = item.NormalTitle.Foreground(muted)
	missing.SelectedTitle = item.SelectedTitle.UnsetBackground().Foreground(muted)

	return Styles{
		app:     lipgloss.NewStyle().Padding(1, 1, 0, 1),
		title:   lipgloss.NewStyle().Foreground(primary).Bold(true),
		status:  lipgloss.NewStyle().Foreground(muted).Padding(0, 0, 1, 2),
		empty:   lipgloss.NewStyle().Foreground(muted).Padding(0, 0, 0, 2),
		filter:  lipgloss.NewStyle().Foreground(primary),
		prompt:  lipgloss.NewStyle().Foreground(primary).Bold(true).Padding(0, 0, 0, 2),
		note:    lipgloss.NewStyle().Foreground(text).Padding(0, 0, 0, 2),
		errText: lipgloss.NewStyle().Foreground(errColor).Padding(0, 0, 0, 2),
		muted:   lipgloss.NewStyle().Foreground(muted),
		item:    item,
		missing: missing,
	}
}

func pick(set string, fallback lipgloss.TerminalColor) lipgloss.TerminalColor {
	if set != "" {
		return lipgloss.Color(set)
	}
	return fallback
}

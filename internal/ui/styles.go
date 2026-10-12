package ui

import (
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/lipgloss"

	"github.com/turtlethom/jug/internal/config"
)

// Styles holds every style the menu uses. Build it with NewStyles and pass it
// in; there are no package-level styles.
type Styles struct {
	app     lipgloss.Style // frame around the whole menu
	title   lipgloss.Style
	status  lipgloss.Style
	empty   lipgloss.Style
	filter  lipgloss.Style // filter prompt and cursor
	item    list.DefaultItemStyles
	missing list.DefaultItemStyles // rows whose folder is gone
}

// NewStyles builds the menu's styles from the user's colors:
//
//   - primary: accent (selected row, title, filter). Default: ANSI cyan.
//   - secondary: normal text. Default: the terminal's own foreground.
//   - tertiary: muted (descriptions, missing rows, status bar). Default: ANSI gray.
//
// The defaults are terminal palette colors, so they follow the terminal's theme.
func NewStyles(c config.Colors) Styles {
	primary := pick(c.Primary, lipgloss.Color("6"))
	text := pick(c.Secondary, lipgloss.NoColor{})
	muted := pick(c.Tertiary, lipgloss.Color("8"))

	bar := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder(), false, false, false, true).
		BorderForeground(primary).
		Padding(0, 0, 0, 1)
	plain := lipgloss.NewStyle().Padding(0, 0, 0, 2)

	item := list.DefaultItemStyles{
		NormalTitle:   plain.Foreground(text),
		NormalDesc:    plain.Foreground(muted),
		SelectedTitle: bar.Foreground(primary).Bold(true),
		SelectedDesc:  bar.Foreground(muted),
		DimmedTitle:   plain.Foreground(muted),
		DimmedDesc:    plain.Foreground(muted),
		FilterMatch:   lipgloss.NewStyle().Underline(true),
	}
	missing := item
	missing.NormalTitle = item.NormalTitle.Foreground(muted)
	missing.SelectedTitle = item.SelectedTitle.Foreground(muted)

	return Styles{
		app:     lipgloss.NewStyle().Padding(1, 1, 0, 1),
		title:   lipgloss.NewStyle().Foreground(primary).Bold(true),
		status:  lipgloss.NewStyle().Foreground(muted).Padding(0, 0, 1, 2),
		empty:   lipgloss.NewStyle().Foreground(muted).Padding(1, 0, 0, 2),
		filter:  lipgloss.NewStyle().Foreground(primary),
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

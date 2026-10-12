package ui

import (
	"os"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/turtlethom/jug/internal/config"
	"github.com/turtlethom/jug/internal/styles"
)

// Run shows the menu and returns the chosen directory, or "" if the user quit.
//
// The menu draws on stderr in the alternate screen, so stdout stays free for
// shell.WriteCDTarget. configPath is where the menu saves changes (steps 4–5).
func Run(cfg *config.Config, configPath string) (string, error) {
	items := make([]list.Item, len(cfg.Bookmarks))
	for i, b := range cfg.Bookmarks {
		items[i] = item{b}
	}
	menu := styles.CreateFCDMenuComponent(items, styles.NewFCDMenuStyles(cfg))

	program := tea.NewProgram(
		NewMenuModel(menu),
		tea.WithAltScreen(),
		tea.WithOutput(os.Stderr),
	)
	final, err := program.Run()
	if err != nil {
		return "", err
	}
	return final.(MenuModel).chosen, nil
}

// item lets a config.Bookmark be shown in a bubbles/list.
type item struct{ config.Bookmark }

func (i item) Title() string       { return i.Label }
func (i item) Description() string { return i.Path }
func (i item) FilterValue() string { return i.Label }

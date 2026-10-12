// Package ui is jug's Bubble Tea menu.
package ui

import (
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/turtlethom/jug/internal/config"
)

// Run shows the menu and returns the chosen directory, or "" if the user quit.
//
// The menu draws on stderr in the alternate screen, so stdout stays free for
// shell.WriteCDTarget. configPath is where the menu saves changes.
func Run(cfg *config.Config, configPath string) (string, error) {
	program := tea.NewProgram(
		New(cfg, configPath),
		tea.WithAltScreen(),
		tea.WithOutput(os.Stderr),
	)
	final, err := program.Run()
	if err != nil {
		return "", err
	}
	return final.(Model).chosen, nil
}

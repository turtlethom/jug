package ui

import (
	"log"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

// Run executes the bubbletea program using a given model
func Run(model tea.Model) tea.Model {
	program := tea.NewProgram(
		model,
		tea.WithAltScreen(),
		tea.WithOutput(os.Stderr),
	)
	final, err := program.Run()
	if err != nil {
		log.Fatal(err)
	}
	return final
}

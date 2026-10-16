// Package ui is pathpin's Bubble Tea menu.
//
// Bubble Tea follows the Elm architecture: the whole menu is one value, Model,
// and three methods drive it.
//
//   - Init returns the first commands to run (here: check every bookmark's
//     folder in the background).
//   - Update receives every message (a key press, a window resize, a
//     command's result) and returns the next Model plus any command to run.
//   - View draws the Model as a string. Bubble Tea redraws after each Update.
//
// A tea.Cmd is a function Bubble Tea runs in the background; the message it
// returns is fed back into Update. Anything that may be slow (checking a
// folder, copying, starting an editor) runs in one, so keys never wait, and
// only Update ever changes the Model.
//
// Update has a value receiver: it works on a copy of the Model and returns
// the new one, so a change takes effect only once it's returned. Helpers that
// take *Model (resize, restyle, ...) edit that copy in place.
//
// There's one Model with a mode field (browse, a y/N prompt, a text field, the
// picker) rather than nested components; Update and View switch on it. The
// list itself (rows, filter, pages, cursor) is bubbles/list, and the item and
// delegate types in model.go tell it how to show a bookmark.
//
// Files:
//
//	run.go      Run: start the program, return the chosen directory
//	model.go    Model, New, Init, Update, View, and each mode's keys
//	styles.go   Styles: every lipgloss style, built from the config's colors
//	themes.go   theme palettes, color checks, the picker's color list
//	picker.go   t, the appearance picker (modeTheme, modeHex)
//	hints.go    the key hints and help under the list
//	actions.go  e, y and o: editor, clipboard, file manager
package ui

import (
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/turtlethom/pathpin/internal/config"
)

// Run shows the menu and returns the chosen directory, or "" if the user quit.
//
// The menu draws on stderr in the alternate screen, so stdout stays free for
// shell.WriteCDTarget. configPath is where the menu saves changes.
func Run(cfg *config.Config, configPath string) (string, error) {
	// The alternate screen is a separate full-screen buffer: when the menu
	// closes, the terminal shows what it showed before.
	program := tea.NewProgram(
		New(cfg, configPath),
		tea.WithAltScreen(),
		tea.WithOutput(os.Stderr),
	)
	final, err := program.Run()
	if err != nil {
		return "", err
	}
	// Run returns the last Model as a tea.Model (an interface); the type
	// assertion gets pathpin's Model back to read what enter chose.
	return final.(Model).chosen, nil
}

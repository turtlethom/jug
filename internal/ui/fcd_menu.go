package ui

import (
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var docStyle = lipgloss.NewStyle().Margin(2, 1)

// Core state of Bubble Tea program
type MenuModel struct {
	List   list.Model // Menu options
	chosen string     // Set only by enter, so quitting never cds (v1 bug)
}

// Init Initializes Bubble Tea Model, can return initial commands
func (m MenuModel) Init() tea.Cmd {
	return nil
}

// Update receives current model and a message, returns new model and a command
func (m MenuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	// Handles keyboard input
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		// While typing a filter, every other key belongs to the filter (v1 bug:
		// q quit mid-filter).
		if m.List.FilterState() == list.Filtering {
			break
		}
		switch msg.String() {
		case "enter":
			if it, ok := m.List.SelectedItem().(item); ok {
				m.chosen = it.Path
			}
			return m, tea.Quit
		case "q":
			return m, tea.Quit
		case "esc":
			// With a filter applied, esc clears it instead (handled by the list).
			if m.List.FilterState() == list.Unfiltered {
				return m, tea.Quit
			}
		}
	// Handles window sizing
	case tea.WindowSizeMsg:
		h, v := docStyle.GetFrameSize()
		m.List.SetSize(msg.Width-h, msg.Height-v)
	}

	m.List, cmd = m.List.Update(msg)
	return m, cmd
}

// View controls how UI looks based on model's state
//
// Returns a string that should be rendered to the terminal
func (m MenuModel) View() string {
	return docStyle.Render(m.List.View())
}

// NewMenuModel is a constructor function for creating a MenuModel instance
func NewMenuModel(fcdListModel list.Model) MenuModel {
	return MenuModel{
		List: fcdListModel,
	}
}

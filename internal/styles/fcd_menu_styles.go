package styles

import (
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/lipgloss"
	"github.com/turtlethom/jug/internal/config"
)

// NewStyles initializes the styles that are configured for bubbles/list component
//
// Returns a Styles struct with all initialized styles for the component
func NewFCDMenuStyles(cfg *config.Config) Styles {
	// Sets colors of the menu based on user configuration
	primary := lipgloss.Color(cfg.Colors.Primary)
	secondary := lipgloss.Color(cfg.Colors.Secondary)
	tertiary := lipgloss.Color(cfg.Colors.Tertiary)

	// Styles for normal items present within bubbles/list component
	NORMAL_PRESETS := lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderLeft(true).
		BorderForeground(secondary)

	// Styles for selected items present within bubbles/list component
	SELECTED_PRESETS := lipgloss.NewStyle().
		BorderStyle(lipgloss.NormalBorder()).
		BorderLeft(true).
		BorderForeground(primary)

	return Styles{
		Primary:   primary,
		Secondary: secondary,
		Tertiary:  tertiary,
		// Styling for FCD Main Menu Title
		MainTitle: lipgloss.NewStyle().
			Foreground(secondary).
			Background(primary).
			Bold(true).
			Padding(0, 1),
		// Styling for normal titles of items in list component
		NormalTitle: lipgloss.NewStyle().
			Inherit(NORMAL_PRESETS).
			Foreground(primary).
			Padding(0, 1),
		// Styling for normal descriptions of items in list component
		NormalDesc: lipgloss.NewStyle().
			Inherit(NORMAL_PRESETS).
			Foreground(secondary).
			Padding(0, 1),
		// Styling for title of selected item in list component
		SelectedTitle: lipgloss.NewStyle().
			Inherit(SELECTED_PRESETS).
			Foreground(secondary).
			Background(primary).
			Padding(0, 1).
			Bold(true),
		// Styling for description of selected item in list component
		SelectedDesc: lipgloss.NewStyle().
			Inherit(SELECTED_PRESETS).
			Foreground(secondary).
			Underline(true).
			Padding(0, 1),
		// Styling for "Filter:"
		StatusBar: lipgloss.NewStyle().
			Foreground(secondary).
			MarginBottom(1).
			Padding(0, 1),
		// Styling for status bar count
		StatusBarCount: lipgloss.NewStyle().
			Foreground(primary),
		// Styling for matching text to filter
		FilterMatch: lipgloss.NewStyle().
			Foreground(secondary).
			Underline(true),
		FilterPrompt: lipgloss.NewStyle().
			Foreground(primary).
			Bold(true),
		FilterCursor: lipgloss.NewStyle().
			Foreground(primary).
			Background(secondary),
	}
}

// Responsible for rendering each item in the FCD Menu
func newFCDMenuDelegate(s Styles) list.DefaultDelegate {
	// Initializing a new delegate that controls how items are rendered within bubbles/list
	delegate := list.NewDefaultDelegate()
	// Normal configuration
	delegate.Styles.NormalTitle = s.NormalTitle
	delegate.Styles.NormalDesc = s.NormalDesc
	// Selected configuration
	delegate.Styles.SelectedTitle = s.SelectedTitle
	delegate.Styles.SelectedDesc = s.SelectedDesc
	// Filter configuration
	delegate.Styles.FilterMatch = s.FilterMatch

	return delegate
}

// CreateFCDMenuComponent creates the FCD Menu component with custom configuration
//
// The bubbles/list component contains strings representing directory paths as defined by Shortcuts in the list,
// as well as the Styles
func CreateFCDMenuComponent(items []list.Item, styles Styles) list.Model {
	// Configured view of each item
	delegate := newFCDMenuDelegate(styles)

	// Instantiating a Bubbles List with items and custom delegate
	list := list.New(items, delegate, 0, 10)

	// Clear out default list title styles
	list.Styles.Title = lipgloss.NewStyle()
	list.Styles.TitleBar = lipgloss.NewStyle()

	// Custom status bar styles
	list.Styles.StatusBar = styles.StatusBar
	// Styles unfiltered items in filter menu
	list.Styles.StatusBarActiveFilter = styles.StatusBarCount
	// Styles the right portion of items count
	list.Styles.StatusBarFilterCount = styles.StatusBarCount
	// Manages Styles For Filter Menu
	list.FilterInput.PromptStyle = styles.FilterPrompt
	list.FilterInput.Cursor.Style = styles.FilterCursor
	// Manages The Title Of The List
	list.Title = styles.MainTitle.Render("jug")

	return list
}

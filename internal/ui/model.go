package ui

import (
	"io"
	"os"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/turtlethom/jug/internal/config"
)

// mode is what the menu is doing right now. Update and View switch on it, so
// each mode only handles its own keys.
type mode int

const (
	modeBrowse mode = iota // the list is active and keys trigger actions
	// modeConfirm arrives in step 4, modeInput in step 5.
)

const emptyMessage = "No bookmarks yet — press a to add this directory"

// Model is the jug menu.
type Model struct {
	mode       mode
	list       list.Model
	styles     Styles
	cfg        *config.Config
	configPath string // where a/r/c save (steps 4–5)
	chosen     string // set only by enter, so quitting never cds
}

// New builds the menu for cfg. It checks once whether each bookmark's folder
// still exists.
func New(cfg *config.Config, configPath string) Model {
	s := NewStyles(cfg.Colors)

	items := make([]list.Item, len(cfg.Bookmarks))
	for i, b := range cfg.Bookmarks {
		items[i] = item{Bookmark: b, missing: !isDir(b.Path)}
	}
	d := delegate{DefaultDelegate: list.NewDefaultDelegate(), missing: s.missing}
	d.Styles = s.item

	l := list.New(items, d, 0, 0)
	l.Title = "jug"
	l.Styles.Title = s.title
	l.Styles.StatusBar = s.status
	l.Styles.StatusEmpty = s.status
	l.FilterInput.PromptStyle = s.filter
	l.FilterInput.Cursor.Style = s.filter
	l.SetStatusBarItemName("bookmark", "bookmarks")
	l.StatusMessageLifetime = 3 * time.Second

	return Model{mode: modeBrowse, list: l, styles: s, cfg: cfg, configPath: configPath}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Messages every mode handles the same way.
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h, v := m.styles.app.GetFrameSize()
		m.list.SetSize(msg.Width-h, msg.Height-v)
		return m, nil
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	}

	switch m.mode {
	case modeBrowse:
		return m.updateBrowse(msg)
	}
	return m, nil
}

func (m Model) updateBrowse(msg tea.Msg) (tea.Model, tea.Cmd) {
	// While typing a filter, every key belongs to the filter (v1 bug: q quit
	// mid-filter), so only intercept keys outside it.
	if key, ok := msg.(tea.KeyMsg); ok && m.list.FilterState() != list.Filtering {
		switch key.String() {
		case "enter":
			return m.choose()
		case "q":
			return m, tea.Quit
		case "esc":
			// With a filter applied, esc clears it instead (handled by the list).
			if m.list.FilterState() == list.Unfiltered {
				return m, tea.Quit
			}
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// choose cds to the highlighted bookmark, unless its folder is gone.
func (m Model) choose() (tea.Model, tea.Cmd) {
	it, ok := m.list.SelectedItem().(item)
	if !ok {
		return m, nil // empty list
	}
	// Check again: the folder may have come back (or vanished) since the menu opened.
	if !isDir(it.Path) {
		it.missing = true
		// GlobalIndex, not Index: with a filter applied they differ.
		m.list.SetItem(m.list.GlobalIndex(), it)
		return m, m.list.NewStatusMessage(config.ContractHome(it.Path) + " no longer exists")
	}
	m.chosen = it.Path
	return m, tea.Quit
}

func (m Model) View() string {
	switch m.mode {
	case modeBrowse:
		if len(m.list.Items()) == 0 {
			return m.styles.app.Render(m.styles.title.Render("jug") + "\n" + m.styles.empty.Render(emptyMessage))
		}
		return m.styles.app.Render(m.list.View())
	}
	return ""
}

// item shows a bookmark in the list.
type item struct {
	config.Bookmark
	missing bool // folder didn't exist when last checked
}

func (i item) Title() string {
	if i.missing {
		return i.Label + " (missing)"
	}
	return i.Label
}
func (i item) Description() string { return config.ContractHome(i.Path) }
func (i item) FilterValue() string { return i.Label }

// delegate draws missing rows with muted styles and everything else normally.
type delegate struct {
	list.DefaultDelegate
	missing list.DefaultItemStyles
}

func (d delegate) Render(w io.Writer, m list.Model, index int, li list.Item) {
	if it, ok := li.(item); ok && it.missing {
		d.Styles = d.missing // d is a copy, so this only affects this row
	}
	d.DefaultDelegate.Render(w, m, index, li)
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

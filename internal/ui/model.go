package ui

import (
	"fmt"
	"io"
	"os"
	"slices"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/turtlethom/jug/internal/config"
)

// mode is what the menu is doing right now. Update and View switch on it, so
// each mode only handles its own keys.
type mode int

const (
	modeBrowse  mode = iota // the list is active and keys trigger actions
	modeConfirm             // a y/N prompt is showing; only y confirms
	// modeInput arrives in step 5.
)

// confirmAction is what a y/N prompt will do if confirmed.
type confirmAction int

const (
	confirmRemove confirmAction = iota
	confirmClear
)

// pending is the action waiting on the prompt.
type pending struct {
	action confirmAction
	label  string // for confirmRemove
}

const emptyMessage = "No bookmarks yet — press a to add this directory"

// Model is the jug menu.
type Model struct {
	mode       mode
	list       list.Model
	styles     Styles
	cfg        *config.Config
	configPath string // where a/r/c save
	chosen     string // set only by enter, so quitting never cds
	pending    pending

	// message is shown on the bottom line until the next key.
	message    string
	messageErr bool
}

// New builds the menu for cfg. It checks once whether each bookmark's folder
// still exists.
func New(cfg *config.Config, configPath string) Model {
	s := NewStyles(cfg.Colors)

	d := delegate{DefaultDelegate: list.NewDefaultDelegate(), missing: s.missing}
	d.Styles = s.item

	l := list.New(itemsFrom(cfg), d, 0, 0)
	l.Title = "jug"
	l.Styles.Title = s.title
	l.Styles.StatusBar = s.status
	l.Styles.StatusEmpty = s.status
	l.FilterInput.PromptStyle = s.filter
	l.FilterInput.Cursor.Style = s.filter
	l.SetStatusBarItemName("bookmark", "bookmarks")
	l.StatusMessageLifetime = 3 * time.Second
	// Add jug's own keys to the list's ? help.
	actionKeys := func() []key.Binding {
		return []key.Binding{
			key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "remove")),
			key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "clear")),
		}
	}
	l.AdditionalShortHelpKeys = actionKeys
	l.AdditionalFullHelpKeys = actionKeys

	return Model{mode: modeBrowse, list: l, styles: s, cfg: cfg, configPath: configPath}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Messages every mode handles the same way.
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		h, v := m.styles.app.GetFrameSize()
		m.list.SetSize(msg.Width-h, msg.Height-v-1) // 1 line for the bottom line
		return m, nil
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		m.message, m.messageErr = "", false
	}

	switch m.mode {
	case modeBrowse:
		return m.updateBrowse(msg)
	case modeConfirm:
		return m.updateConfirm(msg)
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
		case "r":
			if it, ok := m.list.SelectedItem().(item); ok {
				m.mode, m.pending = modeConfirm, pending{action: confirmRemove, label: it.Label}
			}
			return m, nil
		case "c":
			if len(m.cfg.Bookmarks) > 0 {
				m.mode, m.pending = modeConfirm, pending{action: confirmClear}
			}
			return m, nil
		}
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// updateConfirm handles a y/N prompt: y or Y confirms, any other key cancels.
func (m Model) updateConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		// Not a key (e.g. a status message timing out): the list still needs it.
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg)
		return m, cmd
	}
	p := m.pending
	m.mode, m.pending = modeBrowse, pending{}
	if k.String() != "y" && k.String() != "Y" {
		return m, nil
	}
	return m.apply(p)
}

// apply carries out a confirmed action: change a copy, save it, and only then
// swap it in, so the screen always matches the file.
func (m Model) apply(p pending) (tea.Model, tea.Cmd) {
	next := &config.Config{Colors: m.cfg.Colors, Bookmarks: slices.Clone(m.cfg.Bookmarks)}
	switch p.action {
	case confirmRemove:
		if err := next.Remove(p.label); err != nil {
			m.message, m.messageErr = err.Error(), true
			return m, nil
		}
	case confirmClear:
		next.Bookmarks = nil
	}
	if err := config.Save(m.configPath, next); err != nil {
		m.message, m.messageErr = "couldn't save: "+err.Error(), true
		return m, nil
	}

	removed := len(m.cfg.Bookmarks) - len(next.Bookmarks)
	m.cfg = next
	switch p.action {
	case confirmRemove:
		// RemoveItem updates a filtered view right away; SetItems would
		// re-filter in the background and briefly show nothing.
		m.list.RemoveItem(m.list.GlobalIndex())
		if n := len(m.list.VisibleItems()); n > 0 && m.list.Index() >= n {
			m.list.Select(n - 1)
		}
		return m, m.list.NewStatusMessage("Removed " + p.label)
	case confirmClear:
		m.list.ResetFilter()
		m.list.SetItems(nil)
		m.message = fmt.Sprintf("Cleared %d bookmarks. Backup: %s",
			removed, config.ContractHome(m.configPath+".bak"))
	}
	return m, nil
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
	body := m.list.View()
	if len(m.list.Items()) == 0 {
		// Same title bar as the list draws, so the title doesn't jump when it empties.
		body = m.list.Styles.TitleBar.Render(m.styles.title.Render("jug")) + "\n" + m.styles.empty.Render(emptyMessage) + "\n"
	}
	return m.styles.app.Render(body + "\n" + m.bottomLine())
}

// bottomLine is the prompt in confirm mode, otherwise the last message.
func (m Model) bottomLine() string {
	switch {
	case m.mode == modeConfirm && m.pending.action == confirmRemove:
		return m.styles.prompt.Render(fmt.Sprintf("Remove %s? y/N", m.pending.label))
	case m.mode == modeConfirm && m.pending.action == confirmClear:
		return m.styles.prompt.Render(fmt.Sprintf("Delete all %d bookmarks? y/N", len(m.cfg.Bookmarks)))
	case m.messageErr:
		return m.styles.errText.Render(m.message)
	}
	return m.styles.note.Render(m.message)
}

// itemsFrom makes list rows for cfg, checking whether each folder exists.
func itemsFrom(cfg *config.Config) []list.Item {
	items := make([]list.Item, len(cfg.Bookmarks))
	for i, b := range cfg.Bookmarks {
		items[i] = item{Bookmark: b, missing: !isDir(b.Path)}
	}
	return items
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

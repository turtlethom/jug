package ui

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/turtlethom/pathpin/internal/config"
)

// mode is what the menu is doing right now. Update and View switch on it, so
// each mode only handles its own keys.
type mode int

const (
	modeBrowse    mode = iota // the list is active and keys trigger actions
	modeConfirm               // a y/N prompt is showing; only y confirms
	modeInput                 // typing a label for a new bookmark
	modeGroup                 // typing a group for the highlighted bookmark (m)
	modeRename                // typing a new label for the highlighted bookmark (n)
	modeGroupName             // typing a new name for the open group's tab (N)
	modeTheme                 // the theme picker (t) is open
	modeHex                   // typing a color for the picker's highlighted row (#)
)

// confirmAction is what a y/N prompt will do if confirmed.
type confirmAction int

const (
	confirmRemove confirmAction = iota
	confirmClear
	confirmMerge
	confirmUngroup
)

// pending is the action waiting on the prompt.
type pending struct {
	action     confirmAction
	label      string // for confirmRemove
	from, into string // for confirmMerge: the group renamed, and the group it joins; for confirmUngroup: from
}

const emptyMessage = "No bookmarks yet — press a to add this directory"

// menuTitle is the badge at the top, in uppercase like the labels. It's the
// tool's name, never the --cmd name.
const menuTitle = "PATHPIN"

// Model is the pathpin menu.
type Model struct {
	mode       mode
	list       list.Model
	styles     Styles
	cfg        *config.Config
	configPath string // where a/r/c save
	chosen     string // set only by enter, so quitting never cds
	pending    pending

	cwd      string // the directory pathpin was launched from; a adds it
	input    textinput.Model
	inputErr string // shown beside the label or group field until the next key

	group   textinput.Model // m's field; suggests existing group names
	hex     textinput.Model // the theme picker's field for any color (#)
	editing string          // the bookmark (label) or group the open field is for

	// message is shown on the bottom line until the next key.
	message    string
	messageErr bool

	// tab is the open tab by name: config.TabAll, a group, or config.TabOther.
	// A name, not a position, because tabs shift when a group empties.
	tab string
	// themeRow is the appearance picker's highlighted row, and draft the copy
	// of the config it shows; m.cfg stays as saved until enter there. A value,
	// not a pointer, so each Model copy has its own.
	themeRow int
	draft    config.Config

	// gone holds the folders found missing, by path: by the checks Init starts
	// (see checkFolder), or by enter. Rows read it instead of the disk. A path
	// is never removed: a folder that comes back is found by enter, which cds.
	gone map[string]bool

	// width and height are the last window size, to resize the list when the
	// tab bar comes or goes.
	width, height int

	// act is what the action keys do outside the menu (see actions).
	act actions
}

// New builds the menu for cfg. It doesn't touch the disk: Init checks the
// folders in the background, and rows show as present until then.
func New(cfg *config.Config, configPath string) Model {
	gone := map[string]bool{}
	l := list.New(itemsFrom(cfg, config.TabAll, gone), delegate{}, 0, 0) // restyle sets the real delegate
	l.Title = menuTitle
	// One column less than the list's default, so the badge's block lines up
	// with the selected row's.
	l.Styles.TitleBar = l.Styles.TitleBar.PaddingLeft(1)
	l.Filter = filterByRune
	l.SetStatusBarItemName("bookmark", "bookmarks")
	l.StatusMessageLifetime = 3 * time.Second
	// pathpin draws the help itself (helpView): bubbles' can't cut its line to
	// a narrow window. The list still toggles it with ? and says which keys apply.
	l.SetShowHelp(false)
	setHelpKeys(&l, tabs(cfg) != nil)

	group := newField("Group: ")
	group.ShowSuggestions = true // tab completes an existing group's name

	cwd, _ := os.Getwd() // "" if it fails; a then reports the folder is gone
	m := Model{
		mode: modeBrowse, list: l, cfg: cfg, configPath: configPath, cwd: cwd,
		input: newField("Label: "), group: group, hex: newField(""), tab: config.TabAll, gone: gone,
		act: systemActions(),
	}
	m.restyle(cfg)
	return m
}

// restyle draws the menu in cfg's colors and [menu] selection. New calls it
// with the saved config, and the appearance picker with its draft on every
// change.
//
// The styles live in several places, not just m.styles: the list keeps its own
// copies (title, status bar, filter), the delegate draws the rows, and each
// text field styles its prompt. So all of them are set here, in one place.
func (m *Model) restyle(cfg *config.Config) {
	s := NewStyles(cfg.Colors, cfg.Menu.Selection)
	m.styles = s

	d := delegate{DefaultDelegate: list.NewDefaultDelegate(), missing: s.missing, pathEnd: s.pathEnd, tag: s.muted}
	d.Styles = s.item
	d.FullHelpFunc = func() [][]key.Binding { return [][]key.Binding{editKeys()} }
	m.list.SetDelegate(d)
	m.list.Styles.Title = s.title
	m.list.Styles.StatusBar = s.status
	m.list.Styles.StatusEmpty = s.status
	m.list.FilterInput.PromptStyle = s.filter
	m.list.FilterInput.Cursor.Style = s.filter
	m.list.Styles.StatusBarFilterCount = s.muted
	m.list.Styles.DividerDot = s.muted.SetString(" • ")
	m.list.Styles.NoItems = s.muted
	m.list.Styles.ArabicPagination = s.muted
	// The paginator keeps its own copy of the dots, made when the list was.
	m.list.Paginator.ActiveDot = s.key.Render("•")
	m.list.Paginator.InactiveDot = s.muted.Render("•")
	// The full help (?) is still bubbles' columns.
	hs := &m.list.Help.Styles
	hs.FullKey, hs.FullDesc, hs.FullSeparator, hs.Ellipsis = s.key, s.muted, s.muted, s.muted
	hs.ShortKey, hs.ShortDesc, hs.ShortSeparator = s.key, s.muted, s.muted

	for _, f := range []*textinput.Model{&m.input, &m.group, &m.hex} {
		f.PromptStyle = s.filter
		f.Cursor.Style = s.filter
		f.PlaceholderStyle = s.muted
		f.CompletionStyle = s.muted
	}
}

// newField makes a text field for the bottom line. restyle colors it.
func newField(prompt string) textinput.Model {
	in := textinput.New()
	in.Prompt = prompt
	in.CharLimit = 40
	return in
}

// bind is a key for the help: its key and what it does.
func bind(k, desc string) key.Binding {
	return key.NewBinding(key.WithKeys(k), key.WithHelp(k, desc))
}

// editKeys are the keys that act on bookmarks. In ? help they're a column of
// their own, through the delegate (see restyle), so the full help stays short
// enough for an 80x24 terminal.
func editKeys() []key.Binding {
	return []key.Binding{bind("a", "add"), bind("r", "remove"), bind("c", "clear"), bind("m", "move"), bind("n", "rename"), bind("e", "edit")}
}

// setHelpKeys adds pathpin's other keys to the list's help. The short help line
// shows only a and r (and tab while there are tabs), so it fits an 80-column
// terminal with q quit and ? more still showing; ? lists every key, the rest
// under the filter keys, with the group keys last since they come and go.
func setHelpKeys(l *list.Model, withTabs bool) {
	tab := key.NewBinding(key.WithKeys("tab", "shift+tab"), key.WithHelp("tab", "group"))
	l.AdditionalShortHelpKeys = func() []key.Binding {
		k := []key.Binding{bind("a", "add"), bind("r", "remove")}
		if withTabs {
			k = append(k, tab)
		}
		return k
	}
	l.AdditionalFullHelpKeys = func() []key.Binding {
		k := []key.Binding{key.NewBinding(key.WithKeys("J", "K", "shift+down", "shift+up"), key.WithHelp("J/K", "reorder")),
			bind("t", "appearance"), bind("y", "copy path"), bind("o", "open")}
		if withTabs {
			k = append(k, bind("N", "rename group"), tab)
		}
		return k
	}
}

// Init checks every bookmark's folder in the background, one command each,
// so opening the menu never waits on the disk, and a folder on a hung mount
// never gets its result while the rest do. Bubble Tea runs the commands in
// their own goroutines and sends each result to Update. (enter still checks
// its folder synchronously, so enter on a hung mount does wait.)
func (m Model) Init() tea.Cmd {
	var checks []tea.Cmd
	for _, b := range m.cfg.Bookmarks {
		checks = append(checks, checkFolder(b.Path))
	}
	return tea.Batch(checks...)
}

// folderGoneMsg says a bookmark's folder doesn't exist.
type folderGoneMsg struct{ path string }

// checkFolder looks at one folder off the UI and reports it only if it's gone.
// Rows start as present, so there's nothing to say otherwise, and a late
// "present" can't undo what enter found since. Bubble Tea skips a nil message.
//
// A command only computes and reports; it never touches the model, which only
// Update changes, one message at a time.
func checkFolder(path string) tea.Cmd {
	return func() tea.Msg {
		if isDir(path) {
			return nil
		}
		return folderGoneMsg{path: path}
	}
}

// markGone records a folder as missing and dims its row, if it's on the open
// tab. Don't call it while a filter is being typed (see Update).
func (m *Model) markGone(path string) {
	// m.gone is shared by every copy of the Model, as the list's rows are: a
	// map or slice is a reference to the same storage, so an older copy
	// doesn't stay as it was. That's fine because Bubble Tea keeps only the
	// newest model.
	m.gone[path] = true

	i := slices.IndexFunc(m.list.Items(), func(li list.Item) bool { return li.(item).Path == path })
	if i < 0 {
		return // on another tab, or removed meanwhile
	}
	it := m.list.Items()[i].(item)
	if it.missing {
		return
	}
	it.missing = true
	at := m.list.Index()
	// With a filter applied, SetItem returns bubbles' re-filter, which would run
	// in the background reading the rows while later keys (r, y) change them.
	// Drop it and re-filter here instead, keeping the cursor.
	m.list.SetItem(i, it)
	if m.list.FilterState() == list.FilterApplied {
		m.list.SetFilterText(m.list.FilterValue())
		m.list.Select(at)
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Messages every mode handles the same way.
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resize()
		return m, nil
	case copiedMsg:
		return m.actionDone("Copied", msg.path, msg.err)
	case openedMsg:
		return m.actionDone("Opened", msg.path, msg.err)
	case editedMsg:
		return m.edited(msg.err)
	case folderGoneMsg:
		if m.list.FilterState() == list.Filtering {
			// The list re-filters in the background on every key while a
			// filter is typed, reading the rows, so changing one now would
			// race with it. Deliver the message again shortly instead.
			return m, tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg { return msg })
		}
		m.markGone(msg.path)
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
	case modeInput:
		return m.updateField(msg, labelField, Model.addFromField)
	case modeGroup:
		return m.updateField(msg, groupField, Model.move)
	case modeRename:
		return m.updateField(msg, labelField, Model.rename)
	case modeGroupName:
		return m.updateField(msg, groupField, Model.renameGroup)
	case modeTheme:
		return m.updateTheme(msg)
	case modeHex:
		return m.updateField(msg, hexField, Model.setHex)
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
		case "a":
			return m.startAdd()
		case "m":
			return m.startMove()
		case "n":
			return m.startRename()
		case "N":
			return m.startRenameGroup()
		case "t":
			return m.startTheme()
		case "y":
			return m.copyPath()
		case "o":
			return m.openFolder()
		case "e":
			return m.editFolder()
		case "J", "K", "shift+down", "shift+up":
			return m.reorder(key.String() == "J" || key.String() == "shift+down")
		case "tab", "shift+tab":
			names := tabs(m.cfg)
			if names == nil {
				return m, nil // no groups, no tabs
			}
			step := 1
			if key.String() == "shift+tab" {
				step = -1
			}
			i := slices.IndexFunc(names, func(n string) bool { return strings.EqualFold(n, m.tab) })
			return m, m.showTab(names[(i+step+len(names))%len(names)])
		}
	}
	var cmd tea.Cmd
	helpHeight := lipgloss.Height(m.helpView())
	m.list, cmd = m.list.Update(msg)
	// ? opens or closes the full help, and filtering changes its keys. The
	// list doesn't count the help (pathpin draws it), so make room here.
	if lipgloss.Height(m.helpView()) != helpHeight {
		m.resize()
	}
	return m, cmd
}

// resize fits the list to the window, leaving room for the bottom line, the
// help, the tab bar while there are groups, and the theme picker while it's
// open. The list keeps a column of margin on each side.
//
// It has a pointer receiver so it can change the Model it's called on. Update
// has a value receiver, so the m it holds is its own copy and calling
// m.resize() there changes that copy, which Update then returns.
func (m *Model) resize() {
	if m.width == 0 {
		return // no window size yet; Bubble Tea sends one at start
	}
	h, v := m.styles.app.GetFrameSize()
	// The frame has no right padding, so the help line can use that column;
	// the list keeps it as a margin.
	h++
	reserved := 1 // the bottom line
	if help := m.helpView(); help != "" {
		reserved += lipgloss.Height(help)
	}
	if tabs(m.cfg) != nil {
		reserved += 3 // a blank line, the tab bar and its rule
	}
	if m.picking() {
		reserved += 1 + len(appearanceRows()) // a blank line, then the rows
	}
	m.list.SetSize(m.width-h, m.height-v-reserved)
}

// showTab opens a tab: it clears any filter and shows that tab's bookmarks.
// Clearing the filter can shorten the full help (no esc clear filter), so it
// resizes too.
func (m *Model) showTab(name string) tea.Cmd {
	m.tab = name
	m.list.ResetFilter()
	cmd := m.list.SetItems(itemsFrom(m.cfg, name, m.gone))
	m.list.Select(0)
	m.resize()
	return cmd
}

// syncTabs follows a change to the bookmarks. Groups come and go with them,
// so it resizes for the tab bar, updates the help, and opens All if the open
// tab is gone (its last bookmark was removed).
func (m *Model) syncTabs() tea.Cmd {
	names := tabs(m.cfg)
	setHelpKeys(&m.list, names != nil)
	m.resize()
	open := slices.ContainsFunc(names, func(n string) bool { return strings.EqualFold(n, m.tab) })
	if m.tab != config.TabAll && !open {
		return m.showTab(config.TabAll)
	}
	return nil
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
	next := m.edit()
	switch p.action {
	case confirmRemove:
		if err := next.Remove(p.label); err != nil {
			m.message, m.messageErr = err.Error(), true
			return m, nil
		}
	case confirmClear:
		next.Bookmarks = nil
	case confirmMerge, confirmUngroup:
		// For confirmUngroup, into is "", which ungroups.
		if err := next.RenameGroup(p.from, p.into); err != nil {
			m.message, m.messageErr = err.Error(), true
			return m, nil
		}
	}
	if err := config.Save(m.configPath, next); err != nil {
		m.message, m.messageErr = "couldn't save: "+err.Error(), true
		return m, nil
	}

	removed := len(m.cfg.Bookmarks) - len(next.Bookmarks)
	ungrouped := groupSize(m.cfg, p.from) // counted before the swap
	m.cfg = next
	switch p.action {
	case confirmRemove:
		// RemoveItem updates a filtered view right away; SetItems would
		// re-filter in the background and briefly show nothing.
		m.list.RemoveItem(m.list.GlobalIndex())
		if n := len(m.list.VisibleItems()); n > 0 && m.list.Index() >= n {
			m.list.Select(n - 1)
		}
		return m, tea.Batch(m.list.NewStatusMessage("Removed "+displayLabel(p.label)), m.syncTabs())
	case confirmClear:
		m.list.ResetFilter()
		m.list.SetItems(nil)
		m.tab = config.TabAll
		m.syncTabs()
		m.message = fmt.Sprintf("Cleared %d bookmarks. Backup: %s",
			removed, config.ContractHome(m.configPath+".bak"))
	case confirmMerge:
		return m, tea.Batch(m.afterGroupRename(p.into), m.list.NewStatusMessage("Merged "+p.from+" into "+p.into))
	case confirmUngroup:
		// The group's tab is gone; its bookmarks are on All (and Other).
		status := fmt.Sprintf("Ungrouped %s from %s", bookmarks(ungrouped), p.from)
		return m, tea.Batch(m.afterGroupRename(config.TabAll), m.list.NewStatusMessage(status))
	}
	return m, nil
}

// groupSize is how many bookmarks are in group, in any spelling.
func groupSize(cfg *config.Config, group string) int {
	n := 0
	for _, b := range cfg.Bookmarks {
		if !b.Ungrouped() && strings.EqualFold(b.Group, group) {
			n++
		}
	}
	return n
}

// bookmarks is "1 bookmark" or "n bookmarks".
func bookmarks(n int) string {
	if n == 1 {
		return "1 bookmark"
	}
	return fmt.Sprintf("%d bookmarks", n)
}

// edit returns a copy of the config to change and save: every setting, and its
// own bookmarks slice, so the menu's config stays as it is until the save works.
func (m Model) edit() *config.Config {
	next := *m.cfg
	next.Bookmarks = slices.Clone(m.cfg.Bookmarks)
	return &next
}

// startAdd opens the label field for the launch directory, unless that
// directory is gone or already saved.
func (m Model) startAdd() (tea.Model, tea.Cmd) {
	if m.cwd == "" || !isDir(m.cwd) {
		m.message, m.messageErr = "can't add: current folder no longer exists", true
		return m, nil
	}
	// Asking for a label that would then be rejected wastes the user's time.
	for _, b := range m.cfg.Bookmarks {
		if b.Path == filepath.Clean(m.cwd) {
			var cmd tea.Cmd
			if !inTab(b, m.tab) {
				cmd = m.showTab(config.TabAll) // it's on another tab
			}
			m.list.ResetFilter()
			m.list.Select(slices.IndexFunc(m.list.Items(), func(li list.Item) bool { return li.(item).Path == b.Path }))
			m.message = "This folder is already saved as " + displayLabel(b.Label)
			return m, cmd
		}
	}
	m.mode, m.inputErr = modeInput, ""
	m.input.Reset()
	m.input.Placeholder = config.SuggestLabel(m.cwd)
	// With Width 0, textinput shows only a placeholder's first letter. Fit the
	// suggestion; longer labels scroll sideways.
	m.input.Width = max(len([]rune(m.input.Placeholder))+1, 12)
	return m, m.input.Focus()
}

// updateField handles the bottom-line field that field picks out of the
// model: esc cancels, enter passes the field's value to save, and every other
// key is typed (so q, r and c are just letters there). Non-key messages go to
// the field (its cursor blink) and the list (status timeouts).
//
// field is a function, not a *textinput.Model, because m is this method's own
// copy of the model: a pointer made by the caller would point into the
// caller's copy, and every keystroke would be lost.
func (m Model) updateField(msg tea.Msg, field func(*Model) *textinput.Model, save func(Model, string) (tea.Model, tea.Cmd)) (tea.Model, tea.Cmd) {
	f := field(&m)
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "esc":
			if m.mode == modeHex {
				m.mode = modeTheme // the hex field belongs to the picker
			} else {
				m.mode = modeBrowse
			}
			f.Blur()
			return m, nil
		case "enter":
			return save(m, f.Value())
		}
		m.inputErr = ""
		var cmd tea.Cmd
		*f, cmd = f.Update(msg)
		return m, cmd
	}
	var fieldCmd, listCmd tea.Cmd
	*f, fieldCmd = f.Update(msg)
	m.list, listCmd = m.list.Update(msg)
	return m, tea.Batch(fieldCmd, listCmd)
}

func labelField(m *Model) *textinput.Model { return &m.input }
func groupField(m *Model) *textinput.Model { return &m.group }
func hexField(m *Model) *textinput.Model   { return &m.hex }

// addFromField adds the launch directory with the label field's value, or
// its suggestion (the placeholder) if the field is empty.
func (m Model) addFromField(value string) (tea.Model, tea.Cmd) {
	label := strings.TrimSpace(value)
	if label == "" {
		label = m.input.Placeholder
	}
	return m.add(label)
}

// add saves a bookmark for the launch directory. Errors keep the field open
// so the label can be fixed.
func (m Model) add(label string) (tea.Model, tea.Cmd) {
	next := m.edit()
	if err := next.Add(label, m.cwd); err != nil {
		m.inputErr = err.Error()
		return m, nil
	}
	if m.tab != config.TabAll && m.tab != config.TabOther {
		// On a group's tab, the new bookmark joins that group.
		if err := next.SetGroup(label, m.tab); err != nil {
			m.inputErr = err.Error()
			return m, nil
		}
	}
	if err := config.Save(m.configPath, next); err != nil {
		m.inputErr = "couldn't save: " + err.Error()
		return m, nil
	}

	m.cfg = next
	m.mode = modeBrowse
	m.input.Blur()
	added := next.Bookmarks[len(next.Bookmarks)-1]
	// Clear any filter so the new row is visible, then select it.
	m.list.ResetFilter()
	n := len(m.list.Items())
	insertCmd := m.list.InsertItem(n, newItem(m.cfg, added, m.tab, m.gone))
	m.list.Select(n)
	return m, tea.Batch(insertCmd, m.list.NewStatusMessage("Added "+displayLabel(added.Label)), m.syncTabs())
}

// startMove opens the group field for the highlighted bookmark, filled with
// its group.
func (m Model) startMove() (tea.Model, tea.Cmd) {
	it, ok := m.list.SelectedItem().(item)
	if !ok {
		return m, nil // empty list
	}
	m.mode, m.inputErr, m.editing = modeGroup, "", it.Label
	m.group.Prompt = "Group: " // N shares this field with its own prompt and hint
	m.group.Placeholder = ""
	names := m.cfg.Groups()
	m.group.SetSuggestions(names)
	m.group.SetValue(it.Group)
	m.group.CursorEnd()
	// Wide enough for any suggestion, so its completion shows; longer names
	// scroll sideways.
	m.group.Width = 12
	for _, n := range names {
		m.group.Width = max(m.group.Width, len([]rune(n))+1)
	}
	return m, m.group.Focus()
}

// move saves the group field's value as the bookmark's group, like add: change
// a copy, save it, then swap it in. Errors keep the field open to fix.
func (m Model) move(group string) (tea.Model, tea.Cmd) {
	next := m.edit()
	if err := next.SetGroup(m.editing, group); err != nil {
		m.inputErr = err.Error()
		return m, nil
	}
	// Unchanged (SetGroup has matched the spelling): don't save. Every save
	// replaces config.toml.bak, so a save that changes nothing would throw
	// away the last real backup.
	before, _ := m.cfg.Find(m.editing)
	if after, _ := next.Find(m.editing); after.Group == before.Group {
		m.mode = modeBrowse
		m.group.Blur()
		status := displayLabel(before.Label) + " stays in " + before.Group
		if before.Group == "" {
			status = displayLabel(before.Label) + " still has no group"
		}
		return m, m.list.NewStatusMessage(status)
	}
	if err := config.Save(m.configPath, next); err != nil {
		m.inputErr = "couldn't save: " + err.Error()
		return m, nil
	}

	m.cfg = next
	m.mode = modeBrowse
	m.group.Blur()
	moved, _ := next.Find(m.editing)
	status := "Moved " + displayLabel(moved.Label) + " to " + moved.Group
	if moved.Group == "" {
		status = displayLabel(moved.Label) + " has no group now"
	}

	// The moved row may leave the open tab; syncTabs then opens All if the tab
	// emptied. Go evaluates the arguments left to right, so rows come first.
	return m, tea.Batch(m.reloadRows(moved.Path), m.list.NewStatusMessage(status), m.syncTabs())
}

// reloadRows rebuilds the open tab's rows after a change, keeping the cursor
// on the bookmark at path if it's still on the tab, else at the same spot. The
// filter is cleared first, as add does, so the rows show right away.
func (m *Model) reloadRows(path string) tea.Cmd {
	at := m.list.Index()
	m.list.ResetFilter()
	cmd := m.list.SetItems(itemsFrom(m.cfg, m.tab, m.gone))
	if i := slices.IndexFunc(m.list.Items(), func(li list.Item) bool { return li.(item).Path == path }); i >= 0 {
		at = i
	}
	m.list.Select(min(at, len(m.list.Items())-1))
	return cmd
}

// setRows rebuilds the open tab's rows from cfg, for a change that's part of
// every row (icons). The cursor stays on its bookmark, and an applied filter
// stays applied: SetItems' re-filter, which bubbles would run in the
// background, is dropped and the rows are re-filtered here instead.
func (m *Model) setRows(cfg *config.Config) {
	path := ""
	if it, ok := m.list.SelectedItem().(item); ok {
		path = it.Path
	}
	m.list.SetItems(itemsFrom(cfg, m.tab, m.gone))
	if m.list.FilterState() == list.FilterApplied {
		m.list.SetFilterText(m.list.FilterValue())
	}
	if i := slices.IndexFunc(m.list.VisibleItems(), func(li list.Item) bool { return li.(item).Path == path }); i >= 0 {
		m.list.Select(i)
	}
}

// startRename opens the label field for the highlighted bookmark, filled with
// its label as stored (not as the menu shows it).
func (m Model) startRename() (tea.Model, tea.Cmd) {
	it, ok := m.list.SelectedItem().(item)
	if !ok {
		return m, nil // empty list
	}
	m.mode, m.inputErr, m.editing = modeRename, "", it.Label
	m.input.Reset()
	m.input.Placeholder = "" // a's suggestion; renaming starts from the label instead
	m.input.SetValue(it.Label)
	m.input.CursorEnd()
	m.input.Width = max(len([]rune(it.Label))+1, 12)
	return m, m.input.Focus()
}

// rename saves the label field's value as the bookmark's new label, like add:
// change a copy, save it, then swap it in. Errors keep the field open to fix.
func (m Model) rename(newLabel string) (tea.Model, tea.Cmd) {
	old, _ := m.cfg.Find(m.editing)
	if strings.TrimSpace(newLabel) == old.Label {
		// Unchanged: don't save, since every save replaces config.toml.bak.
		// (A case change isn't equal: labels are stored as typed.)
		m.mode = modeBrowse
		m.input.Blur()
		return m, nil
	}
	next := m.edit()
	if err := next.Rename(m.editing, newLabel); err != nil {
		m.inputErr = err.Error()
		return m, nil
	}
	if err := config.Save(m.configPath, next); err != nil {
		m.inputErr = "couldn't save: " + err.Error()
		return m, nil
	}

	m.cfg = next
	m.mode = modeBrowse
	m.input.Blur()
	renamed, _ := next.Find(newLabel)
	status := "Renamed " + displayLabel(old.Label) + " to " + displayLabel(renamed.Label)
	if strings.EqualFold(old.Label, renamed.Label) {
		// The menu shows labels in uppercase, so a case change looks like none.
		status = fmt.Sprintf("%s is now saved as %q", displayLabel(old.Label), renamed.Label)
	}
	return m, tea.Batch(m.reloadRows(old.Path), m.list.NewStatusMessage(status))
}

// startRenameGroup opens the group field to rename the open group's tab,
// filled with its name and suggesting the other groups (to merge into).
func (m Model) startRenameGroup() (tea.Model, tea.Cmd) {
	names := m.cfg.Groups()
	i := slices.IndexFunc(names, func(n string) bool { return strings.EqualFold(n, m.tab) })
	if i < 0 { // All, Other, or no groups
		m.message = "Open a group's tab to rename it"
		return m, nil
	}
	m.mode, m.inputErr, m.editing = modeGroupName, "", names[i]
	m.group.Prompt = "Rename group: "
	m.group.Placeholder = ungroupHint // shows once the field is cleared
	m.group.SetSuggestions(slices.Delete(slices.Clone(names), i, i+1))
	m.group.SetValue(names[i])
	m.group.CursorEnd()
	m.group.Width = len(ungroupHint) + 1 // the field cuts its placeholder to its width
	for _, n := range names {
		m.group.Width = max(m.group.Width, len([]rune(n))+1)
	}
	return m, m.group.Focus()
}

// ungroupHint is N's placeholder: clearing the field ungroups the group.
const ungroupHint = "empty: ungroup all"

// renameGroup renames the open group to the field's value, for every bookmark
// in it, like add: change a copy, save it, then swap it in. A name matching
// another group asks before merging, and an empty name before ungrouping;
// errors keep the field open.
func (m Model) renameGroup(newName string) (tea.Model, tea.Cmd) {
	newName = strings.TrimSpace(newName)
	if newName == m.editing {
		// Unchanged: don't save, since every save replaces config.toml.bak.
		// (A case change isn't equal: it re-cases the group.)
		m.mode = modeBrowse
		m.group.Blur()
		return m, nil
	}
	next := m.edit()
	// RenameGroup checks the name before the merge or ungroup question, so a
	// reserved name is reported here rather than after a prompt.
	if err := next.RenameGroup(m.editing, newName); err != nil {
		m.inputErr = err.Error()
		return m, nil
	}
	if newName == "" {
		m.group.Blur()
		m.mode, m.pending = modeConfirm, pending{action: confirmUngroup, from: m.editing}
		return m, nil
	}
	names := m.cfg.Groups()
	if j := slices.IndexFunc(names, func(n string) bool {
		return strings.EqualFold(n, newName) && !strings.EqualFold(n, m.editing)
	}); j >= 0 {
		m.group.Blur()
		m.mode, m.pending = modeConfirm, pending{action: confirmMerge, from: m.editing, into: names[j]}
		return m, nil
	}
	if err := config.Save(m.configPath, next); err != nil {
		m.inputErr = "couldn't save: " + err.Error()
		return m, nil
	}

	m.cfg = next
	m.mode = modeBrowse
	m.group.Blur()
	status := "Renamed group " + m.editing + " to " + newName
	return m, tea.Batch(m.afterGroupRename(newName), m.list.NewStatusMessage(status))
}

// afterGroupRename follows a group rename or merge: the open tab takes the
// group's new name (tabs are kept by name), the rows are rebuilt with the
// cursor on the same bookmark, and the tabs and help are refreshed.
func (m *Model) afterGroupRename(to string) tea.Cmd {
	path := ""
	if it, ok := m.list.SelectedItem().(item); ok {
		path = it.Path
	}
	m.tab = to
	return tea.Batch(m.reloadRows(path), m.syncTabs())
}

// reorder moves the highlighted bookmark one place down (or up) on the open
// tab and saves, like every change. Nothing happens at the tab's edge; with a
// filter applied, it says to clear the filter first, since neighbours in a
// filtered list aren't neighbours in the file.
func (m Model) reorder(down bool) (tea.Model, tea.Cmd) {
	it, ok := m.list.SelectedItem().(item)
	if !ok {
		return m, nil // empty list
	}
	if m.list.FilterState() == list.FilterApplied {
		m.message = "Clear the filter to reorder"
		return m, nil
	}
	// within is a closure over the open tab, so config.Move can ask whether a
	// bookmark is on it without knowing about tabs. nil (on All) means any.
	var within func(config.Bookmark) bool
	if m.tab != config.TabAll {
		tab := m.tab
		within = func(b config.Bookmark) bool { return inTab(b, tab) }
	}
	next := m.edit()
	moved, err := next.Move(it.Label, down, within)
	if err != nil {
		m.message, m.messageErr = err.Error(), true
		return m, nil
	}
	if !moved {
		return m, nil // at the edge: nothing to save
	}
	if err := config.Save(m.configPath, next); err != nil {
		m.message, m.messageErr = "couldn't save: "+err.Error(), true
		return m, nil
	}
	m.cfg = next
	// Reordering on All can change which group appears first, so the tabs too.
	return m, tea.Batch(m.reloadRows(it.Path), m.syncTabs())
}

// choose cds to the highlighted bookmark, unless its folder is gone.
func (m Model) choose() (tea.Model, tea.Cmd) {
	it, ok := m.list.SelectedItem().(item)
	if !ok {
		return m, nil // empty list
	}
	// Check again: the folder may have come back (or vanished) since the menu opened.
	if cmd := m.refuseGone(it.Path); cmd != nil {
		return m, cmd
	}
	m.chosen = it.Path
	return m, tea.Quit
}

// refuseGone checks the folder on disk for enter and the action keys. If it's
// gone, it dims the row and returns the status saying so; else nil.
func (m *Model) refuseGone(path string) tea.Cmd {
	if isDir(path) {
		return nil
	}
	m.markGone(path)
	return m.list.NewStatusMessage(config.ContractHome(path) + " no longer exists")
}

func (m Model) View() string {
	body := m.list.View()
	// The tab bar and its rule go under the list's title line (the filter
	// field while typing), after a blank line; the list's title padding gives
	// the blank line below them. They're drawn here, not in the list's title,
	// because the list cuts its title to the window width as one line.
	if bar := m.tabBar(); bar != "" {
		if title, rest, ok := strings.Cut(body, "\n"); ok {
			body = title + "\n\n" + bar + "\n" + m.tabRule() + "\n" + rest
		}
	}
	if help := m.helpView(); help != "" {
		body += "\n" + help
	}
	if len(m.list.Items()) == 0 {
		// Same title bar as the list draws, so the title doesn't jump when it
		// empties. No help: the message says what to do.
		body = m.list.Styles.TitleBar.Render(m.styles.title.Render(menuTitle)) + "\n" + m.styles.empty.Render(emptyMessage) + "\n"
	}
	if m.picking() {
		// The empty menu's body already ends its last line; the picker's
		// margin is the one blank line either way.
		body = strings.TrimSuffix(body, "\n") + "\n" + m.pickerView()
	}
	return m.styles.app.Render(body + "\n" + m.bottomLine())
}

// tabBar draws the tabs with the open one as a block like the title badge, or
// "" when there are no groups. It's cut to the list's width if there are many.
func (m Model) tabBar() string {
	names := tabs(m.cfg)
	if names == nil {
		return ""
	}
	parts := make([]string, len(names))
	for i, n := range names {
		style := m.styles.tab
		if strings.EqualFold(n, m.tab) {
			style = m.styles.tabOpen
		}
		parts[i] = style.Render(n)
	}
	return m.styles.tabBar.MaxWidth(m.list.Width()).Render(strings.Join(parts, " "))
}

// tabRule is the line under the tab bar, from the open tab's column to the
// list's right edge.
func (m Model) tabRule() string {
	w := m.list.Width() - m.styles.tabBar.GetPaddingLeft()
	return m.styles.tabBar.Render(m.styles.rule.Render(strings.Repeat("─", max(w, 0))))
}

// bottomLine is the prompt in confirm mode, otherwise the last message.
func (m Model) bottomLine() string {
	switch {
	case m.mode == modeConfirm && m.pending.action == confirmRemove:
		return m.styles.prompt.Render(fmt.Sprintf("Remove %s? y/N", displayLabel(m.pending.label)))
	case m.mode == modeConfirm && m.pending.action == confirmClear:
		return m.styles.prompt.Render(fmt.Sprintf("Delete all %d bookmarks? y/N", len(m.cfg.Bookmarks)))
	case m.mode == modeConfirm && m.pending.action == confirmMerge:
		return m.styles.prompt.Render(fmt.Sprintf("Merge %s into %s? y/N", m.pending.from, m.pending.into))
	case m.mode == modeConfirm && m.pending.action == confirmUngroup:
		n := groupSize(m.cfg, m.pending.from)
		all := "all "
		if n == 1 {
			all = ""
		}
		return m.styles.prompt.Render(fmt.Sprintf("Ungroup %s%s in %s? y/N", all, bookmarks(n), m.pending.from))
	case m.mode == modeTheme && !m.messageErr:
		return m.styles.note.Render(m.hints(0, pickerKeys...))
	case m.mode == modeInput || m.mode == modeGroup || m.mode == modeRename || m.mode == modeGroupName || m.mode == modeHex:
		field, keys := m.input, m.hints(0, hint{"enter", "save"}, hint{"esc", "cancel"})
		switch m.mode {
		case modeHex:
			field, keys = m.hex, m.hints(0, hint{"enter", "set"}, hint{"esc", "back"})
		case modeGroup:
			field, keys = m.group, m.hints(0, hint{"tab", "complete"}, hint{"", "empty = no group"})
		case modeGroupName:
			field = m.group
		}
		if m.inputErr != "" {
			keys = m.styles.errText.UnsetPadding().Render(m.inputErr)
		}
		return m.styles.note.Render(field.View() + "    " + keys)
	case m.messageErr:
		return m.styles.errText.Render(m.message)
	}
	return m.styles.note.Render(m.message)
}

// itemsFrom makes list rows for the bookmarks on a tab, in config order, with
// the folders in gone marked missing.
func itemsFrom(cfg *config.Config, tab string, gone map[string]bool) []list.Item {
	var items []list.Item
	for _, b := range cfg.Bookmarks {
		if inTab(b, tab) {
			items = append(items, newItem(cfg, b, tab, gone))
		}
	}
	return items
}

// newItem is the list row for b on tab. On All, the filter also matches b's
// group, and the row shows it as a tag, in the group's first spelling. (On a
// group's tab, every row shares the group.)
func newItem(cfg *config.Config, b config.Bookmark, tab string, gone map[string]bool) item {
	it := item{Bookmark: b, missing: gone[b.Path], icons: cfg.Menu.Icons, matchGroup: tab == config.TabAll}
	if tab == config.TabAll {
		it.tag = cfg.GroupOf(b) // "" if ungrouped, or in a hand-edited All/Other
	}
	return it
}

// tabs lists the menu's tabs: All, the groups in the order they first appear,
// then Other if some bookmarks are ungrouped. It's nil when none has a group,
// so the menu shows no tab bar.
func tabs(cfg *config.Config) []string {
	groups := cfg.Groups()
	if len(groups) == 0 {
		return nil
	}
	names := append([]string{config.TabAll}, groups...)
	if slices.ContainsFunc(cfg.Bookmarks, config.Bookmark.Ungrouped) {
		names = append(names, config.TabOther)
	}
	return names
}

// inTab reports whether b shows on a tab: All shows every bookmark, Other the
// ungrouped ones, and a group's tab its bookmarks (in any case).
func inTab(b config.Bookmark, tab string) bool {
	switch tab {
	case config.TabAll:
		return true
	case config.TabOther:
		return b.Ungrouped()
	}
	return strings.EqualFold(b.Group, tab)
}

// item shows a bookmark in the list.
type item struct {
	config.Bookmark
	missing    bool   // folder didn't exist when last checked
	icons      bool   // [menu] icons: show an icon before the label
	matchGroup bool   // the filter also matches the group (on the All tab)
	tag        string // the group, shown after the label (on the All tab)
}

func (i item) Title() string {
	t := i.icon() + displayLabel(i.Label)
	if i.missing {
		t += " (missing)"
	}
	return t
}
func (i item) Description() string { return config.ContractHome(i.Path) }

// FilterValue starts the same way as Title (icon, then the label in
// uppercase): the list underlines matches by position in Title, counting in
// FilterValue. On All, groupSep and the group follow; filterByRune matches the
// two parts separately.
func (i item) FilterValue() string {
	v := i.icon() + displayLabel(i.Label)
	if i.matchGroup && i.Group != "" {
		v += groupSep + i.Group
	}
	return v
}

// groupSep separates the label from the group in a FilterValue. It's an
// invisible control character, so it can't be typed into the filter.
const groupSep = "\x1f"

// icon is a Nerd Font icon and a space before the label, or "" with icons off.
// A missing folder gets ✗, which most fonts have.
func (i item) icon() string {
	switch {
	case !i.icons:
		return ""
	case i.missing:
		return "✗ "
	case config.ContractHome(i.Path) == "~":
		return "\uf015 " // home
	default:
		return "\uf07b " // folder
	}
}

// displayLabel is how the menu shows a label: in uppercase. The config keeps
// labels as typed, so only use this for text on screen, never for lookups.
func displayLabel(label string) string { return strings.ToUpper(label) }

// filterByRune is the list's filter. Each target is a row's FilterValue: the
// label line, then on All groupSep and the group.
//
//   - Labels and groups are matched separately, so a query can't take some
//     letters from the label and the rest from the group. Rows matched by label
//     come first, then rows matched only by group, which underline nothing.
//   - Match positions are converted from bytes, which the fuzzy matcher
//     reports, to characters, which the list underlines; otherwise anything
//     non-ASCII before a match (an icon, or Ö in a label) shifts the underline.
func filterByRune(term string, targets []string) []list.Rank {
	labels := make([]string, len(targets))
	groups := make([]string, len(targets))
	for i, t := range targets {
		labels[i], groups[i], _ = strings.Cut(t, groupSep)
	}
	ranks := list.DefaultFilter(term, labels)
	byLabel := make(map[int]bool, len(ranks))
	for _, r := range ranks {
		byLabel[r.Index] = true
		for j, b := range r.MatchedIndexes {
			r.MatchedIndexes[j] = utf8.RuneCountInString(labels[r.Index][:b])
		}
	}
	for _, r := range list.DefaultFilter(term, groups) {
		if !byLabel[r.Index] {
			ranks = append(ranks, list.Rank{Index: r.Index})
		}
	}
	return ranks
}

// delegate draws each row as its label and path. The list draws the label
// (selection, filter matches), then Render adds the group tag; renderPath
// draws the path. Missing rows use muted styles.
type delegate struct {
	list.DefaultDelegate
	missing list.DefaultItemStyles
	pathEnd lipgloss.TerminalColor // a path's last folder
	tag     lipgloss.Style         // the group tag after a label
}

func (d delegate) Height() int { return 2 } // label + path

func (d delegate) Render(w io.Writer, m list.Model, index int, li list.Item) {
	it, ok := li.(item)
	if !ok || m.Width() <= 0 {
		return
	}
	if it.missing {
		d.Styles = d.missing // d is a copy, so this only affects this row
	}
	d.ShowDescription = false
	var title strings.Builder
	d.DefaultDelegate.Render(&title, m, index, li)
	fmt.Fprint(w, title.String()+d.renderTag(m, index, it, lipgloss.Width(title.String()))+"\n"+d.renderPath(m, index, it))
}

// renderTag is the row's group tag, "· Config", to go after a label line this
// wide, or "" if the row has none or it doesn't fit whole. After the selected
// block it starts with a space; other label lines already end in one (the
// title's right padding).
func (d delegate) renderTag(m list.Model, index int, it item, width int) string {
	if it.tag == "" {
		return ""
	}
	tag := d.tag.Render("· " + it.tag)
	if d.onBlock(m, index) {
		tag = " " + tag
	}
	if width+lipgloss.Width(tag) > m.Width() {
		return ""
	}
	return tag
}

// onBlock reports whether the list draws this row's label as a block: it's
// the selected row, no filter is being typed (DefaultDelegate's rule), and its
// selected style has a background (block mode, and not a missing row).
func (d delegate) onBlock(m list.Model, index int) bool {
	_, noBlock := d.Styles.SelectedTitle.GetBackground().(lipgloss.NoColor)
	return index == m.Index() && m.FilterState() != list.Filtering && !noBlock
}

// renderPath draws a row's path in the style the list would use for it, cut to
// fit, with the last folder in the label color. Missing rows, and every row
// while the filter field is empty, stay dimmed.
func (d delegate) renderPath(m list.Model, index int, it item) string {
	s := d.Styles
	emptyFilter := m.FilterState() == list.Filtering && m.FilterValue() == ""
	style := s.NormalDesc
	switch {
	case emptyFilter:
		style = s.DimmedDesc
	case index == m.Index() && m.FilterState() != list.Filtering:
		style = s.SelectedDesc
	}

	path := cutMiddle(it.Description(), m.Width()-s.NormalDesc.GetHorizontalFrameSize())
	if it.missing || emptyFilter {
		return style.Render(path)
	}
	head, last := splitLast(path)
	// The head carries the row's padding and bar; the last folder is inline.
	return style.Render(head) + style.Inline(true).Foreground(d.pathEnd).Render(last)
}

// splitLast splits a path before its last folder: "~/a/b" → "~/a/", "b".
// "~" and "/" are all last folder.
func splitLast(path string) (head, last string) {
	i := strings.LastIndex(path, "/")
	if i < 0 || i == len(path)-1 {
		return "", path
	}
	return path[:i+1], path[i+1:]
}

// cutMiddle fits path into width columns. It keeps the start and the last
// folder and drops whole folders in between: ~/Desktop/…/pathpin. A last folder
// too long for that is cut at the end instead: ~/a/xxxxx….
func cutMiddle(path string, width int) string {
	if lipgloss.Width(path) <= width {
		return path
	}
	head, last := splitLast(path)
	tail := "…/" + last
	room := width - lipgloss.Width(tail)
	if head == "" || room < 0 {
		return cutEnd(path, width)
	}
	kept := ""
	for _, folder := range strings.SplitAfter(head, "/") {
		if lipgloss.Width(kept+folder) > room {
			break
		}
		kept += folder
	}
	return kept + tail
}

// cutEnd fits s into width columns, ending in "…" if it had to cut. It cuts
// between characters, never inside one.
func cutEnd(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	var b strings.Builder
	used := 0
	for _, r := range s {
		w := lipgloss.Width(string(r))
		if used+w > width-1 {
			break
		}
		b.WriteRune(r)
		used += w
	}
	return b.String() + "…"
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

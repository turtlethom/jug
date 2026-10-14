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
	modeBrowse  mode = iota // the list is active and keys trigger actions
	modeConfirm             // a y/N prompt is showing; only y confirms
	modeInput               // typing a label for a new bookmark
	modeGroup               // typing a group for the highlighted bookmark (m)
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

	group  textinput.Model // m's field; suggests existing group names
	moving string          // label of the bookmark the group field is for

	// message is shown on the bottom line until the next key.
	message    string
	messageErr bool

	// tab is the open tab by name: config.TabAll, a group, or config.TabOther.
	// A name, not a position, because tabs shift when a group empties.
	tab string
	// width and height are the last window size, to resize the list when the
	// tab bar comes or goes.
	width, height int
}

// New builds the menu for cfg. It checks once whether each bookmark's folder
// still exists.
func New(cfg *config.Config, configPath string) Model {
	s := NewStyles(cfg.Colors)

	d := delegate{DefaultDelegate: list.NewDefaultDelegate(), missing: s.missing, pathEnd: s.pathEnd}
	d.Styles = s.item

	l := list.New(itemsFrom(cfg, config.TabAll), d, 0, 0)
	l.Title = menuTitle
	l.Styles.Title = s.title
	// One column less than the list's default, so the badge's block lines up
	// with the selected row's.
	l.Styles.TitleBar = l.Styles.TitleBar.PaddingLeft(1)
	l.Filter = filterByRune
	l.Styles.StatusBar = s.status
	l.Styles.StatusEmpty = s.status
	l.FilterInput.PromptStyle = s.filter
	l.FilterInput.Cursor.Style = s.filter
	l.SetStatusBarItemName("bookmark", "bookmarks")
	l.StatusMessageLifetime = 3 * time.Second
	setHelpKeys(&l, tabs(cfg) != nil)

	group := newField(s, "Group: ")
	group.ShowSuggestions = true // tab completes an existing group's name

	cwd, _ := os.Getwd() // "" if it fails; a then reports the folder is gone
	return Model{
		mode: modeBrowse, list: l, styles: s, cfg: cfg, configPath: configPath, cwd: cwd,
		input: newField(s, "Label: "), group: group, tab: config.TabAll,
	}
}

// newField makes a text field for the bottom line.
func newField(s Styles, prompt string) textinput.Model {
	in := textinput.New()
	in.Prompt = prompt
	in.CharLimit = 40
	in.PromptStyle = s.filter
	in.Cursor.Style = s.filter
	in.PlaceholderStyle = s.muted
	in.CompletionStyle = s.muted
	return in
}

// setHelpKeys adds pathpin's own keys to the list's ? help: tab only while
// there are tabs to switch.
func setHelpKeys(l *list.Model, withTabs bool) {
	keys := func() []key.Binding {
		k := []key.Binding{
			key.NewBinding(key.WithKeys("a"), key.WithHelp("a", "add")),
			key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "remove")),
			key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "clear")),
			key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "move")),
		}
		if withTabs {
			k = append(k, key.NewBinding(key.WithKeys("tab", "shift+tab"), key.WithHelp("tab", "group")))
		}
		return k
	}
	l.AdditionalShortHelpKeys = keys
	l.AdditionalFullHelpKeys = keys
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Messages every mode handles the same way.
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resize()
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
		return m.updateInput(msg)
	case modeGroup:
		return m.updateGroup(msg)
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
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

// resize fits the list to the window, leaving room for the bottom line and,
// while there are groups, the tab bar.
//
// It has a pointer receiver so it can change the Model it's called on. Update
// has a value receiver, so the m it holds is its own copy and calling
// m.resize() there changes that copy, which Update then returns.
func (m *Model) resize() {
	if m.width == 0 {
		return // no window size yet; Bubble Tea sends one at start
	}
	h, v := m.styles.app.GetFrameSize()
	reserved := 1 // the bottom line
	if tabs(m.cfg) != nil {
		reserved++
	}
	m.list.SetSize(m.width-h, m.height-v-reserved)
}

// showTab opens a tab: it clears any filter and shows that tab's bookmarks.
func (m *Model) showTab(name string) tea.Cmd {
	m.tab = name
	m.list.ResetFilter()
	cmd := m.list.SetItems(itemsFrom(m.cfg, name))
	m.list.Select(0)
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
		return m, tea.Batch(m.list.NewStatusMessage("Removed "+displayLabel(p.label)), m.syncTabs())
	case confirmClear:
		m.list.ResetFilter()
		m.list.SetItems(nil)
		m.tab = config.TabAll
		m.syncTabs()
		m.message = fmt.Sprintf("Cleared %d bookmarks. Backup: %s",
			removed, config.ContractHome(m.configPath+".bak"))
	}
	return m, nil
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

// updateInput handles the label field: enter saves, esc cancels, and every
// other key is typed (so q, r and c are just letters here).
func (m Model) updateInput(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "esc":
			m.mode = modeBrowse
			m.input.Blur()
			return m, nil
		case "enter":
			label := strings.TrimSpace(m.input.Value())
			if label == "" {
				label = m.input.Placeholder
			}
			return m.add(label)
		}
		m.inputErr = ""
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	// Not a key: the cursor blink belongs to the field, status timeouts to the list.
	var inCmd, listCmd tea.Cmd
	m.input, inCmd = m.input.Update(msg)
	m.list, listCmd = m.list.Update(msg)
	return m, tea.Batch(inCmd, listCmd)
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
	insertCmd := m.list.InsertItem(n, item{Bookmark: added, icons: m.cfg.Menu.Icons, matchGroup: m.tab == config.TabAll})
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
	m.mode, m.inputErr, m.moving = modeGroup, "", it.Label
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

// updateGroup handles the group field: enter saves, esc cancels, and every
// other key is typed (tab completes a suggestion here, it doesn't switch tabs).
func (m Model) updateGroup(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.String() {
		case "esc":
			m.mode = modeBrowse
			m.group.Blur()
			return m, nil
		case "enter":
			return m.move(m.group.Value())
		}
		m.inputErr = ""
		var cmd tea.Cmd
		m.group, cmd = m.group.Update(msg)
		return m, cmd
	}
	// Not a key: the cursor blink belongs to the field, status timeouts to the list.
	var fieldCmd, listCmd tea.Cmd
	m.group, fieldCmd = m.group.Update(msg)
	m.list, listCmd = m.list.Update(msg)
	return m, tea.Batch(fieldCmd, listCmd)
}

// move saves the group field's value as the bookmark's group, like add: change
// a copy, save it, then swap it in. Errors keep the field open to fix.
func (m Model) move(group string) (tea.Model, tea.Cmd) {
	next := m.edit()
	if err := next.SetGroup(m.moving, group); err != nil {
		m.inputErr = err.Error()
		return m, nil
	}
	// Unchanged (SetGroup has matched the spelling): don't save. Every save
	// replaces config.toml.bak, so a save that changes nothing would throw
	// away the last real backup.
	before, _ := m.cfg.Find(m.moving)
	if after, _ := next.Find(m.moving); after.Group == before.Group {
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
	moved, _ := next.Find(m.moving)
	status := "Moved " + displayLabel(moved.Label) + " to " + moved.Group
	if moved.Group == "" {
		status = displayLabel(moved.Label) + " has no group now"
	}

	// Rebuild the open tab's rows, since the moved one may leave it. Keep the
	// cursor on it if it's still here, else at the same spot. The filter is
	// cleared first, as add does, so the rows show right away.
	at := m.list.Index()
	m.list.ResetFilter()
	setCmd := m.list.SetItems(itemsFrom(m.cfg, m.tab))
	if i := slices.IndexFunc(m.list.Items(), func(li list.Item) bool { return li.(item).Path == moved.Path }); i >= 0 {
		at = i
	}
	m.list.Select(min(at, len(m.list.Items())-1))
	return m, tea.Batch(setCmd, m.list.NewStatusMessage(status), m.syncTabs())
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
	// The tab bar goes right under the list's title line (the filter field
	// while typing). It's drawn here, not in the list's title, because the
	// list cuts its title to the window width as one line.
	if bar := m.tabBar(); bar != "" {
		if title, rest, ok := strings.Cut(body, "\n"); ok {
			body = title + "\n" + bar + "\n" + rest
		}
	}
	if len(m.list.Items()) == 0 {
		// Same title bar as the list draws, so the title doesn't jump when it empties.
		body = m.list.Styles.TitleBar.Render(m.styles.title.Render(menuTitle)) + "\n" + m.styles.empty.Render(emptyMessage) + "\n"
	}
	return m.styles.app.Render(body + "\n" + m.bottomLine())
}

// tabBar draws the tabs with the open one in the accent color, or "" when
// there are no groups. It's cut to the list's width if there are many.
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
	return m.styles.tabBar.MaxWidth(m.list.Width()).Render(strings.Join(parts, "  "))
}

// bottomLine is the prompt in confirm mode, otherwise the last message.
func (m Model) bottomLine() string {
	switch {
	case m.mode == modeConfirm && m.pending.action == confirmRemove:
		return m.styles.prompt.Render(fmt.Sprintf("Remove %s? y/N", displayLabel(m.pending.label)))
	case m.mode == modeConfirm && m.pending.action == confirmClear:
		return m.styles.prompt.Render(fmt.Sprintf("Delete all %d bookmarks? y/N", len(m.cfg.Bookmarks)))
	case m.mode == modeInput || m.mode == modeGroup:
		field, hint := m.input, m.styles.muted.Render("enter save · esc cancel")
		if m.mode == modeGroup {
			field, hint = m.group, m.styles.muted.Render("tab complete · empty = no group")
		}
		if m.inputErr != "" {
			hint = m.styles.errText.UnsetPadding().Render(m.inputErr)
		}
		return m.styles.note.Render(field.View() + "    " + hint)
	case m.messageErr:
		return m.styles.errText.Render(m.message)
	}
	return m.styles.note.Render(m.message)
}

// itemsFrom makes list rows for the bookmarks on a tab, in config order,
// checking whether each folder exists.
func itemsFrom(cfg *config.Config, tab string) []list.Item {
	var items []list.Item
	for _, b := range cfg.Bookmarks {
		if inTab(b, tab) {
			items = append(items, item{
				Bookmark:   b,
				missing:    !isDir(b.Path),
				icons:      cfg.Menu.Icons,
				matchGroup: tab == config.TabAll, // on a group's tab, every row shares the group
			})
		}
	}
	return items
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
	missing    bool // folder didn't exist when last checked
	icons      bool // [menu] icons: show an icon before the label
	matchGroup bool // the filter also matches the group (on the All tab)
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
// (selection, filter matches); renderPath draws the path. Missing rows use
// muted styles.
type delegate struct {
	list.DefaultDelegate
	missing list.DefaultItemStyles
	pathEnd lipgloss.TerminalColor // a path's last folder
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
	d.DefaultDelegate.Render(w, m, index, li)
	fmt.Fprint(w, "\n"+d.renderPath(m, index, it))
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

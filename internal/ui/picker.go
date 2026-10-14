package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/turtlethom/pathpin/internal/config"
)

// The appearance picker (t) is a panel under the list with a row per setting:
// the theme, each [colors] key, and [menu] selection and icons. It edits
// m.draft, a copy of the config taken when it opens, and redraws the menu from
// it on every change, so the menu above is the preview. enter saves the draft
// like any change; esc redraws from m.cfg, which the picker never touches.

// pickerRow is a row of the picker: a setting, the values ←→ cycle through,
// and how to read and set it in a config. get and set are functions for the
// same reason as labelField: they must work on the caller's own copy.
type pickerRow struct {
	key     string          // as in config.toml
	hint    string          // what the setting changes
	def     string          // how the default ("") shows: terminal, theme's, block, off
	values  func() []string // what ←→ cycle through; values()[0] is the default, "" (d)
	get     func(*config.Config) string
	set     func(*config.Config, string)
	color   bool // # types a color
	rebuild bool // a change rebuilds the list's rows (icons are part of each row)
}

func appearanceRows() []pickerRow {
	color := func(key, hint string, field func(*config.Colors) *string) pickerRow {
		return pickerRow{
			key: key, hint: hint, def: "theme's", values: colorValues, color: true,
			get: func(c *config.Config) string { return *field(&c.Colors) },
			set: func(c *config.Config, v string) { *field(&c.Colors) = v },
		}
	}
	return []pickerRow{
		{
			key: "theme", def: terminalTheme, values: themeValues,
			get: func(c *config.Config) string { return c.Colors.Theme },
			set: func(c *config.Config, v string) { c.Colors.Theme = v },
		},
		color("primary", "title, selected row", func(c *config.Colors) *string { return &c.Primary }),
		color("secondary", "labels", func(c *config.Colors) *string { return &c.Secondary }),
		color("tertiary", "paths", func(c *config.Colors) *string { return &c.Tertiary }),
		color("selected_text", "text on primary", func(c *config.Colors) *string { return &c.SelectedText }),
		color("error", "errors", func(c *config.Colors) *string { return &c.Error }),
		{
			key: "selection", hint: "selected row: block or bar", def: selectBlock,
			values: func() []string { return []string{"", selectBar} },
			get:    func(c *config.Config) string { return c.Menu.Selection },
			set:    func(c *config.Config, v string) { c.Menu.Selection = v },
		},
		{
			key: "icons", hint: "needs a Nerd Font", def: "off", rebuild: true,
			values: func() []string { return []string{"", "on"} },
			get: func(c *config.Config) string {
				if c.Menu.Icons {
					return "on"
				}
				return ""
			},
			set: func(c *config.Config, v string) { c.Menu.Icons = v == "on" },
		},
	}
}

// themeValues are the themes in ThemeNames order, with terminal as "" (the
// default, which is how it's saved).
func themeValues() []string {
	names := ThemeNames()
	for i, n := range names {
		if n == terminalTheme {
			names[i] = ""
		}
	}
	return names
}

// colorValues are the theme's color (""), then colorChoices.
func colorValues() []string {
	values := []string{""}
	for _, c := range colorChoices() {
		values = append(values, c.value)
	}
	return values
}

// pickerKeys is the picker's bottom line.
const pickerKeys = "←→ change · # hex · d reset · enter save · esc cancel"

// picking reports whether the picker shows: its rows, or its hex field.
func (m Model) picking() bool { return m.mode == modeTheme || m.mode == modeHex }

// startTheme opens the picker on the theme row, with a copy of the config as
// the draft. The list's help hides meanwhile, since its keys don't apply.
func (m Model) startTheme() (tea.Model, tea.Cmd) {
	m.mode, m.themeRow, m.draft = modeTheme, 0, *m.edit()
	m.list.SetShowHelp(false)
	m.resize() // the list shrinks to make room for the rows
	return m, nil
}

// closePicker goes back to browsing. The caller restyles first if needed.
func (m Model) closePicker() Model {
	m.mode = modeBrowse
	m.list.SetShowHelp(true)
	m.resize()
	return m
}

func (m Model) updateTheme(msg tea.Msg) (tea.Model, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		var cmd tea.Cmd
		m.list, cmd = m.list.Update(msg) // status message timeouts
		return m, cmd
	}
	rows := appearanceRows()
	row := rows[m.themeRow]
	switch k.String() {
	case "up", "k":
		m.themeRow = (m.themeRow + len(rows) - 1) % len(rows)
	case "down", "j":
		m.themeRow = (m.themeRow + 1) % len(rows)
	case "left", "h", "right", "l":
		step := 1
		if k.String() == "left" || k.String() == "h" {
			step = -1
		}
		row.set(&m.draft, cycle(row.values(), row.get(&m.draft), step))
		m.preview(row)
	case "d":
		row.set(&m.draft, row.values()[0]) // the theme's color, terminal, block, off
		m.preview(row)
	case "#":
		if row.color {
			return m.startHex()
		}
	case "enter":
		return m.saveLook()
	case "esc":
		m.restyle(m.cfg)
		if m.draft.Menu.Icons != m.cfg.Menu.Icons {
			m.setRows(m.cfg)
		}
		return m.closePicker(), nil
	}
	return m, nil
}

// preview redraws the menu from the draft after row changed.
func (m *Model) preview(row pickerRow) {
	m.restyle(&m.draft)
	if row.rebuild {
		m.setRows(&m.draft)
	}
}

// cycle is the value step places from current in values, wrapping. A value
// that isn't in values (unknown, or a color typed with #) counts as the
// default, values[0].
func cycle(values []string, current string, step int) string {
	i := max(0, indexFold(values, current))
	return values[(i+step+len(values))%len(values)]
}

// indexFold is the index of s in list in any case, or -1.
func indexFold(list []string, s string) int {
	for i, v := range list {
		if strings.EqualFold(v, s) {
			return i
		}
	}
	return -1
}

// startHex opens the bottom-line field for the highlighted color row, with
// the row's key as the prompt, filled with the row's color (hex or ANSI), or
// just # when it has none.
func (m Model) startHex() (tea.Model, tea.Cmd) {
	row := appearanceRows()[m.themeRow]
	value := row.get(&m.draft)
	if !validColor(value) {
		value = "#"
	}
	m.mode, m.inputErr = modeHex, ""
	m.hex.Prompt = row.key + ": "
	m.hex.SetValue(value)
	m.hex.CursorEnd()
	m.hex.Width = 12
	return m, m.hex.Focus()
}

// setHex puts the hex field's value in the draft and goes back to the picker.
// Nothing is saved until enter there. A bad value keeps the field open.
func (m Model) setHex(value string) (tea.Model, tea.Cmd) {
	value = strings.TrimSpace(value)
	if !validColor(value) {
		m.inputErr = "want #RGB, #RRGGBB or 0-255"
		return m, nil
	}
	appearanceRows()[m.themeRow].set(&m.draft, value)
	m.restyle(&m.draft)
	m.mode = modeTheme
	m.hex.Blur()
	return m, nil
}

// saveLook saves the draft like the other changes: change a copy, save it,
// then swap it in. A failed save keeps the picker open with the error.
func (m Model) saveLook() (tea.Model, tea.Cmd) {
	if sameLook(&m.draft, m.cfg) {
		// Every save replaces the one backup, so a no-op save would lose it.
		m.restyle(m.cfg) // the draft may differ in case only
		m = m.closePicker()
		return m, m.list.NewStatusMessage("Appearance unchanged")
	}
	next := m.draft
	if err := config.Save(m.configPath, &next); err != nil {
		m.message, m.messageErr = "couldn't save: "+err.Error(), true
		return m, nil
	}
	m.cfg = &next
	m = m.closePicker()
	return m, m.list.NewStatusMessage("Appearance saved")
}

// sameLook reports whether a and b draw the same: every row's value matches
// in any case, and a default's name (terminal, block) is the same as "".
func sameLook(a, b *config.Config) bool {
	for _, row := range appearanceRows() {
		if !strings.EqualFold(row.canon(row.get(a)), row.canon(row.get(b))) {
			return false
		}
	}
	return true
}

// canon is value, or "" when it names the default (terminal, block).
func (r pickerRow) canon(value string) string {
	if strings.EqualFold(value, r.def) {
		return ""
	}
	return value
}

// pickerView draws the rows: the key, the value (with arrows on the
// highlighted row), and a muted hint, in columns.
func (m Model) pickerView() string {
	text := m.styles.note.UnsetPadding()
	lines := make([]string, 0, len(appearanceRows()))
	for i, row := range appearanceRows() {
		marker, key, arrowL, arrowR := "  ", text.Render(fmt.Sprintf("%-14s", row.key)), "  ", "  "
		if i == m.themeRow {
			marker = m.styles.picked.Render("› ")
			key = m.styles.picked.Render(fmt.Sprintf("%-14s", row.key))
			arrowL, arrowR = m.styles.picked.Render("◂ "), m.styles.picked.Render(" ▸")
		}
		value := arrowL + m.pickerValue(i) + arrowR
		value += strings.Repeat(" ", max(0, 30-lipgloss.Width(value)))
		lines = append(lines, marker+key+value+m.styles.muted.Render(row.hint))
	}
	return m.styles.picker.MaxWidth(m.list.Width()).Render(strings.Join(lines, "\n"))
}

// pickerValue draws a row's draft value: the default's name, a theme or
// option, or a color as a swatch and name · value. Values the menu ignores are
// marked.
func (m Model) pickerValue(i int) string {
	text := m.styles.note.UnsetPadding()
	row := appearanceRows()[i]
	value := row.canon(row.get(&m.draft))
	switch {
	case value == "" && row.color:
		return m.styles.muted.Render(row.def)
	case value == "":
		return text.Render(row.def)
	case row.color && !validColor(value):
		return text.Render(value + " (ignored)")
	case !row.color:
		if j := indexFold(row.values(), value); j >= 0 {
			return text.Render(row.values()[j]) // as the menu names it, whatever the case
		}
		return text.Render(value + " (unknown)")
	}
	swatch := lipgloss.NewStyle().Foreground(lipgloss.Color(value)).Render("██")
	for _, c := range colorChoices() {
		if strings.EqualFold(c.value, value) {
			return swatch + text.Render(" "+c.name+" · "+c.value)
		}
	}
	return swatch + text.Render(" "+value)
}

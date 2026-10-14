package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/turtlethom/pathpin/internal/config"
)

// The theme picker (t) is a panel under the list with a row per [colors] key.
// It edits m.draft, a copy of the saved colors, and restyles the menu from it
// on every change, so the menu above is the preview. enter saves the draft;
// esc restyles from m.cfg.Colors, which the picker never touches.

// colorRow is a row of the picker: a [colors] key and the field it sets.
type colorRow struct {
	key  string // as in config.toml
	hint string // what the color is used for
	// slot picks the row's field out of a Colors. It's a function for the same
	// reason as labelField: it must point into the caller's own copy.
	slot func(*config.Colors) *string
}

func colorRows() []colorRow {
	return []colorRow{
		{"theme", "", func(c *config.Colors) *string { return &c.Theme }},
		{"primary", "title, selected row", func(c *config.Colors) *string { return &c.Primary }},
		{"secondary", "labels", func(c *config.Colors) *string { return &c.Secondary }},
		{"tertiary", "paths", func(c *config.Colors) *string { return &c.Tertiary }},
		{"selected_text", "text on primary", func(c *config.Colors) *string { return &c.SelectedText }},
		{"error", "errors", func(c *config.Colors) *string { return &c.Error }},
	}
}

// pickerKeys is the picker's bottom line.
const pickerKeys = "←→ change · # hex · d theme's · enter save · esc cancel"

// picking reports whether the picker shows: its rows, or its hex field.
func (m Model) picking() bool { return m.mode == modeTheme || m.mode == modeHex }

// startTheme opens the picker on the theme row, with the saved colors as the
// draft. The list's help hides meanwhile, since its keys don't apply.
func (m Model) startTheme() (tea.Model, tea.Cmd) {
	m.mode, m.themeRow, m.draft = modeTheme, 0, m.cfg.Colors
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
	rows := colorRows()
	slot := rows[m.themeRow].slot(&m.draft)
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
		if m.themeRow == 0 {
			*slot = cycleTheme(*slot, step)
		} else {
			*slot = cycleColor(*slot, step)
		}
		m.restyle(m.draft)
	case "d":
		*slot = "" // the theme's color; on the theme row, terminal
		m.restyle(m.draft)
	case "#":
		if m.themeRow > 0 {
			return m.startHex()
		}
	case "enter":
		return m.saveColors()
	case "esc":
		m.restyle(m.cfg.Colors)
		return m.closePicker(), nil
	}
	return m, nil
}

// cycleTheme is the theme step places from name in ThemeNames, wrapping.
// terminal is saved as "" (the default), and an unknown name counts as
// terminal, which is what the menu draws for it.
func cycleTheme(name string, step int) string {
	names := ThemeNames()
	i := max(0, indexFold(names, name))
	next := names[(i+step+len(names))%len(names)]
	if next == terminalTheme {
		return ""
	}
	return next
}

// cycleColor is the color step places from value, going through theme's ("")
// and then colorChoices, wrapping. A value that isn't a choice (typed with #,
// or not a color) counts as theme's.
func cycleColor(value string, step int) string {
	values := []string{""}
	for _, c := range colorChoices() {
		values = append(values, c.value)
	}
	i := max(0, indexFold(values, value))
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
	row := colorRows()[m.themeRow]
	value := *row.slot(&m.draft)
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
	*colorRows()[m.themeRow].slot(&m.draft) = value
	m.restyle(m.draft)
	m.mode = modeTheme
	m.hex.Blur()
	return m, nil
}

// saveColors saves the draft like the other changes: change a copy, save it,
// then swap it in. A failed save keeps the picker open with the error.
func (m Model) saveColors() (tea.Model, tea.Cmd) {
	if sameColors(m.draft, m.cfg.Colors) {
		// Every save replaces the one backup, so a no-op save would lose it.
		m.restyle(m.cfg.Colors) // the draft may differ in case only
		m = m.closePicker()
		return m, m.list.NewStatusMessage("Colors unchanged")
	}
	next := m.edit()
	next.Colors = m.draft
	if err := config.Save(m.configPath, next); err != nil {
		m.message, m.messageErr = "couldn't save: "+err.Error(), true
		return m, nil
	}
	m.cfg = next
	m = m.closePicker()
	return m, m.list.NewStatusMessage("Colors saved")
}

// sameColors reports whether a and b draw the same: values match in any case,
// and terminal is the same as no theme.
func sameColors(a, b config.Colors) bool {
	for _, row := range colorRows() {
		x, y := *row.slot(&a), *row.slot(&b)
		if row.key == "theme" {
			x, y = noTerminal(x), noTerminal(y)
		}
		if !strings.EqualFold(x, y) {
			return false
		}
	}
	return true
}

// noTerminal is name, or "" for terminal (the default).
func noTerminal(name string) string {
	if strings.EqualFold(name, terminalTheme) {
		return ""
	}
	return name
}

// pickerView draws the rows: the key, the value (with arrows on the
// highlighted row), and a muted hint, in columns.
func (m Model) pickerView() string {
	text := m.styles.note.UnsetPadding()
	lines := make([]string, 0, len(colorRows()))
	for i, row := range colorRows() {
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

// pickerValue draws a row's draft value: a theme name, or a color as a swatch
// and name · value. Values the menu ignores are marked.
func (m Model) pickerValue(i int) string {
	text := m.styles.note.UnsetPadding()
	value := *colorRows()[i].slot(&m.draft)
	switch {
	case i == 0 && value == "":
		return text.Render(terminalTheme)
	case i == 0 && !KnownTheme(value):
		return text.Render(value + " (unknown)")
	case i == 0:
		return text.Render(value)
	case value == "":
		return m.styles.muted.Render("theme's")
	case !validColor(value):
		return text.Render(value + " (ignored)")
	}
	swatch := lipgloss.NewStyle().Foreground(lipgloss.Color(value)).Render("██")
	for _, c := range colorChoices() {
		if strings.EqualFold(c.value, value) {
			return swatch + text.Render(" "+c.name+" · "+c.value)
		}
	}
	return swatch + text.Render(" "+value)
}

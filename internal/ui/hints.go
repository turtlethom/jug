package ui

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
)

// hint is one "key desc" pair in a line of key hints. A hint with no key is
// just a note, drawn muted.
type hint struct{ key, desc string }

// hints draws hs as "key desc • key desc", with keys in the accent and the
// rest muted, like every key hint in the menu. If they don't fit width, the
// line stops before the first one that doesn't and ends with " …". A width of
// 0 or less means no limit.
func (m Model) hints(width int, hs ...hint) string {
	parts := make([]string, len(hs))
	for i, h := range hs {
		p := m.styles.muted.Render(h.desc)
		if h.key != "" {
			p = m.styles.key.Render(h.key) + m.styles.muted.Render(" "+h.desc)
		}
		if i > 0 {
			p = m.styles.muted.Render(" • ") + p
		}
		parts[i] = p
	}
	tail := m.styles.muted.Render(" …")
	n := len(parts)
	for width > 0 && n > 0 && lipgloss.Width(strings.Join(parts[:n], ""))+ellipsis(n < len(parts), tail) > width {
		n--
	}
	line := strings.Join(parts[:n], "")
	if n < len(parts) && lipgloss.Width(line+tail) <= width {
		line += tail
	}
	return line
}

// ellipsis is the width of tail if the line is cut, else 0.
func ellipsis(cut bool, tail string) int {
	if cut {
		return lipgloss.Width(tail)
	}
	return 0
}

// bindingHints are the hints for the enabled bindings, in order.
func bindingHints(bindings []key.Binding) []hint {
	var hs []hint
	for _, b := range bindings {
		if b.Enabled() {
			hs = append(hs, hint{b.Help().Key, b.Help().Desc})
		}
	}
	return hs
}

// helpView is the help under the list: one line of the keys that apply now,
// or with ? bubbles' full help, each line cut to the window. It's empty while
// the picker is open, since the picker's keys are on the bottom line.
//
// pathpin draws this instead of the list because bubbles' help can't cut its
// line to a narrow window: once even its … doesn't fit, it adds every key left.
// The width is the window's but for the frame's left padding, so the help line
// may use the column of margin the list keeps on the right.
func (m Model) helpView() string {
	if m.picking() {
		return ""
	}
	width := m.width - m.styles.app.GetHorizontalFrameSize() - m.styles.help.GetHorizontalFrameSize()
	if m.list.Help.ShowAll {
		h := m.list.Help
		h.Width = 0 // all the columns; they're cut to the window below
		full := h.FullHelpView(m.list.FullHelp())
		if width > 0 {
			full = lipgloss.NewStyle().MaxWidth(width).Render(full)
		}
		return m.styles.help.Render(full)
	}
	return m.styles.help.Render(m.hints(width, bindingHints(m.list.ShortHelp())...))
}

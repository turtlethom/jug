package ui

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/turtlethom/pathpin/internal/config"
)

// actions are what the action keys do outside the menu. New fills in the real
// ones; tests replace them. copy and open only ever run inside a tea.Cmd, never
// in Update, so a slow clipboard tool or file manager can't freeze the menu.
// editor only builds the editor's command; Bubble Tea runs it (ExecProcess).
type actions struct {
	copy   func(text string) error    // y: put text on the clipboard
	open   func(dir string) error     // o: show dir in the file manager
	editor func(dir string) *exec.Cmd // e: the editor, to run in dir
}

func systemActions() actions { return actions{copy: copyText, open: openDir, editor: editorIn} }

// copiedMsg, openedMsg and editedMsg report how y, o and e went, for the
// status line.
type (
	copiedMsg struct {
		path string
		err  error
	}
	openedMsg struct {
		path string
		err  error
	}
	editedMsg struct {
		path string
		err  error
	}
)

// copyPath copies the highlighted bookmark's full path (not ~, which only an
// unquoted shell expands), even if its folder is gone: it's only text.
func (m Model) copyPath() (tea.Model, tea.Cmd) {
	it, ok := m.list.SelectedItem().(item)
	if !ok {
		return m, nil // empty list
	}
	toClipboard := m.act.copy // the command gets the function, never the model
	return m, func() tea.Msg { return copiedMsg{it.Path, toClipboard(it.Path)} }
}

// openFolder opens the highlighted folder in the file manager, unless it's
// gone (as enter would find).
func (m Model) openFolder() (tea.Model, tea.Cmd) {
	it, ok := m.list.SelectedItem().(item)
	if !ok {
		return m, nil // empty list
	}
	if cmd := m.refuseGone(it.Path); cmd != nil {
		return m, cmd
	}
	open := m.act.open
	return m, func() tea.Msg { return openedMsg{it.Path, open(it.Path)} }
}

// editFolder hands the terminal to the editor in the highlighted folder,
// unless it's gone. tea.ExecProcess pauses the menu, runs the editor, and
// sends editedMsg when it exits; building the command here starts nothing.
func (m Model) editFolder() (tea.Model, tea.Cmd) {
	it, ok := m.list.SelectedItem().(item)
	if !ok {
		return m, nil // empty list
	}
	if cmd := m.refuseGone(it.Path); cmd != nil {
		return m, cmd
	}
	return m, tea.ExecProcess(m.act.editor(it.Path), func(err error) tea.Msg {
		return editedMsg{it.Path, err}
	})
}

// edited follows the editor: nothing to say if it went well (you just saw
// it), else the error on the bottom line.
func (m Model) edited(err error) (tea.Model, tea.Cmd) {
	if err != nil {
		m.message, m.messageErr = err.Error(), true
	}
	return m, nil
}

// actionDone shows how an action went: "Copied ~/notes" as a status, or the
// error on the bottom line.
func (m Model) actionDone(verb, path string, err error) (tea.Model, tea.Cmd) {
	if err != nil {
		m.message, m.messageErr = err.Error(), true
		return m, nil
	}
	return m, m.list.NewStatusMessage(verb + " " + config.ContractHome(path))
}

// clipboardCommand is the command that copies its stdin to the clipboard:
// pbcopy on macOS; on Linux wl-copy under Wayland, else xclip, else xsel. It's
// nil if none is installed. getenv and lookPath are os.Getenv and
// exec.LookPath, passed in so tests can pick the system.
func clipboardCommand(goos string, getenv func(string) string, lookPath func(string) (string, error)) []string {
	if goos == "darwin" {
		return []string{"pbcopy"}
	}
	has := func(name string) bool { _, err := lookPath(name); return err == nil }
	switch {
	case getenv("WAYLAND_DISPLAY") != "" && has("wl-copy"):
		return []string{"wl-copy"}
	case has("xclip"):
		return []string{"xclip", "-selection", "clipboard"}
	case has("xsel"):
		return []string{"xsel", "--clipboard", "--input"}
	}
	return nil
}

// openCommand is the command that shows dir in the file manager.
func openCommand(goos, dir string) []string {
	if goos == "darwin" {
		return []string{"open", dir}
	}
	return []string{"xdg-open", dir}
}

// editorCommand is the command that edits the current folder: $VISUAL, else
// $EDITOR, else vi (like git), split on spaces (so "code -w" works), with "."
// for the folder. A variable of only spaces counts as unset. getenv is
// os.Getenv, passed in for tests.
func editorCommand(getenv func(string) string) []string {
	for _, name := range []string{"VISUAL", "EDITOR"} {
		if argv := strings.Fields(getenv(name)); len(argv) > 0 {
			return append(argv, ".")
		}
	}
	return []string{"vi", "."}
}

// editorIn is the editor command, run in dir.
func editorIn(dir string) *exec.Cmd {
	argv := editorCommand(os.Getenv)
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = dir
	return cmd
}

// copyText puts text on the clipboard with the system's tool.
func copyText(text string) error {
	argv := clipboardCommand(runtime.GOOS, os.Getenv, exec.LookPath)
	if argv == nil {
		return errors.New("no clipboard tool found: install wl-clipboard or xclip")
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin = strings.NewReader(text)
	// No output pipes (nil is /dev/null): xclip and wl-copy stay in the
	// background to serve the clipboard, and would hold a pipe open, so
	// waiting for its output would wait until something else is copied.
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", argv[0], err)
	}
	return nil
}

// openDir shows dir in the file manager. The opener is started, not waited
// for, since some keep running; its output would draw over the menu, so it
// goes nowhere.
func openDir(dir string) error {
	argv := openCommand(runtime.GOOS, dir)
	cmd := exec.Command(argv[0], argv[1:]...)
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }() // reap it whenever it exits; nothing to report by then
	return nil
}

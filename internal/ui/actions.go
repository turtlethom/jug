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
// ones; tests replace them. They only ever run inside a tea.Cmd, never in
// Update, so a slow clipboard tool or file manager can't freeze the menu.
type actions struct {
	copy func(text string) error // y: put text on the clipboard
	open func(dir string) error  // o: show dir in the file manager
}

func systemActions() actions { return actions{copy: copyText, open: openDir} }

// copiedMsg and openedMsg report how y and o went, for the status line.
type (
	copiedMsg struct {
		path string
		err  error
	}
	openedMsg struct {
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

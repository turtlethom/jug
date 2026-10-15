package ui

import (
	"errors"
	"os/exec"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/turtlethom/pathpin/internal/config"
)

// fakeActions records what y and o asked for, instead of touching the
// clipboard or opening a file manager.
type fakeActions struct {
	copied, opened   []string
	copyErr, openErr error
}

func faked(m Model, f *fakeActions) Model {
	m.act.copy = func(text string) error { f.copied = append(f.copied, text); return f.copyErr }
	m.act.open = func(dir string) error { f.opened = append(f.opened, dir); return f.openErr }
	return m
}

// act presses k, checks that nothing happened yet (the action belongs in the
// command, not in Update), then runs the command and delivers its message.
func act(t *testing.T, m Model, f *fakeActions, k tea.KeyMsg) Model {
	t.Helper()
	m, cmd := press(m, k)
	if len(f.copied)+len(f.opened) > 0 {
		t.Fatalf("%s acted inside Update; it should only return a command", k)
	}
	if cmd == nil {
		t.Fatalf("%s returned no command", k)
	}
	return deliver(m, cmd())
}

// y copies the highlighted bookmark's full path (not ~) and says so with the ~
// path. The menu stays open.
func TestCopyPath(t *testing.T) {
	f := &fakeActions{}
	m := faked(twoBookmarks(t), f) // NVIM, highlighted, is ~/.config/nvim
	want := m.cfg.Bookmarks[0].Path
	m = act(t, m, f, runes("y"))
	if !slices.Equal(f.copied, []string{want}) {
		t.Errorf("want %q copied, got %q", want, f.copied)
	}
	if m.mode != modeBrowse || m.chosen != "" || !strings.Contains(plain(m.View()), "Copied ~/.config/nvim") {
		t.Errorf("want the menu open with a Copied status:\n%s", plain(m.View()))
	}
}

// A clipboard error (no tool, or the tool failing) shows on the bottom line.
func TestCopyError(t *testing.T) {
	f := &fakeActions{copyErr: errors.New("No clipboard tool found: install wl-clipboard or xclip")}
	m := act(t, faked(twoBookmarks(t), f), f, runes("y"))
	if !strings.Contains(plain(m.View()), "No clipboard tool found") || !m.messageErr {
		t.Errorf("want the error on the bottom line:\n%s", plain(m.View()))
	}
}

// y copies a missing folder's path too: it's only text.
func TestCopyMissingFolder(t *testing.T) {
	f := &fakeActions{}
	m := faked(newTestModel(t, config.Bookmark{Label: "GONE", Path: "!gone"}), f)
	want := m.cfg.Bookmarks[0].Path
	act(t, m, f, runes("y"))
	if !slices.Equal(f.copied, []string{want}) {
		t.Errorf("want %q copied, got %q", want, f.copied)
	}
}

// o opens the highlighted folder and says so. The menu stays open.
func TestOpenFolder(t *testing.T) {
	f := &fakeActions{}
	m := faked(twoBookmarks(t), f)
	m, _ = press(m, tea.KeyMsg{Type: tea.KeyDown}) // NOTES
	want := m.cfg.Bookmarks[1].Path
	m = act(t, m, f, runes("o"))
	if !slices.Equal(f.opened, []string{want}) {
		t.Errorf("want %q opened, got %q", want, f.opened)
	}
	if m.mode != modeBrowse || m.chosen != "" || !strings.Contains(plain(m.View()), "Opened ~/notes") {
		t.Errorf("want the menu open with an Opened status:\n%s", plain(m.View()))
	}
}

// An opener that fails shows its error on the bottom line.
func TestOpenError(t *testing.T) {
	f := &fakeActions{openErr: errors.New("xdg-open: executable file not found in $PATH")}
	m := act(t, faked(twoBookmarks(t), f), f, runes("o"))
	if !strings.Contains(plain(m.View()), "xdg-open: executable file not found") || !m.messageErr {
		t.Errorf("want the error on the bottom line:\n%s", plain(m.View()))
	}
}

// o refuses a missing folder like enter does, and dims its row.
func TestOpenMissingFolder(t *testing.T) {
	f := &fakeActions{}
	m := faked(unstarted(t, config.Bookmark{Label: "GONE", Path: "!gone"}), f) // not checked yet
	m, _ = press(m, runes("o"))
	if len(f.opened) > 0 {
		t.Errorf("o should not open a missing folder, opened %q", f.opened)
	}
	if !strings.Contains(plain(m.View()), "~/gone no longer exists") || !rowFor(t, m, "GONE").missing {
		t.Errorf("want the folder reported and dimmed:\n%s", plain(m.View()))
	}
}

// While a filter is typed, y and o are typed into it.
func TestActionKeysWhileFiltering(t *testing.T) {
	f := &fakeActions{}
	m := faked(twoBookmarks(t), f)
	for _, k := range []string{"/", "y", "o"} {
		m, _ = press(m, runes(k))
	}
	if m.list.FilterValue() != "yo" || len(f.copied)+len(f.opened) > 0 {
		t.Errorf("want yo typed and nothing done; filter %q, copied %q, opened %q", m.list.FilterValue(), f.copied, f.opened)
	}
}

// On an empty menu, y and o do nothing.
func TestActionKeysOnEmptyMenu(t *testing.T) {
	f := &fakeActions{}
	m := faked(newTestModel(t), f)
	for _, k := range []string{"y", "o"} {
		if _, cmd := press(m, runes(k)); cmd != nil {
			t.Errorf("%s on an empty menu should do nothing", k)
		}
	}
}

// The full help lists y and o.
func TestActionKeysInHelp(t *testing.T) {
	m, _ := press(twoBookmarks(t), runes("?"))
	for _, k := range [][2]string{{"y", "copy path"}, {"o", "open"}} {
		if !helpShows(m, k[0], k[1]) {
			t.Errorf("the full help should list %s %s:\n%s", k[0], k[1], plain(m.View()))
		}
	}
}

// clipboardCommand picks the tool that copies its stdin: pbcopy on macOS; on
// Linux wl-copy under Wayland, else xclip, else xsel; nil if none is there.
func TestClipboardCommand(t *testing.T) {
	env := func(wayland string) func(string) string {
		return func(k string) string {
			if k == "WAYLAND_DISPLAY" {
				return wayland
			}
			return ""
		}
	}
	installed := func(tools ...string) func(string) (string, error) {
		return func(name string) (string, error) {
			if slices.Contains(tools, name) {
				return "/usr/bin/" + name, nil
			}
			return "", exec.ErrNotFound
		}
	}
	for name, tc := range map[string]struct {
		goos     string
		wayland  string
		tools    []string
		wantArgv []string
	}{
		"macOS":                   {"darwin", "", nil, []string{"pbcopy"}},
		"Wayland":                 {"linux", "wayland-0", []string{"wl-copy", "xclip"}, []string{"wl-copy"}},
		"Wayland without wl-copy": {"linux", "wayland-0", []string{"xclip"}, []string{"xclip", "-selection", "clipboard"}},
		"X11":                     {"linux", "", []string{"wl-copy", "xclip"}, []string{"xclip", "-selection", "clipboard"}},
		"xsel only":               {"linux", "", []string{"xsel"}, []string{"xsel", "--clipboard", "--input"}},
		"nothing":                 {"linux", "wayland-0", nil, nil},
	} {
		if got := clipboardCommand(tc.goos, env(tc.wayland), installed(tc.tools...)); !slices.Equal(got, tc.wantArgv) {
			t.Errorf("%s: got %q, want %q", name, got, tc.wantArgv)
		}
	}
}

// openCommand opens a folder in the file manager: open on macOS, xdg-open
// elsewhere.
func TestOpenCommand(t *testing.T) {
	if got := openCommand("darwin", "/x/y"); !slices.Equal(got, []string{"open", "/x/y"}) {
		t.Errorf("macOS: got %q", got)
	}
	if got := openCommand("linux", "/x/y"); !slices.Equal(got, []string{"xdg-open", "/x/y"}) {
		t.Errorf("Linux: got %q", got)
	}
}

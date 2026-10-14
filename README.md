# pathpin

**pin your paths.** A bookmark-first directory navigator for the terminal: pin the folders you
use, then jump to them from a menu or by label. Written in Go with
[Bubble Tea](https://github.com/charmbracelet/bubbletea). Works in bash, zsh and fish on Linux
and macOS.

> pathpin is the successor to `fcd`, and still pre-release. A full README with a demo comes with v2.0.

## Install

Until the v2.0 release, build from the `v2` branch (needs Go 1.25):

```bash
git clone -b v2 https://github.com/turtlethom/pathpin.git
cd pathpin && go build -o ~/.local/bin/pathpin .
```

Then add one line to your shell config, so pathpin can change your shell's directory:

| Shell | File | Line |
|---|---|---|
| bash | `~/.bashrc` | `eval "$(pathpin init bash)"` |
| zsh | `~/.zshrc` | `eval "$(pathpin init zsh)"` |
| fish | `~/.config/fish/config.fish` | `pathpin init fish \| source` |

### Pick your own command name

The command is `pathpin` by default. To type something shorter, name it with `--cmd`:

```bash
eval "$(pathpin init bash --cmd pp)"
```

Now `pp` opens the menu and `pp nvim` jumps. Any name made of letters, digits, `_` and `-` works.
The function hides any other command with the same name in your interactive shell, so pick one
you don't already use. (The examples below use `pathpin`.)

## Use

```bash
pathpin              # open the menu
pathpin nvim         # jump to the bookmark labeled NVIM (any case)
pathpin add          # bookmark this folder, labeled with its name in uppercase
pathpin add work     # ...or with your own label
pathpin add -g Work  # ...in a group (--group; tab completes group names)
pathpin ls           # list bookmarks by group (tab-separated when piped)
```

In the menu:

| Key | Action |
|---|---|
| `enter` | go to the highlighted folder |
| `/` | filter |
| `a` | add the folder you opened pathpin from |
| `r` | remove the highlighted bookmark (asks first) |
| `c` | clear all bookmarks (asks first) |
| `m` | move the highlighted bookmark to a group (`tab` completes a name; empty removes its group) |
| `n` | rename the highlighted bookmark |
| `N` | rename the open group (renaming it to another group's name merges them, after asking) |
| `J` / `K` (or `shift+↓` / `shift+↑`) | move the highlighted bookmark down / up |
| `tab` / `shift+tab` | next / previous group (when you have groups) |
| `q` / `esc` | quit without changing directory |
| `?` | help |

## Config

Bookmarks live in `~/.config/pathpin/config.toml` (or `$XDG_CONFIG_HOME/pathpin/config.toml`).
Paths inside your home folder are stored as `~/...`, so the file works across machines and in a
dotfiles repo (symlinks are followed).

```toml
[[bookmark]]
label = "NVIM"
path = "~/.config/nvim"

[colors]
theme = "catppuccin-mocha"
```

- Before every change, pathpin copies the previous file to `config.toml.bak`. It keeps **one**
  step back, so the next change replaces it.
- pathpin rewrites the file when you add or remove from the menu, so **comments aren't kept**.
- A file pathpin can't read is reported and left untouched, never reset.

### Groups

Press `m` on a bookmark to put it in a group (or use `pathpin add --group`, or set `group` in the
config), and the menu shows a tab for each group, plus All and, for bookmarks without a group,
Other:

```toml
[[bookmark]]
label = "NVIM"
path = "~/.config/nvim"
group = "Config"
```

`tab` and `shift+tab` switch tabs, and `/` filters the tab you're on. On All, it matches group names
too, so `school` finds everything in School.
Adding a bookmark with `a` while a group's tab is open puts it in that group, and `N` renames the
open group (or merges it into another). Don't name a group `All` or `Other`: those are the tab
names.

`pathpin ls` lists bookmarks under their groups. Piped, each line is
`LABEL<tab>/absolute/path<tab>GROUP` (empty group if there's none), so `cut -f1,2` gives label and
path.

### Themes

Pick a theme in `[colors]`:

| Theme | |
|---|---|
| `terminal` (default) | your terminal's own colors, so it follows your terminal theme |
| `catppuccin-latte` | light |
| `catppuccin-mocha`, `gruvbox-dark`, `monokai-pro-ristretto`, `nord`, `one-dark`, `tokyo-night` | dark |

Then change any single color on top of it. Each takes a hex color (`"#56B6C2"`) or an ANSI
number (`"6"`):

```toml
[colors]
theme = "nord"
primary = "#56B6C2"        # accent: the selected row, title and filter
secondary = "#ECEFF4"      # labels
tertiary = "#7B88A1"       # paths and other muted text
selected_text = "#2E3440"  # text on the accent
error = "#BF616A"          # error messages
```

Mistakes don't break the menu. A theme name pathpin doesn't know falls back to `terminal`, a value
that isn't a color is ignored, and pathpin says which line to fix when the menu closes.

### Icons

With a [Nerd Font](https://www.nerdfonts.com/) in your terminal, turn on icons before each label:
a folder, a house for your home folder, and ✗ for a folder that no longer exists.

```toml
[menu]
icons = true
```

Coming from `fcd`? Your bookmarks are imported automatically the first time you run pathpin, and
fcd's file is left in place.

## License

MIT

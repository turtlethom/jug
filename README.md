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
pathpin ls           # list bookmarks (tab-separated when piped)
```

In the menu:

| Key | Action |
|---|---|
| `enter` | go to the highlighted folder |
| `/` | filter |
| `a` | add the folder you opened pathpin from |
| `r` | remove the highlighted bookmark (asks first) |
| `c` | clear all bookmarks (asks first) |
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

A theme name pathpin doesn't know is reported when the menu closes, and the menu uses `terminal`.

Coming from `fcd`? Your bookmarks are imported automatically the first time you run pathpin, and
fcd's file is left in place.

## License

MIT

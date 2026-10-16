# Architecture

How pathpin is put together, for anyone reading or changing the code. The code's
own comments go into detail; this page is the map. Read it top to bottom once,
then use the section headings to find your way back.

## The big picture

A program can't change its parent shell's directory, so pathpin is two parts:
a small shell function (printed by `pathpin init`) and the Go binary. They talk
through a temp file.

```
 you type `pp`
      │
      ▼
 shell function pp            (internal/shell/posix.sh, pathpin.fish)
   makes a temp file, runs:  PATHPIN_CD_FILE=<tmp> PATHPIN_CMD=pp pathpin
      │
      ▼
 pathpin binary
   main.go ──► cmd.Execute()                     (cmd/root.go)
                 │ loadConfig: config.LoadOrMigrate   (internal/config)
                 ├─ `pathpin`         ──► ui.Run: the menu   (internal/ui)
                 ├─ `pathpin <label>` ──► jump
                 ├─ `pathpin add` / `ls` / `init`
                 ▼
               handoff ──► shell.WriteCDTarget(dir) writes dir to <tmp>
      │
      ▼
 shell function reads <tmp>, runs `cd dir`, deletes <tmp>
```

Stdout and stderr stay free: the menu draws on stderr, and the directory never
goes through stdout unless no wrapper is set up (then it's printed, so
`cd "$(pathpin notes)"` still works).

## Packages

| Package | What it does | Knows about |
|---|---|---|
| `main.go` | Calls `cmd.Execute()`. Nothing else, so everything is testable. | cmd |
| `cmd/` | The command line (Cobra): the root command (menu or jump), `add`, `ls`, `init`. | config, shell, ui |
| `internal/config/` | The TOML file: types, load, save, fcd migration, and every bookmark/group rule (`Add`, `SetGroup`, `Move`, ...). | nothing of pathpin's |
| `internal/shell/` | The wrapper scripts and the cd-file handoff. | nothing of pathpin's |
| `internal/ui/` | The Bubble Tea menu. | config |

Dependencies only point down the table: `config` and `shell` know nothing about
the menu, so the rules for bookmarks live in one place and the CLI and the menu
agree on them.

## Life of a jump

What happens when you type `pp`, pick a bookmark, and press enter:

1. **Shell:** the `pp` function makes a temp file and runs
   `PATHPIN_CD_FILE=<tmp> PATHPIN_CMD=pp command pathpin`.
2. **`cmd.Execute`** builds the command tree (`newRootCmd`) and runs it. With no
   arguments, the root's `RunE` runs.
3. **`loadConfig`** finds the config (`config.DefaultPath`) and calls
   `config.LoadOrMigrate`. On the very first run it imports fcd's JSON and
   prints one line about it.
4. **`warnAboutStyle`** prints a line for each bad theme, color or selection
   value (the menu falls back for all of them).
5. **`ui.Run`** starts Bubble Tea with `New(cfg, configPath)` on the alternate
   screen. `Init` starts a background check of every bookmark's folder.
6. Keys go to **`Update`**, which redraws through **`View`** after each one.
   `enter` calls `choose`: it checks the folder still exists, sets
   `m.chosen`, and returns `tea.Quit`.
7. **`ui.Run`** returns `m.chosen`. The root's `RunE` calls **`handoff`**, which
   calls `shell.WriteCDTarget` to write the path into the temp file.
8. **Shell:** the function reads the file and `cd`s there, then returns
   pathpin's exit status.

`pp notes` (a jump) skips steps 4–6: `jump` finds the bookmark and goes
straight to `handoff`.

## The menu (internal/ui)

### Bubble Tea in one diagram

```
              ┌──────────────── tea.Msg ◄────────────────┐
              ▼                                          │
 key/resize ─► Update(msg) ──► new Model ──► View() ──► screen
                    │
                    └──► tea.Cmd (runs in the background) ┘
```

- **Model** is all the menu's state. Bubble Tea keeps the current one.
- **Update** gets every message and returns the next Model, plus maybe a
  command. It has a value receiver: it changes its own copy and returns it.
- **View** turns the Model into a string. It never changes anything.
- **A command** (`tea.Cmd`) is a function Bubble Tea runs on another goroutine.
  Its return value is a message, which comes back into `Update`. Slow things
  (disk checks, the clipboard, the file manager) run as commands, and commands
  never touch the Model.

### One model, many modes

There's one `Model` with a `mode` field instead of nested components. `Update`
first handles the messages every mode treats the same way, then switches on
the mode:

| Mode | Opened by | Handled by | Leaves to |
|---|---|---|---|
| `modeBrowse` | start | `updateBrowse` | any mode below; `enter`/`q`/`esc` quit |
| `modeConfirm` | `r`, `c`, `N` (merge or ungroup) | `updateConfirm` → `apply` | browse (`y` applies, any other key cancels) |
| `modeInput` | `a` | `updateField` → `addFromField` | browse |
| `modeGroup` | `m` | `updateField` → `move` | browse |
| `modeRename` | `n` | `updateField` → `rename` | browse |
| `modeGroupName` | `N` | `updateField` → `renameGroup` | browse, or confirm for a merge/ungroup |
| `modeTheme` | `t` | `updateTheme` | browse (`enter` saves, `esc` cancels), or hex |
| `modeHex` | `#` in the picker | `updateField` → `setHex` | back to the picker |

The four text-field modes share `updateField`, which is handed the field to
edit (`labelField`, `groupField`, `hexField`) and what `enter` does.

Keys that don't belong to pathpin (arrows, `/`, `?`, `g`, paging) go to the
embedded `bubbles/list` through `m.list, cmd = m.list.Update(msg)`. That line
is the whole "nested component" pattern: pass the message on, keep the
component it returns.

### Messages

| Message | Sent by | Handled in |
|---|---|---|
| `tea.KeyMsg` | the keyboard | `Update`, then the mode's function |
| `tea.WindowSizeMsg` | Bubble Tea at start and on resize | `Update` → `resize` |
| `folderGoneMsg` | `checkFolder` commands from `Init` | `Update` → `markGone` (dims the row) |
| `copiedMsg`, `openedMsg` | the `y` and `o` commands (`actions.go`) | `actionDone` (status line) |
| `editedMsg` | `tea.ExecProcess` when the editor exits (`e`) | `edited` |
| the list's own (status timeouts, filter results) | `bubbles/list` | passed back to the list |

### How a change is saved: edit, save, swap

Every change in the menu (add, remove, clear, move, rename, reorder, the
picker) follows the same steps:

```go
next := m.edit()                       // a copy of the config, with its own bookmarks
if err := next.Add(label, m.cwd); ...  // change the copy (config's rules)
if err := config.Save(m.configPath, next); ...  // write it to disk
m.cfg = next                           // only now does the menu use it
// then rebuild the rows (reloadRows / InsertItem / RemoveItem), syncTabs, status
```

If the change or the save fails, the menu still shows the old config, so the
screen always matches the file. Saving is synchronous on purpose: quick
repeated changes (`J J J`) can't save out of order.

A change that changes nothing (same group, same label) saves nothing, because
every save replaces the one `.bak` backup.

### What's on screen

`View` stacks these, top to bottom:

```
 PATHPIN                       ← the list's title (a badge)
                               ← blank line            ┐
 All  Config  School  Other    ← tabBar()              │ only with groups
 ─────────────────────────     ← tabRule()             ┘
                               ← blank line
 ▌ NVIM · Config               ← rows: item + delegate
 ▌ ~/.config/nvim
   NOTES
   ~/notes
 3 bookmarks                   ← status bar (the list's)
                               ← blank line
 ↑/k up • ↓/j down • / filter  ← helpView() (hidden while picking)
 theme      ◂ nord ▸           ← pickerView() (only with t)
 Remove NVIM? y/N              ← bottomLine(): prompt, field, or message
```

`resize` gives the list whatever height is left after the help, tabs, picker
and bottom line, so the whole menu always fits the window.

### Rows: item, delegate, filter

- **`item`** wraps a `config.Bookmark` (embedded, so `it.Label` works). It
  meets `bubbles/list`'s interfaces by having `Title`, `Description` and
  `FilterValue`; Go interfaces need no declaration.
- **`delegate`** draws a row. It embeds `list.DefaultDelegate`, lets it draw
  the label line (selection block, filter underlines), then adds the group tag
  (`renderTag`) and draws the path itself (`renderPath`), cutting long paths in
  the middle (`cutMiddle`).
- **`filterByRune`** replaces the list's filter. On All it matches labels and
  groups separately, and it converts the fuzzy matcher's byte positions to
  character positions so underlines land on the right letters.
- **`itemsFrom` / `newItem`** build the rows for the open tab from the config.
  Tabs are derived from the bookmarks each time (`tabs`, `inTab`); there's no
  separate list of groups to keep in sync.

### Styles and themes

`themes.go` has the palettes (five colors each). `NewStyles` turns a theme plus
the `[colors]` overrides into `Styles`. The list, the delegate and the text
fields each keep their own copies of some styles, so `restyle` is the one place
that updates all of them; the picker calls it on every change to preview.

### Concurrency rules

- Only `Update` changes the Model. Commands get the values they need (a path, a
  function) and return a message.
- Never let `bubbles/list`'s background re-filter run while rows can change:
  `markGone` and `setRows` re-filter synchronously with `SetFilterText`, and a
  `folderGoneMsg` that arrives while a filter is being typed is re-sent 100 ms
  later with `tea.Tick`.
- `enter` still checks its folder synchronously, so `enter` on a hung network
  mount waits.

## Config (internal/config)

```toml
[[bookmark]]
label = "NVIM"
path  = "~/.config/nvim"
group = "Config"

[colors]
theme = "nord"

[menu]
icons = true
selection = "bar"
```

- **Location:** `$XDG_CONFIG_HOME/pathpin/config.toml`, else
  `~/.config/pathpin/config.toml` (Linux and macOS alike).
- **Load** never resets a broken file: it returns an error and leaves the
  file alone. Unknown keys are an error too, since saving would drop them.
  `~` paths are expanded to absolute paths in memory.
- **Save** follows symlinks (dotfiles), copies the old file to
  `config.toml.bak` (one level), writes a temp file next to the target, syncs
  it, and renames it over the target, so a crash never leaves half a file.
  Paths inside `$HOME` are written as `~/...`. It copies the whole `Config`
  (`out := *c`), so a new setting is saved without touching `Save`.
- **Migration:** if there's no config yet but fcd's JSON exists,
  `LoadOrMigrate` converts it, saves the TOML, and leaves the JSON in place.
- **Rules live here, not in the menu:** labels match in any case and are stored
  as typed; a group is just a `group` field (no group tables), spelled as it
  first appears; `All` and `Other` are reserved for the menu's tabs; `Move`
  swaps with the nearest neighbour on the open tab.

## Shell integration (internal/shell)

- `pathpin init <shell> [--cmd name]` (`cmd/init.go`) prints the embedded
  script for bash/zsh (`posix.sh`) or fish (`pathpin.fish`), with
  `__PATHPIN_CMD__` replaced by the chosen name, followed by Cobra's completions
  for that name.
- `--cmd` names are checked against a strict pattern, since they're pasted
  into code the user evals.
- `shell.CmdName()` reads `PATHPIN_CMD` so errors and help say `pp` when
  that's what you type. Setup instructions always say `pathpin`, the binary.

## Tests

- **Spec first:** each feature's tests were written before its code, so the
  test names describe the behavior (`TestTabsFitWindow`, `TestPickerKeysLine`).
- **Menu tests** call `Update` directly with `tea.KeyMsg` values: no terminal,
  no extra dependency. Helpers in `internal/ui/menu_test.go` and
  `folders_test.go`:
  - `newTestModel` makes a temp `$HOME`, creates the folders (a path starting
    with `!` is left missing), saves a config, starts the model, and sizes it
    to 80×24.
  - `press(m, key)` sends one key; `runes("abc")` makes a key message for
    typed text.
  - `started`, `runCmd`, `checkNow`, `deliver` and `unstarted` control when
    commands run, to test the background folder checks.
  - `onDisk` reads back what the menu saved.
- **Seams:** the real actions (`y`, `o`, `e`) are function fields on the Model
  (`m.act`), which tests replace; `cmd` tests pass a fake menu to
  `newRootCmd`; commands write through `cmd.OutOrStdout()` so tests capture
  output.
- **CI** (`.github/workflows/ci.yml`) runs `gofmt`, `go vet`, `go build` and
  `go test -race` on Linux and macOS, with zsh and fish installed for the
  shell tests.

## Recipes

- **A new menu key:** add a `case` in `updateBrowse` that calls a new method
  (`startX` for a key that opens a field or prompt), add it to the help
  (`editKeys` or `setHelpKeys`), and check that `?` still fits 80×24
  (`hints_test.go`). If it changes the config, follow edit, save, swap.
- **A new config setting:** add the field with a `toml:"...,omitempty"` tag.
  Load refuses unknown keys, so older binaries will reject a config that uses
  it. If the picker should show it, add a row in `appearanceRows`.
- **A new theme:** add a line in `themes()` using `hex(...)`. The tests check
  the text-on-accent contrast.
- **A new subcommand:** a `newXCmd` in `cmd/`, added in `newRootCmd`. A
  bookmark label that matches its name will get the "also a command" warning
  automatically.

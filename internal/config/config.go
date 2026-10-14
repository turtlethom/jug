// Package config loads, saves, and edits pathpin's bookmark configuration.
//
// On disk it is TOML at $XDG_CONFIG_HOME/pathpin/config.toml (falling back to
// ~/.config/pathpin/config.toml on both Linux and macOS):
//
//	[[bookmark]]
//	label = "NVIM"
//	path  = "~/.config/nvim"
//	group = "Config"
//
//	[colors]
//	primary = "#56B6C2"
//
// The doc comments are the spec, and config_test.go checks them.
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

// Bookmark is a saved directory.
//
// In memory, Path is always absolute. On disk, paths inside $HOME are written
// as "~/..." so a config can be shared across machines through dotfiles.
type Bookmark struct {
	Label string `toml:"label"`
	Path  string `toml:"path"`
	Group string `toml:"group,omitempty"`
}

// Colors pick the menu's theme and override its colors one at a time. Empty
// means "use the theme's". Theme names are checked by the ui package, so an
// unknown one loads fine and the menu warns about it.
type Colors struct {
	Theme        string `toml:"theme,omitempty"`
	Primary      string `toml:"primary,omitempty"`
	Secondary    string `toml:"secondary,omitempty"`
	Tertiary     string `toml:"tertiary,omitempty"`
	SelectedText string `toml:"selected_text,omitempty"`
	Error        string `toml:"error,omitempty"`
}

// Menu holds the menu's display options. Each is off unless set.
type Menu struct {
	Icons bool `toml:"icons,omitempty"` // Nerd Font icons before labels
}

// Config is the whole configuration file.
type Config struct {
	Bookmarks []Bookmark `toml:"bookmark"`
	Colors    Colors     `toml:"colors"`
	Menu      Menu       `toml:"menu,omitempty"` // omitempty: no empty [menu] table on save
}

var (
	// ErrDuplicate means a bookmark with the same label or path already exists.
	ErrDuplicate = errors.New("bookmark already exists")
	// ErrNotFound means no bookmark has the given label.
	ErrNotFound = errors.New("bookmark not found")
	// ErrGroupNotFound means no bookmark has the given group.
	ErrGroupNotFound = errors.New("group not found")
	// ErrReservedGroup means a group name is one of the menu's tab names.
	ErrReservedGroup = errors.New(`"All" and "Other" are the menu's tab names; pick another group name`)
)

// The menu's tab names besides the groups. SetGroup refuses them as group
// names, so a tab name always means one thing.
const (
	TabAll   = "All"   // every bookmark
	TabOther = "Other" // bookmarks without a group
)

// IsTabName reports whether name is one of the menu's own tab names, in any
// case and ignoring surrounding spaces.
func IsTabName(name string) bool {
	name = strings.TrimSpace(name)
	return strings.EqualFold(name, TabAll) || strings.EqualFold(name, TabOther)
}

// DefaultPath returns where pathpin's config lives: $XDG_CONFIG_HOME/pathpin/config.toml
// if XDG_CONFIG_HOME is set to an absolute path, otherwise
// $HOME/.config/pathpin/config.toml. This is the same on Linux and macOS.
func DefaultPath() (string, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if !filepath.IsAbs(base) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "pathpin", "config.toml"), nil
}

// LegacyPath returns where fcd v1 kept its config:
// filepath.Join(os.UserConfigDir(), "fcd", "fcd_config.json").
func LegacyPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "fcd", "fcd_config.json"), nil
}

// Load reads the config at path.
//
//   - A missing file is not an error: return an empty Config and do not create the file.
//   - A file that fails to parse returns an error and is left untouched. Never reset it.
//   - "~/..." paths are expanded to absolute paths.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &Config{}, nil
	}
	if err != nil {
		return nil, err // already names the file
	}

	var c Config
	md, err := toml.Decode(string(data), &c)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	// Save only writes known fields, so an unknown key (a typo, or a field
	// from a newer pathpin) would be silently dropped on the next save.
	if keys := md.Undecoded(); len(keys) > 0 {
		names := make([]string, len(keys))
		for i, k := range keys {
			names[i] = strconv.Quote(k.String())
		}
		return nil, fmt.Errorf("%s: unknown key %s", path, strings.Join(names, ", "))
	}
	if err := validate(&c); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &c, nil
}

// validate rejects what Add would never store (empty or duplicate labels,
// relative paths) and normalizes the rest: labels are trimmed, and paths are
// expanded and cleaned. Missing folders and duplicate paths are allowed.
func validate(c *Config) error {
	for i := range c.Bookmarks {
		b := &c.Bookmarks[i]
		b.Label = strings.TrimSpace(b.Label)
		b.Group = strings.TrimSpace(b.Group)
		if b.Label == "" {
			return fmt.Errorf("bookmark %d: label is empty", i+1)
		}
		if c.index(b.Label) != i {
			return fmt.Errorf("bookmark %d (%q): duplicate label", i+1, b.Label)
		}
		p, err := expandHome(b.Path)
		if err != nil {
			return err
		}
		if !filepath.IsAbs(p) {
			return fmt.Errorf("bookmark %d (%q): path %q is not absolute", i+1, b.Label, b.Path)
		}
		b.Path = filepath.Clean(p)
	}
	return nil
}

// Save writes c to path.
//
//   - Create parent directories as needed.
//   - If path is a symlink (common with dotfiles), write to the file it points
//     to and leave the symlink in place.
//   - Write atomically: write a temp file in the same directory, then rename it
//     over the target. Leave no temp files behind.
//   - If a config already exists, first copy its contents to path + ".bak",
//     replacing any older backup. The backup is a regular file next to path,
//     even when path is a symlink, so a dotfiles repo doesn't collect backups.
//     The first save of a new config makes no backup.
//   - Paths inside $HOME are written as "~/..." ($HOME itself as "~"). Others
//     are written as-is. Watch out: /home/tom2 is not inside /home/tom.
//   - Do not modify c.
//   - A symlink pointing at a missing file is an error. The written file keeps
//     the existing file's permissions, or 0644 for a new one.
func Save(path string, c *Config) error {
	target, err := filepath.EvalSymlinks(path)
	if errors.Is(err, fs.ErrNotExist) {
		if _, lerr := os.Lstat(path); lerr == nil {
			return fmt.Errorf("%s is a symlink to a missing file", path)
		}
		target, err = path, nil // first save
	}
	if err != nil {
		return err
	}

	mode := fs.FileMode(0o644)
	old, err := os.ReadFile(target)
	switch {
	case err == nil:
		info, err := os.Stat(target)
		if err != nil {
			return err
		}
		mode = info.Mode().Perm()
		// Never overwrite without a backup.
		if err := os.WriteFile(path+".bak", old, mode); err != nil {
			return fmt.Errorf("backing up %s: %w", path, err)
		}
	case !errors.Is(err, fs.ErrNotExist):
		return err
	}

	// Copy every setting, then rebuild the bookmarks with ~ paths, so a new
	// Config field is saved without a change here.
	out := *c
	out.Bookmarks = nil
	for _, b := range c.Bookmarks {
		b.Path = ContractHome(b.Path)
		out.Bookmarks = append(out.Bookmarks, b)
	}
	var buf bytes.Buffer
	buf.WriteString(fileHeader)
	enc := toml.NewEncoder(&buf)
	enc.Indent = "" // match hand-written TOML
	if err := enc.Encode(out); err != nil {
		return err
	}

	dir := filepath.Dir(target)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(target)+".*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) // no-op once renamed

	if _, err := tmp.Write(buf.Bytes()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), target)
}

const fileHeader = "# Written by pathpin. Comments are not kept; use group = \"...\" to organize.\n\n"

// ContractHome is the reverse of expandHome: the home directory becomes "~"
// and paths inside it become "~/...". Others, or all of them if the home
// directory is unknown, are returned unchanged.
func ContractHome(p string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	home = filepath.Clean(home)
	if p == home {
		return "~"
	}
	// The separator keeps /home/tom2 from matching /home/tom.
	if rest, ok := strings.CutPrefix(p, home+string(filepath.Separator)); ok {
		return "~/" + rest
	}
	return p
}

// MigrateFCD converts an fcd v1 JSON config into a Config.
//
// v1 format: {"shortcuts":[{"label":"NVIM","path":"/abs/path"}],
// "userColors":{"primary":"#56B6C2","secondary":"","tertiary":""}}
//
// Keep every bookmark even if its directory no longer exists, and never modify
// or delete the JSON file.
func MigrateFCD(jsonPath string) (*Config, error) {
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, fmt.Errorf("migrating fcd config %s: %w", jsonPath, err)
	}
	var v1 fcdConfig
	if err := json.Unmarshal(data, &v1); err != nil {
		return nil, fmt.Errorf("migrating fcd config %s: %w", jsonPath, err)
	}

	c := &Config{
		Colors: Colors{
			Primary:   v1.UserColors.Primary,
			Secondary: v1.UserColors.Secondary,
			Tertiary:  v1.UserColors.Tertiary,
		},
	}
	for _, s := range v1.Shortcuts {
		c.Bookmarks = append(c.Bookmarks, Bookmark{Label: s.Label, Path: s.Path})
	}
	return c, nil
}

// fcdConfig mirrors fcd v1's JSON so the public types stay TOML-only.
type fcdConfig struct {
	Shortcuts []struct {
		Label string `json:"label"`
		Path  string `json:"path"`
	} `json:"shortcuts"`
	UserColors struct {
		Primary   string `json:"primary"`
		Secondary string `json:"secondary"`
		Tertiary  string `json:"tertiary"`
	} `json:"userColors"`
}

// LoadOrMigrate is what pathpin calls on startup.
//
//   - If path exists, Load it (ignore legacyPath).
//   - Otherwise, if legacyPath exists, MigrateFCD it, Save the result to path,
//     and report migrated = true.
//   - Otherwise, return an empty Config without creating any file.
//   - A migrated config that Load would reject is not saved; the error names
//     legacyPath so the user knows which file to fix.
func LoadOrMigrate(path, legacyPath string) (cfg *Config, migrated bool, err error) {
	// Load treats a missing file as empty, so check existence first.
	// Stat follows symlinks: a dangling one counts as missing, and Save
	// reports it below.
	if _, err := os.Stat(path); err == nil {
		cfg, err := Load(path)
		return cfg, false, err
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, false, err
	}

	cfg, err = MigrateFCD(legacyPath)
	if errors.Is(err, fs.ErrNotExist) {
		return &Config{}, false, nil // fresh install
	}
	if err != nil {
		return nil, false, err
	}
	if err := validate(cfg); err != nil {
		return nil, false, fmt.Errorf("migrating fcd config %s: %w", legacyPath, err)
	}
	if err := Save(path, cfg); err != nil {
		return nil, false, err
	}
	return cfg, true, nil
}

// Add appends a bookmark.
//
//   - label is trimmed of surrounding spaces and must not be empty.
//   - dir may be relative (resolved against the working directory) or start
//     with "~/"; it is stored as an absolute, cleaned path.
//   - dir must exist and be a directory. It may be outside $HOME.
//   - A label that matches an existing one case-insensitively, or a path that
//     matches an existing one, fails with an error wrapping ErrDuplicate.
func (c *Config) Add(label, dir string) error {
	label = strings.TrimSpace(label)
	if label == "" {
		return errors.New("label is empty")
	}

	path, err := expandHome(dir)
	if err != nil {
		return err
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", path)
	}

	if c.index(label) >= 0 {
		return fmt.Errorf("label %q: %w", label, ErrDuplicate)
	}
	for _, b := range c.Bookmarks {
		if b.Path == path {
			return fmt.Errorf("%s is already saved as %q: %w", ContractHome(path), b.Label, ErrDuplicate)
		}
	}

	c.Bookmarks = append(c.Bookmarks, Bookmark{Label: label, Path: path})
	return nil
}

// SuggestLabel is the label offered when bookmarking dir: its name in
// uppercase, like fcd's labels. $HOME suggests HOME, and / suggests ROOT.
func SuggestLabel(dir string) string {
	dir = filepath.Clean(dir)
	if home, err := os.UserHomeDir(); err == nil && dir == filepath.Clean(home) {
		return "HOME"
	}
	if dir == string(filepath.Separator) {
		return "ROOT"
	}
	return strings.ToUpper(filepath.Base(dir))
}

// expandHome replaces a leading "~" or "~/" in p with the home directory.
// Anything else, including "~user/...", is returned unchanged.
func expandHome(p string) (string, error) {
	if p != "~" && !strings.HasPrefix(p, "~/") {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, p[1:]), nil
}

// Remove deletes the bookmark whose label matches case-insensitively.
// If none matches, it returns an error wrapping ErrNotFound.
func (c *Config) Remove(label string) error {
	i := c.index(label)
	if i < 0 {
		return fmt.Errorf("%q: %w", label, ErrNotFound)
	}
	c.Bookmarks = slices.Delete(c.Bookmarks, i, i+1)
	return nil
}

// Find returns the bookmark whose label matches case-insensitively.
func (c *Config) Find(label string) (Bookmark, bool) {
	i := c.index(label)
	if i < 0 {
		return Bookmark{}, false
	}
	return c.Bookmarks[i], true
}

// Groups lists group names in the order they first appear, each spelled as it
// first appears. Ungrouped bookmarks aren't a group, and neither is a
// hand-edited group named like a tab (see Ungrouped). The list is derived from
// the bookmarks each time, so it can't disagree with them.
func (c *Config) Groups() []string {
	var names []string
	for _, b := range c.Bookmarks {
		if !b.Ungrouped() && groupIndex(names, b.Group) < 0 {
			names = append(names, b.Group)
		}
	}
	return names
}

// Ungrouped reports whether b goes under Other: it has no group, or a
// hand-edited group named like a tab (SetGroup refuses those names), which
// would otherwise make a second All or Other.
func (b Bookmark) Ungrouped() bool {
	return b.Group == "" || IsTabName(b.Group)
}

// GroupOf is b's group as the menu shows it: the group's first spelling, or ""
// if b is ungrouped. A bookmark that isn't in c keeps its own group.
func (c *Config) GroupOf(b Bookmark) string {
	if b.Ungrouped() {
		return ""
	}
	names := c.Groups()
	if i := groupIndex(names, b.Group); i >= 0 {
		return names[i]
	}
	return b.Group
}

// SetGroup moves the bookmark whose label matches case-insensitively to
// group, trimmed. A name that matches an existing group case-insensitively
// takes that group's spelling, so the file stays consistent; "" removes the
// group. If no label matches, it returns an error wrapping ErrNotFound; a tab
// name (TabAll, TabOther) as group returns one wrapping ErrReservedGroup.
func (c *Config) SetGroup(label, group string) error {
	i := c.index(label)
	if i < 0 {
		return fmt.Errorf("%q: %w", label, ErrNotFound)
	}
	group = strings.TrimSpace(group)
	if IsTabName(group) {
		return fmt.Errorf("%q: %w", group, ErrReservedGroup)
	}
	names := c.Groups()
	if j := groupIndex(names, group); j >= 0 {
		group = names[j]
	}
	c.Bookmarks[i].Group = group
	return nil
}

// Rename changes the label of the bookmark whose label matches
// case-insensitively. newLabel is trimmed and stored as typed. It must not be
// empty or match another bookmark's label case-insensitively (an error
// wrapping ErrDuplicate), but a bookmark may change its own label's case. If
// no label matches, it returns an error wrapping ErrNotFound.
func (c *Config) Rename(label, newLabel string) error {
	i := c.index(label)
	if i < 0 {
		return fmt.Errorf("%q: %w", label, ErrNotFound)
	}
	newLabel = strings.TrimSpace(newLabel)
	if newLabel == "" {
		return errors.New("label is empty")
	}
	if j := c.index(newLabel); j >= 0 && j != i {
		return fmt.Errorf("label %q: %w", newLabel, ErrDuplicate)
	}
	c.Bookmarks[i].Label = newLabel
	return nil
}

// RenameGroup renames the group matching group case-insensitively, for every
// bookmark in it. newName is trimmed. A case-only change re-cases the group; a
// name matching another group case-insensitively merges into it, taking that
// group's spelling. An empty name ungroups every bookmark in the group, as
// SetGroup does for one. A tab name (TabAll, TabOther) fails with
// ErrReservedGroup, and an unknown group with ErrGroupNotFound.
func (c *Config) RenameGroup(group, newName string) error {
	names := c.Groups()
	g := groupIndex(names, group)
	if g < 0 {
		return fmt.Errorf("%q: %w", group, ErrGroupNotFound)
	}
	newName = strings.TrimSpace(newName) // "" ungroups
	if IsTabName(newName) {
		return fmt.Errorf("%q: %w", newName, ErrReservedGroup)
	}
	if j := groupIndex(names, newName); j >= 0 && j != g {
		newName = names[j] // merge into the other group, in its spelling
	}
	for i := range c.Bookmarks {
		if b := &c.Bookmarks[i]; !b.Ungrouped() && strings.EqualFold(b.Group, names[g]) {
			b.Group = newName
		}
	}
	return nil
}

// Move swaps the bookmark whose label matches case-insensitively with its
// neighbour below (or above): the nearest bookmark that way that within
// accepts, or any bookmark if within is nil. The neighbour may be far away in
// the file, as when the menu shows one group. Swapping leaves everything else
// in its place, so moving within a group never changes the order groups first
// appear in (the menu's tab order). It reports false, changing nothing, when
// there's no neighbour that way. If no label matches, it returns an error
// wrapping ErrNotFound.
func (c *Config) Move(label string, down bool, within func(Bookmark) bool) (bool, error) {
	i := c.index(label)
	if i < 0 {
		return false, fmt.Errorf("%q: %w", label, ErrNotFound)
	}
	step := -1
	if down {
		step = 1
	}
	for j := i + step; j >= 0 && j < len(c.Bookmarks); j += step {
		if within != nil && !within(c.Bookmarks[j]) {
			continue
		}
		c.Bookmarks[i], c.Bookmarks[j] = c.Bookmarks[j], c.Bookmarks[i]
		return true, nil
	}
	return false, nil
}

// groupIndex returns the position of group in names, matching
// case-insensitively, or -1.
func groupIndex(names []string, group string) int {
	return slices.IndexFunc(names, func(n string) bool { return strings.EqualFold(n, group) })
}

// index returns the position of the first bookmark whose label matches
// label case-insensitively, ignoring surrounding spaces, or -1 if none does.
func (c *Config) index(label string) int {
	label = strings.TrimSpace(label)
	return slices.IndexFunc(c.Bookmarks, func(b Bookmark) bool {
		return strings.EqualFold(b.Label, label)
	})
}

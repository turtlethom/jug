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

// Colors are the user's theme colors. Empty means "use the default".
type Colors struct {
	Primary   string `toml:"primary,omitempty"`
	Secondary string `toml:"secondary,omitempty"`
	Tertiary  string `toml:"tertiary,omitempty"`
}

// Config is the whole configuration file.
type Config struct {
	Bookmarks []Bookmark `toml:"bookmark"`
	Colors    Colors     `toml:"colors"`
}

var (
	// ErrDuplicate means a bookmark with the same label or path already exists.
	ErrDuplicate = errors.New("bookmark already exists")
	// ErrNotFound means no bookmark has the given label.
	ErrNotFound = errors.New("bookmark not found")
)

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

	out := Config{Colors: c.Colors}
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

// index returns the position of the first bookmark whose label matches
// label case-insensitively, ignoring surrounding spaces, or -1 if none does.
func (c *Config) index(label string) int {
	label = strings.TrimSpace(label)
	return slices.IndexFunc(c.Bookmarks, func(b Bookmark) bool {
		return strings.EqualFold(b.Label, label)
	})
}

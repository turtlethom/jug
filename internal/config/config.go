// Package config loads, saves, and edits jug's bookmark configuration.
//
// On disk it is TOML at $XDG_CONFIG_HOME/jug/config.toml (falling back to
// ~/.config/jug/config.toml on both Linux and macOS):
//
//	[[bookmark]]
//	label = "NVIM"
//	path  = "~/.config/nvim"
//	group = "Config"
//
//	[colors]
//	primary = "#56B6C2"
//
// TODO(you): every function below is a stub. Make the tests in
// config_test.go pass (`go test ./internal/config/`). The doc comments are
// the spec. Suggested TOML library: github.com/BurntSushi/toml.
package config

import "errors"

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

	errNotImplemented = errors.New("not implemented")
)

// DefaultPath returns where jug's config lives: $XDG_CONFIG_HOME/jug/config.toml
// if XDG_CONFIG_HOME is set, otherwise $HOME/.config/jug/config.toml.
func DefaultPath() (string, error) {
	return "", errNotImplemented
}

// LegacyPath returns where fcd v1 kept its config:
// filepath.Join(os.UserConfigDir(), "fcd", "fcd_config.json").
func LegacyPath() (string, error) {
	return "", errNotImplemented
}

// Load reads the config at path.
//
//   - A missing file is not an error: return an empty Config and do not create the file.
//   - A file that fails to parse returns an error and is left untouched. Never reset it.
//   - "~/..." paths are expanded to absolute paths.
func Load(path string) (*Config, error) {
	return nil, errNotImplemented
}

// Save writes c to path.
//
//   - Create parent directories as needed.
//   - If path is a symlink (common with dotfiles), write to the file it points
//     to and leave the symlink in place.
//   - Write atomically: write a temp file in the same directory, then rename it
//     over the target. Leave no temp files behind.
//   - Paths inside $HOME are written as "~/..." ($HOME itself as "~"). Others
//     are written as-is. Watch out: /home/tom2 is not inside /home/tom.
//   - Do not modify c.
func Save(path string, c *Config) error {
	return errNotImplemented
}

// MigrateFCD converts an fcd v1 JSON config into a Config.
//
// v1 format: {"shortcuts":[{"label":"NVIM","path":"/abs/path"}],
// "userColors":{"primary":"#56B6C2","secondary":"","tertiary":""}}
//
// Keep every bookmark even if its directory no longer exists, and never modify
// or delete the JSON file.
func MigrateFCD(jsonPath string) (*Config, error) {
	return nil, errNotImplemented
}

// LoadOrMigrate is what jug calls on startup.
//
//   - If path exists, Load it (ignore legacyPath).
//   - Otherwise, if legacyPath exists, MigrateFCD it, Save the result to path,
//     and report migrated = true.
//   - Otherwise, return an empty Config without creating any file.
func LoadOrMigrate(path, legacyPath string) (cfg *Config, migrated bool, err error) {
	return nil, false, errNotImplemented
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
	return errNotImplemented
}

// Remove deletes the bookmark whose label matches case-insensitively.
// If none matches, it returns an error wrapping ErrNotFound.
func (c *Config) Remove(label string) error {
	return errNotImplemented
}

// Find returns the bookmark whose label matches case-insensitively.
func (c *Config) Find(label string) (Bookmark, bool) {
	return Bookmark{}, false
}

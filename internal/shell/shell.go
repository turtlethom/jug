// Package shell provides pathpin's shell integration: the wrapper functions printed
// by `pathpin init` and the cd-file handoff they rely on.
//
// A child process cannot change its parent shell's directory, so the wrapper
// function creates a temp file, runs the real binary with CDFileEnv pointing at
// it, and cds to whatever path the binary wrote there.
package shell

import (
	_ "embed"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

// CDFileEnv is the environment variable the shell wrapper sets to a temp file.
// pathpin writes the directory to change into there.
const CDFileEnv = "PATHPIN_CD_FILE"

// CmdEnv is the environment variable the shell wrapper sets to the name the
// user types (from `pathpin init <shell> --cmd <name>`), so messages and help
// can use it.
const CmdEnv = "PATHPIN_CMD"

// DefaultCmd is the shell command's name when --cmd isn't given.
const DefaultCmd = "pathpin"

// placeholder marks the function name in the embedded scripts.
const placeholder = "__PATHPIN_CMD__"

// validCmd allows names that are safe as a function in bash, zsh and fish.
var validCmd = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)

//go:embed posix.sh
var posixScript string

//go:embed pathpin.fish
var fishScript string

// Shells returns the shells `pathpin init` supports.
func Shells() []string { return []string{"bash", "zsh", "fish"} }

// ValidateCmd reports whether name can be used as the shell command's name:
// letters, digits, _ and -, not starting with a digit or -.
func ValidateCmd(name string) error {
	if !validCmd.MatchString(name) {
		return fmt.Errorf("invalid command name %q: use letters, digits, _ and -, starting with a letter or _", name)
	}
	return nil
}

// CmdName is the name the user types, as set by the wrapper, or DefaultCmd.
func CmdName() string {
	if name := os.Getenv(CmdEnv); ValidateCmd(name) == nil {
		return name
	}
	return DefaultCmd
}

// Script returns the wrapper function for the given shell, named name.
func Script(shell, name string) (string, error) {
	if err := ValidateCmd(name); err != nil {
		return "", err
	}
	var script string
	switch shell {
	case "bash", "zsh":
		script = posixScript
	case "fish":
		script = fishScript
	default:
		return "", fmt.Errorf("unsupported shell %q (supported: bash, zsh, fish)", shell)
	}
	return strings.ReplaceAll(script, placeholder, name), nil
}

// WriteCDTarget hands dir to the shell wrapper so it can cd there after pathpin
// exits. Without the wrapper (CDFileEnv unset) it prints dir to w instead, so
// `cd "$(pathpin ...)"` still works.
func WriteCDTarget(w io.Writer, dir string) error {
	if f := os.Getenv(CDFileEnv); f != "" {
		return os.WriteFile(f, []byte(dir), 0o600)
	}
	_, err := fmt.Fprintln(w, dir)
	return err
}

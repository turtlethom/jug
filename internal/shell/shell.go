// Package shell provides pathpin's shell integration: the wrapper functions printed
// by `pathpin init` and the cd-file handoff they rely on.
//
// A child process cannot change its parent shell's directory, so the wrapper
// function creates a temp file, runs the real binary with CDFileEnv pointing at
// it, and cds to whatever path the binary wrote there.
//
// The flow for `pp` (set up with `pathpin init bash --cmd pp`):
//
//	shell function pp  ->  pathpin binary (menu, jump, ...)
//	                         | WriteCDTarget(dir)
//	                         v
//	                       temp file  ->  pp reads it and runs cd
//
// Stdout and stderr stay free for the menu and normal output.
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

// placeholder marks the function name in the embedded scripts. Script replaces
// every occurrence with the name the user picked.
const placeholder = "__PATHPIN_CMD__"

// validCmd allows names that are safe as a function in bash, zsh and fish.
// The name is pasted into shell code that the user evals, so anything else
// (spaces, quotes, ;) could break the script or run a command.
var validCmd = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_-]*$`)

// The //go:embed lines copy each script's text into the binary at build time,
// so pathpin needs no files next to it. The blank import of "embed" above
// enables them.
//
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
	// Validated like --cmd, since any program could set the variable.
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
		// The wrapper made the file; 0o600 (owner only) matters only if it
		// was somehow missing and gets created here.
		return os.WriteFile(f, []byte(dir), 0o600)
	}
	_, err := fmt.Fprintln(w, dir)
	return err
}

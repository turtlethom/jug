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
)

// CDFileEnv is the environment variable the shell wrapper sets to a temp file.
// pathpin writes the directory to change into there.
const CDFileEnv = "PATHPIN_CD_FILE"

//go:embed posix.sh
var posixScript string

//go:embed pathpin.fish
var fishScript string

// Shells returns the shells `pathpin init` supports.
func Shells() []string { return []string{"bash", "zsh", "fish"} }

// Script returns the wrapper function for the given shell.
func Script(shell string) (string, error) {
	switch shell {
	case "bash", "zsh":
		return posixScript, nil
	case "fish":
		return fishScript, nil
	}
	return "", fmt.Errorf("unsupported shell %q (supported: bash, zsh, fish)", shell)
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

package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/turtlethom/pathpin/internal/shell"
)

func runPathpin(t *testing.T, args ...string) (string, error) {
	t.Helper()
	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

func TestInit(t *testing.T) {
	completion := map[string]string{
		"bash": "__start_pathpin",
		"zsh":  "compdef _pathpin pathpin",
		"fish": "complete -c pathpin",
	}
	for _, sh := range shell.Shells() {
		out, err := runPathpin(t, "init", sh)
		if err != nil {
			t.Fatalf("pathpin init %s: %v", sh, err)
		}
		if !strings.Contains(out, shell.CDFileEnv) {
			t.Errorf("pathpin init %s: missing wrapper function", sh)
		}
		if !strings.Contains(out, completion[sh]) {
			t.Errorf("pathpin init %s: missing completions (%q)", sh, completion[sh])
		}
	}
}

func TestInitRejectsUnknownShell(t *testing.T) {
	if _, err := runPathpin(t, "init", "powershell"); err == nil {
		t.Error("pathpin init powershell should fail")
	}
	if _, err := runPathpin(t, "init"); err == nil {
		t.Error("pathpin init with no shell should fail")
	}
}

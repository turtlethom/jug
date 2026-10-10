package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/turtlethom/jug/internal/shell"
)

func runJug(t *testing.T, args ...string) (string, error) {
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
		"bash": "__start_jug",
		"zsh":  "compdef _jug jug",
		"fish": "complete -c jug",
	}
	for _, sh := range shell.Shells() {
		out, err := runJug(t, "init", sh)
		if err != nil {
			t.Fatalf("jug init %s: %v", sh, err)
		}
		if !strings.Contains(out, shell.CDFileEnv) {
			t.Errorf("jug init %s: missing wrapper function", sh)
		}
		if !strings.Contains(out, completion[sh]) {
			t.Errorf("jug init %s: missing completions (%q)", sh, completion[sh])
		}
	}
}

func TestInitRejectsUnknownShell(t *testing.T) {
	if _, err := runJug(t, "init", "powershell"); err == nil {
		t.Error("jug init powershell should fail")
	}
	if _, err := runJug(t, "init"); err == nil {
		t.Error("jug init with no shell should fail")
	}
}

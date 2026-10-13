package cmd

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

func TestInitCmdNamesFunctionAndCompletions(t *testing.T) {
	completion := map[string]string{
		"bash": "__start_pp",
		"zsh":  "compdef _pp pp",
		"fish": "complete -c pp",
	}
	for _, sh := range shell.Shells() {
		out, err := runPathpin(t, "init", sh, "--cmd", "pp")
		if err != nil {
			t.Fatalf("pathpin init %s --cmd pp: %v", sh, err)
		}
		if !strings.Contains(out, shell.CmdEnv+"=pp") || !strings.Contains(out, "command pathpin") {
			t.Errorf("%s: want a pp function that runs pathpin", sh)
		}
		if !strings.Contains(out, completion[sh]) {
			t.Errorf("%s: completions should register for pp (%q)", sh, completion[sh])
		}
	}
	if _, err := runPathpin(t, "init", "bash", "--cmd", "p p"); err == nil {
		t.Error("an invalid --cmd should fail")
	}
}

// TestInitWrongShell sources `pathpin init` output in a real bash or zsh. In
// the shell it was made for, completions register silently. In the other one
// (bash's line pasted into ~/.zshrc, say), the function still works but
// completions are skipped with a one-line hint, instead of failing on every
// Tab with errors like `__start_pp:9: command not found: _init_completion`.
func TestInitWrongShell(t *testing.T) {
	// Each shell loads its completion system the way an interactive setup
	// would. zsh also loads bashcompinit, which is how a bash script's
	// `complete` call succeeds in zsh and then breaks on Tab.
	shells := map[string]struct {
		args      []string
		setup     string
		completes string
	}{
		"bash": {
			args:      []string{"--noprofile", "--norc", "-c"},
			completes: "complete -p %s >/dev/null 2>&1",
		},
		"zsh": {
			args:      []string{"-f", "-c"},
			setup:     "autoload -U compinit bashcompinit; compinit -u -D; bashcompinit; ",
			completes: "(( $+_comps[%s] ))",
		},
	}
	cases := []struct {
		output, run string
		args        []string
		hint        string
	}{
		{output: "bash", run: "bash"},
		{output: "zsh", run: "zsh"},
		{output: "bash", run: "zsh", args: []string{"--cmd", "pp"},
			hint: `pathpin: this is the bash integration; in zsh use: eval "$(pathpin init zsh --cmd pp)"`},
		{output: "zsh", run: "bash", args: []string{"--cmd", "pp"},
			hint: `pathpin: this is the zsh integration; in bash use: eval "$(pathpin init bash --cmd pp)"`},
		{output: "bash", run: "zsh",
			hint: `pathpin: this is the bash integration; in zsh use: eval "$(pathpin init zsh)"`},
	}
	for _, c := range cases {
		name := c.output + " output in " + c.run
		if len(c.args) > 0 {
			name += " " + strings.Join(c.args, " ")
		}
		t.Run(name, func(t *testing.T) {
			if _, err := exec.LookPath(c.run); err != nil {
				t.Skipf("%s not installed", c.run)
			}
			out, err := runPathpin(t, append([]string{"init", c.output}, c.args...)...)
			if err != nil {
				t.Fatalf("pathpin init %s: %v", c.output, err)
			}
			file := filepath.Join(t.TempDir(), "init")
			if err := os.WriteFile(file, []byte(out), 0o644); err != nil {
				t.Fatal(err)
			}

			// Without --cmd the function is pathpin, so check for that name.
			fn := "pathpin"
			if len(c.args) > 0 {
				fn = "pp"
			}
			sh := shells[c.run]
			completes := fmt.Sprintf(sh.completes, fn)
			body := sh.setup + "source " + file + "; " +
				"type " + fn + " >/dev/null 2>&1 && echo defined; " +
				completes + " && echo completes; true"
			cmd := exec.Command(c.run, append(sh.args, body)...)
			// stderr must be exactly the hint, so keep startup files out:
			// --norc doesn't stop non-interactive bash reading $BASH_ENV.
			cmd.Env = append(os.Environ(), "HOME="+t.TempDir(), "BASH_ENV=")
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("%s: %v\nstdout:\n%s\nstderr:\n%s", c.run, err, &stdout, &stderr)
			}

			if !strings.Contains(stdout.String(), "defined") {
				t.Errorf("the %s function should be defined in either shell\nstderr:\n%s", fn, &stderr)
			}
			wantStderr := ""
			if c.hint != "" {
				wantStderr = c.hint + "\n"
			}
			if stderr.String() != wantStderr {
				t.Errorf("stderr:\n got %q\nwant %q", stderr.String(), wantStderr)
			}
			registered := strings.Contains(stdout.String(), "completes")
			if c.hint == "" && !registered {
				t.Error("completions should register in the matching shell")
			}
			if c.hint != "" && registered {
				t.Error("completions should be skipped in the wrong shell")
			}
		})
	}
}

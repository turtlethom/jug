package shell

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestScript(t *testing.T) {
	for _, sh := range Shells() {
		s, err := Script(sh, "pp")
		if err != nil {
			t.Fatalf("Script(%q): %v", sh, err)
		}
		// The function gets the chosen name; the binary is always pathpin.
		for _, want := range []string{CDFileEnv, CmdEnv + "=pp", "command pathpin"} {
			if !strings.Contains(s, want) {
				t.Errorf("Script(%q, pp) is missing %q", sh, want)
			}
		}
		if strings.Contains(s, placeholder) {
			t.Errorf("Script(%q, pp) left the placeholder in", sh)
		}
	}
	if _, err := Script("powershell", DefaultCmd); err == nil {
		t.Error("Script(powershell) should fail")
	}
	if _, err := Script("bash", "pp; rm -rf ~"); err == nil {
		t.Error("an invalid name must not reach the script")
	}
}

func TestValidateCmd(t *testing.T) {
	for _, ok := range []string{"pathpin", "pp", "j", "_p", "path-pin", "pp2"} {
		if err := ValidateCmd(ok); err != nil {
			t.Errorf("ValidateCmd(%q): %v", ok, err)
		}
	}
	for _, bad := range []string{"", "-pp", "2pp", "p p", "pp;", "p/p", "p$p"} {
		if err := ValidateCmd(bad); err == nil {
			t.Errorf("ValidateCmd(%q) should fail", bad)
		}
	}
}

func TestCmdName(t *testing.T) {
	t.Setenv(CmdEnv, "")
	if got := CmdName(); got != DefaultCmd {
		t.Errorf("unset: got %q, want %q", got, DefaultCmd)
	}
	t.Setenv(CmdEnv, "pp")
	if got := CmdName(); got != "pp" {
		t.Errorf("set: got %q, want pp", got)
	}
	t.Setenv(CmdEnv, "bad name")
	if got := CmdName(); got != DefaultCmd {
		t.Errorf("invalid: got %q, want the default", got)
	}
}

func TestWriteCDTarget(t *testing.T) {
	t.Run("with wrapper", func(t *testing.T) {
		f := filepath.Join(t.TempDir(), "cd")
		t.Setenv(CDFileEnv, f)
		var out bytes.Buffer
		if err := WriteCDTarget(&out, "/some/dir"); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != "/some/dir" {
			t.Errorf("cd file = %q, want %q", got, "/some/dir")
		}
		if out.Len() != 0 {
			t.Errorf("printed %q, want nothing", out.String())
		}
	})
	t.Run("without wrapper", func(t *testing.T) {
		t.Setenv(CDFileEnv, "")
		var out bytes.Buffer
		if err := WriteCDTarget(&out, "/some/dir"); err != nil {
			t.Fatal(err)
		}
		if out.String() != "/some/dir\n" {
			t.Errorf("printed %q, want %q", out.String(), "/some/dir\n")
		}
	})
}

// fakePathpin stands in for the real binary: it echoes its args, exits 3 on
// "fail", and otherwise writes $TARGET to the cd file.
const fakePathpin = `#!/bin/sh
echo "out:$*"
echo "cmd:$PATHPIN_CMD"
if [ "$1" = "fail" ]; then exit 3; fi
printf '%s' "$TARGET" > "$PATHPIN_CD_FILE"
`

// TestWrapper runs the real wrapper in each installed shell against fakePathpin.
func TestWrapper(t *testing.T) {
	shells := map[string][]string{
		"bash": {"--noprofile", "--norc", "-c"},
		"zsh":  {"-f", "-c"},
		"fish": {"--no-config", "-c"},
	}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "pathpin"), []byte(fakePathpin), 0o755); err != nil {
		t.Fatal(err)
	}
	target, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	for _, sh := range Shells() {
		t.Run(sh, func(t *testing.T) {
			if _, err := exec.LookPath(sh); err != nil {
				t.Skipf("%s not installed", sh)
			}
			script, _ := Script(sh, DefaultCmd)
			file := filepath.Join(t.TempDir(), "init")
			if err := os.WriteFile(file, []byte(script), 0o644); err != nil {
				t.Fatal(err)
			}

			run := func(body string) string {
				t.Helper()
				cmd := exec.Command(sh, append(shells[sh], "source "+file+"; "+body)...)
				cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"), "TARGET="+target)
				out, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("%s: %v\n%s", sh, err, out)
				}
				return string(out)
			}

			out := run(`pathpin hello; echo "pwd:$PWD"`)
			if !strings.Contains(out, "out:hello") {
				t.Errorf("binary stdout not passed through:\n%s", out)
			}
			if !strings.Contains(out, "pwd:"+target) {
				t.Errorf("did not cd to %s:\n%s", target, out)
			}

			status := "$?"
			if sh == "fish" {
				status = "$status"
			}
			out = run(`pathpin fail; echo "status:` + status + `"; echo "pwd:$PWD"`)
			if !strings.Contains(out, "status:3") {
				t.Errorf("exit status not propagated:\n%s", out)
			}
			if strings.Contains(out, "pwd:"+target) {
				t.Errorf("cd'd even though the binary wrote nothing:\n%s", out)
			}

			// --cmd pp: the function is pp, it still runs pathpin, and it
			// tells the binary its name.
			script, _ = Script(sh, "pp")
			if err := os.WriteFile(file, []byte(script), 0o644); err != nil {
				t.Fatal(err)
			}
			out = run(`pp hello; echo "pwd:$PWD"`)
			if !strings.Contains(out, "out:hello") || !strings.Contains(out, "cmd:pp") || !strings.Contains(out, "pwd:"+target) {
				t.Errorf("pp should run pathpin with %s=pp and cd:\n%s", CmdEnv, out)
			}
		})
	}
}

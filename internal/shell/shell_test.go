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
		s, err := Script(sh)
		if err != nil {
			t.Fatalf("Script(%q): %v", sh, err)
		}
		if !strings.Contains(s, CDFileEnv) || !strings.Contains(s, "command jug") {
			t.Errorf("Script(%q) does not use %s and `command jug`", sh, CDFileEnv)
		}
	}
	if _, err := Script("powershell"); err == nil {
		t.Error("Script(powershell) should fail")
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

// fakeJug stands in for the real binary: it echoes its args, exits 3 on
// "fail", and otherwise writes $TARGET to the cd file.
const fakeJug = `#!/bin/sh
echo "out:$*"
if [ "$1" = "fail" ]; then exit 3; fi
printf '%s' "$TARGET" > "$JUG_CD_FILE"
`

// TestWrapper runs the real wrapper in each installed shell against fakeJug.
func TestWrapper(t *testing.T) {
	shells := map[string][]string{
		"bash": {"--noprofile", "--norc", "-c"},
		"zsh":  {"-f", "-c"},
		"fish": {"--no-config", "-c"},
	}
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "jug"), []byte(fakeJug), 0o755); err != nil {
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
			script, _ := Script(sh)
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

			out := run(`jug hello; echo "pwd:$PWD"`)
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
			out = run(`jug fail; echo "status:` + status + `"; echo "pwd:$PWD"`)
			if !strings.Contains(out, "status:3") {
				t.Errorf("exit status not propagated:\n%s", out)
			}
			if strings.Contains(out, "pwd:"+target) {
				t.Errorf("cd'd even though the binary wrote nothing:\n%s", out)
			}
		})
	}
}

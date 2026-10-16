package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/turtlethom/pathpin/internal/shell"
)

// newInitCmd builds `pathpin init <shell> [--cmd name]`, which prints the
// shell function from internal/shell plus Cobra's completions for that shell.
// The user evals the output from their shell config.
func newInitCmd() *cobra.Command {
	var name string
	c := &cobra.Command{
		Use:   "init <bash|zsh|fish>",
		Short: "Print shell integration (lets pathpin change your directory)",
		Long: `Print the shell function and completions that let pathpin change your
shell's directory. Add one line to your shell config:

  bash   ~/.bashrc                    eval "$(pathpin init bash)"
  zsh    ~/.zshrc                     eval "$(pathpin init zsh)"
  fish   ~/.config/fish/config.fish   pathpin init fish | source

The command is called pathpin unless you pick a name with --cmd, for example
eval "$(pathpin init bash --cmd pp)". The function shadows any command with
the same name in interactive shells.`,
		Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		ValidArgs: shell.Shells(),
		RunE: func(cmd *cobra.Command, args []string) error {
			script, err := shell.Script(args[0], name)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if _, err := fmt.Fprint(out, script); err != nil {
				return err
			}

			// Completions register for the name the user will type. Cobra
			// takes that name from the root's Use, so change it before
			// generating them.
			root := cmd.Root()
			root.Use = name + " [label]"
			switch args[0] {
			case "bash":
				// The wrapper works in zsh too, but bash completions break on
				// every Tab there, so skip them and say which line to use.
				fmt.Fprintln(out, `if [ -n "${ZSH_VERSION-}" ]; then`)
				// %%s prints a literal %s for printf; %s is the hint.
				fmt.Fprintf(out, "    printf '%%s\\n' '%s' >&2\n", wrongShellHint("bash", "zsh", name))
				fmt.Fprintln(out, "else")
				if err := root.GenBashCompletionV2(out, true); err != nil {
					return err
				}
				_, err := fmt.Fprintln(out, "fi")
				return err
			case "zsh":
				// Likewise in bash. compdef only exists after compinit; skip
				// completions rather than print an error on every shell start.
				fmt.Fprintln(out, `if [ -n "${BASH_VERSION-}" ]; then`)
				fmt.Fprintf(out, "    printf '%%s\\n' '%s' >&2\n", wrongShellHint("zsh", "bash", name))
				fmt.Fprintln(out, "elif (( $+functions[compdef] )); then")
				if err := root.GenZshCompletion(out); err != nil {
					return err
				}
				_, err := fmt.Fprintln(out, "fi")
				return err
			case "fish":
				return root.GenFishCompletion(out, true)
			}
			return nil
		},
	}
	c.Flags().StringVar(&name, "cmd", shell.DefaultCmd, "name of the shell command to define")
	return c
}

// wrongShellHint is the line the integration for shell `from` prints when
// shell `to` loads it, giving the setup line to use there. It goes inside
// single quotes, which is safe because shell.ValidateCmd already checked name.
func wrongShellHint(from, to, name string) string {
	setup := "pathpin init " + to
	if name != shell.DefaultCmd {
		setup += " --cmd " + name
	}
	return fmt.Sprintf(`pathpin: this is the %s integration; in %s use: eval "$(%s)"`, from, to, setup)
}

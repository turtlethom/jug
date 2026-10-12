package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/turtlethom/pathpin/internal/shell"
)

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

			// Completions register for the name the user will type.
			root := cmd.Root()
			root.Use = name + " [label]"
			switch args[0] {
			case "bash":
				return root.GenBashCompletionV2(out, true)
			case "zsh":
				// compdef only exists after compinit; skip completions rather
				// than print an error on every shell start.
				fmt.Fprintln(out, "if (( $+functions[compdef] )); then")
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

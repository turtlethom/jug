package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/turtlethom/jug/internal/config"
)

func newAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add [label]",
		Short: "Bookmark the current directory",
		Long: `Bookmark the current directory. Without a label, jug uses the folder's
name in uppercase (~/.config/nvim becomes NVIM).`,
		Args: cobra.ArbitraryArgs, // words of a label, joined with spaces
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, path, err := loadConfig(cmd)
			if err != nil {
				return err
			}
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			label := strings.Join(args, " ")
			if strings.TrimSpace(label) == "" {
				label = config.SuggestLabel(cwd)
			}
			if err := cfg.Add(label, cwd); err != nil {
				return err
			}
			if err := config.Save(path, cfg); err != nil {
				return err
			}

			b := cfg.Bookmarks[len(cfg.Bookmarks)-1]
			errOut := cmd.ErrOrStderr()
			fmt.Fprintf(errOut, "jug: added %s → %s\n", b.Label, config.ContractHome(b.Path))
			if name, ok := commandNamed(cmd.Root(), b.Label); ok {
				fmt.Fprintf(errOut, "jug: note: %q is also a command, so 'jug %s' won't jump here.\n"+
					"     Use the menu, or type it in another case: 'jug %s'.\n",
					b.Label, name, otherCase(b.Label))
			}
			return nil
		},
	}
}

// commandNamed reports whether label matches a subcommand name or alias in
// any case. Names come from Cobra, so new commands are covered automatically.
func commandNamed(root *cobra.Command, label string) (string, bool) {
	for _, c := range root.Commands() {
		for _, name := range append([]string{c.Name()}, c.Aliases...) {
			if strings.EqualFold(name, label) {
				return name, true
			}
		}
	}
	return "", false
}

// otherCase returns label in a case Cobra won't match against a lowercase
// command name.
func otherCase(label string) string {
	if up := strings.ToUpper(label); up != label {
		return up
	}
	return strings.ToLower(label[:1]) + label[1:]
}

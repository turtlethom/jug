// Package cmd defines jug's command-line interface.
package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/turtlethom/jug/internal/config"
	"github.com/turtlethom/jug/internal/shell"
	"github.com/turtlethom/jug/internal/ui"
)

// menuFunc shows the menu and returns the chosen directory, or "" on quit.
// Tests pass a fake, since the real menu needs a terminal.
type menuFunc func(cfg *config.Config, configPath string) (string, error)

// NewRootCmd builds the jug command tree.
func NewRootCmd() *cobra.Command {
	return newRootCmd(ui.Run)
}

func newRootCmd(menu menuFunc) *cobra.Command {
	root := &cobra.Command{
		Use:   "jug",
		Short: "jug — grab your directories",
		Long: `jug is a bookmark-first directory navigator.

Run 'jug' to open the menu, or 'jug <label>' to jump straight to a bookmark.
Set up your shell first: see 'jug init --help'.`,
		SilenceUsage:  true,
		SilenceErrors: true, // Execute prints them with a "jug:" prefix
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := config.DefaultPath()
			if err != nil {
				return err
			}
			legacy, err := config.LegacyPath()
			if err != nil {
				return err
			}
			cfg, migrated, err := config.LoadOrMigrate(path, legacy)
			if err != nil {
				return err
			}
			// Printed before the alternate screen opens, so it shows once the menu closes.
			if migrated {
				fmt.Fprintf(cmd.ErrOrStderr(), "jug: imported %d bookmarks from %s (left in place)\n",
					len(cfg.Bookmarks), config.ContractHome(legacy))
			}

			dir, err := menu(cfg, path)
			if err != nil || dir == "" {
				return err
			}
			out := cmd.OutOrStdout()
			if err := shell.WriteCDTarget(out, dir); err != nil {
				return err
			}
			// Without the wrapper, the path was just printed and nothing seems to
			// happen. Skip the hint for `cd "$(jug)"`, where stdout is captured.
			if os.Getenv(shell.CDFileEnv) == "" && isTerminal(out) {
				fmt.Fprint(cmd.ErrOrStderr(), integrationHint())
			}
			return nil
		},
	}
	// `jug init <shell>` prints completions along with the wrapper, so the
	// default completion command is only needed for debugging.
	root.CompletionOptions.HiddenDefaultCmd = true

	root.AddCommand(newInitCmd())
	return root
}

// Execute runs jug and prints any error as "jug: <err>".
func Execute() {
	if err := NewRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "jug:", err)
		os.Exit(1)
	}
}

func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// integrationHint tells the user how to set up the wrapper for their shell.
func integrationHint() string {
	const intro = "jug: shell integration isn't set up, so jug can't change your directory.\n"
	switch filepath.Base(os.Getenv("SHELL")) {
	case "bash":
		return intro + "     Add this to ~/.bashrc:  eval \"$(jug init bash)\"\n"
	case "zsh":
		return intro + "     Add this to ~/.zshrc:  eval \"$(jug init zsh)\"\n"
	case "fish":
		return intro + "     Add this to ~/.config/fish/config.fish:  jug init fish | source\n"
	}
	return intro + "     See 'jug init --help'.\n"
}

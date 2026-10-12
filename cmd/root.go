// Package cmd defines pathpin's command-line interface.
package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/turtlethom/pathpin/internal/config"
	"github.com/turtlethom/pathpin/internal/shell"
	"github.com/turtlethom/pathpin/internal/ui"
)

// menuFunc shows the menu and returns the chosen directory, or "" on quit.
// Tests pass a fake, since the real menu needs a terminal.
type menuFunc func(cfg *config.Config, configPath string) (string, error)

// NewRootCmd builds the pathpin command tree.
func NewRootCmd() *cobra.Command {
	return newRootCmd(ui.Run)
}

func newRootCmd(menu menuFunc) *cobra.Command {
	root := &cobra.Command{
		Use:   "pathpin [label]",
		Short: "pathpin — pin your paths",
		Long: `pathpin is a bookmark-first directory navigator.

Run 'pathpin' to open the menu, or 'pathpin <label>' to jump straight to a bookmark.
Set up your shell first: see 'pathpin init --help'.`,
		Args:          cobra.ArbitraryArgs, // words of a label, joined with spaces
		SilenceUsage:  true,
		SilenceErrors: true, // Execute prints them with a "pathpin:" prefix
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, path, err := loadConfig(cmd)
			if err != nil {
				return err
			}
			if len(args) > 0 {
				return jump(cmd, cfg, strings.Join(args, " "))
			}
			dir, err := menu(cfg, path)
			if err != nil || dir == "" {
				return err
			}
			return handoff(cmd, dir)
		},
		ValidArgsFunction: completeLabels,
	}
	// `pathpin init <shell>` prints completions along with the wrapper, so the
	// default completion command is only needed for debugging.
	root.CompletionOptions.HiddenDefaultCmd = true

	root.AddCommand(newInitCmd(), newAddCmd(), newLsCmd())
	return root
}

// Execute runs pathpin and prints any error as "pathpin: <err>".
func Execute() {
	if err := NewRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "pathpin:", err)
		os.Exit(1)
	}
}

// loadConfig loads pathpin's config, migrating fcd's on the first run of any
// command, and returns it with the path to save it to.
func loadConfig(cmd *cobra.Command) (*config.Config, string, error) {
	path, err := config.DefaultPath()
	if err != nil {
		return nil, "", err
	}
	legacy, err := config.LegacyPath()
	if err != nil {
		return nil, "", err
	}
	cfg, migrated, err := config.LoadOrMigrate(path, legacy)
	if err != nil {
		return nil, "", err
	}
	// Printed before the menu's alternate screen opens, so it shows once the menu closes.
	if migrated {
		fmt.Fprintf(cmd.ErrOrStderr(), "pathpin: imported %d bookmarks from %s (left in place)\n",
			len(cfg.Bookmarks), config.ContractHome(legacy))
	}
	return cfg, path, nil
}

// jump cds to the bookmark with this label, without opening the menu.
func jump(cmd *cobra.Command, cfg *config.Config, label string) error {
	b, ok := cfg.Find(label)
	if !ok {
		return fmt.Errorf("no bookmark %q (see 'pathpin ls')", label)
	}
	if !isDir(b.Path) {
		return fmt.Errorf("%s no longer exists", config.ContractHome(b.Path))
	}
	return handoff(cmd, b.Path)
}

// handoff gives dir to the shell wrapper to cd into.
func handoff(cmd *cobra.Command, dir string) error {
	out := cmd.OutOrStdout()
	if err := shell.WriteCDTarget(out, dir); err != nil {
		return err
	}
	// Without the wrapper, the path was just printed and nothing seems to
	// happen. Skip the hint for `cd "$(pathpin)"`, where stdout is captured.
	if os.Getenv(shell.CDFileEnv) == "" && isTerminal(out) {
		fmt.Fprint(cmd.ErrOrStderr(), integrationHint())
	}
	return nil
}

// completeLabels completes `pathpin <TAB>` with bookmark labels. It only reads the
// config: migrating (and printing about it) mid-completion would be surprising.
func completeLabels(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	path, err := config.DefaultPath()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	cfg, err := config.Load(path)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var labels []string
	for _, b := range cfg.Bookmarks {
		labels = append(labels, b.Label)
	}
	return labels, cobra.ShellCompDirectiveNoFileComp
}

func isDir(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
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
	const intro = "pathpin: shell integration isn't set up, so pathpin can't change your directory.\n"
	switch filepath.Base(os.Getenv("SHELL")) {
	case "bash":
		return intro + "     Add this to ~/.bashrc:  eval \"$(pathpin init bash)\"\n"
	case "zsh":
		return intro + "     Add this to ~/.zshrc:  eval \"$(pathpin init zsh)\"\n"
	case "fish":
		return intro + "     Add this to ~/.config/fish/config.fish:  pathpin init fish | source\n"
	}
	return intro + "     See 'pathpin init --help'.\n"
}

// Package cmd defines jug's command-line interface.
package cmd

import (
	"errors"
	"os"

	"github.com/spf13/cobra"
)

// NewRootCmd builds the jug command tree.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "jug",
		Short: "jug — grab your directories",
		Long: `jug is a bookmark-first directory navigator.

Run 'jug' to open the menu, or 'jug <label>' to jump straight to a bookmark.
Set up your shell first: see 'jug init --help'.`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// TODO(you, week 2): load the config with config.LoadOrMigrate and
			// run the menu from internal/ui. Hand the chosen directory to the
			// shell with shell.WriteCDTarget.
			return errors.New("the menu arrives in week 2")
		},
	}
	// `jug init <shell>` prints completions along with the wrapper, so the
	// default completion command is only needed for debugging.
	root.CompletionOptions.HiddenDefaultCmd = true

	root.AddCommand(newInitCmd())
	return root
}

// Execute runs jug. Cobra has already printed any error.
func Execute() {
	if err := NewRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

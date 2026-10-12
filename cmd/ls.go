package cmd

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"github.com/turtlethom/pathpin/internal/config"
)

func newLsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "List bookmarks",
		Long: `List bookmarks. In a terminal, paths show ~ and missing folders are marked.
Piped, each line is LABEL<tab>/absolute/path, so 'pathpin ls | cut -f2' gives paths.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _, err := loadConfig(cmd)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if !isTerminal(out) {
				for _, b := range cfg.Bookmarks {
					fmt.Fprintf(out, "%s\t%s\n", b.Label, b.Path)
				}
				return nil
			}
			if len(cfg.Bookmarks) == 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), "No bookmarks yet — run 'pathpin add' in a folder")
				return nil
			}
			_, err = fmt.Fprint(out, formatList(cfg.Bookmarks))
			return err
		},
	}
}

// formatList lays bookmarks out for a person: aligned labels, ~ paths, and a
// marker on folders that no longer exist.
func formatList(bookmarks []config.Bookmark) string {
	width := 0
	for _, b := range bookmarks {
		width = max(width, utf8.RuneCountInString(b.Label))
	}
	var sb strings.Builder
	for _, b := range bookmarks {
		pad := strings.Repeat(" ", width-utf8.RuneCountInString(b.Label))
		fmt.Fprintf(&sb, "%s%s  %s", b.Label, pad, config.ContractHome(b.Path))
		if !isDir(b.Path) {
			sb.WriteString("  (missing)")
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

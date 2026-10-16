package cmd

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"github.com/turtlethom/pathpin/internal/config"
)

// newLsCmd builds `ls`; name is what the user types, for the help text.
func newLsCmd(name string) *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: "List bookmarks",
		Long: fmt.Sprintf(`List bookmarks. In a terminal, they're listed under their groups, paths show ~,
and missing folders are marked. Piped, each line is LABEL<tab>/absolute/path<tab>GROUP
(the group is empty if there's none), so '%s ls | cut -f2' gives paths, and a
script can read all three with: while IFS=$'\t' read -r label path group`, name),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, _, err := loadConfig(cmd)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if !isTerminal(out) {
				for _, b := range cfg.Bookmarks {
					fmt.Fprintf(out, "%s\t%s\t%s\n", b.Label, b.Path, cfg.GroupOf(b))
				}
				return nil
			}
			if len(cfg.Bookmarks) == 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "No bookmarks yet — run '%s add' in a folder\n", cmd.Root().Name())
				return nil
			}
			_, err = fmt.Fprint(out, formatList(cfg.Bookmarks))
			return err
		},
	}
}

// formatList lays bookmarks out for a person: aligned labels, ~ paths, and a
// marker on folders that no longer exist. With groups, rows are indented under
// a heading per group, in the menu's tab order (groups, then Other), with
// labels aligned across all of them.
func formatList(bookmarks []config.Bookmark) string {
	width := 0
	for _, b := range bookmarks {
		width = max(width, utf8.RuneCountInString(b.Label))
	}
	var sb strings.Builder
	// row and section are closures: small functions that write into sb and
	// share width, so the code below reads as "a row", "a section".
	row := func(indent string, b config.Bookmark) {
		pad := strings.Repeat(" ", width-utf8.RuneCountInString(b.Label))
		fmt.Fprintf(&sb, "%s%s%s  %s", indent, b.Label, pad, config.ContractHome(b.Path))
		if !isDir(b.Path) {
			sb.WriteString("  (missing)")
		}
		sb.WriteString("\n")
	}

	// A Config just to reuse the group rules the menu uses (Groups, GroupOf).
	cfg := &config.Config{Bookmarks: bookmarks}
	groups := cfg.Groups()
	if len(groups) == 0 {
		for _, b := range bookmarks {
			row("", b)
		}
		return sb.String()
	}
	section := func(heading string, in func(config.Bookmark) bool) {
		var rows []config.Bookmark
		for _, b := range bookmarks {
			if in(b) {
				rows = append(rows, b)
			}
		}
		if len(rows) == 0 {
			return
		}
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(heading + "\n")
		for _, b := range rows {
			row("  ", b)
		}
	}
	for _, g := range groups {
		section(g, func(b config.Bookmark) bool { return cfg.GroupOf(b) == g })
	}
	// config.Bookmark.Ungrouped is a method used as a plain function:
	// func(b config.Bookmark) bool, the same type section expects.
	section(config.TabOther, config.Bookmark.Ungrouped)
	return sb.String()
}

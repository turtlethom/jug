package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/turtlethom/pathpin/internal/config"
)

func newAddCmd() *cobra.Command {
	var group string
	c := &cobra.Command{
		Use:   "add [label]",
		Short: "Bookmark the current directory",
		Long: `Bookmark the current directory. Without a label, the folder's name is
used in uppercase (~/.config/nvim becomes NVIM). --group puts it in a group;
a group that exists in another case is joined with its spelling.`,
		Args:              cobra.ArbitraryArgs,     // words of a label, joined with spaces
		ValidArgsFunction: cobra.NoFileCompletions, // a new label: nothing to complete
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
			// Both change only cfg, so a refused group saves nothing.
			if group != "" {
				if err := cfg.SetGroup(label, group); err != nil {
					return err
				}
			}
			if err := config.Save(path, cfg); err != nil {
				return err
			}

			b := cfg.Bookmarks[len(cfg.Bookmarks)-1]
			errOut := cmd.ErrOrStderr()
			me := cmd.Root().Name()
			inGroup := ""
			if b.Group != "" {
				inGroup = " (" + b.Group + ")"
			}
			fmt.Fprintf(errOut, "%s: added %s → %s%s\n", me, b.Label, config.ContractHome(b.Path), inGroup)
			if sub, ok := commandNamed(cmd.Root(), b.Label); ok {
				fmt.Fprintf(errOut, "%[1]s: note: %[2]q is also a command, so '%[1]s %[3]s' won't jump here.\n"+
					"     Use the menu, or type it in another case: '%[1]s %[4]s'.\n",
					me, b.Label, sub, otherCase(b.Label))
			}
			return nil
		},
	}
	c.Flags().StringVarP(&group, "group", "g", "", "put the bookmark in this group")
	_ = c.RegisterFlagCompletionFunc("group", completeGroups) // only fails for an unknown flag
	return c
}

// completeGroups completes --group with the existing group names. It keeps
// what was typed and completes the rest ("sc" gives "school"), because shells
// drop candidates whose case doesn't match, and groups match in any case.
// Like completeLabels, it only reads the config.
func completeGroups(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	path, err := config.DefaultPath()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	cfg, err := config.Load(path)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	typed := []rune(toComplete)
	var names []string
	for _, g := range cfg.Groups() {
		if name := []rune(g); len(name) >= len(typed) && strings.EqualFold(string(name[:len(typed)]), toComplete) {
			names = append(names, toComplete+string(name[len(typed):]))
		}
	}
	return names, cobra.ShellCompDirectiveNoFileComp
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

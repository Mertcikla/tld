package rename

import (
	"fmt"

	"github.com/mertcikla/tld/v2/internal/cmdutil"
	"github.com/mertcikla/tld/v2/internal/completion"
	"github.com/mertcikla/tld/v2/internal/term"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/spf13/cobra"
)

func NewRenameCmd(wdir *string) *cobra.Command {
	var from string
	var to string
	var dryRun bool

	c := &cobra.Command{
		Use:   "rename",
		Short: "Rename an element in elements.yaml",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if from == "" || to == "" {
				return fmt.Errorf("--from and --to are required")
			}
			if dryRun {
				if err := cmdutil.WithWorkspaceDryRun(*wdir, func(cloneDir string) error {
					return workspace.RenameElement(cloneDir, from, to)
				}); err != nil {
					if cmdutil.WantsJSONFromCmd(cmd) {
						return cmdutil.WriteCommandError(cmd.OutOrStdout(), cmdutil.CompactFromCmd(cmd), "rename", err)
					}
					return fmt.Errorf("dry-run rename element: %w", err)
				}
				if cmdutil.WantsJSONFromCmd(cmd) {
					return cmdutil.WriteMutation(cmd.OutOrStdout(), cmdutil.CompactFromCmd(cmd), "rename", "dry-run", fmt.Sprintf("%s -> %s", from, to))
				}
				term.Successf(cmd.OutOrStdout(), "dry-run: renamed %s → %s", from, to)
				return nil
			}
			err := workspace.RenameElement(*wdir, from, to)
			if err != nil {
				if cmdutil.WantsJSONFromCmd(cmd) {
					return cmdutil.WriteCommandError(cmd.OutOrStdout(), cmdutil.CompactFromCmd(cmd), "rename", err)
				}
				return fmt.Errorf("rename element: %w", err)
			}
			if cmdutil.WantsJSONFromCmd(cmd) {
				return cmdutil.WriteMutation(cmd.OutOrStdout(), cmdutil.CompactFromCmd(cmd), "rename", "rename", fmt.Sprintf("%s -> %s", from, to))
			}
			term.Successf(cmd.OutOrStdout(), "renamed %s → %s", from, to)
			return nil
		},
	}

	c.Flags().StringVar(&from, "from", "", "current element ref (required)")
	c.Flags().StringVar(&to, "to", "", "new element ref (required)")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "preview the change without writing files")
	_ = c.MarkFlagRequired("from")
	_ = c.MarkFlagRequired("to")

	_ = c.RegisterFlagCompletionFunc("from", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return completion.ElementRefs(wdir)
	})
	return c
}

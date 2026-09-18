package rename

import (
	"fmt"

	"github.com/mertcikla/tld/v2/cmd/crudsync"
	"github.com/mertcikla/tld/v2/internal/cmdutil"
	"github.com/mertcikla/tld/v2/internal/completion"
	"github.com/mertcikla/tld/v2/internal/term"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/spf13/cobra"
)

func NewRenameCmd(wdir *string, format *string, compact *bool) *cobra.Command {
	var from string
	var to string
	var yamlOnly bool
	var target string
	var dataDir string

	c := &cobra.Command{
		Use:   "rename",
		Short: "Rename an element (applies instantly)",
		Long: `Rename an element and apply instantly.

Use --yaml-only to stage the YAML change without applying.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if from == "" || to == "" {
				return fmt.Errorf("--from and --to are required")
			}
			formatVal := ""
			compactVal := false
			if format != nil {
				formatVal = *format
			} else if cmd.Root() != nil {
				if f := cmd.Root().PersistentFlags().Lookup("format"); f != nil {
					formatVal = f.Value.String()
				}
			}
			if compact != nil {
				compactVal = *compact
			} else if cmd.Root() != nil {
				if f := cmd.Root().PersistentFlags().Lookup("compact"); f != nil {
					compactVal = f.Value.String() == "true"
				}
			}
			err := workspace.RenameElement(*wdir, from, to)
			if err != nil {
				if cmdutil.WantsJSON(formatVal) {
					return cmdutil.WriteCommandError(cmd.OutOrStdout(), compactVal, "rename", err)
				}
				return fmt.Errorf("rename element: %w", err)
			}
			if yamlOnly {
				if cmdutil.WantsJSON(formatVal) {
					return cmdutil.WriteMutation(cmd.OutOrStdout(), compactVal, "rename", "rename", fmt.Sprintf("%s -> %s", from, to))
				}
				term.Successf(cmd.OutOrStdout(), "renamed %s → %s", from, to)
				term.Info(cmd.OutOrStdout(), "YAML only (--yaml-only): not applied.")
				return nil
			}
			if !cmdutil.WantsJSON(formatVal) {
				term.Successf(cmd.OutOrStdout(), "renamed %s → %s", from, to)
			}
			_, syncErr := crudsync.SyncAndReport(cmd, *wdir, crudsync.Options{
				Target:  target,
				DataDir: dataDir,
				Command: "rename",
				Format:  formatVal,
				Compact: compactVal,
			}, cmd.OutOrStdout())
			return syncErr
		},
	}

	c.Flags().StringVar(&from, "from", "", "current element ref (required)")
	c.Flags().StringVar(&to, "to", "", "new element ref (required)")
	c.Flags().BoolVar(&yamlOnly, "yaml-only", false, "write YAML only without applying")
	c.Flags().StringVar(&target, "target", "", "apply target: auto, local, or remote")
	c.Flags().StringVar(&dataDir, "data-dir", "", "data directory for local target state")
	_ = c.MarkFlagRequired("from")
	_ = c.MarkFlagRequired("to")

	_ = c.RegisterFlagCompletionFunc("from", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return completion.ElementRefs(wdir)
	})
	return c
}

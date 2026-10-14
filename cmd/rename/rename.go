package rename

import (
	"fmt"

	"github.com/mertcikla/tld/v2/internal/cmdutil"
	"github.com/mertcikla/tld/v2/internal/completion"
	"github.com/mertcikla/tld/v2/internal/term"
	"github.com/spf13/cobra"
)

func NewRenameCmd(wdir *string) *cobra.Command {
	var from string
	var to string
	var dryRun bool

	c := &cobra.Command{
		Use:    "rename",
		Short:  "Deprecated alias for updating a workspace-local element ref",
		Hidden: true,
		Long: `Deprecated: use 'tld update element <ref> ref <new-ref>'.

Rename an element ref in a YAML workspace and update its references.

Requires a YAML workspace (run 'tld init' first, or pass --yaml).
This changes the workspace-local ref, not the element's display name.

To change a display name in either mode, use:
  tld update element <ref-or-id> name <new-name>`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Cobra's automatic deprecation notice follows the command's output
			// writer. Use stderr explicitly to keep JSON stdout parseable.
			cmd.PrintErrln("Command \"rename\" is deprecated; use 'tld update element <ref> ref <new-ref>'.")
			if from == "" || to == "" {
				return fmt.Errorf("--from and --to are required")
			}
			return runWorkspaceRename(cmd, *wdir, from, to, dryRun)
		},
	}

	c.Flags().StringVar(&from, "from", "", "current workspace-local element ref (required)")
	c.Flags().StringVar(&to, "to", "", "new workspace-local element ref (required)")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "preview the change without writing")
	_ = c.MarkFlagRequired("from")
	_ = c.MarkFlagRequired("to")

	_ = c.RegisterFlagCompletionFunc("from", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return completion.ElementRefs(wdir)
	})
	return c
}

func fail(cmd *cobra.Command, err error) error {
	if cmdutil.WantsJSONFromCmd(cmd) {
		return cmdutil.WriteCommandError(cmd.OutOrStdout(), cmdutil.CompactFromCmd(cmd), "rename", err)
	}
	return err
}

func runWorkspaceRename(cmd *cobra.Command, wdir, from, to string, dryRun bool) error {
	if err := cmdutil.RenameWorkspaceElement(cmd, wdir, from, to, dryRun); err != nil {
		return fail(cmd, err)
	}
	if dryRun {
		if cmdutil.WantsJSONFromCmd(cmd) {
			return cmdutil.WriteMutation(cmd.OutOrStdout(), cmdutil.CompactFromCmd(cmd), "rename", "dry-run", fmt.Sprintf("%s -> %s", from, to))
		}
		term.Successf(cmd.OutOrStdout(), "dry-run: renamed %s → %s", from, to)
		return nil
	}
	if cmdutil.WantsJSONFromCmd(cmd) {
		return cmdutil.WriteMutation(cmd.OutOrStdout(), cmdutil.CompactFromCmd(cmd), "rename", "rename", fmt.Sprintf("%s -> %s", from, to))
	}
	term.Successf(cmd.OutOrStdout(), "renamed %s → %s", from, to)
	return nil
}

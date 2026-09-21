package remove

import (
	"fmt"

	"github.com/mertcikla/tld/v2/internal/cmdutil"
	"github.com/mertcikla/tld/v2/internal/completion"
	"github.com/mertcikla/tld/v2/internal/exec"
	"github.com/mertcikla/tld/v2/internal/term"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/spf13/cobra"
)

func NewRemoveCmd(wdir, format *string, compact *bool) *cobra.Command {
	c := &cobra.Command{
		Use:   "remove",
		Short: "Remove workspace resources",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			return cobra.NoArgs(cmd, args)
		},
	}
	c.AddCommand(newElementCmd(wdir, format, compact))
	c.AddCommand(newConnectorCmd(wdir, format, compact))
	return c
}

func newElementCmd(wdir, format *string, compact *bool) *cobra.Command {
	var dryRun bool
	var target string
	var dataDir string
	c := &cobra.Command{
		Use:   "element <ref>",
		Short: "Remove an element from elements.yaml",
		Args:  cobra.ExactArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) != 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return completion.ElementRefs(wdir)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			ref := args[0]
			if dryRun {
				err := cmdutil.WithWorkspaceDryRun(*wdir, func(cloneDir string) error {
					return workspace.RemoveElement(cloneDir, ref)
				})
				if err != nil {
					if cmdutil.WantsJSON(*format) {
						return cmdutil.WriteCommandError(cmd.OutOrStdout(), *compact, "remove element", err)
					}
					return fmt.Errorf("dry-run remove element: %w", err)
				}
				if cmdutil.WantsJSON(*format) {
					return cmdutil.WriteMutation(cmd.OutOrStdout(), *compact, "remove element", "dry-run", ref)
				}
				term.Successf(cmd.OutOrStdout(), "dry-run: del: %s", ref)
				return nil
			}
			// Validate the removal and capture server IDs before mutating
			// anything. The server delete runs first: a failed YAML write can
			// be retried, but dropped cache metadata could not (the server
			// copy would be orphaned).
			preWS, err := cmdutil.LoadWorkspace(*wdir)
			if err != nil {
				if cmdutil.WantsJSON(*format) {
					return cmdutil.WriteCommandError(cmd.OutOrStdout(), *compact, "remove element", err)
				}
				return err
			}
			if err := workspace.CheckElementRemoval(preWS, ref); err != nil {
				if cmdutil.WantsJSON(*format) {
					return cmdutil.WriteCommandError(cmd.OutOrStdout(), *compact, "remove element", err)
				}
				return err
			}
			var elementID, viewID int32
			if preWS.Meta != nil {
				if m, ok := preWS.Meta.Elements[ref]; ok && m != nil {
					elementID = int32(m.ID)
				}
				if m, ok := preWS.Meta.Views[ref]; ok && m != nil {
					viewID = int32(m.ID)
				}
			}
			if err := runRemoveElementServer(cmd, preWS, target, dataDir, elementID, viewID); err != nil {
				if cmdutil.WantsJSON(*format) {
					return cmdutil.WriteCommandError(cmd.OutOrStdout(), *compact, "remove element", err)
				}
				return err
			}
			if err := workspace.RemoveElement(*wdir, ref); err != nil {
				if cmdutil.WantsJSON(*format) {
					return cmdutil.WriteCommandError(cmd.OutOrStdout(), *compact, "remove element", err)
				}
				return fmt.Errorf("remove element: %w", err)
			}
			if cmdutil.WantsJSON(*format) {
				return cmdutil.WriteMutation(cmd.OutOrStdout(), *compact, "remove element", "remove", ref)
			}
			term.Successf(cmd.OutOrStdout(), "del: %s", ref)
			return nil
		},
	}
	c.Flags().BoolVar(&dryRun, "dry-run", false, "preview the change without writing files")
	c.Flags().StringVar(&target, "target", "", "sync target: auto, local, remote, or cloud")
	c.Flags().StringVar(&dataDir, "data-dir", "", "data directory for local target state")
	return c
}

// runRemoveElementServer deletes the element (and its view, if any) on the
// server synchronously, before the YAML cache entry is removed. A NotFound is
// tolerated so a retry after a partial failure can converge.
func runRemoveElementServer(cmd *cobra.Command, ws *workspace.Workspace, target, dataDir string, elementID, viewID int32) error {
	if elementID == 0 {
		// No cached ID (e.g. hand-written YAML): nothing to delete server-side.
		return nil
	}
	runner, err := exec.NewRunner(ws.Config, target, dataDir, false)
	if err != nil {
		return err
	}
	defer func() { _ = runner.Close() }()
	if runner.Name() == exec.TargetRemote {
		if err := cmdutil.EnsureAPIKey(ws.Config.APIKey); err != nil {
			return err
		}
	}
	ctx := cmd.Context()
	if err := runner.DeleteElement(ctx, elementID); err != nil && !exec.IsNotFound(err) {
		return cmdutil.WithUnauthorizedHint("server delete element failed", err)
	}
	if viewID != 0 {
		// The element delete may already cascade; a missing view is not an error.
		if err := runner.DeleteView(ctx, viewID); err != nil && !exec.IsNotFound(err) {
			return cmdutil.WithUnauthorizedHint("server delete view failed", err)
		}
	}
	return nil
}

func newConnectorCmd(wdir, format *string, compact *bool) *cobra.Command {
	var (
		view    string
		from    string
		to      string
		label   string
		dryRun  bool
		target  string
		dataDir string
	)

	c := &cobra.Command{
		Use:   "connector",
		Short: "Remove matching connector(s) from connectors.yaml",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if dryRun {
				var n int
				err := cmdutil.WithWorkspaceDryRun(*wdir, func(cloneDir string) error {
					var runErr error
					n, runErr = workspace.RemoveConnectorWithLabel(cloneDir, view, from, to, label)
					return runErr
				})
				if err != nil {
					if cmdutil.WantsJSON(*format) {
						return cmdutil.WriteCommandError(cmd.OutOrStdout(), *compact, "remove connector", err)
					}
					return fmt.Errorf("dry-run remove connector: %w", err)
				}
				if cmdutil.WantsJSON(*format) {
					return cmdutil.WriteMutation(cmd.OutOrStdout(), *compact, "remove connector", "dry-run", fmt.Sprintf("%s:%s:%s", view, from, to))
				}
				if n == 0 {
					term.Info(cmd.OutOrStdout(), "Dry-run: no matching connectors found — nothing would be removed.")
				} else {
					term.Successf(cmd.OutOrStdout(), "dry-run: del: %d", n)
				}
				return nil
			}
			// Capture connector IDs and refuse ambiguity before mutating
			// anything. The server delete runs first so a failed YAML write
			// can be retried with the cached IDs still intact.
			preWS, err := cmdutil.LoadWorkspace(*wdir)
			if err != nil {
				if cmdutil.WantsJSON(*format) {
					return cmdutil.WriteCommandError(cmd.OutOrStdout(), *compact, "remove connector", err)
				}
				return err
			}
			matchedKeys := matchConnectorKeys(preWS, view, from, to, label)
			if err := workspace.CheckConnectorRemoval(matchedKeys, label); err != nil {
				if cmdutil.WantsJSON(*format) {
					return cmdutil.WriteCommandError(cmd.OutOrStdout(), *compact, "remove connector", err)
				}
				return err
			}
			if err := runRemoveConnectorsServer(cmd, preWS, target, dataDir, matchedKeys); err != nil {
				if cmdutil.WantsJSON(*format) {
					return cmdutil.WriteCommandError(cmd.OutOrStdout(), *compact, "remove connector", err)
				}
				return err
			}
			n, err := workspace.RemoveConnectorWithLabel(*wdir, view, from, to, label)
			if err != nil {
				if cmdutil.WantsJSON(*format) {
					return cmdutil.WriteCommandError(cmd.OutOrStdout(), *compact, "remove connector", err)
				}
				return fmt.Errorf("remove connector: %w", err)
			}
			if cmdutil.WantsJSON(*format) {
				return cmdutil.WriteMutation(cmd.OutOrStdout(), *compact, "remove connector", "remove", fmt.Sprintf("%s:%s:%s", view, from, to))
			}
			if n == 0 {
				term.Info(cmd.OutOrStdout(), "No matching connectors found — nothing removed.")
			} else {
				term.Successf(cmd.OutOrStdout(), "del: %d", n)
			}
			return nil
		},
	}

	c.Flags().StringVar(&view, "view", "", "view ref (required)")
	c.Flags().StringVar(&from, "from", "", "source element ref (required)")
	c.Flags().StringVar(&to, "to", "", "target element ref (required)")
	c.Flags().StringVar(&label, "label", "", "connector label")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "preview the change without writing files")
	c.Flags().StringVar(&target, "target", "", "sync target: auto, local, remote, or cloud")
	c.Flags().StringVar(&dataDir, "data-dir", "", "data directory for local target state")
	_ = c.MarkFlagRequired("view")
	_ = c.MarkFlagRequired("from")
	_ = c.MarkFlagRequired("to")

	elementComp := func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return completion.ElementRefs(wdir)
	}
	_ = c.RegisterFlagCompletionFunc("view", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return completion.ViewRefs(wdir)
	})
	_ = c.RegisterFlagCompletionFunc("from", elementComp)
	_ = c.RegisterFlagCompletionFunc("to", elementComp)
	return c
}

// matchConnectorKeys returns YAML keys matching the view/source/target/label
// selector (empty label matches any single connector).
func matchConnectorKeys(ws *workspace.Workspace, view, from, to, label string) []string {
	var out []string
	if ws == nil {
		return out
	}
	for key, c := range ws.Connectors {
		if c == nil {
			continue
		}
		if c.View != view || c.Source != from || c.Target != to {
			continue
		}
		if label != "" && c.Label != label {
			continue
		}
		out = append(out, key)
	}
	return out
}

// runRemoveConnectorsServer deletes the given connectors on the server
// synchronously, before the YAML cache entries are removed. Keys without
// cached IDs are skipped, and a NotFound is tolerated so a retry after a
// partial failure can converge.
func runRemoveConnectorsServer(cmd *cobra.Command, ws *workspace.Workspace, target, dataDir string, keys []string) error {
	var ids []int32
	if ws.Meta != nil {
		for _, k := range keys {
			if m, ok := ws.Meta.Connectors[k]; ok && m != nil && m.ID != 0 {
				ids = append(ids, int32(m.ID))
			}
		}
	}
	if len(ids) == 0 {
		return nil
	}
	runner, err := exec.NewRunner(ws.Config, target, dataDir, false)
	if err != nil {
		return err
	}
	defer func() { _ = runner.Close() }()
	if runner.Name() == exec.TargetRemote {
		if err := cmdutil.EnsureAPIKey(ws.Config.APIKey); err != nil {
			return err
		}
	}
	ctx := cmd.Context()
	for _, id := range ids {
		if err := runner.DeleteConnector(ctx, id); err != nil && !exec.IsNotFound(err) {
			return cmdutil.WithUnauthorizedHint("server delete connector failed", err)
		}
	}
	return nil
}

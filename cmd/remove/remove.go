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
		Short: "Remove an element",
		Args:  cobra.ExactArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) != 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return completion.ElementRefs(wdir)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			ref := args[0]
			sess, err := cmdutil.OpenSession(cmd, *wdir, target, dataDir)
			if err != nil {
				if cmdutil.WantsJSON(*format) {
					return cmdutil.WriteCommandError(cmd.OutOrStdout(), *compact, "remove element", err)
				}
				return err
			}
			defer func() { _ = sess.Close() }()
			ws, err := sess.LoadWorkspace()
			if err != nil {
				if cmdutil.WantsJSON(*format) {
					return cmdutil.WriteCommandError(cmd.OutOrStdout(), *compact, "remove element", err)
				}
				return err
			}
			ref, err = cmdutil.ResolveElementArg(ws, ref)
			if err != nil {
				if cmdutil.WantsJSON(*format) {
					return cmdutil.WriteCommandError(cmd.OutOrStdout(), *compact, "remove element", err)
				}
				return err
			}
			if dryRun {
				if sess.HasWorkspace() {
					err := cmdutil.WithWorkspaceDryRun(*wdir, func(cloneDir string) error {
						return workspace.RemoveElement(cloneDir, ref)
					})
					if err != nil {
						if cmdutil.WantsJSON(*format) {
							return cmdutil.WriteCommandError(cmd.OutOrStdout(), *compact, "remove element", err)
						}
						return fmt.Errorf("dry-run remove element: %w", err)
					}
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
			if err := workspace.CheckElementRemoval(ws, ref); err != nil {
				if cmdutil.WantsJSON(*format) {
					return cmdutil.WriteCommandError(cmd.OutOrStdout(), *compact, "remove element", err)
				}
				return err
			}
			var elementID, viewID int32
			if ws.Meta != nil {
				if m, ok := ws.Meta.Elements[ref]; ok && m != nil {
					elementID = int32(m.ID)
				}
				if m, ok := ws.Meta.Views[ref]; ok && m != nil {
					viewID = int32(m.ID)
				}
			}
			if err := runRemoveElementServer(cmd, sess, elementID, viewID); err != nil {
				if cmdutil.WantsJSON(*format) {
					return cmdutil.WriteCommandError(cmd.OutOrStdout(), *compact, "remove element", err)
				}
				return err
			}
			if sess.HasWorkspace() {
				if err := workspace.RemoveElement(*wdir, ref); err != nil {
					err = sess.CacheError(err)
					if cmdutil.WantsJSON(*format) {
						return cmdutil.WriteCommandError(cmd.OutOrStdout(), *compact, "remove element", err)
					}
					return err
				}
				cached, err := workspace.Load(*wdir)
				if err != nil {
					return sess.CacheError(err)
				}
				if err := exec.SaveCache(*wdir, cached); err != nil {
					return sess.CacheError(err)
				}
			}
			if cmdutil.WantsJSON(*format) {
				return cmdutil.WriteResourceMutation(cmd.OutOrStdout(), *compact, "remove element", "remove", ref, "element", elementID)
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
func runRemoveElementServer(cmd *cobra.Command, sess *cmdutil.Session, elementID, viewID int32) error {
	if err := sess.RequireAppliedCache(); err != nil {
		return err
	}
	if elementID == 0 {
		// No cached ID (e.g. hand-written YAML): nothing to delete server-side.
		return nil
	}
	runner, err := sess.Runner()
	if err != nil {
		return err
	}
	ctx := sess.Context(cmd.Context())
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
	var view, from, to, label, target, dataDir string
	var dryRun bool
	c := &cobra.Command{
		Use:     "connector [key-or-id]",
		Short:   "Remove a connector by its listed key or ID",
		Long:    "Remove a connector using a key or ID from 'tld list connectors'. The legacy --view/--from/--to selectors remain available; ambiguous matches are refused.",
		Example: "  tld remove connector 'root/api~db/reads'\n  tld remove connector 42 --dry-run",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				for _, flag := range []string{"view", "from", "to", "label"} {
					if cmd.Flags().Changed(flag) {
						return fmt.Errorf("a connector key or ID cannot be combined with --%s", flag)
					}
				}
			} else if view == "" || from == "" || to == "" {
				return fmt.Errorf("a connector key or ID is required; alternatively supply --view, --from, and --to")
			}
			sess, err := cmdutil.OpenSession(cmd, *wdir, target, dataDir)
			if err != nil {
				return err
			}
			defer func() { _ = sess.Close() }()
			ws, err := sess.LoadWorkspace()
			if err != nil {
				return err
			}
			var keys []string
			if len(args) == 1 {
				key, err := cmdutil.ResolveConnectorArg(ws, args[0])
				if err != nil {
					return err
				}
				keys = []string{key}
			} else {
				if view, err = cmdutil.ResolveViewArg(ws, view); err != nil {
					return err
				}
				if from, err = cmdutil.ResolveElementArg(ws, from); err != nil {
					return err
				}
				if to, err = cmdutil.ResolveElementArg(ws, to); err != nil {
					return err
				}
				keys = matchConnectorKeys(ws, view, from, to, label)
				if err := workspace.CheckConnectorRemoval(keys, label); err != nil {
					return err
				}
			}
			items := make([]cmdutil.JSONItem, 0, len(keys))
			for _, key := range keys {
				var id int32
				if ws.Meta != nil && ws.Meta.Connectors[key] != nil {
					id = int32(ws.Meta.Connectors[key].ID)
				}
				action := "remove"
				if dryRun {
					action = "dry-run"
				}
				items = append(items, cmdutil.JSONItem{Ref: key, ID: id, ResourceType: "connector", Action: action})
			}
			if !dryRun && len(keys) > 0 {
				if err := runRemoveConnectorsServer(cmd, sess, ws, keys); err != nil {
					return err
				}
				if sess.HasWorkspace() {
					for _, key := range keys {
						delete(ws.Connectors, key)
						if ws.Meta != nil {
							delete(ws.Meta.Connectors, key)
						}
						if err := workspace.DeleteCurrentConnectorMetadataEntries(*wdir, key); err != nil {
							return sess.CacheError(err)
						}
					}
					if err := exec.SaveCache(sess.Wdir, ws); err != nil {
						return sess.CacheError(err)
					}
				}
			}
			if cmdutil.WantsJSON(*format) {
				return cmdutil.WriteJSON(cmd.OutOrStdout(), *compact, cmdutil.JSONOutput{Command: "remove connector", Status: "ok", Items: items, Summary: map[string]int{"count": len(keys)}})
			}
			if len(keys) == 0 {
				term.Info(cmd.OutOrStdout(), "No matching connectors found — nothing removed.")
			} else if dryRun {
				term.Successf(cmd.OutOrStdout(), "dry-run: del: %d", len(keys))
			} else {
				term.Successf(cmd.OutOrStdout(), "del: %d", len(keys))
			}
			return nil
		},
	}
	c.Flags().StringVar(&view, "view", "", "view ref (legacy selector)")
	c.Flags().StringVar(&from, "from", "", "source element ref (legacy selector)")
	c.Flags().StringVar(&to, "to", "", "target element ref (legacy selector)")
	c.Flags().StringVar(&label, "label", "", "connector label (legacy selector)")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "preview without changing the target or cache")
	c.Flags().StringVar(&target, "target", "", "target: auto, local, remote, or cloud")
	c.Flags().StringVar(&dataDir, "data-dir", "", "data directory for local target state")
	c.ValidArgsFunction = func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return completion.ConnectorKeys(wdir)
	}
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
func runRemoveConnectorsServer(cmd *cobra.Command, sess *cmdutil.Session, ws *workspace.Workspace, keys []string) error {
	if err := sess.RequireAppliedCache(); err != nil {
		return err
	}
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
	runner, err := sess.Runner()
	if err != nil {
		return err
	}
	ctx := sess.Context(cmd.Context())
	for _, id := range ids {
		if err := runner.DeleteConnector(ctx, id); err != nil && !exec.IsNotFound(err) {
			return cmdutil.WithUnauthorizedHint("server delete connector failed", err)
		}
	}
	return nil
}

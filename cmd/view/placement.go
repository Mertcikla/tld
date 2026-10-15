package view

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mertcikla/tld/v2/internal/cmdutil"
	"github.com/mertcikla/tld/v2/internal/completion"
	"github.com/mertcikla/tld/v2/internal/exec"
	"github.com/mertcikla/tld/v2/internal/term"
	"github.com/spf13/cobra"
)

func newPlacementCmd(wdir, format *string, compact *bool, remove bool) *cobra.Command {
	name, summary := "place", "Place an existing element in a view; use --from to move it"
	if remove {
		name, summary = "unplace", "Remove an element from one view, keeping the element and other placements"
	}
	var target, dataDir, from string
	var dryRun bool
	c := &cobra.Command{
		Use: name + " <view> <element>", Short: summary,
		Long: summary + ". Select views and elements by ref, name, or ID.\nExisting placements retain their coordinates. Remove or update connectors in\nthe old view before unplacing or moving their endpoints. Placing shared endpoints can copy their existing relationships into this view;\nthese copies are persisted connectors and appear in list output. Operations are repeatable.",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			sess, err := cmdutil.OpenSession(cmd, *wdir, target, dataDir)
			if err != nil {
				return err
			}
			defer func() { _ = sess.Close() }()
			ws, err := sess.LoadWorkspace()
			if err != nil {
				return err
			}
			viewRef, err := cmdutil.ResolveViewArg(ws, args[0])
			if err != nil {
				return err
			}
			ref, err := cmdutil.ResolveElementArg(ws, args[1])
			if err != nil {
				return err
			}
			if !remove && ref == viewRef {
				return fmt.Errorf("an element cannot be placed in its own view; choose another view")
			}
			oldView := ""
			if remove {
				oldView = viewRef
			} else if cmd.Flags().Changed("from") {
				oldView, err = cmdutil.ResolveViewArg(ws, from)
				if err != nil {
					return err
				}
			}
			if oldView == viewRef && !remove {
				oldView = ""
			}
			if oldView != "" {
				var blockers []string
				for key, c := range ws.Connectors {
					if c.View == oldView && (c.Source == ref || c.Target == ref) {
						blockers = append(blockers, key)
					}
				}
				if len(blockers) > 0 {
					sort.Strings(blockers)
					return cmdutil.WithHint(fmt.Errorf("element %q is connected in view %q: %s", ref, oldView, strings.Join(blockers, ", ")), "Use 'tld update connector' or 'tld remove connector <key-or-id>' on these connectors first.")
				}
			}
			action := name
			var elementID int32
			if ws.Meta != nil && ws.Meta.Elements[ref] != nil {
				elementID = int32(ws.Meta.Elements[ref].ID)
			}
			if dryRun {
				action = "dry-run"
			} else {
				if err := sess.RequireAppliedCache(); err != nil {
					return err
				}
				runner, err := sess.Runner()
				if err != nil {
					return err
				}
				ctx := sess.Context(cmd.Context())
				elementID, err = exec.EnsureElementID(ctx, runner, ws, sess.Wdir, ref)
				if err != nil {
					return err
				}
				if !remove {
					viewID, err := exec.ResolveParentViewID(ctx, runner, ws, sess.Wdir, viewRef)
					if err != nil {
						return err
					}
					exported, err := runner.ExportWorkspace(ctx)
					if err != nil {
						return err
					}
					// Repeating place must not replay stale YAML coordinates over a UI move.
					placed := false
					for _, p := range exported.GetPlacements() {
						if p.GetViewId() == viewID && p.GetElementId() == elementID {
							placed = true
							break
						}
					}
					if !placed {
						var x, y float64
						if p := cmdutil.FindPlacement(ws, ref, viewRef); p != nil {
							x, y = p.PositionX, p.PositionY
						} else {
							x, y, _ = cmdutil.AutoPlace(ws, viewRef, ref, nil)
						}
						if err := runner.AddPlacement(ctx, viewID, elementID, x, y); err != nil {
							return err
						}
					}
				}
				if oldView != "" && cmdutil.FindPlacement(ws, ref, oldView) != nil {
					viewID, err := exec.ResolveParentViewID(ctx, runner, ws, sess.Wdir, oldView)
					if err == nil {
						err = runner.RemovePlacement(ctx, viewID, elementID)
					}
					if err != nil && !exec.IsNotFound(err) {
						if !remove {
							return &cmdutil.CommandError{Cause: err, Code: "partial_mutation", Applied: true, Hint: "The destination placement was saved. Retry the same command to finish the move."}
						}
						return err
					}
				}
				if sess.HasWorkspace() {
					if err := sess.RefreshCache(ws); err != nil {
						return sess.CacheError(err)
					}
				}
			}
			if cmdutil.WantsJSON(*format) {
				return cmdutil.WriteJSON(cmd.OutOrStdout(), *compact, cmdutil.JSONOutput{Command: "view " + name, Status: "ok", Items: []cmdutil.JSONItem{{Ref: ref, ID: elementID, ResourceType: "element", Action: action}}, Extra: map[string]any{"view": viewRef, "from": oldView}})
			}
			term.Successf(cmd.OutOrStdout(), "%s: %s in %s", action, ref, viewRef)
			return nil
		},
	}
	addTargetFlags(c, &target, &dataDir)
	c.Flags().BoolVar(&dryRun, "dry-run", false, "preview without changing the target or cache")
	if !remove {
		c.Flags().StringVar(&from, "from", "", "move from this view after placing in the destination")
	}
	c.ValidArgsFunction = func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			return completion.ViewRefs(wdir)
		}
		return completion.ElementRefs(wdir)
	}
	return c
}

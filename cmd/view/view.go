package view

import (
	"fmt"
	"strings"

	"github.com/mertcikla/tld/v2/internal/cmdutil"
	"github.com/mertcikla/tld/v2/internal/completion"
	"github.com/mertcikla/tld/v2/internal/exec"
	"github.com/mertcikla/tld/v2/internal/term"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/spf13/cobra"
)

// NewViewCmd groups commands that manage the diagram (view) owned by an
// element. In the workspace model a view exists iff its owner element has
// has_view=true, so every subcommand keeps the YAML cache and the server in
// step.
func NewViewCmd(wdir, format *string, compact *bool) *cobra.Command {
	c := &cobra.Command{
		Use:   "view",
		Short: "Manage diagrams (views) owned by elements",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return cmd.Help()
			}
			return cobra.NoArgs(cmd, args)
		},
	}

	c.AddCommand(
		newCreateCmd(wdir, format, compact),
		newRenameCmd(wdir, format, compact),
		newSetLevelCmd(wdir, format, compact),
		newDeleteCmd(wdir, format, compact),
	)
	return c
}

func newCreateCmd(wdir, format *string, compact *bool) *cobra.Command {
	var (
		name    string
		label   string
		target  string
		dataDir string
		dryRun  bool
	)
	c := &cobra.Command{
		Use:   "create <element-ref>",
		Short: "Create (or ensure) the view owned by an element",
		Long: `Create the diagram owned by an element and set has_view on it.

The element must already exist (ref, name, or database ID); create it first with
'tld add'. Use --name and --label to set the view display name and level label.`,
		Args: cobra.ExactArgs(1),
		ValidArgsFunction: func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
			if len(args) != 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return completion.ElementRefs(wdir)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			ref := args[0]
			sess, err := cmdutil.OpenSession(cmd, *wdir, target, dataDir)
			if err != nil {
				return failf(cmd, *format, *compact, "view create", err)
			}
			defer func() { _ = sess.Close() }()
			ws, err := sess.LoadWorkspace()
			if err != nil {
				return failf(cmd, *format, *compact, "view create", err)
			}
			ref, err = cmdutil.ResolveElementArg(ws, ref)
			if err != nil {
				return failf(cmd, *format, *compact, "view create", err)
			}
			el := ws.Elements[ref]
			if el == nil {
				return failf(cmd, *format, *compact, "view create", fmt.Errorf("element %q not found; create it first with 'tld add'", ref))
			}
			viewName := strings.TrimSpace(name)
			if viewName == "" {
				viewName = el.Name
			}
			if viewName == "" {
				return failf(cmd, *format, *compact, "view create", fmt.Errorf("view name is required: pass --name or set the element name"))
			}
			if dryRun {
				return reportDryRun(cmd, *format, *compact, ref, viewName)
			}
			runner, err := sess.Runner()
			if err != nil {
				return failf(cmd, *format, *compact, "view create", err)
			}
			ctx := sess.Context(cmd.Context())
			// ResolveParentViewID creates the view, sets has_view, and records
			// the view metadata in the YAML cache.
			viewID, err := exec.ResolveParentViewID(ctx, runner, ws, *wdir, ref)
			if err != nil {
				return failf(cmd, *format, *compact, "view create", err)
			}
			var labelPtr *string
			if cmd.Flags().Changed("label") {
				labelPtr = &label
			} else if el.ViewLabel != "" {
				labelPtr = &el.ViewLabel
			}
			if _, err := runner.UpdateView(ctx, viewID, viewName, labelPtr); err != nil {
				return failf(cmd, *format, *compact, "view create", cmdutil.WithUnauthorizedHint("server update view failed", err))
			}
			if err := applyViewFields(sess, *wdir, ref, viewName, labelPtr); err != nil {
				return failf(cmd, *format, *compact, "view create", err)
			}
			if cmdutil.WantsJSON(*format) {
				return cmdutil.WriteMutation(cmd.OutOrStdout(), *compact, "view create", "create", ref)
			}
			term.Successf(cmd.OutOrStdout(), "view: %s (name=%q, id=%d)", ref, viewName, viewID)
			return nil
		},
	}
	c.Flags().StringVar(&name, "name", "", "view display name (default: element name)")
	c.Flags().StringVar(&label, "label", "", "view level label, e.g. Container")
	addTargetFlags(c, &target, &dataDir)
	c.Flags().BoolVar(&dryRun, "dry-run", false, "preview the change without calling the server")
	return c
}

func newRenameCmd(wdir, format *string, compact *bool) *cobra.Command {
	var (
		target  string
		dataDir string
		dryRun  bool
	)
	c := &cobra.Command{
		Use:   "rename <element-ref> <name>",
		Short: "Rename the view owned by an element",
		Args:  cobra.ExactArgs(2),
		ValidArgsFunction: func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
			if len(args) != 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return completion.ViewRefs(wdir)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, newName := args[0], strings.TrimSpace(args[1])
			if newName == "" {
				return failf(cmd, *format, *compact, "view rename", fmt.Errorf("new view name is required"))
			}
			sess, err := cmdutil.OpenSession(cmd, *wdir, target, dataDir)
			if err != nil {
				return failf(cmd, *format, *compact, "view rename", err)
			}
			defer func() { _ = sess.Close() }()
			ws, err := sess.LoadWorkspace()
			if err != nil {
				return failf(cmd, *format, *compact, "view rename", err)
			}
			ref, err = cmdutil.ResolveElementArg(ws, ref)
			if err != nil {
				return failf(cmd, *format, *compact, "view rename", err)
			}
			if err := requireView(ws, ref); err != nil {
				return failf(cmd, *format, *compact, "view rename", err)
			}
			if dryRun {
				return reportDryRun(cmd, *format, *compact, ref, newName)
			}
			return updateView(cmd, sess, ws, *wdir, *format, *compact, "view rename", ref, &newName, nil)
		},
	}
	addTargetFlags(c, &target, &dataDir)
	c.Flags().BoolVar(&dryRun, "dry-run", false, "preview the change without calling the server")
	return c
}

func newSetLevelCmd(wdir, format *string, compact *bool) *cobra.Command {
	var (
		target  string
		dataDir string
		dryRun  bool
	)
	c := &cobra.Command{
		Use:   "set-level <element-ref> <label>",
		Short: "Set the level label of the view owned by an element",
		Args:  cobra.ExactArgs(2),
		ValidArgsFunction: func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
			if len(args) != 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return completion.ViewRefs(wdir)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, label := args[0], args[1]
			sess, err := cmdutil.OpenSession(cmd, *wdir, target, dataDir)
			if err != nil {
				return failf(cmd, *format, *compact, "view set-level", err)
			}
			defer func() { _ = sess.Close() }()
			ws, err := sess.LoadWorkspace()
			if err != nil {
				return failf(cmd, *format, *compact, "view set-level", err)
			}
			ref, err = cmdutil.ResolveElementArg(ws, ref)
			if err != nil {
				return failf(cmd, *format, *compact, "view set-level", err)
			}
			if err := requireView(ws, ref); err != nil {
				return failf(cmd, *format, *compact, "view set-level", err)
			}
			if dryRun {
				return reportDryRun(cmd, *format, *compact, ref, label)
			}
			return updateView(cmd, sess, ws, *wdir, *format, *compact, "view set-level", ref, nil, &label)
		},
	}
	addTargetFlags(c, &target, &dataDir)
	c.Flags().BoolVar(&dryRun, "dry-run", false, "preview the change without calling the server")
	return c
}

func newDeleteCmd(wdir, format *string, compact *bool) *cobra.Command {
	var (
		target  string
		dataDir string
		dryRun  bool
	)
	c := &cobra.Command{
		Use:   "delete <element-ref>",
		Short: "Delete the view owned by an element (keeps the element)",
		Long: `Delete the diagram owned by an element. The element itself is kept; only
its view and view metadata are removed. The view must be empty: remove or
reparent any elements placed in it, and delete any connectors that belong to
it, first.`,
		Args: cobra.ExactArgs(1),
		ValidArgsFunction: func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
			if len(args) != 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return completion.ViewRefs(wdir)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			ref := args[0]
			sess, err := cmdutil.OpenSession(cmd, *wdir, target, dataDir)
			if err != nil {
				return failf(cmd, *format, *compact, "view delete", err)
			}
			defer func() { _ = sess.Close() }()
			ws, err := sess.LoadWorkspace()
			if err != nil {
				return failf(cmd, *format, *compact, "view delete", err)
			}
			ref, err = cmdutil.ResolveElementArg(ws, ref)
			if err != nil {
				return failf(cmd, *format, *compact, "view delete", err)
			}
			if err := requireView(ws, ref); err != nil {
				return failf(cmd, *format, *compact, "view delete", err)
			}
			if err := ensureViewEmpty(ws, ref); err != nil {
				return failf(cmd, *format, *compact, "view delete", err)
			}
			if dryRun {
				return reportDryRun(cmd, *format, *compact, ref, "")
			}
			return deleteView(cmd, sess, ws, *wdir, *format, *compact, ref)
		},
	}
	addTargetFlags(c, &target, &dataDir)
	c.Flags().BoolVar(&dryRun, "dry-run", false, "preview the change without calling the server")
	return c
}

func addTargetFlags(c *cobra.Command, target, dataDir *string) {
	c.Flags().StringVar(target, "target", "", "sync target: auto, local, remote, or cloud")
	c.Flags().StringVar(dataDir, "data-dir", "", "data directory for local target state")
}

// requireView reports an error unless the element owns a view (locally or via
// recorded metadata).
func requireView(ws *workspace.Workspace, ref string) error {
	if err := workspace.ValidateElementRef(ref); err != nil {
		return err
	}
	el := ws.Elements[ref]
	if el == nil {
		return fmt.Errorf("element %q not found", ref)
	}
	if el.HasView {
		return nil
	}
	if ws.Meta != nil {
		if m, ok := ws.Meta.Views[ref]; ok && m != nil && m.ID != 0 {
			return nil
		}
	}
	return fmt.Errorf("element %q has no view; create one with 'tld view create %s'", ref, ref)
}

// ensureViewEmpty refuses to delete a view that still has elements placed in it
// or connectors bound to it, so no YAML references are orphaned.
func ensureViewEmpty(ws *workspace.Workspace, ref string) error {
	var children []string
	for childRef, el := range ws.Elements {
		if el == nil {
			continue
		}
		for _, placement := range el.Placements {
			if placement.ParentRef == ref {
				children = append(children, childRef)
				break
			}
		}
	}
	if len(children) > 0 {
		return fmt.Errorf("view %q still has %d element(s) placed in it: %s. Move or remove them first", ref, len(children), strings.Join(children, ", "))
	}
	var connectors []string
	for key, connector := range ws.Connectors {
		if connector != nil && connector.View == ref {
			connectors = append(connectors, key)
		}
	}
	if len(connectors) > 0 {
		return fmt.Errorf("view %q still has %d connector(s) bound to it: %s. Remove them first", ref, len(connectors), strings.Join(connectors, ", "))
	}
	return nil
}

// updateView sets the view name and/or level label on the server and in YAML.
func updateView(cmd *cobra.Command, sess *cmdutil.Session, ws *workspace.Workspace, wdir, format string, compact bool, command, ref string, newName, newLabel *string) error {
	fail := func(err error) error { return failf(cmd, format, compact, command, err) }
	el := ws.Elements[ref]
	runner, err := sess.Runner()
	if err != nil {
		return fail(err)
	}
	ctx := sess.Context(cmd.Context())
	viewID, err := exec.ResolveParentViewID(ctx, runner, ws, wdir, ref)
	if err != nil {
		return fail(err)
	}
	name := el.Name
	if el.ViewName != "" {
		name = el.ViewName
	}
	if newName != nil {
		name = *newName
	}
	var labelPtr *string
	switch {
	case newLabel != nil:
		labelPtr = newLabel
	case el.ViewLabel != "":
		labelPtr = &el.ViewLabel
	}
	if _, err := runner.UpdateView(ctx, viewID, name, labelPtr); err != nil {
		return fail(cmdutil.WithUnauthorizedHint("server update view failed", err))
	}
	if err := applyViewFields(sess, wdir, ref, name, newLabel); err != nil {
		return fail(err)
	}
	if cmdutil.WantsJSON(format) {
		return cmdutil.WriteMutation(cmd.OutOrStdout(), compact, command, "update", ref)
	}
	term.Successf(cmd.OutOrStdout(), "view updated: %s (name=%q, id=%d)", ref, name, viewID)
	return nil
}

// applyViewFields writes the view display fields back into elements.yaml. An
// empty newLabel clears the recorded level label. It is a no-op in DB mode.
func applyViewFields(sess *cmdutil.Session, wdir, ref, name string, newLabel *string) error {
	if !sess.HasWorkspace() {
		return nil
	}
	if name != "" {
		if err := workspace.UpdateElementField(wdir, ref, "view_name", name); err != nil {
			return fmt.Errorf("update YAML cache: %w", err)
		}
	}
	if newLabel != nil {
		if err := workspace.UpdateElementField(wdir, ref, "view_label", *newLabel); err != nil {
			return fmt.Errorf("update YAML cache: %w", err)
		}
	}
	return nil
}

func deleteView(cmd *cobra.Command, sess *cmdutil.Session, ws *workspace.Workspace, wdir, format string, compact bool, ref string) error {
	fail := func(err error) error { return failf(cmd, format, compact, "view delete", err) }
	runner, err := sess.Runner()
	if err != nil {
		return fail(err)
	}
	ctx := sess.Context(cmd.Context())
	if ws.Meta != nil {
		if m, ok := ws.Meta.Views[ref]; ok && m != nil && m.ID != 0 {
			if err := runner.DeleteView(ctx, int32(m.ID)); err != nil && !exec.IsNotFound(err) {
				return fail(cmdutil.WithUnauthorizedHint("server delete view failed", err))
			}
		}
	}
	if sess.HasWorkspace() {
		if err := workspace.UpdateElementField(wdir, ref, "has_view", "false"); err != nil {
			return fail(fmt.Errorf("update YAML cache: %w", err))
		}
		el := ws.Elements[ref]
		if el != nil && el.ViewName != "" {
			if err := workspace.UpdateElementField(wdir, ref, "view_name", ""); err != nil {
				return fail(fmt.Errorf("update YAML cache: %w", err))
			}
		}
		if el != nil && el.ViewLabel != "" {
			if err := workspace.UpdateElementField(wdir, ref, "view_label", ""); err != nil {
				return fail(fmt.Errorf("update YAML cache: %w", err))
			}
		}
		if err := workspace.DeleteCurrentViewMetadataEntries(wdir, ref); err != nil {
			return fail(fmt.Errorf("drop view metadata: %w", err))
		}
	}
	if cmdutil.WantsJSON(format) {
		return cmdutil.WriteMutation(cmd.OutOrStdout(), compact, "view delete", "delete", ref)
	}
	term.Successf(cmd.OutOrStdout(), "view deleted: %s", ref)
	return nil
}

func reportDryRun(cmd *cobra.Command, format string, compact bool, ref, name string) error {
	if cmdutil.WantsJSON(format) {
		return cmdutil.WriteMutation(cmd.OutOrStdout(), compact, "view", "dry-run", ref)
	}
	if name != "" {
		term.Successf(cmd.OutOrStdout(), "dry-run: view %s (name=%q)", ref, name)
		return nil
	}
	term.Successf(cmd.OutOrStdout(), "dry-run: view %s", ref)
	return nil
}

func failf(cmd *cobra.Command, format string, compact bool, command string, err error) error {
	if cmdutil.WantsJSON(format) {
		return cmdutil.WriteCommandError(cmd.OutOrStdout(), compact, command, err)
	}
	return err
}

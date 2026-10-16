package connect

import (
	"context"
	"fmt"
	"strings"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/mertcikla/tld/v2/internal/cmdutil"
	"github.com/mertcikla/tld/v2/internal/completion"
	"github.com/mertcikla/tld/v2/internal/exec"
	"github.com/mertcikla/tld/v2/internal/term"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/mertcikla/tld/v2/pkg/api"
	"github.com/spf13/cobra"
)

func NewConnectCmd(wdir, format *string, compact *bool) *cobra.Command {
	var (
		from         string
		to           string
		dryRun       bool
		label        string
		description  string
		relationship string
		direction    string
		style        string
		url          string
		tags         string
		legacyView   string
		noReposition bool
		target       string
		dataDir      string
	)

	c := &cobra.Command{
		Use:   "connect",
		Short: "Add or update a labeled relationship between two elements",
		Long: `Add a connector between two existing elements.

--from and --to accept element refs, display names, or numeric element IDs from
'tld list elements'. The connector belongs to the source element's parent view
by default. Use --view to choose a particular view when elements have multiple
placements; root selects the top-level diagram.
Endpoints need not be placed in that view: off-view relationships can appear
through UI context nodes, ancestor proxies, or badges. Connecting never places
an endpoint; use 'view place' only when it belongs directly in this view.

Use --label for the visible interaction and --relationship for semantic metadata
such as calls or queries. Direction is forward, backward, both, or none.
Repeating the same view, endpoints, and label updates the existing connector.
Changes apply to the target immediately and refresh the YAML cache when present.
Use 'tld list connectors' to find the resulting key for inspection or updates.`,
		Example: `  tld connect --from api --to db --label reads
  tld connect --from api --to db --view platform --label "reads and writes" --direction both
  tld list connectors --view platform`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			sess, err := cmdutil.OpenSession(cmd, *wdir, target, dataDir)
			if err != nil {
				return err
			}
			defer func() { _ = sess.Close() }()
			ws, err := sess.LoadWorkspace()
			if err != nil {
				return fmt.Errorf("load workspace: %w", err)
			}
			from, err = cmdutil.ResolveElementArg(ws, from)
			if err != nil {
				return err
			}
			to, err = cmdutil.ResolveElementArg(ws, to)
			if err != nil {
				return err
			}
			if err := validateConnectorEndpointsExist(ws, from, to); err != nil {
				return err
			}
			view := legacyView
			if strings.EqualFold(strings.TrimSpace(view), "auto") {
				view = ""
			}
			if view == "" {
				view, err = inferConnectorView(ws, from, to)
				if err != nil {
					return err
				}
			} else {
				resolvedView, resolveErr := cmdutil.ResolveViewArg(ws, view)
				if resolveErr != nil {
					if err := validateConnectorViewExists(ws, view); err != nil {
						return err
					}
					return resolveErr
				}
				view = resolvedView
				if err := validateConnectorViewExists(ws, view); err != nil {
					return err
				}
			}
			spec := &workspace.Connector{
				View:         view,
				Source:       from,
				Target:       to,
				Label:        label,
				Description:  description,
				Relationship: relationship,
				Direction:    direction,
				Style:        style,
				URL:          url,
				Tags:         workspace.ParseTagList(tags),
			}
			if err := workspace.ValidateConnectorValues(spec); err != nil {
				return err
			}
			if dryRun {
				if sess.HasWorkspace() {
					if err := cmdutil.WithWorkspaceDryRun(*wdir, func(cloneDir string) error {
						return workspace.AppendConnector(cloneDir, spec)
					}); err != nil {
						if cmdutil.WantsJSON(*format) {
							return cmdutil.WriteCommandError(cmd.OutOrStdout(), *compact, "connect", err)
						}
						return fmt.Errorf("dry-run connect: %w", err)
					}
				}
				if cmdutil.WantsJSON(*format) {
					return writeConnectJSON(cmd, ws, spec, *compact, "dry-run", 0)
				}
				term.Successf(cmd.OutOrStdout(), "dry-run: connect: %s -> %s", from, to)
				term.Infof(cmd.OutOrStdout(), "connector view: %s", view)
				printVisibility(cmd, ws, spec)
				return nil
			}
			return runConnect(cmd, sess, ws, *format, *compact, spec, from, to, view, noReposition)
		},
	}

	c.Flags().StringVar(&from, "from", "", "source element ref (required)")
	c.Flags().StringVar(&to, "to", "", "target element ref (required)")
	c.Flags().StringVar(&label, "label", "", "connector label")
	c.Flags().StringVar(&description, "description", "", "connector description")
	c.Flags().StringVar(&relationship, "relationship", "", "semantic relationship type")
	c.Flags().StringVar(&direction, "direction", "forward", "forward|backward|both|none")
	c.Flags().StringVar(&style, "style", "bezier", "bezier|straight|step|smoothstep")
	c.Flags().StringVar(&url, "url", "", "external URL")
	c.Flags().StringVar(&tags, "tags", "", "comma-separated tags")
	c.Flags().StringVar(&legacyView, "view", "", "explicit connector view ref (default: source element's view)")
	c.Flags().BoolVar(&noReposition, "no-reposition", false, "do not auto-place a freshly connected target element")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "preview the change without writing files")
	c.Flags().StringVar(&target, "target", "", "sync target: auto, local, remote, or cloud")
	c.Flags().StringVar(&dataDir, "data-dir", "", "data directory for local target state")
	_ = c.Flags().MarkHidden("style")
	_ = c.MarkFlagRequired("from")
	_ = c.MarkFlagRequired("to")

	elementComp := func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return completion.ElementRefs(wdir)
	}
	_ = c.RegisterFlagCompletionFunc("from", elementComp)
	_ = c.RegisterFlagCompletionFunc("to", elementComp)
	_ = c.RegisterFlagCompletionFunc("direction", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return completion.ConnectorDirections(), cobra.ShellCompDirectiveNoFileComp
	})
	_ = c.RegisterFlagCompletionFunc("view", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		refs, directive := completion.ViewRefs(wdir)
		return append([]string{"auto"}, refs...), directive
	})
	return c
}

// runConnect creates the connector on the server synchronously, then refreshes
// the local YAML cache.
func runConnect(cmd *cobra.Command, sess *cmdutil.Session, ws *workspace.Workspace, format string, compact bool, spec *workspace.Connector, from, to, view string, noReposition bool) error {
	fail := func(err error) error {
		if cmdutil.WantsJSON(format) {
			return cmdutil.WriteCommandError(cmd.OutOrStdout(), compact, "connect", err)
		}
		return err
	}
	if err := sess.RequireAppliedCache(); err != nil {
		return fail(err)
	}
	runner, err := sess.Runner()
	if err != nil {
		return fail(err)
	}
	ctx := sess.Context(cmd.Context())
	sourceID, err := exec.EnsureElementID(ctx, runner, ws, sess.Wdir, from)
	if err != nil {
		return fail(err)
	}
	targetID, err := exec.EnsureElementID(ctx, runner, ws, sess.Wdir, to)
	if err != nil {
		return fail(err)
	}
	viewID, err := exec.ResolveParentViewID(ctx, runner, ws, sess.Wdir, view)
	if err != nil {
		return fail(fmt.Errorf("resolve connector view: %w", err))
	}
	direction := spec.Direction
	if direction == "" {
		direction = "forward"
	}
	style := spec.Style
	if style == "" {
		style = "bezier"
	}
	created, err := ensureConnector(ctx, cmd, runner, api.ConnectorInput{
		ViewID:       viewID,
		SourceID:     sourceID,
		TargetID:     targetID,
		Label:        optStr(spec.Label),
		Description:  optStr(spec.Description),
		Relationship: optStr(spec.Relationship),
		Direction:    direction,
		Style:        style,
		URL:          optStr(spec.URL),
		Tags:         spec.Tags,
	})
	if err != nil {
		return fail(cmdutil.WithUnauthorizedHint("server create connector failed", err))
	}
	// Optional fields omitted on a retry are preserved by the target. Cache the
	// returned values so the YAML cannot silently clear those fields.
	spec.Description, spec.Relationship, spec.URL = created.GetDescription(), created.GetRelationship(), created.GetUrl()
	spec.Direction, spec.Style, spec.Tags = created.GetDirection(), created.GetStyle(), created.GetTags()
	spec.SourceHandle, spec.TargetHandle = created.GetSourceHandle(), created.GetTargetHandle()
	// Auto-place a freshly-connected target so the new edge reads cleanly. Only
	// targets that already live in this view and have no other connectors here
	// (i.e. just added) are moved, so deliberately wired-up elements are never
	// disturbed and cross-view targets are left alone. Failures here are
	// non-fatal: the connector itself was already created.
	moved := false
	var movedX, movedY float64
	if !noReposition && cmdutil.FindPlacement(ws, to, view) != nil && targetIsFresh(ws, view, to) {
		if x, y, ok := cmdutil.AutoPlace(ws, view, to, []workspace.Connector{*spec}); ok {
			if err := runner.UpdatePlacement(ctx, viewID, targetID, x, y); err != nil {
				term.Warnf(cmd.ErrOrStderr(), "connected but could not reposition %q: %v", to, err)
			} else {
				moved, movedX, movedY = true, x, y
			}
		}
	}
	if sess.HasWorkspace() {
		if err := workspace.AppendConnector(sess.Wdir, spec); err != nil {
			return fail(sess.CacheError(err))
		}
		if moved {
			if err := workspace.SetPlacementPosition(sess.Wdir, to, view, movedX, movedY); err != nil {
				return fail(sess.CacheError(err))
			}
		}
		if err := exec.RecordConnectorMeta(ctx, sess.Wdir, workspace.ConnectorKey(spec), created); err != nil {
			return fail(sess.CacheError(err))
		}
	}
	if cmdutil.WantsJSON(format) {
		return writeConnectJSON(cmd, ws, spec, compact, "ok", created.GetId())
	}
	term.Successf(cmd.OutOrStdout(), "connect: %s -> %s (id=%d)", from, to, created.GetId())
	term.Infof(cmd.OutOrStdout(), "connector view: %s", view)
	printVisibility(cmd, ws, spec)
	return nil
}

func writeConnectJSON(cmd *cobra.Command, ws *workspace.Workspace, spec *workspace.Connector, compact bool, status string, id int32) error {
	return cmdutil.WriteJSON(cmd.OutOrStdout(), compact, cmdutil.JSONOutput{Command: "connect", Status: status, DryRun: status == "dry-run", Items: []cmdutil.JSONItem{{Ref: workspace.ConnectorKey(spec), ResourceType: "connector", Action: "connect", ID: id}}, Extra: map[string]any{"visibility": ws.DiagnoseConnector(spec)}})
}

func printVisibility(cmd *cobra.Command, ws *workspace.Workspace, spec *workspace.Connector) {
	d := ws.DiagnoseConnector(spec)
	term.Infof(cmd.OutOrStdout(), "visibility: %s", d.Description())
	if d.Mode == "off-view" {
		term.Info(cmd.OutOrStdout(), workspace.OffViewHint)
	}
}

// Match against the target, not just cached metadata: a retry after a failed
// cache write must not create another copy of the same relationship.
func ensureConnector(ctx context.Context, cmd *cobra.Command, runner exec.Runner, input api.ConnectorInput) (*diagv1.Connector, error) {
	connectors, err := runner.ListConnectors(ctx)
	if err != nil {
		return nil, err
	}
	var match *diagv1.Connector
	label := ""
	if input.Label != nil {
		label = *input.Label
	}
	for _, c := range connectors {
		if c.GetViewId() != input.ViewID || c.GetSourceElementId() != input.SourceID || c.GetTargetElementId() != input.TargetID || c.GetLabel() != label {
			continue
		}
		if match != nil {
			return nil, cmdutil.WithHint(fmt.Errorf("multiple connectors match this relationship"), "Run 'tld list connectors' and remove duplicate connector IDs before retrying.")
		}
		match = c
	}
	if match != nil {
		if !cmd.Flags().Changed("direction") {
			input.Direction = match.GetDirection()
		}
		if !cmd.Flags().Changed("style") {
			input.Style = match.GetStyle()
		}
		if !cmd.Flags().Changed("tags") {
			input.Tags = match.GetTags()
		}
		return runner.UpdateConnector(ctx, match.GetId(), input)
	}
	return runner.CreateConnector(ctx, input)
}

func optStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func validateConnectorRefs(from, to, view string) error {
	if err := workspace.ValidateElementRef(from); err != nil {
		return fmt.Errorf("invalid --from: %w", err)
	}
	if err := workspace.ValidateElementRef(to); err != nil {
		return fmt.Errorf("invalid --to: %w", err)
	}
	if view != "" {
		if err := workspace.ValidateParentRef(view); err != nil {
			return fmt.Errorf("invalid --view: %w", err)
		}
	}
	return nil
}

func validateConnectorViewExists(ws *workspace.Workspace, view string) error {
	if view == workspace.RootRef {
		return nil
	}
	if ws == nil {
		return fmt.Errorf("workspace is required")
	}
	if _, ok := ws.Elements[view]; !ok {
		return fmt.Errorf("view ref %q not found", view)
	}
	return nil
}

func validateConnectorEndpointsExist(ws *workspace.Workspace, from, to string) error {
	if ws == nil {
		return fmt.Errorf("workspace is required")
	}
	if _, ok := ws.Elements[from]; !ok {
		return fmt.Errorf("source element %q not found", from)
	}
	if _, ok := ws.Elements[to]; !ok {
		return fmt.Errorf("target element %q not found", to)
	}
	return nil
}

func inferConnectorView(ws *workspace.Workspace, from, to string) (string, error) {
	if ws == nil {
		return "", fmt.Errorf("workspace is required")
	}
	fromElement, ok := ws.Elements[from]
	if !ok {
		return "", fmt.Errorf("source element %q not found", from)
	}
	return parentOfElement(fromElement), nil
}

// targetIsFresh reports whether ref has no connectors in view yet. A fresh
// target was almost certainly just added and is safe to auto-place.
func targetIsFresh(ws *workspace.Workspace, view, ref string) bool {
	if ws == nil || ref == "" {
		return false
	}
	normalizedView := cmdutil.NormalizeParentRef(view)
	for _, connector := range ws.Connectors {
		if connector == nil {
			continue
		}
		if cmdutil.NormalizeParentRef(connector.View) != normalizedView {
			continue
		}
		if connector.Source == ref || connector.Target == ref {
			return false
		}
	}
	return true
}

func parentOfElement(element *workspace.Element) string {
	if element == nil || len(element.Placements) == 0 {
		return workspace.RootRef
	}
	best := workspace.RootRef
	for _, placement := range element.Placements {
		candidate := normalizeParentRef(placement.ParentRef)
		if candidate != workspace.RootRef {
			best = candidate
			break
		}
	}
	return best
}

func normalizeParentRef(ref string) string {
	if ref == "" {
		return workspace.RootRef
	}
	return ref
}

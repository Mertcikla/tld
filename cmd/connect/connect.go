package connect

import (
	"fmt"
	"strings"

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
		legacyView   string
		target       string
		dataDir      string
	)

	c := &cobra.Command{
		Use:   "connect",
		Short: "Add a connector between two elements",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := validateConnectorRefs(from, to, legacyView); err != nil {
				return err
			}
			ws, err := workspace.Load(*wdir)
			if err != nil {
				return fmt.Errorf("load workspace: %w", err)
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
			} else if err := validateConnectorViewExists(ws, view); err != nil {
				return err
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
			}
			if dryRun {
				if err := cmdutil.WithWorkspaceDryRun(*wdir, func(cloneDir string) error {
					return workspace.AppendConnector(cloneDir, spec)
				}); err != nil {
					if cmdutil.WantsJSON(*format) {
						return cmdutil.WriteCommandError(cmd.OutOrStdout(), *compact, "connect", err)
					}
					return fmt.Errorf("dry-run connect: %w", err)
				}
				if cmdutil.WantsJSON(*format) {
					return cmdutil.WriteMutation(cmd.OutOrStdout(), *compact, "connect", "dry-run", fmt.Sprintf("%s:%s", from, to))
				}
				term.Successf(cmd.OutOrStdout(), "dry-run: connect: %s -> %s", from, to)
				term.Infof(cmd.OutOrStdout(), "connector view: %s", view)
				return nil
			}
			return runConnect(cmd, *wdir, *format, *compact, target, dataDir, spec, from, to, view)
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
	c.Flags().StringVar(&legacyView, "view", "", "explicit connector view ref (default: source element's view)")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "preview the change without writing files")
	c.Flags().StringVar(&target, "target", "", "sync target: auto, local, or remote")
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
func runConnect(cmd *cobra.Command, wdir, format string, compact bool, target, dataDir string, spec *workspace.Connector, from, to, view string) error {
	fail := func(err error) error {
		if cmdutil.WantsJSON(format) {
			return cmdutil.WriteCommandError(cmd.OutOrStdout(), compact, "connect", err)
		}
		return err
	}
	ws, err := cmdutil.LoadWorkspace(wdir)
	if err != nil {
		return fail(err)
	}
	runner, err := exec.NewRunner(ws.Config, target, dataDir, false)
	if err != nil {
		return fail(err)
	}
	defer func() { _ = runner.Close() }()
	if runner.Name() == exec.TargetRemote {
		if err := cmdutil.EnsureAPIKey(ws.Config.APIKey); err != nil {
			return fail(err)
		}
	}
	ctx := cmd.Context()
	sourceID, err := exec.EnsureElementID(ctx, runner, ws, wdir, from)
	if err != nil {
		return fail(err)
	}
	targetID, err := exec.EnsureElementID(ctx, runner, ws, wdir, to)
	if err != nil {
		return fail(err)
	}
	viewID, err := exec.ResolveParentViewID(ctx, runner, ws, wdir, view)
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
	created, err := runner.CreateConnector(ctx, api.ConnectorInput{
		ViewID:       viewID,
		SourceID:     sourceID,
		TargetID:     targetID,
		Label:        optStr(spec.Label),
		Description:  optStr(spec.Description),
		Relationship: optStr(spec.Relationship),
		Direction:    direction,
		Style:        style,
		URL:          optStr(spec.URL),
	})
	if err != nil {
		return fail(cmdutil.WithUnauthorizedHint("server create connector failed", err))
	}
	if err := workspace.AppendConnector(wdir, spec); err != nil {
		return fail(fmt.Errorf("update YAML cache: %w", err))
	}
	if err := exec.RecordConnectorMeta(wdir, workspace.ConnectorKey(spec), created); err != nil {
		return fail(fmt.Errorf("update cache metadata: %w", err))
	}
	if cmdutil.WantsJSON(format) {
		return cmdutil.WriteMutation(cmd.OutOrStdout(), compact, "connect", "connect", fmt.Sprintf("%s:%s", from, to))
	}
	term.Successf(cmd.OutOrStdout(), "ok (id=%d)", created.GetId())
	term.Infof(cmd.OutOrStdout(), "connector view: %s", view)
	return nil
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

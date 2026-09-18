package update

import (
	"context"
	"fmt"
	"strconv"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/mertcikla/tld/v2/internal/cmdutil"
	"github.com/mertcikla/tld/v2/internal/completion"
	"github.com/mertcikla/tld/v2/internal/exec"
	"github.com/mertcikla/tld/v2/internal/planner"
	"github.com/mertcikla/tld/v2/internal/term"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/mertcikla/tld/v2/pkg/api"
	"github.com/spf13/cobra"
)

func NewUpdateCmd(wdir, format *string, compact *bool) *cobra.Command {
	c := &cobra.Command{
		Use:   "update",
		Short: "Update a resource field with a value",
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
	var target string
	var dataDir string
	c := &cobra.Command{
		Use:   "element <ref> <field> <value>",
		Short: "Update an element field",
		Args:  cobra.ExactArgs(3),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			switch len(args) {
			case 0:
				return completion.ElementRefs(wdir)
			case 1:
				return completion.ElementFields(), cobra.ShellCompDirectiveNoFileComp
			default:
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, field, value := args[0], args[1], args[2]
			if err := workspace.UpdateElementField(*wdir, ref, field, value); err != nil {
				if cmdutil.WantsJSON(*format) {
					return cmdutil.WriteCommandError(cmd.OutOrStdout(), *compact, "update element", err)
				}
				return fmt.Errorf("update element: %w", err)
			}
			if err := runUpdateElementServer(cmd, *wdir, target, dataDir, ref, field, value); err != nil {
				if cmdutil.WantsJSON(*format) {
					return cmdutil.WriteCommandError(cmd.OutOrStdout(), *compact, "update element", err)
				}
				return err
			}
			if cmdutil.WantsJSON(*format) {
				return cmdutil.WriteMutation(cmd.OutOrStdout(), *compact, "update element", "update", ref)
			}
			term.Successf(cmd.OutOrStdout(), "updated %q: %s=%q", ref, field, value)
			return nil
		},
	}
	c.Flags().StringVar(&target, "target", "", "sync target: auto, local, or remote")
	c.Flags().StringVar(&dataDir, "data-dir", "", "data directory for local target state")
	return c
}

func newConnectorCmd(wdir, format *string, compact *bool) *cobra.Command {
	var target string
	var dataDir string
	c := &cobra.Command{
		Use:   "connector <ref> <field> <value>",
		Short: "Update a connector field",
		Args:  cobra.ExactArgs(3),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			switch len(args) {
			case 0:
				return completion.ConnectorKeys(wdir)
			case 1:
				return completion.ConnectorFields(), cobra.ShellCompDirectiveNoFileComp
			case 2:
				if args[1] == "direction" {
					return completion.ConnectorDirections(), cobra.ShellCompDirectiveNoFileComp
				}
				return nil, cobra.ShellCompDirectiveNoFileComp
			default:
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			ref, field, value := args[0], args[1], args[2]
			// Capture the pre-update spec for server ID resolution (the key may
			// change when view/source/target/label are updated).
			preWS, err := cmdutil.LoadWorkspace(*wdir)
			if err != nil {
				if cmdutil.WantsJSON(*format) {
					return cmdutil.WriteCommandError(cmd.OutOrStdout(), *compact, "update connector", err)
				}
				return err
			}
			preSpec := preWS.Connectors[ref]
			var preID int32
			if preWS.Meta != nil {
				if m, ok := preWS.Meta.Connectors[ref]; ok && m != nil {
					preID = int32(m.ID)
				}
			}
			if err := workspace.UpdateConnectorField(*wdir, ref, field, value); err != nil {
				if cmdutil.WantsJSON(*format) {
					return cmdutil.WriteCommandError(cmd.OutOrStdout(), *compact, "update connector", err)
				}
				return fmt.Errorf("update connector: %w", err)
			}
			if err := runUpdateConnectorServer(cmd, *wdir, target, dataDir, ref, field, value, preSpec, preID); err != nil {
				if cmdutil.WantsJSON(*format) {
					return cmdutil.WriteCommandError(cmd.OutOrStdout(), *compact, "update connector", err)
				}
				return err
			}
			if cmdutil.WantsJSON(*format) {
				return cmdutil.WriteMutation(cmd.OutOrStdout(), *compact, "update connector", "update", ref)
			}
			term.Successf(cmd.OutOrStdout(), "updated %q: %s=%q", ref, field, value)
			return nil
		},
	}
	c.Flags().StringVar(&target, "target", "", "sync target: auto, local, or remote")
	c.Flags().StringVar(&dataDir, "data-dir", "", "data directory for local target state")
	return c
}

// serverSyncedElementFields are YAML element fields mirrored to the server.
func serverSyncedElementFields(field string) bool {
	switch field {
	case "name", "kind", "description", "technology", "url", "logo_url",
		"repo", "branch", "language", "file_path", "view_label", "view_name",
		"bypass_noise_gate":
		return true
	default:
		return false
	}
}

// runUpdateElementServer mirrors a YAML element field change to the server.
// YAML-only metadata fields (owner, symbol, density, etc.) skip the server call.
func runUpdateElementServer(cmd *cobra.Command, wdir, target, dataDir, ref, field, value string) error {
	if field == "ref" {
		// Rename is a local-alias change; the server ID is unchanged.
		if err := workspace.RenameCurrentElementMetadata(wdir, ref, value); err != nil {
			return fmt.Errorf("update rename metadata: %w", err)
		}
		if err := workspace.RenameCurrentViewMetadata(wdir, ref, value); err != nil {
			return fmt.Errorf("update rename metadata: %w", err)
		}
		return nil
	}
	if !serverSyncedElementFields(field) {
		return nil
	}
	ws, err := cmdutil.LoadWorkspace(wdir)
	if err != nil {
		return err
	}
	el := ws.Elements[ref]
	if el == nil {
		return fmt.Errorf("element %q not found", ref)
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
	elementID, err := exec.EnsureElementID(ctx, runner, ws, wdir, ref)
	if err != nil {
		return err
	}
	switch field {
	case "view_label", "view_name":
		// View display fields live on the owned view.
		viewID, err := exec.ResolveParentViewID(ctx, runner, ws, wdir, ref)
		if err != nil {
			// No view yet: promote the element, then set the name.
			viewID, err = exec.EnsureElementView(ctx, runner, elementID, el.Name, &el.ViewLabel)
			if err != nil {
				return fmt.Errorf("ensure element view: %w", err)
			}
		}
		name := el.ViewName
		if name == "" {
			name = el.Name
		}
		if _, err := runner.UpdateView(ctx, viewID, name); err != nil {
			return cmdutil.WithUnauthorizedHint("server update view failed", err)
		}
		return exec.RecordElementMeta(wdir, ref, mustElement(ctx, runner, elementID), viewID, nil)
	default:
		bypass := true
		existing, err := runner.GetElement(ctx, elementID)
		if err != nil {
			return cmdutil.WithUnauthorizedHint("server get element failed", err)
		}
		input := api.ElementInput{
			Name:            existing.GetName(),
			Description:     optStrFromProto(existing.Description),
			Kind:            optStrFromProto(existing.Kind),
			Technology:      optStrFromProto(existing.Technology),
			URL:             optStrFromProto(existing.Url),
			LogoURL:         optStrFromProto(existing.LogoUrl),
			TechLinks:       existing.TechnologyLinks,
			Tags:            existing.Tags,
			Repo:            optStrFromProto(existing.Repo),
			Branch:          optStrFromProto(existing.Branch),
			Language:        optStrFromProto(existing.Language),
			FilePath:        optStrFromProto(existing.FilePath),
			BypassNoiseGate: &bypass,
			HasView:         existing.HasView,
			ViewLabel:       optStrFromProto(existing.ViewLabel),
		}
		applyElementField(&input, el, field, value)
		updated, err := runner.UpdateElement(ctx, elementID, input)
		if err != nil {
			return cmdutil.WithUnauthorizedHint("server update element failed", err)
		}
		var viewID int32
		if ws.Meta != nil {
			if m, ok := ws.Meta.Views[ref]; ok && m != nil {
				viewID = int32(m.ID)
			}
		}
		return exec.RecordElementMeta(wdir, ref, updated, viewID, nil)
	}
}

func mustElement(ctx context.Context, runner exec.Runner, id int32) *diagv1.Element {
	el, _ := runner.GetElement(ctx, id)
	return el
}

func optStrFromProto(s *string) *string {
	if s == nil || *s == "" {
		return nil
	}
	return s
}

func strOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func runCreateConnectorFromSpec(ctx context.Context, cmd *cobra.Command, ws *workspace.Workspace, runner exec.Runner, wdir string, spec *workspace.Connector) error {
	sourceID, err := exec.EnsureElementID(ctx, runner, ws, wdir, spec.Source)
	if err != nil {
		return err
	}
	targetID, err := exec.EnsureElementID(ctx, runner, ws, wdir, spec.Target)
	if err != nil {
		return err
	}
	viewID, err := exec.ResolveParentViewID(ctx, runner, ws, wdir, spec.View)
	if err != nil {
		return fmt.Errorf("resolve connector view: %w", err)
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
		Label:        strOrNil(spec.Label),
		Description:  strOrNil(spec.Description),
		Relationship: strOrNil(spec.Relationship),
		Direction:    direction,
		Style:        style,
		URL:          strOrNil(spec.URL),
		SourceHandle: strOrNil(spec.SourceHandle),
		TargetHandle: strOrNil(spec.TargetHandle),
	})
	if err != nil {
		return cmdutil.WithUnauthorizedHint("server create connector failed", err)
	}
	return exec.RecordConnectorMeta(wdir, workspace.ConnectorKey(spec), created)
}

func applyElementField(input *api.ElementInput, el *workspace.Element, field, value string) {
	switch field {
	case "name":
		input.Name = el.Name
	case "kind":
		input.Kind = strOrNil(el.Kind)
	case "description":
		input.Description = strOrNil(el.Description)
	case "technology":
		input.Technology = strOrNil(el.Technology)
		input.TechLinks = planner.TechnologyLinksForElement(el.Technology, el.Language)
	case "url":
		input.URL = strOrNil(el.URL)
	case "logo_url":
		input.LogoURL = strOrNil(el.LogoURL)
	case "repo":
		input.Repo = strOrNil(el.Repo)
	case "branch":
		input.Branch = strOrNil(el.Branch)
	case "language":
		input.Language = strOrNil(el.Language)
	case "file_path":
		input.FilePath = strOrNil(el.FilePath)
	case "view_label":
		input.ViewLabel = strOrNil(el.ViewLabel)
	case "bypass_noise_gate":
		if parsed, err := strconv.ParseBool(value); err == nil {
			input.BypassNoiseGate = &parsed
		}
	}
}

// runUpdateConnectorServer mirrors a YAML connector field change to the server.
func runUpdateConnectorServer(cmd *cobra.Command, wdir, target, dataDir, ref, field, value string, preSpec *workspace.Connector, preID int32) error {
	ws, err := cmdutil.LoadWorkspace(wdir)
	if err != nil {
		return err
	}
	// The YAML update may have renamed the key; find the current spec.
	spec := ws.Connectors[ref]
	currentKey := ref
	if spec == nil {
		// Key changed: locate by matching the updated fields.
		for key, c := range ws.Connectors {
			if preSpec != nil && c.Source == preSpec.Source && c.Target == preSpec.Target {
				spec = c
				currentKey = key
				break
			}
		}
	}
	if spec == nil {
		return fmt.Errorf("connector %q not found after update", ref)
	}
	_ = value
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
	connectorID := preID
	if connectorID == 0 && ws.Meta != nil {
		if m, ok := ws.Meta.Connectors[currentKey]; ok && m != nil {
			connectorID = int32(m.ID)
		}
	}
	if connectorID == 0 {
		// No cached ID (hand-written YAML): fall back to create.
		return runCreateConnectorFromSpec(ctx, cmd, ws, runner, wdir, spec)
	}
	sourceID, err := exec.EnsureElementID(ctx, runner, ws, wdir, spec.Source)
	if err != nil {
		return err
	}
	targetID, err := exec.EnsureElementID(ctx, runner, ws, wdir, spec.Target)
	if err != nil {
		return err
	}
	viewID, err := exec.ResolveParentViewID(ctx, runner, ws, wdir, spec.View)
	if err != nil {
		return fmt.Errorf("resolve connector view: %w", err)
	}
	direction := spec.Direction
	if direction == "" {
		direction = "forward"
	}
	style := spec.Style
	if style == "" {
		style = "bezier"
	}
	updated, err := runner.UpdateConnector(ctx, connectorID, api.ConnectorInput{
		ViewID:       viewID,
		SourceID:     sourceID,
		TargetID:     targetID,
		Label:        strOrNil(spec.Label),
		Description:  strOrNil(spec.Description),
		Relationship: strOrNil(spec.Relationship),
		Direction:    direction,
		Style:        style,
		URL:          strOrNil(spec.URL),
		SourceHandle: strOrNil(spec.SourceHandle),
		TargetHandle: strOrNil(spec.TargetHandle),
	})
	if err != nil {
		return cmdutil.WithUnauthorizedHint("server update connector failed", err)
	}
	// Move the meta entry when the key changed.
	if currentKey != ref && ws.Meta != nil && ws.Meta.Connectors != nil {
		if m, ok := ws.Meta.Connectors[ref]; ok {
			ws.Meta.Connectors[currentKey] = m
			delete(ws.Meta.Connectors, ref)
		}
	}
	return exec.RecordConnectorMeta(wdir, currentKey, updated)
}

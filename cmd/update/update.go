package update

import (
	"context"
	"fmt"
	"strconv"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/mertcikla/tld/v2/internal/cmdutil"
	"github.com/mertcikla/tld/v2/internal/completion"
	"github.com/mertcikla/tld/v2/internal/exec"
	"github.com/mertcikla/tld/v2/internal/tech"
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
			fail := func(err error) error {
				if cmdutil.WantsJSON(*format) {
					return cmdutil.WriteCommandError(cmd.OutOrStdout(), *compact, "update element", err)
				}
				return err
			}
			switch {
			case field == "ref":
				// Rename is a local-alias change; the server ID is unchanged.
				if err := workspace.UpdateElementField(*wdir, ref, field, value); err != nil {
					return fail(fmt.Errorf("update element: %w", err))
				}
				if err := renameElementMetadata(*wdir, ref, value); err != nil {
					return fail(err)
				}
			case !serverSyncedElementFields(field):
				// YAML-only metadata (owner, symbol, density, etc.).
				if err := workspace.UpdateElementField(*wdir, ref, field, value); err != nil {
					return fail(fmt.Errorf("update element: %w", err))
				}
			default:
				// Server first: a failed server write must not leave the YAML
				// cache claiming a change the server never saw.
				updated, viewID, err := runUpdateElementServer(cmd.Context(), *wdir, target, dataDir, ref, field, value)
				if err != nil {
					return fail(err)
				}
				if err := workspace.UpdateElementField(*wdir, ref, field, value); err != nil {
					return fail(fmt.Errorf("update YAML cache: %w", err))
				}
				if updated != nil {
					if err := exec.RecordElementMeta(*wdir, ref, updated, viewID, nil); err != nil {
						return fail(fmt.Errorf("update cache metadata: %w", err))
					}
				}
			}
			if cmdutil.WantsJSON(*format) {
				return cmdutil.WriteMutation(cmd.OutOrStdout(), *compact, "update element", "update", ref)
			}
			term.Successf(cmd.OutOrStdout(), "updated %q: %s=%q", ref, field, value)
			return nil
		},
	}
	c.Flags().StringVar(&target, "target", "", "sync target: auto, local, remote, or cloud")
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
			fail := func(err error) error {
				if cmdutil.WantsJSON(*format) {
					return cmdutil.WriteCommandError(cmd.OutOrStdout(), *compact, "update connector", err)
				}
				return err
			}
			// Validate before touching server state; the desired spec is
			// derived from the pre-update YAML so the key change is known.
			preWS, err := cmdutil.LoadWorkspace(*wdir)
			if err != nil {
				return fail(err)
			}
			if err := workspace.ValidateConnectorFieldChange(preWS, ref, field, value); err != nil {
				return fail(fmt.Errorf("update connector: %w", err))
			}
			preSpec := preWS.Connectors[ref]
			var preID int32
			if preWS.Meta != nil {
				if m, ok := preWS.Meta.Connectors[ref]; ok && m != nil {
					preID = int32(m.ID)
				}
			}
			// Server first: a failed server write must not leave the YAML
			// cache claiming a change the server never saw.
			updated, newKey, err := runUpdateConnectorServer(cmd.Context(), preWS, *wdir, target, dataDir, ref, field, value, preSpec, preID)
			if err != nil {
				return fail(err)
			}
			if err := workspace.UpdateConnectorField(*wdir, ref, field, value); err != nil {
				return fail(fmt.Errorf("update YAML cache: %w", err))
			}
			if updated != nil {
				if err := exec.RecordConnectorMeta(*wdir, newKey, updated); err != nil {
					return fail(fmt.Errorf("update cache metadata: %w", err))
				}
			}
			if cmdutil.WantsJSON(*format) {
				return cmdutil.WriteMutation(cmd.OutOrStdout(), *compact, "update connector", "update", ref)
			}
			term.Successf(cmd.OutOrStdout(), "updated %q: %s=%q", ref, field, value)
			return nil
		},
	}
	c.Flags().StringVar(&target, "target", "", "sync target: auto, local, remote, or cloud")
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

// renameElementMetadata moves lockfile metadata after a local ref rename.
func renameElementMetadata(wdir, ref, newRef string) error {
	if err := workspace.RenameCurrentElementMetadata(wdir, ref, newRef); err != nil {
		return fmt.Errorf("update rename metadata: %w", err)
	}
	if err := workspace.RenameCurrentViewMetadata(wdir, ref, newRef); err != nil {
		return fmt.Errorf("update rename metadata: %w", err)
	}
	return nil
}

// runUpdateElementServer mirrors a YAML element field change to the server and
// returns the updated element plus its owned view ID (0 when none). It only
// performs server calls: callers refresh the YAML cache afterwards.
func runUpdateElementServer(ctx context.Context, wdir, target, dataDir, ref, field, value string) (*diagv1.Element, int32, error) {
	ws, err := cmdutil.LoadWorkspace(wdir)
	if err != nil {
		return nil, 0, err
	}
	el := ws.Elements[ref]
	if el == nil {
		return nil, 0, fmt.Errorf("element %q not found", ref)
	}
	runner, err := exec.NewRunner(ws.Config, target, dataDir, false)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = runner.Close() }()
	if runner.Name() == exec.TargetRemote {
		if err := cmdutil.EnsureAPIKey(ws.Config.APIKey); err != nil {
			return nil, 0, err
		}
	}
	elementID, err := exec.EnsureElementID(ctx, runner, ws, wdir, ref)
	if err != nil {
		return nil, 0, err
	}
	switch field {
	case "view_label", "view_name":
		// View display fields live on the owned view. ResolveParentViewID
		// creates the view when the element has none and records it in the
		// YAML cache.
		viewID, err := exec.ResolveParentViewID(ctx, runner, ws, wdir, ref)
		if err != nil {
			return nil, 0, fmt.Errorf("ensure element view: %w", err)
		}
		name := el.ViewName
		if field == "view_name" {
			name = value
		}
		if name == "" {
			name = el.Name
		}
		var label *string
		if field == "view_label" {
			label = &value
		}
		if _, err := runner.UpdateView(ctx, viewID, name, label); err != nil {
			return nil, 0, cmdutil.WithUnauthorizedHint("server update view failed", err)
		}
		updatedElement, err := runner.GetElement(ctx, elementID)
		if err != nil {
			return nil, 0, cmdutil.WithUnauthorizedHint("server get element failed", err)
		}
		return updatedElement, viewID, nil
	default:
		bypass := true
		existing, err := runner.GetElement(ctx, elementID)
		if err != nil {
			return nil, 0, cmdutil.WithUnauthorizedHint("server get element failed", err)
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
			return nil, 0, cmdutil.WithUnauthorizedHint("server update element failed", err)
		}
		var viewID int32
		if ws.Meta != nil {
			if m, ok := ws.Meta.Views[ref]; ok && m != nil {
				viewID = int32(m.ID)
			}
		}
		return updated, viewID, nil
	}
}

func optStrFromProto(s *string) *string {
	if s == nil || *s == "" {
		return nil
	}
	return s
}

// runCreateConnectorFromSpec creates the connector described by spec on the
// server when no cached ID exists (hand-written YAML).
func runCreateConnectorFromSpec(ctx context.Context, ws *workspace.Workspace, runner exec.Runner, wdir string, spec *workspace.Connector) (*diagv1.Connector, error) {
	sourceID, err := exec.EnsureElementID(ctx, runner, ws, wdir, spec.Source)
	if err != nil {
		return nil, err
	}
	targetID, err := exec.EnsureElementID(ctx, runner, ws, wdir, spec.Target)
	if err != nil {
		return nil, err
	}
	viewID, err := exec.ResolveParentViewID(ctx, runner, ws, wdir, spec.View)
	if err != nil {
		return nil, fmt.Errorf("resolve connector view: %w", err)
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
		Label:        &spec.Label,
		Description:  &spec.Description,
		Relationship: &spec.Relationship,
		Direction:    direction,
		Style:        style,
		URL:          &spec.URL,
		SourceHandle: &spec.SourceHandle,
		TargetHandle: &spec.TargetHandle,
	})
	if err != nil {
		return nil, cmdutil.WithUnauthorizedHint("server create connector failed", err)
	}
	return created, nil
}

// applyElementField overrides the changed field on input. Values are set
// explicitly, including empty strings, so clearing a field in YAML clears it on
// the server instead of being treated as "leave unchanged".
func applyElementField(input *api.ElementInput, el *workspace.Element, field, value string) {
	switch field {
	case "name":
		input.Name = value
	case "kind":
		input.Kind = &value
	case "description":
		input.Description = &value
	case "technology":
		input.Technology = &value
		input.TechLinks = tech.TechnologyLinksForElement(value, el.Language)
	case "url":
		input.URL = &value
	case "logo_url":
		input.LogoURL = &value
	case "repo":
		input.Repo = &value
	case "branch":
		input.Branch = &value
	case "language":
		input.Language = &value
	case "file_path":
		input.FilePath = &value
	case "view_label":
		input.ViewLabel = &value
	case "bypass_noise_gate":
		if parsed, err := strconv.ParseBool(value); err == nil {
			input.BypassNoiseGate = &parsed
		}
	}
}

// runUpdateConnectorServer mirrors a connector field change to the server. The
// desired spec is derived from the pre-update YAML plus the field change so the
// renamed YAML key is known before the local cache is written. It returns the
// updated connector and its new key; callers refresh the YAML cache afterwards.
func runUpdateConnectorServer(ctx context.Context, ws *workspace.Workspace, wdir, target, dataDir, ref, field, value string, preSpec *workspace.Connector, preID int32) (*diagv1.Connector, string, error) {
	if preSpec == nil {
		return nil, ref, fmt.Errorf("connector %q not found", ref)
	}
	spec := *preSpec
	workspace.ApplyConnectorField(&spec, field, value)
	currentKey := workspace.ConnectorKey(&spec)

	runner, err := exec.NewRunner(ws.Config, target, dataDir, false)
	if err != nil {
		return nil, currentKey, err
	}
	defer func() { _ = runner.Close() }()
	if runner.Name() == exec.TargetRemote {
		if err := cmdutil.EnsureAPIKey(ws.Config.APIKey); err != nil {
			return nil, currentKey, err
		}
	}
	connectorID := preID
	if connectorID == 0 && ws.Meta != nil {
		if m, ok := ws.Meta.Connectors[ref]; ok && m != nil {
			connectorID = int32(m.ID)
		}
	}
	if connectorID == 0 {
		// No cached ID (hand-written YAML): fall back to create.
		created, err := runCreateConnectorFromSpec(ctx, ws, runner, wdir, &spec)
		return created, currentKey, err
	}
	sourceID, err := exec.EnsureElementID(ctx, runner, ws, wdir, spec.Source)
	if err != nil {
		return nil, currentKey, err
	}
	targetID, err := exec.EnsureElementID(ctx, runner, ws, wdir, spec.Target)
	if err != nil {
		return nil, currentKey, err
	}
	viewID, err := exec.ResolveParentViewID(ctx, runner, ws, wdir, spec.View)
	if err != nil {
		return nil, currentKey, fmt.Errorf("resolve connector view: %w", err)
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
		Label:        &spec.Label,
		Description:  &spec.Description,
		Relationship: &spec.Relationship,
		Direction:    direction,
		Style:        style,
		URL:          &spec.URL,
		SourceHandle: &spec.SourceHandle,
		TargetHandle: &spec.TargetHandle,
	})
	if err != nil {
		return nil, currentKey, cmdutil.WithUnauthorizedHint("server update connector failed", err)
	}
	return updated, currentKey, nil
}

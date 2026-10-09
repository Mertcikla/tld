package update

import (
	"context"
	"fmt"
	"slices"
	"strings"

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
			appendMode, _ := cmd.Flags().GetBool("append")
			removeMode, _ := cmd.Flags().GetBool("remove")
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			mode, err := tagUpdateMode(field, appendMode, removeMode)
			if err != nil {
				return fail(err)
			}
			if !serverSyncedElementFields(field) {
				return fail(unsupportedElementFieldError(ref, field))
			}
			sess, err := cmdutil.OpenSession(cmd, *wdir, target, dataDir)
			if err != nil {
				return fail(err)
			}
			defer func() { _ = sess.Close() }()
			ws, err := sess.LoadWorkspace()
			if err != nil {
				return fail(err)
			}
			if ref, err = cmdutil.ResolveElementArg(ws, ref); err != nil {
				return fail(err)
			}
			if mode != "" {
				el := ws.Elements[ref]
				if el == nil {
					return fail(fmt.Errorf("element %q not found", ref))
				}
				value = resolveTagValue(el.Tags, value, mode)
			}
			if dryRun {
				if sess.HasWorkspace() {
					if err := cmdutil.WithWorkspaceDryRun(*wdir, func(cloneDir string) error {
						return workspace.UpdateElementField(cloneDir, ref, field, value)
					}); err != nil {
						return fail(fmt.Errorf("dry-run update element: %w", err))
					}
				}
				if cmdutil.WantsJSON(*format) {
					return cmdutil.WriteMutation(cmd.OutOrStdout(), *compact, "update element", "dry-run", ref)
				}
				term.Successf(cmd.OutOrStdout(), "dry-run: update %q: %s=%q", ref, field, value)
				return nil
			}
			// Target first: a failed target write must not leave the YAML
			// cache claiming a change the target never saw.
			updated, viewID, err := runUpdateElementServer(cmd, sess, ws, ref, field, value)
			if err != nil {
				return fail(err)
			}
			if sess.HasWorkspace() {
				if err := workspace.UpdateElementField(*wdir, ref, field, value); err != nil {
					return fail(fmt.Errorf("update YAML cache: %w", err))
				}
				if updated != nil {
					if err := exec.RecordElementMeta(sess.Context(cmd.Context()), *wdir, ref, updated, viewID, nil); err != nil {
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
	c.Flags().Bool("append", false, "with tags: add to existing tags instead of replacing them")
	c.Flags().Bool("remove", false, "with tags: remove the given tags")
	c.Flags().Bool("dry-run", false, "preview the change without writing files or calling the server")
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
			appendMode, _ := cmd.Flags().GetBool("append")
			removeMode, _ := cmd.Flags().GetBool("remove")
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			mode, err := tagUpdateMode(field, appendMode, removeMode)
			if err != nil {
				return fail(err)
			}
			sess, err := cmdutil.OpenSession(cmd, *wdir, target, dataDir)
			if err != nil {
				return fail(err)
			}
			defer func() { _ = sess.Close() }()
			ws, err := sess.LoadWorkspace()
			if err != nil {
				return fail(err)
			}
			ref, err = cmdutil.ResolveConnectorArg(ws, ref)
			if err != nil {
				return fail(err)
			}
			if dryRun {
				if sess.HasWorkspace() {
					if err := cmdutil.WithWorkspaceDryRun(*wdir, func(cloneDir string) error {
						return workspace.UpdateConnectorField(cloneDir, ref, field, value)
					}); err != nil {
						return fail(fmt.Errorf("dry-run update connector: %w", err))
					}
				}
				if cmdutil.WantsJSON(*format) {
					return cmdutil.WriteMutation(cmd.OutOrStdout(), *compact, "update connector", "dry-run", ref)
				}
				term.Successf(cmd.OutOrStdout(), "dry-run: update %q: %s=%q", ref, field, value)
				return nil
			}
			// Validate before touching server state; the desired spec is
			// derived from the pre-update state so the key change is known.
			if mode != "" {
				connector := ws.Connectors[ref]
				if connector == nil {
					return fail(fmt.Errorf("connector %q not found", ref))
				}
				value = resolveTagValue(connector.Tags, value, mode)
			}
			if err := workspace.ValidateConnectorFieldChange(ws, ref, field, value); err != nil {
				return fail(fmt.Errorf("update connector: %w", err))
			}
			preSpec := ws.Connectors[ref]
			var preID int32
			if ws.Meta != nil {
				if m, ok := ws.Meta.Connectors[ref]; ok && m != nil {
					preID = int32(m.ID)
				}
			}
			// Server first: a failed server write must not leave the YAML
			// cache claiming a change the server never saw.
			updated, newKey, err := runUpdateConnectorServer(cmd, sess, ws, ref, field, value, preSpec, preID)
			if err != nil {
				return fail(err)
			}
			if sess.HasWorkspace() {
				if err := workspace.UpdateConnectorField(*wdir, ref, field, value); err != nil {
					return fail(fmt.Errorf("update YAML cache: %w", err))
				}
				if updated != nil {
					if err := exec.RecordConnectorMeta(sess.Context(cmd.Context()), *wdir, newKey, updated); err != nil {
						return fail(fmt.Errorf("update cache metadata: %w", err))
					}
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
	c.Flags().Bool("append", false, "with tags: add to existing tags instead of replacing them")
	c.Flags().Bool("remove", false, "with tags: remove the given tags")
	c.Flags().Bool("dry-run", false, "preview the change without writing files or calling the server")
	return c
}

// tagUpdateMode validates the --append/--remove flags and returns "append",
// "remove", or "" (plain replace). Flags are only valid for the tags field.
func tagUpdateMode(field string, appendMode, removeMode bool) (string, error) {
	if appendMode && removeMode {
		return "", fmt.Errorf("--append and --remove are mutually exclusive")
	}
	if (appendMode || removeMode) && field != "tags" {
		return "", fmt.Errorf("--append/--remove are only valid with the tags field")
	}
	switch {
	case appendMode:
		return "append", nil
	case removeMode:
		return "remove", nil
	default:
		return "", nil
	}
}

// resolveTagValue rewrites a tags value string using the given mode against the
// current tags so the existing field-update path can apply it verbatim.
func resolveTagValue(current []string, value, mode string) string {
	switch mode {
	case "append":
		return strings.Join(workspace.UnionTags(current, workspace.ParseTagList(value)), ",")
	case "remove":
		return strings.Join(workspace.SubtractTags(current, workspace.ParseTagList(value)), ",")
	default:
		return value
	}
}

// serverSyncedElementFields reports whether `update element` accepts the field.
// Every accepted field mirrors to the target store.
func serverSyncedElementFields(field string) bool {
	return slices.Contains(completion.ElementFields(), field)
}

// unsupportedElementFieldError explains why a field is not accepted. Fields
// without a database column (owner, symbol, has_view, density_level) are only
// editable directly in workspace YAML, and ref renames go through `tld rename`.
func unsupportedElementFieldError(ref, field string) error {
	switch field {
	case "ref":
		return fmt.Errorf(`"ref" cannot be updated in place; use 'tld rename --from %s --to <new-ref>'`, ref)
	case "owner", "symbol", "has_view", "density_level":
		return fmt.Errorf("field %q is not supported by 'update element'; edit the workspace YAML directly", field)
	default:
		return fmt.Errorf("unknown element field %q; known fields: %s", field, strings.Join(completion.ElementFields(), ", "))
	}
}

// runUpdateElementServer mirrors a YAML element field change to the server and
// returns the updated element plus its owned view ID (0 when none). In DB mode
// it is the only write: no cache refresh follows.
func runUpdateElementServer(cmd *cobra.Command, sess *cmdutil.Session, ws *workspace.Workspace, ref, field, value string) (*diagv1.Element, int32, error) {
	el := ws.Elements[ref]
	if el == nil {
		return nil, 0, fmt.Errorf("element %q not found", ref)
	}
	runner, err := sess.Runner()
	if err != nil {
		return nil, 0, err
	}
	ctx := sess.Context(cmd.Context())
	elementID, err := exec.EnsureElementID(ctx, runner, ws, sess.Wdir, ref)
	if err != nil {
		return nil, 0, err
	}
	switch field {
	case "view_label", "view_name":
		// View display fields live on the owned view. ResolveParentViewID
		// creates the view when the element has none and records it in the
		// YAML cache.
		viewID, err := exec.ResolveParentViewID(ctx, runner, ws, sess.Wdir, ref)
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
		existing, err := runner.GetElement(ctx, elementID)
		if err != nil {
			return nil, 0, cmdutil.WithUnauthorizedHint("server get element failed", err)
		}
		bypass := existing.GetBypassNoiseGate()
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
			RepositoryID:    optStrFromProto(existing.RepositoryId),
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
		Tags:         spec.Tags,
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
	case "tags":
		input.Tags = workspace.ParseTagList(value)
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
	case "repository_id":
		input.RepositoryID = &value
	case "branch":
		input.Branch = &value
	case "language":
		input.Language = &value
	case "file_path":
		input.FilePath = &value
	case "view_label":
		input.ViewLabel = &value
	}
}

// runUpdateConnectorServer mirrors a connector field change to the server. The
// desired spec is derived from the pre-update state plus the field change so
// the renamed key is known before the local cache is written. It returns the
// updated connector and its new key; callers refresh the YAML cache in
// workspace mode.
func runUpdateConnectorServer(cmd *cobra.Command, sess *cmdutil.Session, ws *workspace.Workspace, ref, field, value string, preSpec *workspace.Connector, preID int32) (*diagv1.Connector, string, error) {
	if preSpec == nil {
		return nil, ref, fmt.Errorf("connector %q not found", ref)
	}
	spec := *preSpec
	workspace.ApplyConnectorField(&spec, field, value)
	currentKey := workspace.ConnectorKey(&spec)

	runner, err := sess.Runner()
	if err != nil {
		return nil, currentKey, err
	}
	ctx := sess.Context(cmd.Context())
	connectorID := preID
	if connectorID == 0 && ws.Meta != nil {
		if m, ok := ws.Meta.Connectors[ref]; ok && m != nil {
			connectorID = int32(m.ID)
		}
	}
	if connectorID == 0 {
		// No cached ID (hand-written YAML): fall back to create.
		created, err := runCreateConnectorFromSpec(ctx, ws, runner, sess.Wdir, &spec)
		return created, currentKey, err
	}
	sourceID, err := exec.EnsureElementID(ctx, runner, ws, sess.Wdir, spec.Source)
	if err != nil {
		return nil, currentKey, err
	}
	targetID, err := exec.EnsureElementID(ctx, runner, ws, sess.Wdir, spec.Target)
	if err != nil {
		return nil, currentKey, err
	}
	viewID, err := exec.ResolveParentViewID(ctx, runner, ws, sess.Wdir, spec.View)
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
	// Prefer the YAML-spec tags so `update connector ... tags` applies. When the
	// YAML spec has no tags (hand-written or not yet pulled), carry the server's
	// existing tags through to avoid clearing them on unrelated updates.
	existing, err := runner.GetConnector(ctx, connectorID)
	if err != nil {
		return nil, currentKey, cmdutil.WithUnauthorizedHint("server get connector failed", err)
	}
	tags := existing.GetTags()
	if spec.Tags != nil {
		tags = spec.Tags
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
		Tags:         tags,
	})
	if err != nil {
		return nil, currentKey, cmdutil.WithUnauthorizedHint("server update connector failed", err)
	}
	return updated, currentKey, nil
}

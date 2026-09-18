package add

import (
	"fmt"
	"strings"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/mertcikla/tld/v2/internal/cmdutil"
	"github.com/mertcikla/tld/v2/internal/completion"
	"github.com/mertcikla/tld/v2/internal/exec"
	"github.com/mertcikla/tld/v2/internal/planner"
	"github.com/mertcikla/tld/v2/internal/tech"
	"github.com/mertcikla/tld/v2/internal/term"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/mertcikla/tld/v2/pkg/api"
	"github.com/spf13/cobra"
)

func NewAddCmd(wdir, format *string, compact *bool) *cobra.Command {
	var (
		description     string
		technology      string
		dryRun          bool
		url             string
		positionX       float64
		positionY       float64
		ref             string
		kind            string
		parent          string
		diagramLabel    string
		legacyViewLabel string
		legacyWithView  bool
		target          string
		dataDir         string
	)

	c := &cobra.Command{
		Use:   "add <name>",
		Short: "Add or update an element in elements.yaml",
		Args:  cobra.ExactArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) != 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return completion.ElementRefsWithNames(wdir)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			normalizedKind, err := validateKind(kind)
			if err != nil {
				return err
			}
			kind = normalizedKind
			r := ref
			if r == "" {
				r = workspace.Slugify(name)
				if r == "" {
					return fmt.Errorf("could not generate a valid reference from name %q. please provide an explicit reference with --ref (e.g. --ref my-element)", name)
				}
			}
			if err := workspace.ValidateElementRef(r); err != nil {
				return err
			}
			placementParent := parent
			if placementParent == "" {
				placementParent = "root"
			}
			if err := workspace.ValidateParentRef(placementParent); err != nil {
				return err
			}
			if placementParent != workspace.RootRef {
				ws, err := workspace.Load(*wdir)
				if err != nil {
					return fmt.Errorf("load workspace: %w", err)
				}
				if _, ok := ws.Elements[placementParent]; !ok {
					return fmt.Errorf("parent ref %q not found", placementParent)
				}
			}
			if diagramLabel == "" {
				diagramLabel = legacyViewLabel
			}
			_ = legacyWithView
			normalizedTechnology, wasNormalized := normalizeTechnology(technology)
			spec := &workspace.Element{
				Name:        name,
				Kind:        kind,
				Description: description,
				Technology:  normalizedTechnology,
				URL:         url,
				HasView:     false,
				ViewLabel:   diagramLabel,
				Placements: []workspace.ViewPlacement{{
					ParentRef: placementParent,
					PositionX: positionX,
					PositionY: positionY,
				}},
			}
			validateAndWarnTechnology(cmd, technology)
			if dryRun {
				if err := cmdutil.WithWorkspaceDryRun(*wdir, func(cloneDir string) error {
					return workspace.UpsertElement(cloneDir, r, spec)
				}); err != nil {
					if cmdutil.WantsJSON(*format) {
						return cmdutil.WriteCommandError(cmd.OutOrStdout(), *compact, "add", err)
					}
					return fmt.Errorf("dry-run add element: %w", err)
				}
				if cmdutil.WantsJSON(*format) {
					return cmdutil.WriteMutation(cmd.OutOrStdout(), *compact, "add", "dry-run", r)
				}
				term.Successf(cmd.OutOrStdout(), "dry-run: add: %s", r)
				term.Infof(cmd.OutOrStdout(), "kind=%s parent=%s", kind, placementParent)
				if wasNormalized {
					term.Infof(cmd.OutOrStdout(), "technology normalized: %q -> %q", technology, normalizedTechnology)
				}
				return nil
			}
			return runAdd(cmd, *wdir, *format, *compact, target, dataDir, r, spec, kind, placementParent, wasNormalized, technology, normalizedTechnology)
		},
	}

	c.Flags().StringVar(&kind, "kind", "service", "short element kind metadata, e.g. service, database, component, function")
	c.Flags().StringVar(&description, "description", "", "description")
	c.Flags().StringVar(&technology, "technology", "", "primary technology")
	c.Flags().StringVar(&url, "url", "", "external URL")
	c.Flags().Float64Var(&positionX, "position-x", 0, "horizontal canvas position")
	c.Flags().Float64Var(&positionY, "position-y", 0, "vertical canvas position")
	c.Flags().StringVar(&ref, "ref", "", "override generated ref (default: slugified name)")
	c.Flags().StringVar(&parent, "parent", "root", "parent element ref or root")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "preview the change without writing files")
	c.Flags().StringVar(&diagramLabel, "diagram-label", "", "optional label for the element's canonical diagram")
	c.Flags().StringVar(&target, "target", "", "sync target: auto, local, or remote")
	c.Flags().StringVar(&dataDir, "data-dir", "", "data directory for local target state")
	c.Flags().BoolVar(&legacyWithView, "with-view", false, "deprecated")
	c.Flags().StringVar(&legacyViewLabel, "view-label", "", "deprecated")
	_ = c.Flags().MarkHidden("with-view")
	_ = c.Flags().MarkHidden("view-label")

	_ = c.RegisterFlagCompletionFunc("ref", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return completion.ElementRefs(wdir)
	})
	_ = c.RegisterFlagCompletionFunc("parent", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return completion.ParentRefs(wdir)
	})
	return c
}

// runAdd writes the element to the server synchronously, then refreshes the
// local YAML cache. Every invocation gets immediate server feedback.
func runAdd(cmd *cobra.Command, wdir, format string, compact bool, target, dataDir, ref string, spec *workspace.Element, kind, placementParent string, wasNormalized bool, technology, normalizedTechnology string) error {
	fail := func(err error) error {
		if cmdutil.WantsJSON(format) {
			return cmdutil.WriteCommandError(cmd.OutOrStdout(), compact, "add", err)
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

	bypass := true
	input := api.ElementInput{
		Name:            spec.Name,
		Description:     strptr(spec.Description),
		Kind:            strptr(spec.Kind),
		Technology:      strptr(spec.Technology),
		URL:             strptr(spec.URL),
		TechLinks:       planner.TechnologyLinksForElement(spec.Technology, ""),
		BypassNoiseGate: &bypass,
		HasView:         spec.HasView,
		ViewLabel:       strptr(spec.ViewLabel),
	}
	ctx := cmd.Context()
	var elementID int32
	var updated bool
	var savedElement *diagv1.Element
	if meta := elementMeta(ws, ref); meta != nil {
		updatedElement, err := runner.UpdateElement(ctx, int32(meta.ID), input)
		if err != nil {
			return fail(cmdutil.WithUnauthorizedHint("server update element failed", err))
		}
		elementID = updatedElement.GetId()
		savedElement = updatedElement
		updated = true
	} else {
		created, err := runner.CreateElement(ctx, input)
		if err != nil {
			return fail(cmdutil.WithUnauthorizedHint("server create element failed", err))
		}
		elementID = created.GetId()
		savedElement = created
	}

	// Ensure placement in the parent view (creating the parent view when needed,
	// mirroring the old planner's canonical-view promotion).
	parentViewID, err := exec.ResolveParentViewID(ctx, runner, ws, wdir, placementParent)
	if err != nil {
		return fail(fmt.Errorf("resolve parent view: %w", err))
	}
	var px, py float64
	if len(spec.Placements) > 0 {
		px, py = spec.Placements[0].PositionX, spec.Placements[0].PositionY
	}
	if err := runner.AddPlacement(ctx, parentViewID, elementID, px, py); err != nil {
		return fail(cmdutil.WithUnauthorizedHint("server place element failed", err))
	}

	// Refresh YAML cache (write-through).
	if err := workspace.UpsertElement(wdir, ref, spec); err != nil {
		return fail(fmt.Errorf("update YAML cache: %w", err))
	}
	// Promote the parent to a view in the cache when the server created one.
	if placementParent != workspace.RootRef {
		if parent, ok := ws.Elements[placementParent]; ok && parent != nil && !parent.HasView {
			parent.HasView = true
			_ = workspace.UpsertElement(wdir, placementParent, parent)
		}
		if err := exec.RecordViewMeta(wdir, placementParent, parentViewID); err != nil {
			return fail(fmt.Errorf("update cache metadata: %w", err))
		}
	}
	if err := exec.RecordElementMeta(wdir, ref, savedElement, 0, nil); err != nil {
		return fail(fmt.Errorf("update cache metadata: %w", err))
	}

	if cmdutil.WantsJSON(format) {
		action := "add"
		if updated {
			action = "update"
		}
		return cmdutil.WriteMutation(cmd.OutOrStdout(), compact, "add", action, ref)
	}
	if updated {
		term.Successf(cmd.OutOrStdout(), "updated: %s (id=%d)", ref, elementID)
	} else {
		term.Successf(cmd.OutOrStdout(), "add: %s (id=%d)", ref, elementID)
	}
	if wasNormalized {
		term.Infof(cmd.OutOrStdout(), "technology normalized: %q -> %q", technology, normalizedTechnology)
	}
	return nil
}

func elementMeta(ws *workspace.Workspace, ref string) *workspace.ResourceMetadata {
	if ws == nil || ws.Meta == nil {
		return nil
	}
	meta, ok := ws.Meta.Elements[ref]
	if !ok || meta == nil || meta.ID == 0 {
		return nil
	}
	return meta
}

func strptr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func validateKind(kind string) (string, error) {
	trimmed := strings.TrimSpace(kind)
	if trimmed == "" {
		return "", fmt.Errorf("--kind is invalid: cannot be empty or whitespace")
	}
	if len([]rune(trimmed)) > 64 {
		return "", fmt.Errorf("--kind %q is too long: use 64 characters or fewer", kind)
	}
	for _, r := range trimmed {
		if r < 32 || r == 127 {
			return "", fmt.Errorf("--kind contains control characters")
		}
	}
	return trimmed, nil
}

func normalizeTechnology(input string) (string, bool) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", false
	}
	slug, _, ok := tech.LookupCatalogFuzzy(trimmed)
	if ok {
		if strings.EqualFold(trimmed, slug) {
			return slug, false
		}
		return slug, true
	}
	missing := tech.Validate(trimmed)
	if len(missing) != 1 || !strings.EqualFold(strings.TrimSpace(missing[0]), trimmed) {
		return trimmed, false
	}
	suggestions := tech.SuggestSimilar(trimmed, 1)
	if len(suggestions) == 0 {
		return trimmed, false
	}
	canonical, _, canonicalOK := tech.LookupCatalog(suggestions[0])
	if !canonicalOK {
		return trimmed, false
	}
	if strings.EqualFold(trimmed, canonical) {
		return canonical, false
	}
	return canonical, true
}

func validateAndWarnTechnology(cmd *cobra.Command, technology string) {
	if technology == "" {
		return
	}
	missing := tech.Validate(technology)
	for _, m := range missing {
		suggestions := tech.SuggestSimilar(m, 5)
		if len(suggestions) == 0 {
			term.Warnf(cmd.OutOrStdout(), "Unknown technology %q", m)
			continue
		}
		term.Warnf(cmd.OutOrStdout(), "Unknown technology %q. similar: %s?", m, joinQuoted(suggestions))
	}
}

func joinQuoted(items []string) string {
	quoted := make([]string, len(items))
	for i, s := range items {
		quoted[i] = `"` + s + `"`
	}
	if len(quoted) == 0 {
		return ""
	}
	if len(quoted) == 1 {
		return quoted[0]
	}
	return fmt.Sprintf("%s, %s", strings.Join(quoted[:len(quoted)-1], ", "), quoted[len(quoted)-1])
}

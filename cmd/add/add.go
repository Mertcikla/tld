package add

import (
	"fmt"
	"strings"

	"github.com/mertcikla/tld/v2/internal/cmdutil"
	"github.com/mertcikla/tld/v2/internal/completion"
	"github.com/mertcikla/tld/v2/internal/tech"
	"github.com/mertcikla/tld/v2/internal/term"
	"github.com/mertcikla/tld/v2/internal/workspace"
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
		filePath        string
		symbol          string
		diagramLabel    string
		legacyViewLabel string
		legacyWithView  bool
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
				FilePath:    strings.TrimSpace(filePath),
				Symbol:      strings.TrimSpace(symbol),
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
			if err := workspace.UpsertElement(*wdir, r, spec); err != nil {
				if cmdutil.WantsJSON(*format) {
					return cmdutil.WriteCommandError(cmd.OutOrStdout(), *compact, "add", err)
				}
				return fmt.Errorf("upsert element: %w", err)
			}
			if cmdutil.WantsJSON(*format) {
				return cmdutil.WriteMutation(cmd.OutOrStdout(), *compact, "add", "add", r)
			}
			term.Successf(cmd.OutOrStdout(), "add: %s", r)
			if wasNormalized {
				term.Infof(cmd.OutOrStdout(), "technology normalized: %q -> %q", technology, normalizedTechnology)
			}
			return nil
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
	c.Flags().StringVar(&filePath, "file", "", "code path or glob this element owns, used by `tld impact` bindings")
	c.Flags().StringVar(&symbol, "symbol", "", "named code symbol within --file (e.g. a class or function)")
	c.Flags().BoolVar(&dryRun, "dry-run", false, "preview the change without writing files")
	c.Flags().StringVar(&diagramLabel, "diagram-label", "", "optional label for the element's canonical diagram")
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

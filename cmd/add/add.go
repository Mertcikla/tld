package add

import (
	"fmt"

	"github.com/mertcikla/tld/internal/cmdutil"
	"github.com/mertcikla/tld/internal/completion"
	"github.com/mertcikla/tld/internal/term"
	"github.com/mertcikla/tld/internal/workspace"
	"github.com/spf13/cobra"
)

func NewAddCmd(wdir, format *string, compact *bool) *cobra.Command {
	var (
		description     string
		technology      string
		url             string
		positionX       float64
		positionY       float64
		ref             string
		kind            string
		parent          string
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
			r := ref
			if r == "" {
				r = workspace.Slugify(name)
			}
			placementParent := parent
			if placementParent == "" {
				placementParent = "root"
			}
			if diagramLabel == "" {
				diagramLabel = legacyViewLabel
			}
			_ = legacyWithView
			spec := &workspace.Element{
				Name:        name,
				Kind:        kind,
				Description: description,
				Technology:  technology,
				URL:         url,
				HasView:     true,
				ViewLabel:   diagramLabel,
				Placements: []workspace.ViewPlacement{{
					ParentRef: placementParent,
					PositionX: positionX,
					PositionY: positionY,
				}},
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
			term.Successf(cmd.OutOrStdout(), "Added element %s to elements.yaml", r)
			term.Hint(cmd.OutOrStdout(), "Run 'tld apply' to push to cloud.")
			return nil
		},
	}

	c.Flags().StringVar(&kind, "kind", "service", "element kind")
	c.Flags().StringVar(&description, "description", "", "description")
	c.Flags().StringVar(&technology, "technology", "", "primary technology")
	c.Flags().StringVar(&url, "url", "", "external URL")
	c.Flags().Float64Var(&positionX, "position-x", 0, "horizontal canvas position")
	c.Flags().Float64Var(&positionY, "position-y", 0, "vertical canvas position")
	c.Flags().StringVar(&ref, "ref", "", "override generated ref (default: slugified name)")
	c.Flags().StringVar(&parent, "parent", "root", "parent element ref or root")
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
	_ = c.RegisterFlagCompletionFunc("kind", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return completion.ElementKinds(), cobra.ShellCompDirectiveNoFileComp
	})
	return c
}

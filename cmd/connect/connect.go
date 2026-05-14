package connect

import (
	"fmt"
	"slices"

	"github.com/mertcikla/tld/v2/internal/cmdutil"
	"github.com/mertcikla/tld/v2/internal/completion"
	"github.com/mertcikla/tld/v2/internal/term"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/spf13/cobra"
)

func NewConnectCmd(wdir, format *string, compact *bool) *cobra.Command {
	var (
		from         string
		to           string
		label        string
		description  string
		relationship string
		direction    string
		style        string
		url          string
		legacyView   string
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
			rootFallback := false
			if view == "" {
				view, rootFallback, err = inferConnectorView(ws, from, to)
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
			if err := workspace.AppendConnector(*wdir, spec); err != nil {
				if cmdutil.WantsJSON(*format) {
					return cmdutil.WriteCommandError(cmd.OutOrStdout(), *compact, "connect", err)
				}
				return fmt.Errorf("append connector: %w", err)
			}
			if cmdutil.WantsJSON(*format) {
				return cmdutil.WriteMutation(cmd.OutOrStdout(), *compact, "connect", "connect", fmt.Sprintf("%s:%s", from, to))
			}
			term.Successf(cmd.OutOrStdout(), "ok")
			term.Infof(cmd.OutOrStdout(), "connector view: %s", view)
			if rootFallback {
				term.Warn(cmd.OutOrStdout(), "No shared parent found; connector was placed in root. Use --view to choose a specific view.")
			}
			return nil
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
	c.Flags().StringVar(&legacyView, "view", "", "explicit view ref for the connector")
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
		return completion.ViewRefs(wdir)
	})
	return c
}

func elementParentRefs(element *workspace.Element) []string {
	if element == nil || len(element.Placements) == 0 {
		return []string{"root"}
	}
	refs := make([]string, 0, len(element.Placements))
	for _, p := range element.Placements {
		parent := p.ParentRef
		if parent == "" {
			parent = "root"
		}
		refs = append(refs, parent)
	}
	return refs
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

func inferConnectorView(ws *workspace.Workspace, from, to string) (string, bool, error) {
	if ws == nil {
		return "", false, fmt.Errorf("workspace is required")
	}
	fromElement, ok := ws.Elements[from]
	if !ok {
		return "", false, fmt.Errorf("source element %q not found", from)
	}
	toElement, ok := ws.Elements[to]
	if !ok {
		return "", false, fmt.Errorf("target element %q not found", to)
	}

	fromParents := elementParentRefs(fromElement)
	toParents := elementParentRefs(toElement)

	for _, f := range fromParents {
		if slices.Contains(toParents, f) {
			return f, false, nil
		}
	}

	return workspace.RootRef, true, nil
}

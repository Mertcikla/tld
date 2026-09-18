package connect

import (
	"fmt"
	"strings"

	"github.com/mertcikla/tld/v2/cmd/crudsync"
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
		dryRun       bool
		label        string
		description  string
		relationship string
		direction    string
		style        string
		url          string
		legacyView   string
		yamlOnly     bool
		target       string
		dataDir      string
	)

	c := &cobra.Command{
		Use:   "connect",
		Short: "Add a connector between two elements (applies instantly)",
		Long: `Add a connector between two elements and apply instantly.

Use --yaml-only to stage the YAML change without applying.
Use --dry-run to preview without writing files.`,
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
			if err := workspace.AppendConnector(*wdir, spec); err != nil {
				if cmdutil.WantsJSON(*format) {
					return cmdutil.WriteCommandError(cmd.OutOrStdout(), *compact, "connect", err)
				}
				return fmt.Errorf("append connector: %w", err)
			}
			if yamlOnly {
				if cmdutil.WantsJSON(*format) {
					return cmdutil.WriteMutation(cmd.OutOrStdout(), *compact, "connect", "connect", fmt.Sprintf("%s:%s", from, to))
				}
				term.Successf(cmd.OutOrStdout(), "ok")
				term.Infof(cmd.OutOrStdout(), "connector view: %s", view)
				term.Info(cmd.OutOrStdout(), "YAML only (--yaml-only): not applied.")
				return nil
			}
			if !cmdutil.WantsJSON(*format) {
				term.Successf(cmd.OutOrStdout(), "ok")
				term.Infof(cmd.OutOrStdout(), "connector view: %s", view)
			}
			_, err = crudsync.SyncAndReport(cmd, *wdir, crudsync.Options{
				Target:  target,
				DataDir: dataDir,
				Command: "connect",
				Format:  *format,
				Compact: *compact,
			}, cmd.OutOrStdout())
			return err
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
	c.Flags().BoolVar(&yamlOnly, "yaml-only", false, "write YAML only without applying")
	c.Flags().StringVar(&target, "target", "", "apply target: auto, local, or remote")
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

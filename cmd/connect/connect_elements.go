package connect

import (
	"github.com/mertcikla/diag/tld/internal/cmdutil"
	"fmt"

	"github.com/mertcikla/diag/tld/internal/workspace"
	"github.com/spf13/cobra"
)


func NewConnectElementsCmd(wdir *string) *cobra.Command {
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
		Use:   "elements",
		Short: "Add a connector between two elements; owner diagram is inferred from their shared parent",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ws, err := workspace.Load(*wdir)
			if err != nil {
				return fmt.Errorf("load workspace: %w", err)
			}
			view := legacyView
			if view == "" {
				view, err = inferConnectorView(ws, from, to)
				if err != nil {
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
			}
			if err := workspace.AppendConnector(*wdir, spec); err != nil {
				if cmdutil.WantsJSON(cmd.Root().PersistentFlags().Lookup("format").Value.String()) {
					return cmdutil.WriteCommandError(cmd.OutOrStdout(), cmd.Root().PersistentFlags().Lookup("compact").Value.String() == "true", "connect", err)
				}
				return fmt.Errorf("append connector: %w", err)
			}
			if cmdutil.WantsJSON(cmd.Root().PersistentFlags().Lookup("format").Value.String()) {
				return cmdutil.WriteMutation(cmd.OutOrStdout(), cmd.Root().PersistentFlags().Lookup("compact").Value.String() == "true", "connect", "connect", fmt.Sprintf("%s:%s", from, to))
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Appended connector %s -> %s in view %s to connectors.yaml\n", from, to, view)
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Change recorded locally in connectors.yaml. Run 'tld apply' to push to cloud.")
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
	c.Flags().StringVar(&legacyView, "view", "", "deprecated")
	_ = c.Flags().MarkHidden("style")
	_ = c.Flags().MarkHidden("view")
	_ = c.MarkFlagRequired("from")
	_ = c.MarkFlagRequired("to")
	return c
}

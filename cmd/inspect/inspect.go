package inspect

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/mertcikla/tld/v2/internal/cmdutil"
	"github.com/mertcikla/tld/v2/internal/inspection"
	"github.com/mertcikla/tld/v2/internal/term"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/spf13/cobra"
)

func NewInspectCmd(wdir, format *string, compact *bool) *cobra.Command {
	var (
		resourceType string
		includeCloud bool
		includeAll   bool
		dataDirFlag  string
	)

	c := &cobra.Command{
		Use:   "inspect <ref>",
		Short: "Inspect a resource across YAML, local DB, and optional cloud state",
		Long: `Inspect an element, owned view, or connector using its ref or ID.

Find element/view refs and canonical connector keys with 'tld list'. For a
connector, pass its full view/source~target[/label] key or numeric ID. Use --type to distinguish
an element from its owned view when they share a ref.

In a YAML workspace, reports YAML and local database state; --cloud also checks
the configured remote workspace. In database mode, reports the selected target.
This command reads state without changing the diagram.`,
		Example: `  tld inspect api --type element
  tld inspect platform --type view
  tld inspect 'root/api~db/reads' --type connector
  tld inspect api --type element --cloud --format json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			resourceType = strings.ToLower(strings.TrimSpace(resourceType))
			sess, err := cmdutil.OpenSession(cmd, *wdir, "", dataDirFlag)
			if err != nil {
				return err
			}
			defer func() { _ = sess.Close() }()
			ws, err := sess.LoadWorkspace()
			if err != nil {
				return err
			}
			ref := args[0]
			switch resourceType {
			case "connector":
				ref, err = cmdutil.ResolveConnectorArg(ws, ref)
			case "element":
				ref, err = cmdutil.ResolveElementArg(ws, ref)
			case "view":
				ref, err = cmdutil.ResolveViewArg(ws, ref)
			default:
				// Preserve element/owned-view ambiguity; only resolve aliases when
				// the supplied value is not already a canonical workspace key.
				if ws.Elements[ref] == nil && ws.Connectors[ref] == nil && ref != workspace.RootRef {
					element, elementErr := cmdutil.ResolveElementArg(ws, ref)
					connector, connectorErr := cmdutil.ResolveConnectorArg(ws, ref)
					if elementErr == nil && connectorErr == nil {
						return fmt.Errorf("selector %q is ambiguous; specify --type element or --type connector", ref)
					}
					if elementErr == nil {
						ref = element
					} else if connectorErr == nil {
						ref = connector
					}
				}
			}
			if err != nil {
				return err
			}

			cfg, err := workspace.LoadGlobalConfig()
			if err != nil {
				return err
			}
			_, effectiveDir := cmdutil.TargetOverrides(cmd, "", dataDirFlag)
			dataDir, err := workspace.ResolveDataDir(cfg, effectiveDir)
			if err != nil {
				return err
			}
			opts := inspection.Options{
				WorkspaceDir: *wdir,
				Ref:          ref,
				Type:         strings.ToLower(strings.TrimSpace(resourceType)),
				DataDir:      dataDir,
				Database:     cfg.Database,
				IncludeLocal: true,
				IncludeCloud: includeCloud || includeAll,
			}
			readSource, err := cmdutil.ReadSource(cmd, sess.HasWorkspace())
			if err != nil {
				return err
			}
			if readSource == "live" {
				opts.Workspace = ws
				opts.TargetOnly = true
				opts.IncludeLocal = false
				opts.IncludeCloud = includeCloud || includeAll
			}
			report, err := inspection.Build(cmd.Context(), opts)
			if err != nil {
				if wantsJSON(*format) {
					return cmdutil.WriteCommandError(cmd.OutOrStdout(), *compact, "inspect", err)
				}
				return err
			}
			if wantsJSON(*format) {
				return writeJSON(cmd.OutOrStdout(), *compact, report)
			}
			return printText(cmd.OutOrStdout(), report)
		},
	}

	c.Flags().StringVar(&resourceType, "type", "", "resource type: element, view, or connector")
	c.Flags().BoolVar(&includeCloud, "cloud", false, "include cloud state")
	c.Flags().BoolVar(&includeAll, "all", false, "include all state sources")
	c.Flags().StringVar(&dataDirFlag, "data-dir", "", "data directory for local DB state")
	cmdutil.AddReadSourceFlag(c)
	return c
}

func wantsJSON(format string) bool {
	return strings.EqualFold(format, "json")
}

func writeJSON(w io.Writer, compact bool, payload any) error {
	enc := json.NewEncoder(w)
	if !compact {
		enc.SetIndent("", "  ")
	}
	return enc.Encode(payload)
}

func printText(out io.Writer, report inspection.Report) error {
	if report.Status == "ambiguous" {
		term.Warnf(out, "ambiguous ref %q", report.Ref)
		term.Label(out, 18, "Matches", inspection.Join(report.Matches))
		term.Hint(out, "Rerun with --type element, --type view, or --type connector.")
		return nil
	}

	term.Label(out, 18, "Resource", fmt.Sprintf("%s %s", report.Type, report.Ref))
	if report.Element != nil {
		printElement(out, report.Element)
		printRelated(out, report.Related)
	}
	if report.Connector != nil {
		printConnector(out, report.Connector)
	}
	if d := report.ViewDiagnostics; d != nil {
		term.Label(out, 18, "Connectors", fmt.Sprintf("%d direct, %d off-view, %d unresolved", d.RenderedConnectors, len(d.OffViewConnectors), len(d.UnresolvedConnectors)))
		for _, key := range d.OffViewConnectors {
			term.Infof(out, "%s: %s", key, d.ConnectorVisibility[key].Description())
		}
		if len(d.OffViewConnectors) > 0 {
			term.Info(out, workspace.OffViewHint)
		}
	}
	printSources(out, report.Sources)
	for _, warning := range report.Warnings {
		term.Warn(out, warning)
	}
	return nil
}

func printElement(out io.Writer, element *inspection.ElementReport) {
	term.Label(out, 18, "Name", element.Name)
	term.Label(out, 18, "Kind", element.Kind)
	labelIfSet(out, "Description", element.Description)
	labelIfSet(out, "Technology", element.Technology)
	labelIfSet(out, "URL", element.URL)
	labelIfSet(out, "Logo URL", element.LogoURL)
	labelIfSet(out, "Repo", element.Repo)
	labelIfSet(out, "Branch", element.Branch)
	labelIfSet(out, "Language", element.Language)
	labelIfSet(out, "File path", element.FilePath)
	labelIfSet(out, "Symbol", element.Symbol)
	if len(element.Tags) > 0 {
		term.Label(out, 18, "Tags", inspection.Join(element.Tags))
	}
	term.Label(out, 18, "Has view", fmt.Sprintf("%t", element.HasView))
	labelIfSet(out, "View label", element.ViewLabel)
	if element.DensityLevel != 0 {
		term.Label(out, 18, "Density level", fmt.Sprintf("%d", element.DensityLevel))
	}
	if element.Metadata != nil {
		term.Label(out, 18, "YAML metadata", metadataText(element.Metadata))
	}
	if len(element.Placements) > 0 {
		term.Label(out, 18, "Placements", placementText(element.Placements))
	}
	term.Label(out, 18, "Derived children", inspection.Join(element.Children))
}

func printConnector(out io.Writer, connector *inspection.ConnectorReport) {
	term.Label(out, 18, "Visibility", connector.Visibility.Description())
	if connector.Visibility.Mode == "off-view" {
		term.Info(out, workspace.OffViewHint)
	}
	term.Label(out, 18, "View", connector.View)
	term.Label(out, 18, "Source", connector.Source)
	term.Label(out, 18, "Target", connector.Target)
	labelIfSet(out, "Label", connector.Label)
	labelIfSet(out, "Description", connector.Description)
	labelIfSet(out, "Relationship", connector.Relationship)
	labelIfSet(out, "Direction", connector.Direction)
	labelIfSet(out, "Style", connector.Style)
	labelIfSet(out, "URL", connector.URL)
	labelIfSet(out, "Source handle", connector.SourceHandle)
	labelIfSet(out, "Target handle", connector.TargetHandle)
	if connector.Metadata != nil {
		term.Label(out, 18, "YAML metadata", metadataText(connector.Metadata))
	}
}

func printRelated(out io.Writer, related inspection.RelatedResources) {
	if len(related.IncomingConnectors) > 0 {
		term.Label(out, 18, "Incoming", inspection.Join(related.IncomingConnectors))
	}
	if len(related.OutgoingConnectors) > 0 {
		term.Label(out, 18, "Outgoing", inspection.Join(related.OutgoingConnectors))
	}
	if len(related.ViewConnectors) > 0 {
		term.Label(out, 18, "In owned view", inspection.Join(related.ViewConnectors))
	}
}

func printSources(out io.Writer, sources []inspection.SourceState) {
	term.Separator(out)
	for _, source := range sources {
		status := "missing"
		if source.Present {
			status = "present"
		}
		parts := []string{status}
		if source.ID != 0 {
			parts = append(parts, fmt.Sprintf("id=%d", source.ID))
		}
		if source.UpdatedAt != "" {
			parts = append(parts, "updated="+source.UpdatedAt)
		}
		if source.Conflict {
			parts = append(parts, "conflict=true")
		}
		if source.Note != "" {
			parts = append(parts, "note="+source.Note)
		}
		term.Label(out, 18, source.Source, strings.Join(parts, " "))
	}
}

func labelIfSet(out io.Writer, label, value string) {
	if strings.TrimSpace(value) != "" {
		term.Label(out, 18, label, value)
	}
}

func metadataText(meta *inspection.MetadataInfo) string {
	if meta == nil {
		return "-"
	}
	parts := make([]string, 0, 3)
	if meta.ID != 0 {
		parts = append(parts, fmt.Sprintf("id=%d", meta.ID))
	}
	if meta.UpdatedAt != "" {
		parts = append(parts, "updated="+meta.UpdatedAt)
	}
	if meta.Conflict {
		parts = append(parts, "conflict=true")
	}
	if len(parts) == 0 {
		return "-"
	}
	return strings.Join(parts, " ")
}

func placementText(placements []inspection.PlacementInfo) string {
	parts := make([]string, 0, len(placements))
	for _, placement := range placements {
		parent := placement.ParentRef
		if parent == "" {
			parent = "root"
		}
		item := fmt.Sprintf("%s@%.0f,%.0f", parent, placement.PositionX, placement.PositionY)
		if placement.VisibilityDelta != 0 {
			item += fmt.Sprintf(" visibility_delta=%d", placement.VisibilityDelta)
		}
		parts = append(parts, item)
	}
	return strings.Join(parts, "; ")
}

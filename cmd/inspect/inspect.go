package inspect

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

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
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := workspace.LoadGlobalConfig()
			if err != nil {
				return err
			}
			dataDir, err := workspace.ResolveDataDir(cfg, dataDirFlag)
			if err != nil {
				return err
			}
			report, err := inspection.Build(cmd.Context(), inspection.Options{
				WorkspaceDir: *wdir,
				Ref:          args[0],
				Type:         strings.ToLower(strings.TrimSpace(resourceType)),
				DataDir:      dataDir,
				IncludeLocal: true,
				IncludeCloud: includeCloud || includeAll,
			})
			if err != nil {
				if wantsJSON(*format) {
					return writeJSON(cmd.OutOrStdout(), *compact, map[string]any{
						"command": "inspect",
						"status":  "error",
						"errors":  []string{err.Error()},
					})
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

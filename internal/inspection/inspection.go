package inspection

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/google/uuid"
	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/internal/client"
	"github.com/mertcikla/tld/v2/internal/cmdutil"
	"github.com/mertcikla/tld/v2/internal/localserver"
	"github.com/mertcikla/tld/v2/internal/store"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	TypeElement   = "element"
	TypeView      = "view"
	TypeConnector = "connector"
)

type Options struct {
	WorkspaceDir string
	Ref          string
	Type         string
	DataDir      string
	IncludeLocal bool
	IncludeCloud bool
}

type Report struct {
	Command   string           `json:"command"`
	Status    string           `json:"status"`
	Ref       string           `json:"ref"`
	Type      string           `json:"type"`
	Matches   []string         `json:"matches,omitempty"`
	Element   *ElementReport   `json:"element,omitempty"`
	Connector *ConnectorReport `json:"connector,omitempty"`
	Sources   []SourceState    `json:"sources"`
	Related   RelatedResources `json:"related,omitempty"`
	Warnings  []string         `json:"warnings,omitempty"`
}

type ElementReport struct {
	Ref          string          `json:"ref"`
	Name         string          `json:"name"`
	Kind         string          `json:"kind"`
	Owner        string          `json:"owner,omitempty"`
	Description  string          `json:"description,omitempty"`
	Technology   string          `json:"technology,omitempty"`
	URL          string          `json:"url,omitempty"`
	LogoURL      string          `json:"logo_url,omitempty"`
	Repo         string          `json:"repo,omitempty"`
	Branch       string          `json:"branch,omitempty"`
	Language     string          `json:"language,omitempty"`
	FilePath     string          `json:"file_path,omitempty"`
	Symbol       string          `json:"symbol,omitempty"`
	Tags         []string        `json:"tags,omitempty"`
	HasView      bool            `json:"has_view"`
	ViewLabel    string          `json:"view_label,omitempty"`
	DensityLevel int             `json:"density_level,omitempty"`
	Placements   []PlacementInfo `json:"placements,omitempty"`
	Children     []string        `json:"derived_children,omitempty"`
	Metadata     *MetadataInfo   `json:"metadata,omitempty"`
}

type ConnectorReport struct {
	Ref          string        `json:"ref"`
	View         string        `json:"view"`
	Source       string        `json:"source"`
	Target       string        `json:"target"`
	Label        string        `json:"label,omitempty"`
	Description  string        `json:"description,omitempty"`
	Relationship string        `json:"relationship,omitempty"`
	Direction    string        `json:"direction,omitempty"`
	Style        string        `json:"style,omitempty"`
	URL          string        `json:"url,omitempty"`
	SourceHandle string        `json:"source_handle,omitempty"`
	TargetHandle string        `json:"target_handle,omitempty"`
	Metadata     *MetadataInfo `json:"metadata,omitempty"`
}

type PlacementInfo struct {
	ParentRef       string  `json:"parent"`
	PositionX       float64 `json:"position_x,omitempty"`
	PositionY       float64 `json:"position_y,omitempty"`
	VisibilityDelta int     `json:"visibility_delta,omitempty"`
}

type MetadataInfo struct {
	ID        int32  `json:"id,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
	Conflict  bool   `json:"conflict,omitempty"`
}

type SourceState struct {
	Source    string `json:"source"`
	Present   bool   `json:"present"`
	ID        int32  `json:"id,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
	Conflict  bool   `json:"conflict,omitempty"`
	Note      string `json:"note,omitempty"`
}

type RelatedResources struct {
	IncomingConnectors []string `json:"incoming_connectors,omitempty"`
	OutgoingConnectors []string `json:"outgoing_connectors,omitempty"`
	ViewConnectors     []string `json:"view_connectors,omitempty"`
}

func Build(ctx context.Context, opts Options) (Report, error) {
	ws, err := cmdutil.LoadWorkspace(opts.WorkspaceDir)
	if err != nil {
		return Report{}, err
	}

	report := Report{
		Command: "inspect",
		Status:  "ok",
		Ref:     opts.Ref,
	}
	matches := matchesFor(ws, opts.Ref)
	if opts.Type == "" {
		switch {
		case len(matches) == 0:
			return Report{}, fmt.Errorf("resource %q not found", opts.Ref)
		case len(matches) > 1:
			report.Status = "ambiguous"
			report.Matches = matches
			return report, nil
		default:
			opts.Type = matches[0]
		}
	}
	if opts.Type == TypeView && ws.Elements[opts.Ref] != nil {
		opts.Type = TypeView
	}
	report.Type = opts.Type

	index := newIndex(ws)
	switch opts.Type {
	case TypeElement, TypeView:
		element := ws.Elements[opts.Ref]
		if element == nil {
			return Report{}, fmt.Errorf("%s %q not found", opts.Type, opts.Ref)
		}
		if opts.Type == TypeView && !element.HasView {
			return Report{}, fmt.Errorf("view %q not found", opts.Ref)
		}
		report.Element = buildElementReport(opts.Ref, element, index, elementMetadata(ws, opts.Ref))
		report.Related = relatedForElement(opts.Ref, index)
		report.Sources = append(report.Sources, yamlElementState(opts.Type, elementMetadataForType(ws, opts.Type, opts.Ref)))
	case TypeConnector:
		connector := ws.Connectors[opts.Ref]
		if connector == nil {
			return Report{}, fmt.Errorf("connector %q not found", opts.Ref)
		}
		report.Connector = buildConnectorReport(opts.Ref, connector, connectorMetadata(ws, opts.Ref))
		report.Sources = append(report.Sources, yamlElementState(TypeConnector, connectorMetadata(ws, opts.Ref)))
	default:
		return Report{}, fmt.Errorf("unsupported inspect type %q", opts.Type)
	}

	if opts.IncludeLocal {
		report.Sources = append(report.Sources, localState(ctx, ws, opts))
	}
	if opts.IncludeCloud {
		report.Sources = append(report.Sources, cloudState(ctx, ws, opts))
	}
	return report, nil
}

type resourceIndex struct {
	children map[string][]string
	incoming map[string][]string
	outgoing map[string][]string
	inView   map[string][]string
}

func newIndex(ws *workspace.Workspace) resourceIndex {
	index := resourceIndex{
		children: make(map[string][]string),
		incoming: make(map[string][]string),
		outgoing: make(map[string][]string),
		inView:   make(map[string][]string),
	}
	for ref, element := range ws.Elements {
		for _, placement := range element.Placements {
			parent := placement.ParentRef
			if parent == "" {
				parent = "root"
			}
			if parent != "root" {
				index.children[parent] = append(index.children[parent], ref)
			}
		}
	}
	for ref, connector := range ws.Connectors {
		index.outgoing[connector.Source] = append(index.outgoing[connector.Source], ref)
		index.incoming[connector.Target] = append(index.incoming[connector.Target], ref)
		index.inView[connector.View] = append(index.inView[connector.View], ref)
	}
	for _, values := range []map[string][]string{index.children, index.incoming, index.outgoing, index.inView} {
		for key := range values {
			sort.Strings(values[key])
		}
	}
	return index
}

func matchesFor(ws *workspace.Workspace, ref string) []string {
	var matches []string
	if element := ws.Elements[ref]; element != nil {
		matches = append(matches, TypeElement)
	}
	if _, ok := ws.Connectors[ref]; ok {
		matches = append(matches, TypeConnector)
	}
	return matches
}

func buildElementReport(ref string, element *workspace.Element, index resourceIndex, meta *workspace.ResourceMetadata) *ElementReport {
	out := &ElementReport{
		Ref:          ref,
		Name:         element.Name,
		Kind:         element.Kind,
		Owner:        element.Owner,
		Description:  element.Description,
		Technology:   element.Technology,
		URL:          element.URL,
		LogoURL:      element.LogoURL,
		Repo:         element.Repo,
		Branch:       element.Branch,
		Language:     element.Language,
		FilePath:     element.FilePath,
		Symbol:       element.Symbol,
		Tags:         cloneStrings(element.Tags),
		HasView:      element.HasView,
		ViewLabel:    element.ViewLabel,
		DensityLevel: element.DensityLevel,
		Children:     cloneStrings(index.children[ref]),
		Metadata:     metadataInfo(meta),
	}
	for _, placement := range element.Placements {
		out.Placements = append(out.Placements, PlacementInfo{
			ParentRef:       placement.ParentRef,
			PositionX:       placement.PositionX,
			PositionY:       placement.PositionY,
			VisibilityDelta: placement.VisibilityDelta,
		})
	}
	return out
}

func buildConnectorReport(ref string, connector *workspace.Connector, meta *workspace.ResourceMetadata) *ConnectorReport {
	return &ConnectorReport{
		Ref:          ref,
		View:         connector.View,
		Source:       connector.Source,
		Target:       connector.Target,
		Label:        connector.Label,
		Description:  connector.Description,
		Relationship: connector.Relationship,
		Direction:    connector.Direction,
		Style:        connector.Style,
		URL:          connector.URL,
		SourceHandle: connector.SourceHandle,
		TargetHandle: connector.TargetHandle,
		Metadata:     metadataInfo(meta),
	}
}

func relatedForElement(ref string, index resourceIndex) RelatedResources {
	return RelatedResources{
		IncomingConnectors: cloneStrings(index.incoming[ref]),
		OutgoingConnectors: cloneStrings(index.outgoing[ref]),
		ViewConnectors:     cloneStrings(index.inView[ref]),
	}
}

func yamlElementState(resourceType string, meta *workspace.ResourceMetadata) SourceState {
	state := SourceState{Source: "yaml", Present: true}
	if meta != nil {
		state.ID = int32(meta.ID)
		state.UpdatedAt = formatTime(meta.UpdatedAt)
		state.Conflict = meta.Conflict
	}
	if resourceType == TypeView && meta == nil {
		state.Note = "view is modeled by element.has_view"
	}
	return state
}

func localState(ctx context.Context, ws *workspace.Workspace, opts Options) SourceState {
	state := SourceState{Source: "local_db"}
	metadata := elementMetadataForType(ws, opts.Type, opts.Ref)
	if metadata == nil || metadata.ID == 0 {
		state.Note = "no metadata id to match local DB resource"
		return state
	}

	dbPath := localserver.DatabasePath(opts.DataDir)
	if _, err := os.Stat(dbPath); err != nil {
		if os.IsNotExist(err) {
			state.Note = "database not found"
			return state
		}
		state.Note = err.Error()
		return state
	}
	sqliteStore, err := store.Open(dbPath, assets.FS)
	if err != nil {
		state.Note = err.Error()
		return state
	}
	defer func() { _ = sqliteStore.Legacy().Close() }()
	adapter := store.NewAPIAdapter(sqliteStore)
	state.ID = int32(metadata.ID)
	switch opts.Type {
	case TypeElement:
		element, err := adapter.GetElement(ctx, state.ID, uuid.Nil)
		if isSQLNotFound(err) {
			return state
		}
		if err != nil {
			state.Note = err.Error()
			return state
		}
		state.Present = true
		state.UpdatedAt = protoTime(element.GetUpdatedAt())
	case TypeView:
		view, err := adapter.GetView(ctx, state.ID, uuid.Nil)
		if isSQLNotFound(err) {
			return state
		}
		if err != nil {
			state.Note = err.Error()
			return state
		}
		state.Present = true
		state.UpdatedAt = protoTime(view.GetUpdatedAt())
	case TypeConnector:
		connector, err := adapter.GetConnector(ctx, state.ID, uuid.Nil)
		if isSQLNotFound(err) {
			return state
		}
		if err != nil {
			state.Note = err.Error()
			return state
		}
		state.Present = true
		state.UpdatedAt = protoTime(connector.GetUpdatedAt())
	}
	return state
}

func cloudState(ctx context.Context, ws *workspace.Workspace, opts Options) SourceState {
	state := SourceState{Source: "cloud"}
	if err := cmdutil.EnsureAPIKey(ws.Config.APIKey); err != nil {
		state.Note = err.Error()
		return state
	}
	targetOrg := ws.Config.WorkspaceID
	if targetOrg == "" {
		state.Note = cmdutil.WorkspaceIDRequired("org-id required in .tld.yaml").Error()
		return state
	}
	c := client.New(ws.Config.ServerURL, ws.Config.APIKey, false)
	resp, err := c.ExportWorkspace(ctx, connect.NewRequest(&diagv1.ExportOrganizationRequest{OrgId: targetOrg}))
	if err != nil {
		state.Note = cmdutil.WithUnauthorizedHint("cloud inspect failed", err).Error()
		return state
	}
	cloudWS := cmdutil.ConvertExportResponse(ws, resp.Msg)
	meta := elementMetadataForType(cloudWS, opts.Type, opts.Ref)
	switch opts.Type {
	case TypeElement, TypeView:
		element := cloudWS.Elements[opts.Ref]
		state.Present = element != nil
		if opts.Type == TypeView {
			state.Present = element != nil && element.HasView
		}
	case TypeConnector:
		state.Present = cloudWS.Connectors[opts.Ref] != nil
	}
	if meta != nil {
		state.ID = int32(meta.ID)
		state.UpdatedAt = formatTime(meta.UpdatedAt)
		state.Conflict = meta.Conflict
	}
	return state
}

func elementMetadataForType(ws *workspace.Workspace, resourceType, ref string) *workspace.ResourceMetadata {
	switch resourceType {
	case TypeElement:
		return elementMetadata(ws, ref)
	case TypeView:
		return viewMetadata(ws, ref)
	case TypeConnector:
		return connectorMetadata(ws, ref)
	default:
		return nil
	}
}

func elementMetadata(ws *workspace.Workspace, ref string) *workspace.ResourceMetadata {
	if ws == nil || ws.Meta == nil {
		return nil
	}
	return ws.Meta.Elements[ref]
}

func viewMetadata(ws *workspace.Workspace, ref string) *workspace.ResourceMetadata {
	if ws == nil || ws.Meta == nil {
		return nil
	}
	return ws.Meta.Views[ref]
}

func connectorMetadata(ws *workspace.Workspace, ref string) *workspace.ResourceMetadata {
	if ws == nil || ws.Meta == nil {
		return nil
	}
	return ws.Meta.Connectors[ref]
}

func metadataInfo(meta *workspace.ResourceMetadata) *MetadataInfo {
	if meta == nil {
		return nil
	}
	return &MetadataInfo{
		ID:        int32(meta.ID),
		UpdatedAt: formatTime(meta.UpdatedAt),
		Conflict:  meta.Conflict,
	}
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func protoTime(value *timestamppb.Timestamp) string {
	if value == nil {
		return ""
	}
	return formatTime(value.AsTime())
}

func isSQLNotFound(err error) bool {
	return err == sql.ErrNoRows
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return append([]string(nil), values...)
}

func Join(values []string) string {
	if len(values) == 0 {
		return "-"
	}
	return strings.Join(values, ", ")
}

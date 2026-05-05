package exportyaml

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/google/uuid"
	"github.com/mertcikla/tld/internal/store"
	watchpkg "github.com/mertcikla/tld/internal/watch"
	"github.com/mertcikla/tld/internal/workspace"
)

type Result struct {
	ElementsWritten   int `json:"elements_written"`
	ConnectorsWritten int `json:"connectors_written"`
	ViewsWritten      int `json:"views_written"`
}

func Export(ctx context.Context, sqliteStore *store.SQLiteStore, watchStore *watchpkg.Store, base *workspace.Workspace, repositoryID int64) (*workspace.Workspace, Result, error) {
	if sqliteStore == nil || watchStore == nil {
		return nil, Result{}, fmt.Errorf("export yaml requires sqlite and watch stores")
	}
	if base == nil {
		return nil, Result{}, fmt.Errorf("export yaml requires a workspace")
	}
	mappings, err := watchStore.Materialization(ctx, repositoryID)
	if err != nil {
		return nil, Result{}, err
	}
	index := buildMappingIndex(mappings)
	api := store.NewAPIAdapter(sqliteStore)
	views, err := api.ListViews(ctx, uuid.Nil)
	if err != nil {
		return nil, Result{}, err
	}
	elements, _, err := api.ListElements(ctx, uuid.Nil, 0, 0, "")
	if err != nil {
		return nil, Result{}, err
	}
	placements, err := api.ListAllPlacements(ctx, uuid.Nil)
	if err != nil {
		return nil, Result{}, err
	}
	connectors, err := api.ListAllConnectors(ctx, uuid.Nil)
	if err != nil {
		return nil, Result{}, err
	}

	out := cloneWorkspace(base)
	removeGenerated(out, index)

	elementRefByID := existingRefsByID(metaElements(base))
	viewRefByID := existingRefsByID(metaViews(base))
	connectorRefByID := existingRefsByID(metaConnectors(base))
	usedRefs := map[string]struct{}{"root": {}}
	for ref := range out.Elements {
		usedRefs[ref] = struct{}{}
	}

	elementByID := elementsByID(elements)
	for _, mapping := range sortedMappings(mappings, "element") {
		elem := elementByID[int32(mapping.ResourceID)]
		if elem == nil {
			continue
		}
		ref := elementRefByID[int32(mapping.ResourceID)]
		if ref == "" || out.Elements[ref] != nil {
			ref = uniqueRef(generatedRef(mapping), usedRefs)
		}
		usedRefs[ref] = struct{}{}
		elementRefByID[int32(mapping.ResourceID)] = ref
		out.Elements[ref] = &workspace.Element{
			Name:        elem.GetName(),
			Kind:        defaultString(elem.GetKind(), "element"),
			Description: elem.GetDescription(),
			Technology:  elem.GetTechnology(),
			URL:         elem.GetUrl(),
			LogoURL:     elem.GetLogoUrl(),
			Repo:        elem.GetRepo(),
			Branch:      elem.GetBranch(),
			Language:    elem.GetLanguage(),
			FilePath:    elem.GetFilePath(),
			Tags:        cloneStrings(elem.GetTags()),
			HasView:     false,
			ViewLabel:   strings.TrimSpace(elem.GetViewLabel()),
		}
		out.Meta.Elements[ref] = &workspace.ResourceMetadata{ID: workspace.ResourceID(elem.Id), UpdatedAt: timestampTime(elem.GetUpdatedAt())}
	}

	viewByID := viewsByID(views)
	for _, mapping := range sortedMappings(mappings, "view") {
		view := viewByID[int32(mapping.ResourceID)]
		if view == nil || view.OwnerElementId == nil {
			continue
		}
		ownerRef := elementRefByID[*view.OwnerElementId]
		if ownerRef == "" || out.Elements[ownerRef] == nil {
			continue
		}
		viewRefByID[int32(mapping.ResourceID)] = ownerRef
		out.Elements[ownerRef].HasView = true
		if out.Elements[ownerRef].ViewLabel == "" && strings.TrimSpace(view.GetLevelLabel()) != "" {
			out.Elements[ownerRef].ViewLabel = strings.TrimSpace(view.GetLevelLabel())
		}
		out.Meta.Views[ownerRef] = &workspace.ResourceMetadata{ID: workspace.ResourceID(view.Id), UpdatedAt: timestampTime(view.GetUpdatedAt())}
	}

	for _, placement := range placements {
		elemRef := elementRefByID[placement.ElementId]
		if elemRef == "" || out.Elements[elemRef] == nil {
			continue
		}
		parentRef := viewRefByID[placement.ViewId]
		if parentRef == "" {
			parentRef = "root"
		}
		out.Elements[elemRef].Placements = append(out.Elements[elemRef].Placements, workspace.ViewPlacement{
			ParentRef: parentRef,
			PositionX: placement.PositionX,
			PositionY: placement.PositionY,
		})
	}

	connectorByID := connectorsByID(connectors)
	for _, mapping := range sortedMappings(mappings, "connector") {
		conn := connectorByID[int32(mapping.ResourceID)]
		if conn == nil {
			continue
		}
		viewRef := viewRefByID[conn.ViewId]
		if viewRef == "" {
			viewRef = "root"
		}
		sourceRef := elementRefByID[conn.SourceElementId]
		targetRef := elementRefByID[conn.TargetElementId]
		if sourceRef == "" || targetRef == "" {
			continue
		}
		ref := connectorRefByID[int32(mapping.ResourceID)]
		spec := &workspace.Connector{
			View:         viewRef,
			Source:       sourceRef,
			Target:       targetRef,
			Label:        conn.GetLabel(),
			Description:  conn.GetDescription(),
			Relationship: conn.GetRelationship(),
			Direction:    conn.GetDirection(),
			Style:        conn.GetStyle(),
			URL:          conn.GetUrl(),
			SourceHandle: conn.GetSourceHandle(),
			TargetHandle: conn.GetTargetHandle(),
		}
		if ref == "" || out.Connectors[ref] != nil {
			ref = workspace.ConnectorKey(spec)
		}
		out.Connectors[ref] = spec
		out.Meta.Connectors[ref] = &workspace.ResourceMetadata{ID: workspace.ResourceID(conn.Id), UpdatedAt: timestampTime(conn.GetUpdatedAt())}
	}

	return out, Result{ElementsWritten: len(index.elementIDs), ConnectorsWritten: len(index.connectorIDs), ViewsWritten: len(index.viewIDs)}, nil
}

type mappingIndex struct {
	elementIDs   map[int32]struct{}
	viewIDs      map[int32]struct{}
	connectorIDs map[int32]struct{}
}

func buildMappingIndex(mappings []watchpkg.MaterializationMapping) mappingIndex {
	index := mappingIndex{elementIDs: map[int32]struct{}{}, viewIDs: map[int32]struct{}{}, connectorIDs: map[int32]struct{}{}}
	for _, mapping := range mappings {
		switch mapping.ResourceType {
		case "element":
			index.elementIDs[int32(mapping.ResourceID)] = struct{}{}
		case "view":
			index.viewIDs[int32(mapping.ResourceID)] = struct{}{}
		case "connector":
			index.connectorIDs[int32(mapping.ResourceID)] = struct{}{}
		}
	}
	return index
}

func removeGenerated(ws *workspace.Workspace, index mappingIndex) {
	for ref, meta := range ws.Meta.Elements {
		if meta != nil {
			if _, ok := index.elementIDs[int32(meta.ID)]; ok {
				delete(ws.Elements, ref)
				delete(ws.Meta.Elements, ref)
				delete(ws.Meta.Views, ref)
			}
		}
	}
	for ref, meta := range ws.Meta.Connectors {
		if meta != nil {
			if _, ok := index.connectorIDs[int32(meta.ID)]; ok {
				delete(ws.Connectors, ref)
				delete(ws.Meta.Connectors, ref)
			}
		}
	}
}

func cloneWorkspace(ws *workspace.Workspace) *workspace.Workspace {
	out := &workspace.Workspace{
		Dir:             ws.Dir,
		Config:          ws.Config,
		WorkspaceConfig: ws.WorkspaceConfig,
		Elements:        map[string]*workspace.Element{},
		Connectors:      map[string]*workspace.Connector{},
		Meta:            ensureMeta(ws.Meta),
		IgnoreRules:     ws.IgnoreRules,
		ActiveRepo:      ws.ActiveRepo,
	}
	for ref, element := range ws.Elements {
		copyElement := *element
		copyElement.Tags = cloneStrings(element.Tags)
		copyElement.Placements = append([]workspace.ViewPlacement(nil), element.Placements...)
		out.Elements[ref] = &copyElement
	}
	for ref, connector := range ws.Connectors {
		copyConnector := *connector
		out.Connectors[ref] = &copyConnector
	}
	return out
}

func ensureMeta(meta *workspace.Meta) *workspace.Meta {
	out := &workspace.Meta{Elements: map[string]*workspace.ResourceMetadata{}, Views: map[string]*workspace.ResourceMetadata{}, Connectors: map[string]*workspace.ResourceMetadata{}}
	if meta == nil {
		return out
	}
	for ref, value := range meta.Elements {
		if value == nil {
			continue
		}
		copyValue := *value
		out.Elements[ref] = &copyValue
	}
	for ref, value := range meta.Views {
		if value == nil {
			continue
		}
		copyValue := *value
		out.Views[ref] = &copyValue
	}
	for ref, value := range meta.Connectors {
		if value == nil {
			continue
		}
		copyValue := *value
		out.Connectors[ref] = &copyValue
	}
	return out
}

func existingRefsByID(meta map[string]*workspace.ResourceMetadata) map[int32]string {
	out := map[int32]string{}
	for ref, item := range meta {
		if item != nil && item.ID != 0 {
			out[int32(item.ID)] = ref
		}
	}
	return out
}

func sortedMappings(mappings []watchpkg.MaterializationMapping, resourceType string) []watchpkg.MaterializationMapping {
	var out []watchpkg.MaterializationMapping
	for _, mapping := range mappings {
		if mapping.ResourceType == resourceType {
			out = append(out, mapping)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].OwnerType == out[j].OwnerType {
			return out[i].OwnerKey < out[j].OwnerKey
		}
		return out[i].OwnerType < out[j].OwnerType
	})
	return out
}

func generatedRef(mapping watchpkg.MaterializationMapping) string {
	base := workspace.Slugify(mapping.OwnerType + "-" + mapping.OwnerKey)
	if base == "" {
		base = "watch-" + strconv.FormatInt(mapping.ResourceID, 10)
	}
	return base
}

func uniqueRef(base string, used map[string]struct{}) string {
	if _, ok := used[base]; !ok {
		return base
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if _, ok := used[candidate]; !ok {
			return candidate
		}
	}
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return append([]string(nil), values...)
}

func elementsByID(items []*diagv1.Element) map[int32]*diagv1.Element {
	out := map[int32]*diagv1.Element{}
	for _, item := range items {
		out[item.Id] = item
	}
	return out
}

func viewsByID(items []*diagv1.View) map[int32]*diagv1.View {
	out := map[int32]*diagv1.View{}
	for _, item := range items {
		out[item.Id] = item
	}
	return out
}

func connectorsByID(items []*diagv1.Connector) map[int32]*diagv1.Connector {
	out := map[int32]*diagv1.Connector{}
	for _, item := range items {
		out[item.Id] = item
	}
	return out
}

func metaElements(ws *workspace.Workspace) map[string]*workspace.ResourceMetadata {
	if ws.Meta == nil || ws.Meta.Elements == nil {
		return nil
	}
	return ws.Meta.Elements
}

func metaViews(ws *workspace.Workspace) map[string]*workspace.ResourceMetadata {
	if ws.Meta == nil || ws.Meta.Views == nil {
		return nil
	}
	return ws.Meta.Views
}

func metaConnectors(ws *workspace.Workspace) map[string]*workspace.ResourceMetadata {
	if ws.Meta == nil || ws.Meta.Connectors == nil {
		return nil
	}
	return ws.Meta.Connectors
}

type protoTimestamp interface {
	AsTime() time.Time
}

func timestampTime(ts protoTimestamp) time.Time {
	if ts == nil {
		return time.Time{}
	}
	return ts.AsTime()
}

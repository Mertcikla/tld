package exec

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/mertcikla/tld/v2/pkg/api"
)

// ImportBatch is a self-contained, idempotent ApplyBatch built from an import
// document. Every element referenced by a placement or connector is pulled into
// the batch, so the server can resolve all refs inside a single transaction.
//
// Re-running the same document is safe: existing elements, views and connectors
// are matched to their server IDs (from the lockfile cache first, then by
// name/endpoints) and updated instead of duplicated.
type ImportBatch struct {
	Unchanged           map[string]bool // resource type + ":" + ref; dependencies stay in the batch
	Views               []workspace.ViewDiagnostics
	PreservedPlacements map[string][]string

	Request *diagv1.ApplyWorkspaceBatchRequest

	// ElementSpecs are the specs to persist to the local YAML cache, keyed by
	// ref. It includes referenced-but-not-defined elements pulled from the
	// workspace cache so their promoted views are recorded too.
	ElementSpecs map[string]*workspace.Element
	// Connectors are the connector specs from the import document.
	Connectors []*workspace.Connector

	ElementsCreated     int
	ElementsUpdated     int
	ElementsUnchanged   int
	ViewsPlanned        int
	ViewsCreated        int
	ConnectorsCreated   int
	ConnectorsUpdated   int
	ConnectorsUnchanged int
}

type ImportFieldError struct{ Field, Hint string }

func (e *ImportFieldError) Error() string        { return "batch import cannot apply connector " + e.Field }
func (e *ImportFieldError) RecoveryCode() string { return "unsupported_field" }
func (e *ImportFieldError) RecoveryHint() string { return e.Hint }

// BuildImportBatch validates the import document and resolves existing server
// IDs so the resulting batch is idempotent. It performs read-only server calls
// (list elements/views/connectors) and never mutates the target.
func BuildImportBatch(
	ctx context.Context,
	runner Runner,
	ws *workspace.Workspace,
	elements map[string]*workspace.Element,
	connectors []*workspace.Connector,
) (*ImportBatch, error) {
	if ws == nil {
		return nil, errors.New("workspace is required")
	}

	batchElements, err := collectImportElements(ws, elements, connectors)
	if err != nil {
		return nil, err
	}

	serverElements, err := runner.ListElements(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("list elements on target: %w", err)
	}
	byName := make(map[string][]*diagv1.Element, len(serverElements))
	byID := make(map[int32]*diagv1.Element, len(serverElements))
	for _, el := range serverElements {
		byName[el.GetName()] = append(byName[el.GetName()], el)
		byID[el.GetId()] = el
	}

	serverViews, err := runner.ListViews(ctx)
	if err != nil {
		return nil, fmt.Errorf("list views on target: %w", err)
	}
	viewByOwner := make(map[int32]*diagv1.View, len(serverViews))
	for _, view := range serverViews {
		if view.OwnerElementId != nil {
			viewByOwner[view.GetOwnerElementId()] = view
		}
	}

	elementIDs := make(map[string]int32, len(batchElements))
	for ref, el := range batchElements {
		id, err := resolveImportElementID(ws, ref, el, byName)
		if err != nil {
			return nil, err
		}
		elementIDs[ref] = id
	}
	// Different YAML refs must never silently alias one persisted element.
	// Otherwise a later remove of an apparently redundant ref deletes both.
	idsToRef := map[int32]string{}
	if ws.Meta != nil && workspace.IsWorkspaceDir(ws.Dir) {
		for ref, meta := range ws.Meta.Elements {
			if meta != nil && meta.ID != 0 && ws.Elements[ref] != nil {
				if old := idsToRef[int32(meta.ID)]; old == "" || ref < old {
					idsToRef[int32(meta.ID)] = ref
				}
			}
		}
	}
	refsByID := map[int32]string{}
	aliasRefs := make([]string, 0, len(elementIDs))
	for ref := range elementIDs {
		aliasRefs = append(aliasRefs, ref)
	}
	sort.Strings(aliasRefs)
	for _, ref := range aliasRefs {
		id := elementIDs[ref]
		if id == 0 {
			continue
		}
		if previous, ok := idsToRef[id]; ok && previous != ref {
			return nil, fmt.Errorf("element ref %q resolves to the same target ID %d as workspace ref %q; reuse the existing ref or rename it with 'tld update element %s ref %s'", ref, id, previous, previous, ref)
		}
		if previous, ok := refsByID[id]; ok && previous != ref {
			return nil, fmt.Errorf("element refs %q and %q resolve to the same target ID %d; use one ref for this element across all views", previous, ref, id)
		}
		refsByID[id] = ref
	}
	// Read current coordinates from the target, rather than replaying a stale
	// cache or treating an omitted coordinate as zero.
	exported, err := runner.ExportWorkspace(ctx)
	if err != nil {
		return nil, fmt.Errorf("read target placements: %w", err)
	}

	// Recompute which refs need an owned view: every placement parent and every
	// connector view, plus elements that explicitly request a view.
	requiredViews := map[string]bool{}
	for ref, el := range batchElements {
		if viewByOwner[elementIDs[ref]] != nil {
			requiredViews[ref] = true
		}
		for _, placement := range el.Placements {
			parent := placement.ParentRef
			if parent == "" {
				parent = workspace.RootRef
			}
			if parent != workspace.RootRef {
				requiredViews[parent] = true
			}
		}
		if el.HasView {
			requiredViews[ref] = true
		}
	}
	for _, c := range connectors {
		if c.View != "" && c.View != workspace.RootRef {
			requiredViews[c.View] = true
		}
	}

	viewIDs := make(map[string]int32, len(requiredViews))
	for ref := range requiredViews {
		if ref == workspace.RootRef {
			continue
		}
		viewIDs[ref] = resolveImportViewID(ws, ref, elementIDs[ref], viewByOwner)
	}

	batch := &ImportBatch{
		Unchanged:    map[string]bool{},
		Request:      &diagv1.ApplyWorkspaceBatchRequest{},
		ElementSpecs: batchElements,
		Connectors:   append([]*workspace.Connector(nil), connectors...),
	}

	refs := make([]string, 0, len(batchElements))
	for ref := range batchElements {
		refs = append(refs, ref)
	}
	sort.Strings(refs)

	for _, ref := range refs {
		el := batchElements[ref]
		pe := &diagv1.BatchElement{
			Ref:             ref,
			Name:            el.Name,
			Kind:            strOrNil(el.Kind),
			Description:     strOrNil(el.Description),
			Technology:      strOrNil(el.Technology),
			Url:             strOrNil(el.URL),
			LogoUrl:         strOrNil(el.LogoURL),
			Tags:            el.Tags,
			Repo:            strOrNil(el.Repo),
			RepositoryId:    strOrNil(el.RepositoryID),
			Branch:          strOrNil(el.Branch),
			Language:        strOrNil(el.Language),
			FilePath:        strOrNil(el.FilePath),
			BypassNoiseGate: el.BypassNoiseGate,
		}
		id := elementIDs[ref]
		_, declared := elements[ref]
		if !declared && id != 0 {
			// Dependencies included only to resolve refs are not metadata patches.
			pe = &diagv1.BatchElement{Ref: ref, Name: el.Name}
			if current := byID[id]; current != nil {
				pe.Name = current.GetName()
			}
		}
		if id != 0 {
			pe.Id = &id
			if pe.BypassNoiseGate == nil && byID[id] != nil {
				bypass := byID[id].GetBypassNoiseGate()
				pe.BypassNoiseGate = &bypass
			}
			if current := byID[id]; current != nil {
				pe.HasView = current.GetHasView()
				if pe.Tags == nil {
					pe.Tags = current.GetTags()
				}
				pe.TechnologyLinks = current.GetTechnologyLinks()
			}
		}
		viewChanged, placementsChanged := false, false
		if requiredViews[ref] {
			pe.HasView = true
			batch.ViewsPlanned++
			if viewID := viewIDs[ref]; viewID != 0 {
				pe.ViewId = &viewID
			} else {
				batch.ViewsCreated++
			}
			pe.ViewLabel = strOrNil(el.ViewLabel)
			if !declared {
				pe.ViewLabel = nil
			}
			if el.DensityLevelSet || el.DensityLevel != 0 {
				density := int32(el.DensityLevel)
				pe.ViewDensityLevel = &density
			}
			view := viewByOwner[id]
			viewChanged = view == nil || view.GetId() != pe.GetViewId() || (pe.ViewLabel != nil && pe.GetViewLabel() != view.GetLevelLabel())
			if !viewChanged && pe.ViewDensityLevel != nil {
				viewChanged = true // Remote exports do not expose density settings.
				if local, ok := runner.(*localRunner); ok {
					density, err := local.appStore.ViewDensityLevel(local.ctx(ctx), int64(pe.GetViewId()))
					if err != nil {
						return nil, err
					}
					viewChanged = density != int(pe.GetViewDensityLevel())
				}
			}
			batch.Unchanged["view:"+ref] = !viewChanged
		}
		for _, placement := range el.Placements {
			if !declared && id != 0 {
				continue
			}
			parent := placement.ParentRef
			if parent == "" {
				parent = workspace.RootRef
			}
			pp := &diagv1.BatchViewPlacement{ParentRef: parent}
			parentID := viewIDs[parent]
			if parent == workspace.RootRef {
				parentID = rootViewID(serverViews)
			}
			var current *diagv1.ElementPlacement
			for _, existing := range exported.GetPlacements() {
				if existing.GetElementId() == id && existing.GetViewId() == parentID {
					current = existing
					if !placement.PositionXSet && placement.PositionX == 0 {
						x := existing.GetPositionX()
						pp.PositionX = &x
					}
					if !placement.PositionYSet && placement.PositionY == 0 {
						y := existing.GetPositionY()
						pp.PositionY = &y
					}
					break
				}
			}
			if placement.PositionXSet || placement.PositionX != 0 {
				x := placement.PositionX
				pp.PositionX = &x
			}
			if placement.PositionYSet || placement.PositionY != 0 {
				y := placement.PositionY
				pp.PositionY = &y
			}
			if placement.VisibilityDeltaSet || placement.VisibilityDelta != 0 {
				delta := int32(placement.VisibilityDelta)
				pp.VisibilityDelta = &delta
			}
			pe.Placements = append(pe.Placements, pp)
			visibilityChanged, err := importVisibilityChanged(ctx, runner, parentID, "element", id, pp.VisibilityDelta)
			if err != nil {
				return nil, err
			}
			placementsChanged = placementsChanged || current == nil || visibilityChanged ||
				(pp.PositionX != nil && pp.GetPositionX() != current.GetPositionX()) ||
				(pp.PositionY != nil && pp.GetPositionY() != current.GetPositionY())
		}
		if id == 0 {
			batch.ElementsCreated++
		} else if api.BatchElementChangesMetadata(pe, byID[id]) || viewChanged || placementsChanged {
			batch.ElementsUpdated++
		} else {
			batch.Unchanged["element:"+ref] = true
			batch.ElementsUnchanged++
		}
		batch.Request.Elements = append(batch.Request.Elements, pe)
	}

	serverConnectors, err := runner.ListConnectors(ctx)
	if err != nil {
		return nil, fmt.Errorf("list connectors on target: %w", err)
	}
	rootID := rootViewID(serverViews)

	sortedConnectors := append([]*workspace.Connector(nil), connectors...)
	sort.Slice(sortedConnectors, func(i, j int) bool {
		return workspace.ConnectorKey(sortedConnectors[i]) < workspace.ConnectorKey(sortedConnectors[j])
	})
	for _, c := range sortedConnectors {
		viewRef := c.View
		if viewRef == "" {
			viewRef = workspace.RootRef
		}
		viewID := viewIDs[viewRef]
		if viewRef == workspace.RootRef {
			viewID = rootID
		}
		id := resolveImportConnectorID(ws, c, viewID, elementIDs[c.Source], elementIDs[c.Target], serverConnectors)
		if c.Tags != nil {
			var currentTags []string
			for _, current := range serverConnectors {
				if current.GetId() == id {
					currentTags = current.GetTags()
					break
				}
			}
			if !slices.Equal(c.Tags, currentTags) {
				return nil, &ImportFieldError{Field: "tags", Hint: fmt.Sprintf("Set tags with 'tld update connector %q tags <comma-separated-tags>', then retry sync.", workspace.ConnectorKey(c))}
			}
		}
		pc := &diagv1.BatchConnector{
			Ref:              workspace.ConnectorKey(c),
			ViewRef:          viewRef,
			SourceElementRef: c.Source,
			TargetElementRef: c.Target,
			Label:            strOrNil(c.Label),
			Description:      strOrNil(c.Description),
			Relationship:     strOrNil(c.Relationship),
			Direction:        strOrNil(c.Direction),
			Style:            strOrNil(c.Style),
			Url:              strOrNil(c.URL),
			SourceHandle:     strOrNil(c.SourceHandle),
			TargetHandle:     strOrNil(c.TargetHandle),
		}
		if c.VisibilityDeltaSet || c.VisibilityDelta != 0 {
			delta := int32(c.VisibilityDelta)
			pc.VisibilityDelta = &delta
		}
		var current *diagv1.Connector
		if id != 0 {
			pc.Id = &id
			for _, candidate := range serverConnectors {
				if candidate.GetId() != id {
					continue
				}
				current = candidate
				if pc.Direction == nil {
					pc.Direction = strOrNil(current.GetDirection())
				}
				if pc.Style == nil {
					pc.Style = strOrNil(current.GetStyle())
				}
				break
			}
		}
		visibilityChanged, err := importVisibilityChanged(ctx, runner, viewID, "connector", id, pc.VisibilityDelta)
		if err != nil {
			return nil, err
		}
		if id == 0 {
			batch.ConnectorsCreated++
		} else if api.BatchConnectorChangesMetadata(pc, current, viewID, elementIDs[c.Source], elementIDs[c.Target]) || visibilityChanged {
			batch.ConnectorsUpdated++
		} else {
			batch.Unchanged["connector:"+pc.Ref] = true
			batch.ConnectorsUnchanged++
		}
		batch.Request.Connectors = append(batch.Request.Connectors, pc)
	}

	return batch, nil
}

func importVisibilityChanged(ctx context.Context, runner Runner, viewID int32, resource string, id int32, delta *int32) (bool, error) {
	if delta == nil {
		return false, nil
	}
	local, ok := runner.(*localRunner)
	if !ok {
		return true, nil
	} // Remote exports do not expose visibility overrides.
	overrides, err := local.appStore.VisibilityOverrides(local.ctx(ctx), int64(viewID))
	if err != nil {
		return false, err
	}
	for _, override := range overrides {
		if override.ResourceType == resource && override.ResourceID == int64(id) {
			return override.LevelDelta != int(*delta), nil
		}
	}
	return *delta != 0, nil
}

// collectImportElements validates the declared elements and closes the set over
// every ref referenced by placements and connectors. Missing refs are looked up
// in the local workspace cache so hand-written connector-only imports work.
func collectImportElements(
	ws *workspace.Workspace,
	elements map[string]*workspace.Element,
	connectors []*workspace.Connector,
) (map[string]*workspace.Element, error) {
	batchElements := make(map[string]*workspace.Element, len(elements))
	for ref, el := range elements {
		if el == nil {
			return nil, fmt.Errorf("elements: %q has no definition", ref)
		}
		if err := workspace.ValidateElementRef(ref); err != nil {
			return nil, fmt.Errorf("elements: invalid ref %q: %w", ref, err)
		}
		if strings.TrimSpace(el.Name) == "" {
			return nil, fmt.Errorf("element %q: name is required", ref)
		}
		copied := *el
		batchElements[ref] = &copied
	}

	required := make(map[string]bool, len(batchElements))
	for ref := range batchElements {
		required[ref] = true
	}
	for ref, el := range batchElements {
		for _, placement := range el.Placements {
			parent := placement.ParentRef
			if parent == "" {
				parent = workspace.RootRef
			}
			if parent == ref {
				return nil, fmt.Errorf("element %q cannot be placed in its own view; choose another placement parent", ref)
			}
			if err := workspace.ValidateParentRef(parent); err != nil {
				return nil, fmt.Errorf("element %q: invalid placement parent %q: %w", ref, parent, err)
			}
			if parent != workspace.RootRef {
				required[parent] = true
			}
		}
	}

	seenConnectors := make(map[string]bool, len(connectors))
	for _, c := range connectors {
		if c == nil {
			return nil, errors.New("connectors: entry is empty")
		}
		if err := workspace.ValidateConnectorValues(c); err != nil {
			return nil, err
		}
		if err := workspace.ValidateElementRef(c.Source); err != nil {
			return nil, fmt.Errorf("connector %s -> %s: invalid source element ref: %w", c.Source, c.Target, err)
		}
		if err := workspace.ValidateElementRef(c.Target); err != nil {
			return nil, fmt.Errorf("connector %s -> %s: invalid target element ref: %w", c.Source, c.Target, err)
		}
		if c.View != "" {
			if err := workspace.ValidateParentRef(c.View); err != nil {
				return nil, fmt.Errorf("connector %s -> %s: invalid view ref %q: %w", c.Source, c.Target, c.View, err)
			}
			if c.View != workspace.RootRef {
				required[c.View] = true
			}
		}
		required[c.Source] = true
		required[c.Target] = true
		key := workspace.ConnectorKey(c)
		if seenConnectors[key] {
			return nil, fmt.Errorf("connectors: %q is defined more than once", key)
		}
		seenConnectors[key] = true
	}

	var missing []string
	for ref := range required {
		if ref == workspace.RootRef {
			continue
		}
		if _, ok := batchElements[ref]; ok {
			continue
		}
		existing := ws.Elements[ref]
		if existing == nil {
			missing = append(missing, ref)
			continue
		}
		copied := *existing
		batchElements[ref] = &copied
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, fmt.Errorf(
			"the import references elements that are neither defined in the file nor present in the workspace: %s.\n"+
				"Add them to the import file, or run `tld pull` to refresh the local cache",
			strings.Join(missing, ", "))
	}

	return batchElements, nil
}

// resolveImportElementID matches a spec to an existing server element. The
// lockfile cache is authoritative; otherwise the server is searched by name,
// using the kind to disambiguate. Ambiguity is an error rather than a guess so
// an import can never silently overwrite an unrelated same-named element.
func resolveImportElementID(ws *workspace.Workspace, ref string, el *workspace.Element, byName map[string][]*diagv1.Element) (int32, error) {
	if ws.Meta != nil {
		if m, ok := ws.Meta.Elements[ref]; ok && m != nil && m.ID != 0 {
			return int32(m.ID), nil
		}
	}
	matches := byName[el.Name]
	if len(matches) == 0 {
		return 0, nil
	}

	var sameKind []*diagv1.Element
	if el.Kind != "" {
		for _, m := range matches {
			if m.GetKind() == el.Kind {
				sameKind = append(sameKind, m)
			}
		}
	}
	switch {
	case len(sameKind) == 1:
		return sameKind[0].GetId(), nil
	case len(sameKind) > 1:
		return 0, fmt.Errorf(
			"cannot resolve element %q: %d server elements are named %q with kind %q. Use a unique name or run `tld pull` to resync IDs",
			ref, len(sameKind), el.Name, el.Kind)
	case len(matches) == 1:
		return matches[0].GetId(), nil
	default:
		return 0, fmt.Errorf(
			"cannot resolve element %q: %d server elements are named %q. Use a unique name or run `tld pull` to resync IDs",
			ref, len(matches), el.Name)
	}
}

func resolveImportViewID(ws *workspace.Workspace, ref string, elementID int32, viewByOwner map[int32]*diagv1.View) int32 {
	if ws.Meta != nil {
		if m, ok := ws.Meta.Views[ref]; ok && m != nil && m.ID != 0 {
			return int32(m.ID)
		}
	}
	if elementID != 0 {
		if view := viewByOwner[elementID]; view != nil {
			return view.GetId()
		}
	}
	return 0
}

func resolveImportConnectorID(
	ws *workspace.Workspace,
	c *workspace.Connector,
	viewID, sourceID, targetID int32,
	serverConnectors []*diagv1.Connector,
) int32 {
	key := workspace.ConnectorKey(c)
	if ws.Meta != nil {
		if m, ok := ws.Meta.Connectors[key]; ok && m != nil && m.ID != 0 {
			return int32(m.ID)
		}
	}
	if viewID == 0 || sourceID == 0 || targetID == 0 {
		return 0
	}
	for _, sc := range serverConnectors {
		if sc.GetViewId() == viewID &&
			sc.GetSourceElementId() == sourceID &&
			sc.GetTargetElementId() == targetID &&
			sc.GetLabel() == c.Label {
			return sc.GetId()
		}
	}
	return 0
}

func rootViewID(views []*diagv1.View) int32 {
	for _, view := range views {
		if view.ParentViewId == nil && view.OwnerElementId == nil {
			return view.GetId()
		}
	}
	for _, view := range views {
		if view.ParentViewId == nil {
			return view.GetId()
		}
	}
	return 0
}

package exec

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

// ImportPlan is a self-contained, idempotent ApplyPlan built from an import
// document. Every element referenced by a placement or connector is pulled into
// the plan, so the server can resolve all refs inside a single transaction.
//
// Re-running the same document is safe: existing elements, views and connectors
// are matched to their server IDs (from the lockfile cache first, then by
// name/endpoints) and updated instead of duplicated.
type ImportPlan struct {
	Request *diagv1.ApplyPlanRequest

	// ElementSpecs are the specs to persist to the local YAML cache, keyed by
	// ref. It includes referenced-but-not-defined elements pulled from the
	// workspace cache so their promoted views are recorded too.
	ElementSpecs map[string]*workspace.Element
	// Connectors are the connector specs from the import document.
	Connectors []*workspace.Connector

	ElementsCreated   int
	ElementsUpdated   int
	ViewsPlanned      int
	ViewsCreated      int
	ConnectorsCreated int
	ConnectorsUpdated int
}

// BuildImportPlan validates the import document and resolves existing server
// IDs so the resulting plan is idempotent. It performs read-only server calls
// (list elements/views/connectors) and never mutates the target.
func BuildImportPlan(
	ctx context.Context,
	runner Runner,
	ws *workspace.Workspace,
	elements map[string]*workspace.Element,
	connectors []*workspace.Connector,
) (*ImportPlan, error) {
	if ws == nil {
		return nil, errors.New("workspace is required")
	}

	planElements, err := collectImportElements(ws, elements, connectors)
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

	elementIDs := make(map[string]int32, len(planElements))
	for ref, el := range planElements {
		id, err := resolveImportElementID(ws, ref, el, byName)
		if err != nil {
			return nil, err
		}
		elementIDs[ref] = id
	}

	// Recompute which refs need an owned view: every placement parent and every
	// connector view, plus elements that explicitly request a view.
	requiredViews := map[string]bool{}
	for ref, el := range planElements {
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

	plan := &ImportPlan{
		Request:      &diagv1.ApplyPlanRequest{},
		ElementSpecs: planElements,
		Connectors:   append([]*workspace.Connector(nil), connectors...),
	}

	refs := make([]string, 0, len(planElements))
	for ref := range planElements {
		refs = append(refs, ref)
	}
	sort.Strings(refs)

	for _, ref := range refs {
		el := planElements[ref]
		pe := &diagv1.PlanElement{
			Ref:             ref,
			Name:            el.Name,
			Kind:            strOrNil(el.Kind),
			Description:     strOrNil(el.Description),
			Technology:      strOrNil(el.Technology),
			Url:             strOrNil(el.URL),
			LogoUrl:         strOrNil(el.LogoURL),
			Tags:            el.Tags,
			Repo:            strOrNil(el.Repo),
			Branch:          strOrNil(el.Branch),
			FilePath:        strOrNil(el.FilePath),
			BypassNoiseGate: el.BypassNoiseGate,
		}
		id := elementIDs[ref]
		if id != 0 {
			pe.Id = &id
			if pe.BypassNoiseGate == nil && byID[id] != nil {
				bypass := byID[id].GetBypassNoiseGate()
				pe.BypassNoiseGate = &bypass
			}
			plan.ElementsUpdated++
		} else {
			plan.ElementsCreated++
		}
		if requiredViews[ref] {
			pe.HasView = true
			plan.ViewsPlanned++
			if viewID := viewIDs[ref]; viewID != 0 {
				pe.ViewId = &viewID
			} else {
				plan.ViewsCreated++
			}
			pe.ViewLabel = strOrNil(el.ViewLabel)
			if el.DensityLevel != 0 {
				density := int32(el.DensityLevel)
				pe.ViewDensityLevel = &density
			}
		}
		for _, placement := range el.Placements {
			parent := placement.ParentRef
			if parent == "" {
				parent = workspace.RootRef
			}
			pp := &diagv1.PlanViewPlacement{ParentRef: parent}
			if placement.PositionXSet || placement.PositionX != 0 {
				x := placement.PositionX
				pp.PositionX = &x
			}
			if placement.PositionYSet || placement.PositionY != 0 {
				y := placement.PositionY
				pp.PositionY = &y
			}
			if placement.VisibilityDelta != 0 {
				delta := int32(placement.VisibilityDelta)
				pp.VisibilityDelta = &delta
			}
			pe.Placements = append(pe.Placements, pp)
		}
		plan.Request.Elements = append(plan.Request.Elements, pe)
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
		pc := &diagv1.PlanConnector{
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
		if id != 0 {
			pc.Id = &id
			plan.ConnectorsUpdated++
		} else {
			plan.ConnectorsCreated++
		}
		plan.Request.Connectors = append(plan.Request.Connectors, pc)
	}

	return plan, nil
}

// collectImportElements validates the declared elements and closes the set over
// every ref referenced by placements and connectors. Missing refs are looked up
// in the local workspace cache so hand-written connector-only imports work.
func collectImportElements(
	ws *workspace.Workspace,
	elements map[string]*workspace.Element,
	connectors []*workspace.Connector,
) (map[string]*workspace.Element, error) {
	planElements := make(map[string]*workspace.Element, len(elements))
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
		planElements[ref] = &copied
	}

	required := make(map[string]bool, len(planElements))
	for ref := range planElements {
		required[ref] = true
	}
	for ref, el := range planElements {
		for _, placement := range el.Placements {
			parent := placement.ParentRef
			if parent == "" {
				parent = workspace.RootRef
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
		if _, ok := planElements[ref]; ok {
			continue
		}
		existing := ws.Elements[ref]
		if existing == nil {
			missing = append(missing, ref)
			continue
		}
		copied := *existing
		planElements[ref] = &copied
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, fmt.Errorf(
			"the import references elements that are neither defined in the file nor present in the workspace: %s.\n"+
				"Add them to the import file, or run `tld pull` to refresh the local cache",
			strings.Join(missing, ", "))
	}

	return planElements, nil
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

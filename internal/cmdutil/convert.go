package cmdutil

import (
	"fmt"
	"slices"
	"strings"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

func ConvertExportResponse(baseWS *workspace.Workspace, msg *diagv1.ExportOrganizationResponse) *workspace.Workspace {
	newWS := &workspace.Workspace{
		Dir:        baseWS.Dir,
		Config:     baseWS.Config,
		Elements:   make(map[string]*workspace.Element),
		Connectors: make(map[string]*workspace.Connector),
		Meta: &workspace.Meta{
			Elements:   make(map[string]*workspace.ResourceMetadata),
			Views:      make(map[string]*workspace.ResourceMetadata),
			Connectors: make(map[string]*workspace.ResourceMetadata),
		},
	}

	objectIDToRef := allocateExportElementRefs(baseWS, msg.Elements)
	existingConnectorRefs := make(map[int32]string)
	if baseWS.Meta != nil {
		for ref, m := range baseWS.Meta.Connectors {
			existingConnectorRefs[int32(m.ID)] = ref
		}
	}

	for _, e := range msg.Elements {
		ref := objectIDToRef[e.Id]
		kind := e.GetKind()
		if kind == "" {
			kind = "element"
		}
		newWS.Elements[ref] = &workspace.Element{
			Name:         e.Name,
			Kind:         kind,
			Description:  e.GetDescription(),
			Technology:   e.GetTechnology(),
			URL:          e.GetUrl(),
			LogoURL:      e.GetLogoUrl(),
			Repo:         e.GetRepo(),
			RepositoryID: e.GetRepositoryId(),
			Branch:       e.GetBranch(),
			Language:     e.GetLanguage(),
			FilePath:     e.GetFilePath(),
			Tags:         cloneStrings(e.GetTags()),
			HasView:      e.GetHasView(),
			ViewLabel:    strings.TrimSpace(e.GetViewLabel()),
		}
		newWS.Meta.Elements[ref] = &workspace.ResourceMetadata{
			ID:        workspace.ResourceID(e.Id),
			UpdatedAt: e.UpdatedAt.AsTime(),
		}
	}

	ownerByDiagramID := buildDiagramOwnerIndex(msg, newWS.Elements, objectIDToRef)

	diagramIDToViewRef := make(map[int32]string)
	for _, d := range msg.Views {
		if ownerRef, ok := ownerByDiagramID[d.Id]; ok {
			diagramIDToViewRef[d.Id] = ownerRef
			element := newWS.Elements[ownerRef]
			element.HasView = true
			if name := strings.TrimSpace(d.Name); name != "" && !strings.EqualFold(name, strings.TrimSpace(element.Name)) {
				element.ViewName = name
			}
			if label := exportedViewLabel(d); element.ViewLabel == "" && label != "" {
				element.ViewLabel = label
			}
			newWS.Meta.Views[ownerRef] = &workspace.ResourceMetadata{
				ID:        workspace.ResourceID(d.Id),
				UpdatedAt: d.UpdatedAt.AsTime(),
			}
			continue
		}

		diagramIDToViewRef[d.Id] = "root"
	}

	for _, p := range msg.Placements {
		elementRef, ok := objectIDToRef[p.ElementId]
		if !ok {
			continue
		}
		parentRef := diagramIDToViewRef[p.ViewId]
		if parentRef == "" {
			parentRef = "root"
		}
		newWS.Elements[elementRef].Placements = append(newWS.Elements[elementRef].Placements, workspace.ViewPlacement{
			ParentRef:    parentRef,
			PositionX:    p.PositionX,
			PositionY:    p.PositionY,
			PositionXSet: true,
			PositionYSet: true,
		})
	}

	for _, e := range msg.Connectors {
		viewRef := diagramIDToViewRef[e.ViewId]
		if viewRef == "" {
			viewRef = "root"
		}
		srcRef, ok2 := objectIDToRef[e.SourceElementId]
		tgtRef, ok3 := objectIDToRef[e.TargetElementId]
		if !ok2 || !ok3 {
			continue
		}

		fallbackKey := workspace.ConnectorKey(&workspace.Connector{
			View:   viewRef,
			Source: srcRef,
			Target: tgtRef,
			Label:  e.GetLabel(),
		})
		key, ok := existingConnectorRefs[e.Id]
		if !ok || !connectorRefMatches(key, viewRef, srcRef, tgtRef, e.GetLabel()) {
			key = fallbackKey
		}

		newWS.Connectors[key] = &workspace.Connector{
			View:         viewRef,
			Source:       srcRef,
			Target:       tgtRef,
			Label:        e.GetLabel(),
			Description:  e.GetDescription(),
			Relationship: e.GetRelationship(),
			Direction:    e.Direction,
			Style:        e.Style,
			URL:          e.GetUrl(),
			SourceHandle: e.GetSourceHandle(),
			TargetHandle: e.GetTargetHandle(),
			Tags:         cloneStrings(e.GetTags()),
		}
		newWS.Meta.Connectors[key] = &workspace.ResourceMetadata{
			ID:        workspace.ResourceID(e.Id),
			UpdatedAt: e.UpdatedAt.AsTime(),
		}
	}

	return newWS
}

func allocateExportElementRefs(base *workspace.Workspace, elements []*diagv1.Element) map[int32]string {
	refs := make(map[int32]string)
	existing := make(map[int32]string)
	used := make(map[string]bool)
	for ref := range base.Elements {
		used[ref] = true
	}
	if base.Meta != nil {
		// Sort cached refs too so even duplicate cached IDs resolve consistently.
		cached := make([]string, 0, len(base.Meta.Elements))
		for ref := range base.Meta.Elements {
			used[ref] = true
			cached = append(cached, ref)
		}
		slices.Sort(cached)
		for _, ref := range cached {
			meta := base.Meta.Elements[ref]
			if meta != nil && meta.ID != 0 {
				if _, exists := existing[int32(meta.ID)]; !exists {
					existing[int32(meta.ID)] = ref
				}
			}
		}
	}

	// A stable ID order makes new refs independent of the export's row order.
	ordered := slices.Clone(elements)
	slices.SortFunc(ordered, func(a, b *diagv1.Element) int {
		if a.Id < b.Id {
			return -1
		}
		if a.Id > b.Id {
			return 1
		}
		return 0
	})
	var collisions []*diagv1.Element
	for _, element := range ordered {
		if ref, exists := existing[element.Id]; exists {
			refs[element.Id] = ref
			continue
		}
		if _, exists := refs[element.Id]; exists {
			continue
		}
		ref := workspace.Slugify(element.Name)
		if ref == "" {
			ref = fmt.Sprintf("element-%d", element.Id)
		}
		if used[ref] {
			collisions = append(collisions, element)
			continue
		}
		refs[element.Id] = ref
		used[ref] = true
	}
	// Assign natural refs first so generated suffixes cannot take another
	// exported element's name. Reserve every candidate as soon as it is used.
	for _, element := range collisions {
		stem := workspace.Slugify(element.Name)
		if stem == "" {
			stem = "element"
		}
		candidate := fmt.Sprintf("%s-%d", stem, element.Id)
		ref := candidate
		for suffix := 2; used[ref]; suffix++ {
			ref = fmt.Sprintf("%s-%d", candidate, suffix)
		}
		refs[element.Id] = ref
		used[ref] = true
	}
	return refs
}

func connectorRefMatches(ref, viewRef, srcRef, tgtRef, label string) bool {
	view, source, target, refLabel, ok := workspace.ParseConnectorKey(workspace.NormalizeConnectorKey(ref))
	if !ok {
		return false
	}
	return view == viewRef && source == srcRef && target == tgtRef && refLabel == label
}

func CountViews(ws *workspace.Workspace) int {
	count := 0
	for _, element := range ws.Elements {
		if element.HasView {
			count++
		}
	}
	return count
}

func exportedViewLabel(view *diagv1.View) string {
	return strings.TrimSpace(view.GetLevelLabel())
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return append([]string(nil), values...)
}

func buildDiagramOwnerIndex(msg *diagv1.ExportOrganizationResponse, elements map[string]*workspace.Element, objectIDToRef map[int32]string) map[int32]string {
	owners := make(map[int32]string)
	usedRefs := make(map[string]struct{})

	for _, diagram := range msg.Views {
		if diagram.OwnerElementId == nil {
			continue
		}
		ownerRef, ok := objectIDToRef[*diagram.OwnerElementId]
		if !ok {
			continue
		}
		owners[diagram.Id] = ownerRef
		usedRefs[ownerRef] = struct{}{}
	}

	for _, navigation := range msg.Navigations {
		if _, ok := owners[navigation.ToViewId]; ok {
			continue
		}
		ownerRef, ok := objectIDToRef[navigation.ElementId]
		if !ok || navigation.ToViewId == 0 {
			continue
		}
		owners[navigation.ToViewId] = ownerRef
		usedRefs[ownerRef] = struct{}{}
	}

	for _, diagram := range msg.Views {
		if _, ok := owners[diagram.Id]; ok {
			continue
		}
		ownerRef, ok := inferDiagramOwnerRef(diagram, elements, usedRefs)
		if !ok {
			continue
		}
		owners[diagram.Id] = ownerRef
		usedRefs[ownerRef] = struct{}{}
	}

	return owners
}

func inferDiagramOwnerRef(diagram *diagv1.View, elements map[string]*workspace.Element, usedRefs map[string]struct{}) (string, bool) {
	strictMatches := make([]string, 0, 1)
	looseMatches := make([]string, 0, 1)

	for ref, element := range elements {
		if element == nil || !element.HasView {
			continue
		}
		if _, used := usedRefs[ref]; used {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(diagram.Name), strings.TrimSpace(element.Name)) {
			continue
		}
		looseMatches = append(looseMatches, ref)
		if diagramMatchesOwnedElement(diagram, element) {
			strictMatches = append(strictMatches, ref)
		}
	}

	switch {
	case len(strictMatches) == 1:
		return strictMatches[0], true
	case len(looseMatches) == 1:
		return looseMatches[0], true
	default:
		return "", false
	}
}

func diagramMatchesOwnedElement(diagram *diagv1.View, element *workspace.Element) bool {
	if element == nil {
		return false
	}
	label := exportedViewLabel(diagram)
	if strings.TrimSpace(element.ViewLabel) == "" {
		return label == ""
	}
	return strings.EqualFold(
		label,
		strings.TrimSpace(element.ViewLabel),
	)
}

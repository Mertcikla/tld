package cmdutil

import (
	"strings"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/mertcikla/tld/internal/workspace"
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

	existingElementRefs := make(map[int32]string)
	if baseWS.Meta != nil {
		for ref, m := range baseWS.Meta.Elements {
			existingElementRefs[int32(m.ID)] = ref
		}
	}
	existingConnectorRefs := make(map[int32]string)
	if baseWS.Meta != nil {
		for ref, m := range baseWS.Meta.Connectors {
			existingConnectorRefs[int32(m.ID)] = ref
		}
	}

	objectIDToRef := make(map[int32]string)
	for _, e := range msg.Elements {
		ref, ok := existingElementRefs[e.Id]
		if !ok {
			ref = workspace.Slugify(e.Name)
		}
		objectIDToRef[e.Id] = ref
		kind := e.GetKind()
		if kind == "" {
			kind = "element"
		}
		newWS.Elements[ref] = &workspace.Element{
			Name:        e.Name,
			Kind:        kind,
			Description: e.GetDescription(),
			Technology:  e.GetTechnology(),
			URL:         e.GetUrl(),
			LogoURL:     e.GetLogoUrl(),
			Repo:        e.GetRepo(),
			Branch:      e.GetBranch(),
			Language:    e.GetLanguage(),
			FilePath:    e.GetFilePath(),
			Tags:        cloneStrings(e.GetTags()),
			HasView:     e.GetHasView(),
			ViewLabel:   strings.TrimSpace(e.GetViewLabel()),
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
			if label := exportedDiagramLabel(d, element.Name); element.ViewLabel == "" && label != "" {
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
			ParentRef: parentRef,
			PositionX: p.PositionX,
			PositionY: p.PositionY,
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

		key, ok := existingConnectorRefs[e.Id]
		if !ok {
			key = viewRef + ":" + srcRef + ":" + tgtRef + ":" + e.GetLabel()
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
		}
		newWS.Meta.Connectors[key] = &workspace.ResourceMetadata{
			ID:        workspace.ResourceID(e.Id),
			UpdatedAt: e.UpdatedAt.AsTime(),
		}
	}

	return newWS
}

func CountElementDiagrams(ws *workspace.Workspace) int {
	count := 0
	for _, element := range ws.Elements {
		if element.HasView {
			count++
		}
	}
	return count
}

func exportedDiagramLabel(diagram *diagv1.View, elementName string) string {
	if label := strings.TrimSpace(diagram.GetLevelLabel()); label != "" {
		return label
	}
	name := strings.TrimSpace(diagram.Name)
	if name != "" && !strings.EqualFold(name, strings.TrimSpace(elementName)) {
		return name
	}
	return ""
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

	for _, navigation := range msg.Navigations {
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
	return strings.EqualFold(
		strings.TrimSpace(exportedDiagramLabel(diagram, element.Name)),
		strings.TrimSpace(element.ViewLabel),
	)
}

// Package planner converts a workspace into an ApplyPlanRequest and builds diagram execution order.
package planner

import (
	"fmt"
	"maps"
	"sort"
	"strings"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/mertcikla/tld/v2/internal/tech"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const syntheticRootViewRef = "root"

// Plan holds the resolved ApplyPlanRequest and a topologically-sorted list of
// diagram refs (parents before children).
type Plan struct {
	Request      *diagv1.ApplyPlanRequest
	DiagramOrder []string // sorted refs
	Model        string
	ViewNames    map[string]string
}

// Build resolves workspace refs into an ApplyPlanRequest, ordering diagrams
// topologically (parents before children) so the server can resolve
// parent_diagram_ref in insertion order.
func Build(ws *workspace.Workspace, recreateIDs bool) (*Plan, error) {
	if usesElementWorkspace(ws) || hasActiveRepositoryConfig(ws) {
		return buildFromElements(ws, recreateIDs)
	}

	return &Plan{
		Request: &diagv1.ApplyPlanRequest{
			OrgId:  ws.Config.WorkspaceID,
			DryRun: new(bool),
		},
		Model: "workspace",
	}, nil
}

func buildFromElements(ws *workspace.Workspace, recreateIDs bool) (*Plan, error) {
	elements := ws.Elements
	rootRef, syntheticRoot, err := resolveRepositoryRootElement(ws)
	if err != nil {
		return nil, err
	}
	if syntheticRoot != nil {
		elements = cloneElements(ws.Elements)
		elements[rootRef] = syntheticRoot
	}

	canonicalViews := canonicalViewRefs(elements, ws.Connectors)

	includedElements := make(map[string]bool)
	elementRefs := make([]string, 0, len(elements))
	for ref := range elements {
		elementRefs = append(elementRefs, ref)
	}
	sort.Strings(elementRefs)

	connectorRefs := make([]string, 0, len(ws.Connectors))
	for ref := range ws.Connectors {
		connectorRefs = append(connectorRefs, ref)
	}
	sort.Strings(connectorRefs)

	req := &diagv1.ApplyPlanRequest{
		OrgId:  ws.Config.WorkspaceID,
		DryRun: new(bool),
	}
	viewNames := make(map[string]string)

	for _, ref := range elementRefs {
		element := elements[ref]
		if ws.ActiveRepo != "" && element.Owner != "" && element.Owner != ws.ActiveRepo {
			continue
		}
		includedElements[ref] = true
		hasView := element.HasView
		if _, ok := canonicalViews[ref]; ok {
			hasView = true
		}
		planElement := &diagv1.PlanElement{
			Ref:     ref,
			Name:    element.Name,
			HasView: hasView,
		}
		if element.Kind != "" {
			planElement.Kind = &element.Kind
		}
		if element.Description != "" {
			planElement.Description = &element.Description
		}
		if element.Technology != "" {
			planElement.Technology = &element.Technology
		}
		planElement.TechnologyLinks = technologyLinksForElement(element.Technology, element.Language)
		if element.URL != "" {
			planElement.Url = &element.URL
		}
		if element.LogoURL != "" {
			planElement.LogoUrl = &element.LogoURL
		}
		if element.Repo != "" {
			planElement.Repo = &element.Repo
		}
		if element.Branch != "" {
			planElement.Branch = &element.Branch
		}
		if element.Language != "" {
			planElement.Language = &element.Language
		}
		if element.FilePath != "" {
			planElement.FilePath = &element.FilePath
		}
		if len(element.Tags) > 0 {
			planElement.Tags = append([]string(nil), element.Tags...)
		}
		if element.ViewLabel != "" {
			planElement.ViewLabel = &element.ViewLabel
		}
		if element.ViewName != "" {
			viewNames[ref] = element.ViewName
		}
		if element.DensityLevel != 0 {
			level := int32(element.DensityLevel)
			planElement.ViewDensityLevel = &level
		}
		for _, placement := range element.Placements {
			parentRef := placement.ParentRef
			if parentRef == "" {
				parentRef = syntheticRootViewRef
			}
			planPlacement := &diagv1.PlanViewPlacement{ParentRef: parentRef}
			if placement.PositionX != 0 {
				planPlacement.PositionX = &placement.PositionX
			}
			if placement.PositionY != 0 {
				planPlacement.PositionY = &placement.PositionY
			}
			if placement.VisibilityDelta != 0 {
				delta := int32(placement.VisibilityDelta)
				planPlacement.VisibilityDelta = &delta
			}
			planElement.Placements = append(planElement.Placements, planPlacement)
		}

		if !recreateIDs && ws.Meta != nil {
			if meta, ok := ws.Meta.Elements[ref]; ok {
				id := int32(meta.ID)
				planElement.Id = &id
				planElement.UpdatedAt = timestamppb.New(meta.UpdatedAt)
			}
			if hasView {
				if meta, ok := ws.Meta.Views[ref]; ok {
					id := int32(meta.ID)
					planElement.ViewId = &id
					planElement.ViewUpdatedAt = timestamppb.New(meta.UpdatedAt)
				}
			}
		}

		req.Elements = append(req.Elements, planElement)
	}

	for _, ref := range connectorRefs {
		connector := ws.Connectors[ref]
		if len(includedElements) > 0 {
			if !includedElements[connector.Source] || !includedElements[connector.Target] {
				continue
			}
		}
		viewRef := connector.View
		if viewRef == "" {
			viewRef = syntheticRootViewRef
		}
		planConnector := &diagv1.PlanConnector{
			Ref:              ref,
			ViewRef:          viewRef,
			SourceElementRef: connector.Source,
			TargetElementRef: connector.Target,
		}
		if connector.Label != "" {
			planConnector.Label = &connector.Label
		}
		if connector.Description != "" {
			planConnector.Description = &connector.Description
		}
		if connector.Relationship != "" {
			planConnector.Relationship = &connector.Relationship
		}
		if connector.Direction != "" {
			planConnector.Direction = &connector.Direction
		}
		if connector.Style != "" {
			planConnector.Style = &connector.Style
		}
		if connector.URL != "" {
			planConnector.Url = &connector.URL
		}
		if connector.SourceHandle != "" {
			planConnector.SourceHandle = &connector.SourceHandle
		}
		if connector.TargetHandle != "" {
			planConnector.TargetHandle = &connector.TargetHandle
		}
		if connector.VisibilityDelta != 0 {
			delta := int32(connector.VisibilityDelta)
			planConnector.VisibilityDelta = &delta
		}

		if !recreateIDs && ws.Meta != nil {
			if meta, ok := ws.Meta.Connectors[ref]; ok {
				id := int32(meta.ID)
				planConnector.Id = &id
				planConnector.UpdatedAt = timestamppb.New(meta.UpdatedAt)
			}
		}

		req.Connectors = append(req.Connectors, planConnector)
	}

	return &Plan{Request: req, Model: "workspace", ViewNames: viewNames}, nil
}

func technologyLinksForElement(technology, language string) []*diagv1.TechnologyLink {
	links := technologyLinksForLabel(technology)
	if len(links) > 0 {
		return links
	}
	return technologyLinksForLanguage(language)
}

func technologyLinksForLabel(label string) []*diagv1.TechnologyLink {
	var links []*diagv1.TechnologyLink
	seen := map[string]struct{}{}
	for _, part := range technologyLabelParts(label) {
		slug, displayLabel := technologyCatalogMatch(part)
		if slug == "" {
			continue
		}
		if _, ok := seen[slug]; ok {
			continue
		}
		seen[slug] = struct{}{}
		links = append(links, catalogTechnologyLink(slug, displayLabel, len(links) == 0))
		if len(links) == 3 {
			break
		}
	}
	return links
}

func technologyLinksForLanguage(language string) []*diagv1.TechnologyLink {
	switch strings.ToLower(strings.TrimSpace(language)) {
	case "go":
		return []*diagv1.TechnologyLink{catalogTechnologyLink("golang", "Go", true)}
	case "typescript":
		return []*diagv1.TechnologyLink{catalogTechnologyLink("typescript", "TypeScript", true)}
	case "javascript":
		return []*diagv1.TechnologyLink{catalogTechnologyLink("javascript", "JavaScript", true)}
	case "python":
		return []*diagv1.TechnologyLink{catalogTechnologyLink("python", "Python", true)}
	case "java":
		return []*diagv1.TechnologyLink{catalogTechnologyLink("java", "Java", true)}
	case "cpp":
		return []*diagv1.TechnologyLink{catalogTechnologyLink("c-plusplus", "C++", true)}
	case "c":
		return []*diagv1.TechnologyLink{catalogTechnologyLink("c", "C", true)}
	case "json":
		return []*diagv1.TechnologyLink{catalogTechnologyLink("json-javascript-object-notation", "JSON", true)}
	default:
		return nil
	}
}

func technologyLabelParts(label string) []string {
	parts := strings.FieldsFunc(label, func(r rune) bool {
		return r == ',' || r == '/' || r == ';' || r == '|'
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 && strings.TrimSpace(label) != "" {
		return []string{strings.TrimSpace(label)}
	}
	return out
}

func technologyCatalogMatch(label string) (string, string) {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "architecture":
		return "architecture", "Architecture"
	case "structural":
		return "structural", "Structural"
	case "container":
		return "docker", "Container"
	default:
		slug, name, ok := tech.LookupCatalogFuzzy(label)
		if !ok {
			return "", ""
		}
		return slug, name
	}
}

func catalogTechnologyLink(slug, label string, primary bool) *diagv1.TechnologyLink {
	return &diagv1.TechnologyLink{
		Type:          "catalog",
		Slug:          &slug,
		Label:         label,
		IsPrimaryIcon: primary,
	}
}

func canonicalViewRefs(elements map[string]*workspace.Element, connectors map[string]*workspace.Connector) map[string]struct{} {
	refs := make(map[string]struct{})
	for ref, element := range elements {
		if element == nil || !element.HasView {
			continue
		}
		refs[ref] = struct{}{}
	}
	for _, element := range elements {
		if element == nil {
			continue
		}
		for _, placement := range element.Placements {
			if placement.ParentRef == "" || placement.ParentRef == syntheticRootViewRef {
				continue
			}
			refs[placement.ParentRef] = struct{}{}
		}
	}
	for _, connector := range connectors {
		if connector == nil || connector.View == "" || connector.View == syntheticRootViewRef {
			continue
		}
		refs[connector.View] = struct{}{}
	}
	return refs
}

func usesElementWorkspace(ws *workspace.Workspace) bool {
	return len(ws.Elements) > 0 || len(ws.Connectors) > 0
}

func hasActiveRepositoryConfig(ws *workspace.Workspace) bool {
	if ws == nil || ws.ActiveRepo == "" || ws.WorkspaceConfig == nil {
		return false
	}
	_, ok := ws.WorkspaceConfig.Repositories[ws.ActiveRepo]
	return ok
}

func resolveRepositoryRootElement(ws *workspace.Workspace) (string, *workspace.Element, error) {
	if ws == nil || ws.ActiveRepo == "" || ws.WorkspaceConfig == nil {
		return "", nil, nil
	}
	repository, ok := ws.WorkspaceConfig.Repositories[ws.ActiveRepo]
	if !ok {
		return "", nil, nil
	}

	if repository.Root != "" {
		element, ok := ws.Elements[repository.Root]
		if !ok || element == nil {
			return "", nil, fmt.Errorf("repository %q root %q not found in elements", ws.ActiveRepo, repository.Root)
		}
		return repository.Root, nil, nil
	}

	candidates := repositoryRootCandidates(ws.Elements, ws.ActiveRepo)
	switch len(candidates) {
	case 0:
		if hasRootLevelElement(ws.Elements, ws.ActiveRepo) {
			return "", nil, nil
		}
		ref := uniqueRepositoryRootRef(ws.ActiveRepo, ws.Elements)
		return ref, &workspace.Element{
			Name:      ws.ActiveRepo,
			Kind:      "repository",
			Owner:     ws.ActiveRepo,
			HasView:   true,
			ViewLabel: ws.ActiveRepo,
			Placements: []workspace.ViewPlacement{{
				ParentRef: syntheticRootViewRef,
			}},
		}, nil
	case 1:
		return candidates[0], nil, nil
	default:
		return "", nil, nil
	}
}

func repositoryRootCandidates(elements map[string]*workspace.Element, repoName string) []string {
	var candidates []string
	for ref, element := range elements {
		if element == nil {
			continue
		}
		if element.Kind != "repository" {
			continue
		}
		if element.Owner != "" && element.Owner != repoName {
			continue
		}
		if !isRootLevelElement(element) {
			continue
		}
		candidates = append(candidates, ref)
	}
	sort.Strings(candidates)
	return candidates
}

func hasRootLevelElement(elements map[string]*workspace.Element, repoName string) bool {
	for _, element := range elements {
		if element == nil {
			continue
		}
		if element.Owner != "" && element.Owner != repoName {
			continue
		}
		if isRootLevelElement(element) {
			return true
		}
	}
	return false
}

func isRootLevelElement(element *workspace.Element) bool {
	if len(element.Placements) == 0 {
		return true
	}
	for _, placement := range element.Placements {
		if placement.ParentRef == "" || placement.ParentRef == syntheticRootViewRef {
			return true
		}
	}
	return false
}

func uniqueRepositoryRootRef(repoName string, elements map[string]*workspace.Element) string {
	base := workspace.Slugify(repoName)
	if base == "" {
		base = "repository"
	}
	if _, taken := elements[base]; !taken {
		return base
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if _, taken := elements[candidate]; !taken {
			return candidate
		}
	}
}

func cloneElements(source map[string]*workspace.Element) map[string]*workspace.Element {
	cloned := make(map[string]*workspace.Element, len(source))
	maps.Copy(cloned, source)
	return cloned
}

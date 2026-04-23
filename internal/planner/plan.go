// Package planner converts a workspace into an ApplyPlanRequest and builds diagram execution order.
package planner

import (
	"fmt"
	"maps"
	"sort"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/mertcikla/tld/internal/workspace"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const syntheticRootViewRef = "root"

// Plan holds the resolved ApplyPlanRequest and a topologically-sorted list of
// diagram refs (parents before children).
type Plan struct {
	Request      *diagv1.ApplyPlanRequest
	DiagramOrder []string // sorted refs
	Model        string
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
		if element.ViewLabel != "" {
			planElement.ViewLabel = &element.ViewLabel
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

		if !recreateIDs && ws.Meta != nil {
			if meta, ok := ws.Meta.Connectors[ref]; ok {
				id := int32(meta.ID)
				planConnector.Id = &id
				planConnector.UpdatedAt = timestamppb.New(meta.UpdatedAt)
			}
		}

		req.Connectors = append(req.Connectors, planConnector)
	}

	return &Plan{Request: req, Model: "workspace"}, nil
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
		if element.Kind != "repository" {
			return "", nil, fmt.Errorf("repository %q root %q must be kind repository, got %q", ws.ActiveRepo, repository.Root, element.Kind)
		}
		return repository.Root, nil, nil
	}

	candidates := repositoryRootCandidates(ws.Elements, ws.ActiveRepo)
	switch len(candidates) {
	case 0:
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
		return "", nil, fmt.Errorf("repository %q has multiple root candidates %v; set repositories[%q].root explicitly", ws.ActiveRepo, candidates, ws.ActiveRepo)
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
		if len(element.Placements) > 0 {
			rooted := false
			for _, placement := range element.Placements {
				if placement.ParentRef == "root" || placement.ParentRef == syntheticRootViewRef || placement.ParentRef == "" {
					rooted = true
					break
				}
			}
			if !rooted {
				continue
			}
		}
		candidates = append(candidates, ref)
	}
	sort.Strings(candidates)
	return candidates
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

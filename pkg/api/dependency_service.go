package api

import (
	"context"
	"fmt"
	"strconv"

	"buf.build/gen/go/tldiagramcom/diagram/connectrpc/go/diag/v1/diagv1connect"
	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
)

var _ diagv1connect.DependencyServiceHandler = (*DependencyService)(nil)

// DependencyService implements diagv1connect.DependencyServiceHandler.
type DependencyService struct {
	Store Store
	Hooks WorkspaceHooks
	diagv1connect.UnimplementedDependencyServiceHandler
}

func (s *DependencyService) hooks() WorkspaceHooks {
	if s.Hooks == nil {
		return NopWorkspaceHooks{}
	}
	return s.Hooks
}

// ListDependencies returns all elements and connectors for the organisation.
func (s *DependencyService) ListDependencies(ctx context.Context, req *connect.Request[diagv1.ListDependenciesRequest]) (*connect.Response[diagv1.ListDependenciesResponse], error) {
	workspaceID := WorkspaceIDFromCtx(ctx)
	if err := s.hooks().CheckRead(ctx, workspaceID); err != nil {
		return nil, err
	}

	elements, err := s.Store.ListElements(ctx, workspaceID, 0, 0, "")
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list elements: %w", err))
	}

	connectors, err := s.Store.ListAllConnectors(ctx, workspaceID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("list connectors: %w", err))
	}

	depElements := make([]*diagv1.DependencyElement, 0, len(elements))
	for _, e := range elements {
		depElements = append(depElements, elementToDependencyProto(e))
	}

	depConnectors := make([]*diagv1.DependencyConnector, 0, len(connectors))
	for _, c := range connectors {
		depConnectors = append(depConnectors, connectorToDependencyProto(c))
	}

	return connect.NewResponse(&diagv1.ListDependenciesResponse{
		Elements:   depElements,
		Connectors: depConnectors,
	}), nil
}

func elementToDependencyProto(e *diagv1.Element) *diagv1.DependencyElement {
	de := &diagv1.DependencyElement{
		Id:          strconv.Itoa(int(e.GetId())),
		Name:        e.GetName(),
		Type:        e.Kind,
		Description: e.Description,
		Technology:  e.Technology,
		Url:         e.Url,
		LogoUrl:     e.LogoUrl,
		Tags:        e.GetTags(),
		Repo:        e.Repo,
		Branch:      e.Branch,
		Language:    e.Language,
		FilePath:    e.FilePath,
		CreatedAt:   e.CreatedAt,
		UpdatedAt:   e.UpdatedAt,
	}

	if len(e.GetTechnologyLinks()) > 0 {
		links := make([]*diagv1.DependencyTechnologyLink, 0, len(e.GetTechnologyLinks()))
		for _, tl := range e.GetTechnologyLinks() {
			slug := tl.GetSlug()
			links = append(links, &diagv1.DependencyTechnologyLink{
				Type:          tl.GetType(),
				Slug:          slug,
				Label:         tl.GetLabel(),
				IsPrimaryIcon: tl.GetIsPrimaryIcon(),
			})
		}
		de.TechnologyLinks = links
	}

	return de
}

func connectorToDependencyProto(c *diagv1.Connector) *diagv1.DependencyConnector {
	return &diagv1.DependencyConnector{
		Id:              strconv.Itoa(int(c.GetId())),
		ViewId:          strconv.Itoa(int(c.GetViewId())),
		SourceElementId: strconv.Itoa(int(c.GetSourceElementId())),
		TargetElementId: strconv.Itoa(int(c.GetTargetElementId())),
		Direction:       c.GetDirection(),
		Style:           c.GetStyle(),
		Label:           c.Label,
		Description:     c.Description,
		Relationship:    c.Relationship,
		Url:             c.Url,
		SourceHandle:    c.SourceHandle,
		TargetHandle:    c.TargetHandle,
		CreatedAt:       c.CreatedAt,
		UpdatedAt:       c.UpdatedAt,
	}
}

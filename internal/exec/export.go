package exec

import (
	"context"
	"fmt"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

// ExportWorkspace fetches the full target state in the shared export format so
// callers can materialize a workspace snapshot from the DB or cloud.
func (r *remoteRunner) ExportWorkspace(ctx context.Context) (*diagv1.ExportOrganizationResponse, error) {
	c := r.client()
	resp, err := c.ExportWorkspace(ctx, connect.NewRequest(&diagv1.ExportOrganizationRequest{
		OrgId: r.orgID,
	}))
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}

func (r *localRunner) ExportWorkspace(ctx context.Context) (*diagv1.ExportOrganizationResponse, error) {
	var (
		views      []*diagv1.View
		elements   []*diagv1.Element
		placements []*diagv1.PlacedElement
		connectors []*diagv1.Connector
		layers     []*diagv1.ViewLayer
	)

	c := r.ctx(ctx)
	g, gctx := errgroup.WithContext(c)
	g.Go(func() error { var e error; views, e = r.adapter.ListViews(gctx, uuid.Nil); return e })
	g.Go(func() error {
		var e error
		elements, _, e = r.adapter.ListElements(gctx, uuid.Nil, 0, 0, "")
		return e
	})
	g.Go(func() error { var e error; placements, e = r.adapter.ListAllPlacements(gctx, uuid.Nil); return e })
	g.Go(func() error { var e error; connectors, e = r.adapter.ListAllConnectors(gctx, uuid.Nil); return e })
	g.Go(func() error { var e error; layers, e = r.adapter.ListAllViewLayers(gctx, uuid.Nil); return e })

	if err := g.Wait(); err != nil {
		return nil, fmt.Errorf("local export query: %w", err)
	}

	exportPlacements := make([]*diagv1.ElementPlacement, 0, len(placements))
	for _, p := range placements {
		exportPlacements = append(exportPlacements, &diagv1.ElementPlacement{
			Id:        p.Id,
			ViewId:    p.ViewId,
			ElementId: p.ElementId,
			PositionX: p.PositionX,
			PositionY: p.PositionY,
		})
	}

	elementToChildView := make(map[int32]*diagv1.View, len(views))
	for _, v := range views {
		if v.OwnerElementId != nil {
			elementToChildView[*v.OwnerElementId] = v
		}
	}
	navigations := make([]*diagv1.ElementNavigation, 0)
	for _, p := range placements {
		childView, ok := elementToChildView[p.ElementId]
		if !ok {
			continue
		}
		navigations = append(navigations, &diagv1.ElementNavigation{
			Id:         p.Id,
			ElementId:  p.ElementId,
			FromViewId: p.ViewId,
			ToViewId:   childView.Id,
		})
	}

	return &diagv1.ExportOrganizationResponse{
		Views:       views,
		Elements:    elements,
		Navigations: navigations,
		Placements:  exportPlacements,
		Connectors:  connectors,
		Layers:      layers,
	}, nil
}

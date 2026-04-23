package store

import (
	"context"
	"embed"

	"github.com/mertcikla/tld/internal/app"
	"github.com/mertcikla/tld/internal/core"
)

type SQLiteStore struct {
	legacy *app.Store
}

var _ core.Store = (*SQLiteStore)(nil)

func Open(dbPath string, migrations embed.FS) (*SQLiteStore, error) {
	legacy, err := app.OpenStore(dbPath, migrations)
	if err != nil {
		return nil, err
	}
	return &SQLiteStore{legacy: legacy}, nil
}

func (s *SQLiteStore) Legacy() *app.Store {
	return s.legacy
}

func (s *SQLiteStore) ViewTree(ctx context.Context) ([]core.ViewTreeNode, error) {
	out, err := s.legacy.ViewTree(ctx)
	if err != nil {
		return nil, err
	}
	return convertSlice(out, func(v app.ViewTreeNode) core.ViewTreeNode { return core.ViewTreeNode(v) }), nil
}

func (s *SQLiteStore) Views(ctx context.Context) ([]core.ViewSummary, error) {
	out, err := s.legacy.Views(ctx)
	if err != nil {
		return nil, err
	}
	return convertSlice(out, func(v app.ViewSummary) core.ViewSummary { return core.ViewSummary(v) }), nil
}

func (s *SQLiteStore) ViewByID(ctx context.Context, id int64) (core.ViewTreeNode, error) {
	out, err := s.legacy.ViewByID(ctx, id)
	if err != nil {
		return core.ViewTreeNode{}, err
	}
	return core.ViewTreeNode(out), nil
}

func (s *SQLiteStore) CreateView(ctx context.Context, name string, levelLabel *string, ownerElementID *int64) (core.ViewSummary, error) {
	out, err := s.legacy.CreateView(ctx, name, levelLabel, ownerElementID)
	if err != nil {
		return core.ViewSummary{}, err
	}
	return core.ViewSummary(out), nil
}

func (s *SQLiteStore) UpdateView(ctx context.Context, id int64, name *string, levelLabel *string) (core.ViewSummary, error) {
	out, err := s.legacy.UpdateView(ctx, id, name, levelLabel)
	if err != nil {
		return core.ViewSummary{}, err
	}
	return core.ViewSummary(out), nil
}

func (s *SQLiteStore) SetViewLevel(ctx context.Context, id int64, level int) error {
	return s.legacy.SetViewLevel(ctx, id, level)
}

func (s *SQLiteStore) DeleteView(ctx context.Context, id int64) error {
	return s.legacy.DeleteView(ctx, id)
}

func (s *SQLiteStore) Placements(ctx context.Context, viewID int64) ([]core.PlacedElement, error) {
	out, err := s.legacy.Placements(ctx, viewID)
	if err != nil {
		return nil, err
	}
	return convertSlice(out, func(v app.PlacedElement) core.PlacedElement { return core.PlacedElement(v) }), nil
}

func (s *SQLiteStore) ElementPlacements(ctx context.Context, viewID int64) ([]core.ElementPlacement, error) {
	out, err := s.legacy.ElementPlacements(ctx, viewID)
	if err != nil {
		return nil, err
	}
	return convertSlice(out, func(v app.ElementPlacement) core.ElementPlacement { return core.ElementPlacement(v) }), nil
}

func (s *SQLiteStore) AddPlacement(ctx context.Context, viewID, elementID int64, x, y float64) (core.ElementPlacement, error) {
	out, err := s.legacy.AddPlacement(ctx, viewID, elementID, x, y)
	if err != nil {
		return core.ElementPlacement{}, err
	}
	return core.ElementPlacement(out), nil
}

func (s *SQLiteStore) UpdatePlacement(ctx context.Context, viewID, elementID int64, x, y float64) error {
	return s.legacy.UpdatePlacement(ctx, viewID, elementID, x, y)
}

func (s *SQLiteStore) DeletePlacement(ctx context.Context, viewID, elementID int64) error {
	return s.legacy.DeletePlacement(ctx, viewID, elementID)
}

func (s *SQLiteStore) Layers(ctx context.Context, viewID int64) ([]core.ViewLayer, error) {
	out, err := s.legacy.Layers(ctx, viewID)
	if err != nil {
		return nil, err
	}
	return convertSlice(out, func(v app.ViewLayer) core.ViewLayer { return core.ViewLayer(v) }), nil
}

func (s *SQLiteStore) CreateLayer(ctx context.Context, viewID int64, name string, tags []string, color *string) (core.ViewLayer, error) {
	out, err := s.legacy.CreateLayer(ctx, viewID, name, tags, color)
	if err != nil {
		return core.ViewLayer{}, err
	}
	return core.ViewLayer(out), nil
}

func (s *SQLiteStore) UpdateLayer(ctx context.Context, id int64, patch core.ViewLayer) (core.ViewLayer, error) {
	out, err := s.legacy.UpdateLayer(ctx, id, app.ViewLayer(patch))
	if err != nil {
		return core.ViewLayer{}, err
	}
	return core.ViewLayer(out), nil
}

func (s *SQLiteStore) DeleteLayer(ctx context.Context, id int64) error {
	return s.legacy.DeleteLayer(ctx, id)
}

func (s *SQLiteStore) ThumbnailSVG(ctx context.Context, viewID int64) (string, error) {
	return s.legacy.ThumbnailSVG(ctx, viewID)
}

func (s *SQLiteStore) Elements(ctx context.Context, limit, offset int, search string) ([]core.LibraryElement, error) {
	out, err := s.legacy.Elements(ctx, limit, offset, search)
	if err != nil {
		return nil, err
	}
	return convertSlice(out, func(v app.LibraryElement) core.LibraryElement { return core.LibraryElement(v) }), nil
}

func (s *SQLiteStore) ElementByID(ctx context.Context, id int64) (core.LibraryElement, error) {
	out, err := s.legacy.ElementByID(ctx, id)
	if err != nil {
		return core.LibraryElement{}, err
	}
	return core.LibraryElement(out), nil
}

func (s *SQLiteStore) CreateElement(ctx context.Context, input core.LibraryElement) (core.LibraryElement, error) {
	out, err := s.legacy.CreateElement(ctx, app.LibraryElement(input))
	if err != nil {
		return core.LibraryElement{}, err
	}
	return core.LibraryElement(out), nil
}

func (s *SQLiteStore) UpdateElement(ctx context.Context, id int64, input core.LibraryElement) (core.LibraryElement, error) {
	out, err := s.legacy.UpdateElement(ctx, id, app.LibraryElement(input))
	if err != nil {
		return core.LibraryElement{}, err
	}
	return core.LibraryElement(out), nil
}

func (s *SQLiteStore) DeleteElement(ctx context.Context, id int64) error {
	return s.legacy.DeleteElement(ctx, id)
}

func (s *SQLiteStore) ListElementPlacements(ctx context.Context, elementID int64) ([]core.ViewPlacement, error) {
	out, err := s.legacy.ListElementPlacements(ctx, elementID)
	if err != nil {
		return nil, err
	}
	return convertSlice(out, func(v app.ViewPlacement) core.ViewPlacement { return core.ViewPlacement(v) }), nil
}

func (s *SQLiteStore) ListElementNavigations(ctx context.Context, elementID int64, fromViewID, toViewID *int64) ([]core.ViewConnector, error) {
	out, err := s.legacy.ListElementNavigations(ctx, elementID, fromViewID, toViewID)
	if err != nil {
		return nil, err
	}
	return convertSlice(out, func(v app.ViewConnector) core.ViewConnector { return core.ViewConnector(v) }), nil
}

func (s *SQLiteStore) Connectors(ctx context.Context, viewID int64) ([]core.Connector, error) {
	out, err := s.legacy.Connectors(ctx, viewID)
	if err != nil {
		return nil, err
	}
	return convertSlice(out, func(v app.Connector) core.Connector { return core.Connector(v) }), nil
}

func (s *SQLiteStore) CreateConnector(ctx context.Context, input core.Connector) (core.Connector, error) {
	out, err := s.legacy.CreateConnector(ctx, app.Connector(input))
	if err != nil {
		return core.Connector{}, err
	}
	return core.Connector(out), nil
}

func (s *SQLiteStore) UpdateConnector(ctx context.Context, id int64, patch core.Connector) (core.Connector, error) {
	out, err := s.legacy.UpdateConnector(ctx, id, app.Connector(patch))
	if err != nil {
		return core.Connector{}, err
	}
	return core.Connector(out), nil
}

func (s *SQLiteStore) DeleteConnector(ctx context.Context, id int64) error {
	return s.legacy.DeleteConnector(ctx, id)
}

func (s *SQLiteStore) Tags(ctx context.Context) (map[string]core.Tag, error) {
	out, err := s.legacy.Tags(ctx)
	if err != nil {
		return nil, err
	}
	converted := make(map[string]core.Tag, len(out))
	for key, value := range out {
		converted[key] = core.Tag(value)
	}
	return converted, nil
}

func (s *SQLiteStore) UpdateTag(ctx context.Context, name, color string, description *string) error {
	return s.legacy.UpdateTag(ctx, name, color, description)
}

func (s *SQLiteStore) Explore(ctx context.Context) (core.ExploreData, error) {
	out, err := s.legacy.Explore(ctx)
	if err != nil {
		return core.ExploreData{}, err
	}
	return core.ExploreData(out), nil
}

func (s *SQLiteStore) Dependencies(ctx context.Context) (map[string]any, error) {
	return s.legacy.Dependencies(ctx)
}

func (s *SQLiteStore) ImportPlan(ctx context.Context, elements []core.PlanElement, connectors []core.PlanConnector) (int64, error) {
	return s.legacy.ImportPlan(
		ctx,
		convertSlice(elements, func(v core.PlanElement) app.PlanElement { return app.PlanElement(v) }),
		convertSlice(connectors, func(v core.PlanConnector) app.PlanConnector { return app.PlanConnector(v) }),
	)
}

func convertSlice[A any, B any](in []A, convert func(A) B) []B {
	if len(in) == 0 {
		return nil
	}
	out := make([]B, 0, len(in))
	for _, item := range in {
		out = append(out, convert(item))
	}
	return out
}

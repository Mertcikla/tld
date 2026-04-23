package core

import "context"

type ViewStore interface {
	ViewTree(ctx context.Context) ([]ViewTreeNode, error)
	Views(ctx context.Context) ([]ViewSummary, error)
	ViewByID(ctx context.Context, id int64) (ViewTreeNode, error)
	CreateView(ctx context.Context, name string, levelLabel *string, ownerElementID *int64) (ViewSummary, error)
	UpdateView(ctx context.Context, id int64, name *string, levelLabel *string) (ViewSummary, error)
	SetViewLevel(ctx context.Context, id int64, level int) error
	DeleteView(ctx context.Context, id int64) error
	Placements(ctx context.Context, viewID int64) ([]PlacedElement, error)
	ElementPlacements(ctx context.Context, viewID int64) ([]ElementPlacement, error)
	AddPlacement(ctx context.Context, viewID, elementID int64, x, y float64) (ElementPlacement, error)
	UpdatePlacement(ctx context.Context, viewID, elementID int64, x, y float64) error
	DeletePlacement(ctx context.Context, viewID, elementID int64) error
	Layers(ctx context.Context, viewID int64) ([]ViewLayer, error)
	CreateLayer(ctx context.Context, viewID int64, name string, tags []string, color *string) (ViewLayer, error)
	UpdateLayer(ctx context.Context, id int64, patch ViewLayer) (ViewLayer, error)
	DeleteLayer(ctx context.Context, id int64) error
	ThumbnailSVG(ctx context.Context, viewID int64) (string, error)
}

type ElementStore interface {
	Elements(ctx context.Context, limit, offset int, search string) ([]LibraryElement, error)
	ElementByID(ctx context.Context, id int64) (LibraryElement, error)
	CreateElement(ctx context.Context, input LibraryElement) (LibraryElement, error)
	UpdateElement(ctx context.Context, id int64, input LibraryElement) (LibraryElement, error)
	DeleteElement(ctx context.Context, id int64) error
	ListElementPlacements(ctx context.Context, elementID int64) ([]ViewPlacement, error)
	ListElementNavigations(ctx context.Context, elementID int64, fromViewID, toViewID *int64) ([]ViewConnector, error)
}

type ConnectorStore interface {
	Connectors(ctx context.Context, viewID int64) ([]Connector, error)
	CreateConnector(ctx context.Context, input Connector) (Connector, error)
	UpdateConnector(ctx context.Context, id int64, patch Connector) (Connector, error)
	DeleteConnector(ctx context.Context, id int64) error
}

type TagStore interface {
	Tags(ctx context.Context) (map[string]Tag, error)
	UpdateTag(ctx context.Context, name, color string, description *string) error
}

type ExploreStore interface {
	Explore(ctx context.Context) (ExploreData, error)
	Dependencies(ctx context.Context) (map[string]any, error)
}

type ImportStore interface {
	ImportPlan(ctx context.Context, elements []PlanElement, connectors []PlanConnector) (int64, error)
}

type Store interface {
	ViewStore
	ElementStore
	ConnectorStore
	TagStore
	ExploreStore
	ImportStore
}

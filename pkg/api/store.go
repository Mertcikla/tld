// Package api defines the shared ConnectRPC workspace service layer.
//
// The Store interface abstracts persistence so both the cloud backend
// (PostgreSQL) and the offline tld app (SQLite) can serve identical wire APIs.
// Pro-only features (versioning, etc.) may return errors wrapping ErrUnimplemented
// from single-tenant deployments.
package api

import (
	"context"
	"encoding/json"
	"errors"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/google/uuid"
	"github.com/mertcikla/tld/v2/pkg/app"
)

// ErrUnimplemented is returned by Store methods that are not supported by a
// particular implementation (e.g. versioning in the offline tld app).
var ErrUnimplemented = errors.New("not implemented")

// ErrMarkdownFileChanged is returned when a save includes a stale file token.
var ErrMarkdownFileChanged = errors.New("markdown file changed on disk")

type contextKey string

const ctxKeyWorkspaceID contextKey = "api_org_id"

// WorkspaceIDFromCtx extracts the org ID injected by auth middleware or test helpers.
// Returns uuid.Nil when no org ID is present (tld single-tenant case).
func WorkspaceIDFromCtx(ctx context.Context) uuid.UUID {
	id, _ := ctx.Value(ctxKeyWorkspaceID).(uuid.UUID)
	return id
}

// WithWorkspaceID returns a context carrying the given org ID.
func WithWorkspaceID(ctx context.Context, id uuid.UUID) context.Context {
	ctx = context.WithValue(ctx, ctxKeyWorkspaceID, id)
	if id != uuid.Nil {
		ctx = app.WithTenantOrgID(ctx, id)
	}
	return ctx
}

// ElementInput holds mutable fields for element create/update.
type ElementInput struct {
	Name            string
	Description     *string
	Kind            *string
	Technology      *string
	URL             *string
	LogoURL         *string
	TechLinks       []*diagv1.TechnologyLink
	Tags            []string
	Repo            *string
	Branch          *string
	Language        *string
	FilePath        *string
	BypassNoiseGate *bool
	HasView         bool
	ViewLabel       *string
}

// ConnectorInput holds mutable fields for connector create/update.
type ConnectorInput struct {
	ViewID       int32
	SourceID     int32
	TargetID     int32
	Label        *string
	Description  *string
	Relationship *string
	Direction    string
	Style        string
	URL          *string
	SourceHandle *string
	TargetHandle *string
	Tags         []string
}

// CollaborationIdentity is the per-request/display identity used by core
// collaboration without creating a user/account system.
type CollaborationIdentity struct {
	ClientID string
	UserID   string
	Username string
}

// RealtimeDrawingInput holds durable freehand/text drawing state for a view.
type RealtimeDrawingInput struct {
	PathID   string
	UserID   string
	Points   json.RawMessage
	Color    string
	Width    float64
	Text     string
	FontSize float64
}

// Store is the persistence interface for the shared workspace API.
// Implementations exist for PostgreSQL (backend) and SQLite (tld).
type Store interface {
	// Views
	ListViews(ctx context.Context, workspaceID uuid.UUID) ([]*diagv1.View, error)
	GetViews(ctx context.Context, workspaceID uuid.UUID, parentViewID *int32, isRoot *bool, search string, limit, offset int) ([]*diagv1.View, int, error)
	GetView(ctx context.Context, id int32, workspaceID uuid.UUID) (*diagv1.View, error)
	GetProjectedViewContent(ctx context.Context, viewID int32, workspaceID uuid.UUID, densityOverride *int32) (*diagv1.ViewContent, error)
	GetViewMarkdown(ctx context.Context, viewID int32, workspaceID uuid.UUID) (*diagv1.ViewMarkdownDocument, string, error)
	CreateView(ctx context.Context, workspaceID uuid.UUID, ownerElementID *int32, name string, label *string, isRoot bool) (*diagv1.View, error)
	UpdateView(ctx context.Context, id int32, workspaceID uuid.UUID, name string, description *string, label *string, tags []string) (*diagv1.View, error)
	CreateViewMarkdown(ctx context.Context, viewID int32, workspaceID uuid.UUID, fileName *string, initialContent *string, targetKind string, path *string) (*diagv1.View, error)
	LinkViewMarkdown(ctx context.Context, viewID int32, workspaceID uuid.UUID, path string) (*diagv1.View, error)
	SaveViewMarkdown(ctx context.Context, viewID int32, workspaceID uuid.UUID, content string, expectedFileVersion *string, force bool) (*diagv1.ViewMarkdownDocument, error)
	UnlinkViewMarkdown(ctx context.Context, viewID int32, workspaceID uuid.UUID, deleteManagedFile bool) (*diagv1.View, error)
	DeleteView(ctx context.Context, id int32, workspaceID uuid.UUID) error

	// Elements
	ListElements(ctx context.Context, workspaceID uuid.UUID, limit, offset int32, search string) ([]*diagv1.Element, int, error)
	GetElement(ctx context.Context, id int32, workspaceID uuid.UUID) (*diagv1.Element, error)
	CreateElement(ctx context.Context, workspaceID uuid.UUID, input ElementInput) (*diagv1.Element, error)
	UpdateElement(ctx context.Context, id int32, workspaceID uuid.UUID, input ElementInput) (*diagv1.Element, error)
	DeleteElement(ctx context.Context, id int32, workspaceID uuid.UUID) error

	// Placements (view ↔ element positions)
	ListPlacements(ctx context.Context, viewID int32) ([]*diagv1.PlacedElement, error)
	ListAllPlacements(ctx context.Context, workspaceID uuid.UUID) ([]*diagv1.PlacedElement, error)
	ListElementPlacements(ctx context.Context, elementID int32, workspaceID uuid.UUID) ([]*diagv1.ViewPlacementInfo, error)
	AddPlacement(ctx context.Context, viewID, elementID int32, x, y float64) (*diagv1.PlacedElement, error)
	UpdatePlacementPosition(ctx context.Context, viewID, elementID int32, x, y float64) error
	RemovePlacement(ctx context.Context, viewID, elementID int32) error

	// Connectors
	ListConnectors(ctx context.Context, viewID int32, workspaceID uuid.UUID) ([]*diagv1.Connector, error)
	ListAllConnectors(ctx context.Context, workspaceID uuid.UUID) ([]*diagv1.Connector, error)
	GetConnector(ctx context.Context, id int32, workspaceID uuid.UUID) (*diagv1.Connector, error)
	CreateConnector(ctx context.Context, workspaceID uuid.UUID, input ConnectorInput) (*diagv1.Connector, error)
	UpdateConnector(ctx context.Context, id int32, workspaceID uuid.UUID, input ConnectorInput) (*diagv1.Connector, error)
	DeleteConnector(ctx context.Context, id int32, workspaceID uuid.UUID) error

	// Element navigations (derived from owner_element_id relationships)
	ListElementNavigations(ctx context.Context, workspaceID uuid.UUID, elementID int32) ([]*diagv1.ElementNavigationInfo, error)
	ListIncomingElementNavigations(ctx context.Context, viewID int32) ([]*diagv1.IncomingElementNavigationInfo, error)

	// View layers
	ListViewLayers(ctx context.Context, viewID int32) ([]*diagv1.ViewLayer, error)
	ListAllViewLayers(ctx context.Context, workspaceID uuid.UUID) ([]*diagv1.ViewLayer, error)
	GetViewLayer(ctx context.Context, id int32) (*diagv1.ViewLayer, error)
	CreateViewLayer(ctx context.Context, viewID int32, name string, tags []string, color string) (*diagv1.ViewLayer, error)
	UpdateViewLayer(ctx context.Context, id int32, name *string, tags []string, color *string) (*diagv1.ViewLayer, error)
	DeleteViewLayer(ctx context.Context, id int32) error

	// Tags
	Tags(ctx context.Context, workspaceID uuid.UUID) (map[string]*diagv1.Tag, error)
	UpdateTag(ctx context.Context, workspaceID uuid.UUID, name, color string, description *string) error
	DeleteTag(ctx context.Context, workspaceID uuid.UUID, name string) error

	// Collaboration
	ListViewThreads(ctx context.Context, workspaceID uuid.UUID, viewID int32, elementID, connectorID *int32) ([]*diagv1.ThreadInfo, error)
	CreateViewThread(ctx context.Context, workspaceID uuid.UUID, viewID int32, elementID, connectorID *int32, createdBy, createdByUsername string) (*diagv1.ThreadInfo, error)
	GetViewThread(ctx context.Context, workspaceID uuid.UUID, viewID, threadID int32) (*diagv1.ThreadInfo, error)
	SetViewThreadResolved(ctx context.Context, workspaceID uuid.UUID, viewID, threadID int32, resolved bool) error
	CreateViewComment(ctx context.Context, workspaceID uuid.UUID, viewID, threadID int32, authorID, authorUsername, body string) (*diagv1.CommentInfo, error)
	ListViewElementReactions(ctx context.Context, workspaceID uuid.UUID, viewID int32, userID string) ([]*diagv1.NodeReactionSummary, error)
	ToggleElementReaction(ctx context.Context, workspaceID uuid.UUID, viewID, elementID int32, userID, emoji string) (bool, error)
	ListDrawings(ctx context.Context, workspaceID uuid.UUID, viewID int32) ([]RealtimeDrawingInput, error)
	UpsertDrawing(ctx context.Context, workspaceID uuid.UUID, viewID int32, input RealtimeDrawingInput) error
	DeleteDrawing(ctx context.Context, workspaceID uuid.UUID, viewID int32, pathID string) error

	// ApplyPlan atomically applies a CLI workspace plan (create/update elements, views, connectors).
	ApplyPlan(ctx context.Context, workspaceID uuid.UUID, req *diagv1.ApplyPlanRequest) (*diagv1.ApplyPlanResponse, error)

	// Versioning may return ErrUnimplemented for single-tenant deployments.
	ListVersions(ctx context.Context, workspaceID uuid.UUID, limit int) ([]*diagv1.WorkspaceVersionInfo, error)
	GetLatestVersion(ctx context.Context, workspaceID uuid.UUID) (*diagv1.WorkspaceVersionInfo, error)
	CreateVersion(ctx context.Context, workspaceID uuid.UUID, versionID, source string, parentID *int32, viewCount, elementCount, connectorCount int, description, workspaceHash *string) (*diagv1.WorkspaceVersionInfo, error)
	GetVersioningEnabled(ctx context.Context, workspaceID uuid.UUID) (bool, error)
	SetVersioningEnabled(ctx context.Context, workspaceID uuid.UUID, enabled bool) error
	// GetWorkspaceResourceCounts returns current view/element/connector counts (for version snapshots).
	GetWorkspaceResourceCounts(ctx context.Context, workspaceID uuid.UUID) (views, elements, connectors int, err error)
}

// TransactionalStore is implemented by Store adapters that can run a logical
// workspace mutation against a transaction-bound store.
type TransactionalStore interface {
	RunInTransaction(ctx context.Context, fn func(context.Context, Store) error) error
}

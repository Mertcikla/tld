package exec

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	diagv1connect "buf.build/gen/go/tldiagramcom/diagram/connectrpc/go/diag/v1/diagv1connect"
	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/google/uuid"
	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/internal/client"
	"github.com/mertcikla/tld/v2/internal/localserver"
	"github.com/mertcikla/tld/v2/internal/store"
	"github.com/mertcikla/tld/v2/internal/term"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/mertcikla/tld/v2/pkg/api"
)

const (
	TargetAuto   = "auto"
	TargetLocal  = "local"
	TargetRemote = "remote"

	CloudAppURL = "https://tldiagram.com/app"
)

// Runner executes single-resource operations synchronously against either the
// local SQLite store or the remote ConnectRPC workspace service.
type Runner interface {
	Name() string
	TargetLabel() string
	Close() error

	CreateElement(ctx context.Context, input api.ElementInput) (*diagv1.Element, error)
	UpdateElement(ctx context.Context, id int32, input api.ElementInput) (*diagv1.Element, error)
	DeleteElement(ctx context.Context, id int32) error
	GetElement(ctx context.Context, id int32) (*diagv1.Element, error)
	ListElements(ctx context.Context, search string) ([]*diagv1.Element, error)

	ListViews(ctx context.Context) ([]*diagv1.View, error)
	CreateView(ctx context.Context, ownerElementID *int32, name string, label *string) (*diagv1.View, error)
	UpdateView(ctx context.Context, id int32, name string, label *string) (*diagv1.View, error)
	DeleteView(ctx context.Context, id int32) error

	AddPlacement(ctx context.Context, viewID, elementID int32, x, y float64) error
	UpdatePlacement(ctx context.Context, viewID, elementID int32, x, y float64) error
	RemovePlacement(ctx context.Context, viewID, elementID int32) error

	CreateConnector(ctx context.Context, input api.ConnectorInput) (*diagv1.Connector, error)
	UpdateConnector(ctx context.Context, id int32, input api.ConnectorInput) (*diagv1.Connector, error)
	DeleteConnector(ctx context.Context, id int32) error
	GetConnector(ctx context.Context, id int32) (*diagv1.Connector, error)
}

// ---------- target resolution (moved from cmd/apply) ----------

func ResolveTarget(cfg workspace.Config, targetOverride string) (string, error) {
	target := strings.ToLower(strings.TrimSpace(targetOverride))
	if target == "" {
		target = strings.ToLower(strings.TrimSpace(cfg.Apply.Target))
	}
	if target == "" {
		target = TargetAuto
	}
	switch target {
	case TargetAuto:
		if strings.TrimSpace(cfg.APIKey) != "" && strings.TrimSpace(cfg.WorkspaceID) != "" {
			return TargetRemote, nil
		}
		return TargetLocal, nil
	case "cloud":
		return TargetRemote, nil
	case TargetLocal, TargetRemote:
		return target, nil
	default:
		return "", fmt.Errorf("target must be auto, local, cloud, or remote")
	}
}

func TargetDisplayName(target string) string {
	if target == TargetRemote {
		return "cloud"
	}
	return target
}

func RenderTargetInfo(out io.Writer, runner Runner) {
	term.Label(out, 20, "Target", TargetDisplayName(runner.Name()))
	switch runner.Name() {
	case TargetRemote:
		term.Label(out, 20, "Cloud API", term.URL(out, runner.TargetLabel()))
	case TargetLocal:
		term.Label(out, 20, "Local DB", term.Path(out, runner.TargetLabel()))
	default:
		term.Label(out, 20, "Target detail", runner.TargetLabel())
	}
}

func NewRunner(cfg workspace.Config, targetOverride, dataDirFlag string, debug bool) (Runner, error) {
	target, err := ResolveTarget(cfg, targetOverride)
	if err != nil {
		return nil, err
	}
	switch target {
	case TargetRemote:
		return &remoteRunner{
			serverURL: cfg.ServerURL,
			apiKey:    cfg.APIKey,
			orgID:     cfg.WorkspaceID,
			debug:     debug,
		}, nil
	case TargetLocal:
		dataDir, err := workspace.ResolveDataDir(&cfg, dataDirFlag)
		if err != nil {
			return nil, err
		}
		return newLocalRunner(&cfg, dataDir)
	default:
		return nil, fmt.Errorf("unknown target %q", target)
	}
}

// IsNotFound reports whether err is a NotFound from the remote workspace
// service or a missing store row. Delete flows treat it as success so a retry
// after a partial failure (server deleted, local cache not) can converge.
func IsNotFound(err error) bool {
	var connErr *connect.Error
	if errors.As(err, &connErr) && connErr.Code() == connect.CodeNotFound {
		return true
	}
	return errors.Is(err, sql.ErrNoRows)
}

// ---------- remote runner ----------

type remoteRunner struct {
	serverURL string
	apiKey    string
	orgID     string
	debug     bool
}

func (r *remoteRunner) Name() string        { return TargetRemote }
func (r *remoteRunner) TargetLabel() string { return client.NormalizeURL(r.serverURL) }
func (r *remoteRunner) Close() error        { return nil }

func (r *remoteRunner) client() diagv1connect.WorkspaceServiceClient {
	return client.New(r.serverURL, r.apiKey, r.debug)
}

func (r *remoteRunner) CreateElement(ctx context.Context, input api.ElementInput) (*diagv1.Element, error) {
	c := r.client()
	req := &diagv1.CreateElementRequest{Name: input.Name}
	if input.Description != nil {
		req.Description = input.Description
	}
	if input.Kind != nil {
		req.Kind = input.Kind
	}
	if input.Technology != nil {
		req.Technology = input.Technology
	}
	if input.URL != nil {
		req.Url = input.URL
	}
	if input.LogoURL != nil {
		req.LogoUrl = input.LogoURL
	}
	req.TechnologyLinks = input.TechLinks
	if input.Tags != nil {
		req.Tags = input.Tags
	}
	if input.Repo != nil {
		req.Repo = input.Repo
	}
	if input.Branch != nil {
		req.Branch = input.Branch
	}
	if input.Language != nil {
		req.Language = input.Language
	}
	if input.FilePath != nil {
		req.FilePath = input.FilePath
	}
	req.BypassNoiseGate = input.BypassNoiseGate
	resp, err := c.CreateElement(ctx, connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return resp.Msg.GetElement(), nil
}

func (r *remoteRunner) UpdateElement(ctx context.Context, id int32, input api.ElementInput) (*diagv1.Element, error) {
	c := r.client()
	req := &diagv1.UpdateElementRequest{ElementId: id, Name: input.Name}
	req.Description = input.Description
	req.Kind = input.Kind
	req.Technology = input.Technology
	req.Url = input.URL
	req.LogoUrl = input.LogoURL
	req.TechnologyLinks = input.TechLinks
	if input.Tags != nil {
		req.Tags = input.Tags
	}
	req.Repo = input.Repo
	req.Branch = input.Branch
	req.Language = input.Language
	req.FilePath = input.FilePath
	req.BypassNoiseGate = input.BypassNoiseGate
	resp, err := c.UpdateElement(ctx, connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return resp.Msg.GetElement(), nil
}

func (r *remoteRunner) DeleteElement(ctx context.Context, id int32) error {
	c := r.client()
	_, err := c.DeleteElement(ctx, connect.NewRequest(&diagv1.DeleteElementRequest{
		ElementId: id,
		OrgId:     r.orgID,
	}))
	return err
}

func (r *remoteRunner) GetElement(ctx context.Context, id int32) (*diagv1.Element, error) {
	c := r.client()
	resp, err := c.GetElement(ctx, connect.NewRequest(&diagv1.GetElementRequest{ElementId: id}))
	if err != nil {
		return nil, err
	}
	return resp.Msg.GetElement(), nil
}

func (r *remoteRunner) ListElements(ctx context.Context, search string) ([]*diagv1.Element, error) {
	c := r.client()
	req := &diagv1.ListElementsRequest{}
	if search != "" {
		req.Search = search
	}
	resp, err := c.ListElements(ctx, connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return resp.Msg.GetElements(), nil
}

func (r *remoteRunner) ListViews(ctx context.Context) ([]*diagv1.View, error) {
	c := r.client()
	resp, err := c.ListViews(ctx, connect.NewRequest(&diagv1.ListViewsRequest{}))
	if err != nil {
		return nil, err
	}
	return resp.Msg.GetViews(), nil
}

func (r *remoteRunner) CreateView(ctx context.Context, ownerElementID *int32, name string, label *string) (*diagv1.View, error) {
	c := r.client()
	req := &diagv1.CreateViewRequest{Name: name, OrgId: r.orgID}
	if ownerElementID != nil {
		req.OwnerElementId = ownerElementID
	}
	if label != nil {
		req.LevelLabel = label
	}
	resp, err := c.CreateView(ctx, connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return resp.Msg.GetView(), nil
}

func (r *remoteRunner) UpdateView(ctx context.Context, id int32, name string, label *string) (*diagv1.View, error) {
	c := r.client()
	req := &diagv1.UpdateViewRequest{
		ViewId: id,
		Name:   name,
	}
	if label != nil {
		req.LevelLabel = label
	}
	resp, err := c.UpdateView(ctx, connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return resp.Msg.GetView(), nil
}

func (r *remoteRunner) DeleteView(ctx context.Context, id int32) error {
	c := r.client()
	_, err := c.DeleteView(ctx, connect.NewRequest(&diagv1.DeleteViewRequest{
		ViewId: id,
		OrgId:  r.orgID,
	}))
	return err
}

func (r *remoteRunner) AddPlacement(ctx context.Context, viewID, elementID int32, x, y float64) error {
	c := r.client()
	_, err := c.CreatePlacement(ctx, connect.NewRequest(&diagv1.CreatePlacementRequest{
		ViewId:    viewID,
		ElementId: elementID,
		PositionX: x,
		PositionY: y,
	}))
	if err != nil {
		// Placement may already exist (e.g. re-add after update); try position update.
		if connect.CodeOf(err) == connect.CodeAlreadyExists {
			return r.UpdatePlacement(ctx, viewID, elementID, x, y)
		}
		return err
	}
	return nil
}

func (r *remoteRunner) UpdatePlacement(ctx context.Context, viewID, elementID int32, x, y float64) error {
	c := r.client()
	_, err := c.UpdatePlacementPosition(ctx, connect.NewRequest(&diagv1.UpdatePlacementPositionRequest{
		ViewId:    viewID,
		ElementId: elementID,
		PositionX: x,
		PositionY: y,
	}))
	return err
}

func (r *remoteRunner) RemovePlacement(ctx context.Context, viewID, elementID int32) error {
	c := r.client()
	_, err := c.DeletePlacement(ctx, connect.NewRequest(&diagv1.DeletePlacementRequest{
		ViewId:    viewID,
		ElementId: elementID,
	}))
	return err
}

func (r *remoteRunner) CreateConnector(ctx context.Context, input api.ConnectorInput) (*diagv1.Connector, error) {
	c := r.client()
	req := &diagv1.CreateConnectorRequest{
		ViewId:          input.ViewID,
		SourceElementId: input.SourceID,
		TargetElementId: input.TargetID,
		Direction:       input.Direction,
		Style:           input.Style,
	}
	req.Label = input.Label
	req.Description = input.Description
	req.Relationship = input.Relationship
	req.Url = input.URL
	req.SourceHandle = input.SourceHandle
	req.TargetHandle = input.TargetHandle
	if input.Tags != nil {
		req.Tags = input.Tags
	}
	resp, err := c.CreateConnector(ctx, connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return resp.Msg.GetConnector(), nil
}

func (r *remoteRunner) UpdateConnector(ctx context.Context, id int32, input api.ConnectorInput) (*diagv1.Connector, error) {
	c := r.client()
	req := &diagv1.UpdateConnectorRequest{ConnectorId: id}
	if input.SourceID != 0 {
		req.SourceElementId = &input.SourceID
	}
	if input.TargetID != 0 {
		req.TargetElementId = &input.TargetID
	}
	req.Label = input.Label
	req.Description = input.Description
	req.Relationship = input.Relationship
	if input.Direction != "" {
		req.Direction = input.Direction
	}
	if input.Style != "" {
		req.Style = input.Style
	}
	req.Url = input.URL
	req.SourceHandle = input.SourceHandle
	req.TargetHandle = input.TargetHandle
	if input.Tags != nil {
		req.Tags = input.Tags
	}
	resp, err := c.UpdateConnector(ctx, connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return resp.Msg.GetConnector(), nil
}

func (r *remoteRunner) DeleteConnector(ctx context.Context, id int32) error {
	c := r.client()
	_, err := c.DeleteConnector(ctx, connect.NewRequest(&diagv1.DeleteConnectorRequest{
		ConnectorId: id,
	}))
	return err
}

func (r *remoteRunner) GetConnector(ctx context.Context, id int32) (*diagv1.Connector, error) {
	c := r.client()
	resp, err := c.ListConnectors(ctx, connect.NewRequest(&diagv1.ListConnectorsRequest{}))
	if err != nil {
		return nil, err
	}
	for _, conn := range resp.Msg.GetConnectors() {
		if conn.GetId() == id {
			return conn, nil
		}
	}
	return nil, fmt.Errorf("connector %d not found", id)
}

// ---------- local runner ----------

type localRunner struct {
	sqliteStore *store.SQLiteStore
	adapter     *store.APIAdapter
	dbPath      string
	dataDir     string
}

func newLocalRunner(cfg *workspace.Config, dataDir string) (*localRunner, error) {
	dbPath := localserver.DatabasePath(dataDir)
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	sqliteStore, err := store.OpenLocal(context.Background(), cfg, dataDir, assets.FS)
	if err != nil {
		return nil, err
	}
	return &localRunner{
		sqliteStore: sqliteStore,
		adapter:     store.NewAPIAdapter(sqliteStore),
		dbPath:      dbPath,
		dataDir:     dataDir,
	}, nil
}

func (r *localRunner) Name() string        { return TargetLocal }
func (r *localRunner) TargetLabel() string { return r.dbPath }
func (r *localRunner) DataDir() string     { return r.dataDir }
func (r *localRunner) Close() error {
	if r.sqliteStore != nil {
		return r.sqliteStore.Legacy().Close()
	}
	return nil
}

func (r *localRunner) ctx(ctx context.Context) context.Context {
	return api.WithWorkspaceID(ctx, uuid.Nil)
}

func (r *localRunner) CreateElement(ctx context.Context, input api.ElementInput) (*diagv1.Element, error) {
	return r.adapter.CreateElement(r.ctx(ctx), uuid.Nil, input)
}

func (r *localRunner) UpdateElement(ctx context.Context, id int32, input api.ElementInput) (*diagv1.Element, error) {
	return r.adapter.UpdateElement(r.ctx(ctx), id, uuid.Nil, input)
}

func (r *localRunner) DeleteElement(ctx context.Context, id int32) error {
	return r.adapter.DeleteElement(r.ctx(ctx), id, uuid.Nil)
}

func (r *localRunner) GetElement(ctx context.Context, id int32) (*diagv1.Element, error) {
	return r.adapter.GetElement(r.ctx(ctx), id, uuid.Nil)
}

func (r *localRunner) ListElements(ctx context.Context, search string) ([]*diagv1.Element, error) {
	els, _, err := r.adapter.ListElements(r.ctx(ctx), uuid.Nil, 0, 0, search)
	return els, err
}

func (r *localRunner) ListViews(ctx context.Context) ([]*diagv1.View, error) {
	return r.adapter.ListViews(r.ctx(ctx), uuid.Nil)
}

func (r *localRunner) CreateView(ctx context.Context, ownerElementID *int32, name string, label *string) (*diagv1.View, error) {
	return r.adapter.CreateView(r.ctx(ctx), uuid.Nil, ownerElementID, name, label, false)
}

func (r *localRunner) UpdateView(ctx context.Context, id int32, name string, label *string) (*diagv1.View, error) {
	existing, err := r.adapter.GetView(r.ctx(ctx), id, uuid.Nil)
	if err != nil {
		return nil, err
	}
	levelLabel := existing.LevelLabel
	if label != nil {
		levelLabel = label
	}
	return r.adapter.UpdateView(r.ctx(ctx), id, uuid.Nil, name, existing.Description, levelLabel, nil)
}

func (r *localRunner) DeleteView(ctx context.Context, id int32) error {
	return r.adapter.DeleteView(r.ctx(ctx), id, uuid.Nil)
}

func (r *localRunner) AddPlacement(ctx context.Context, viewID, elementID int32, x, y float64) error {
	c := r.ctx(ctx)
	_, err := r.adapter.AddPlacement(c, viewID, elementID, x, y)
	if err != nil {
		// Idempotent re-add: fall back to position update.
		if updateErr := r.adapter.UpdatePlacementPosition(c, viewID, elementID, x, y); updateErr == nil {
			return nil
		}
		return err
	}
	return nil
}

func (r *localRunner) UpdatePlacement(ctx context.Context, viewID, elementID int32, x, y float64) error {
	return r.adapter.UpdatePlacementPosition(r.ctx(ctx), viewID, elementID, x, y)
}

func (r *localRunner) RemovePlacement(ctx context.Context, viewID, elementID int32) error {
	return r.adapter.RemovePlacement(r.ctx(ctx), viewID, elementID)
}

func (r *localRunner) CreateConnector(ctx context.Context, input api.ConnectorInput) (*diagv1.Connector, error) {
	return r.adapter.CreateConnector(r.ctx(ctx), uuid.Nil, input)
}

func (r *localRunner) UpdateConnector(ctx context.Context, id int32, input api.ConnectorInput) (*diagv1.Connector, error) {
	return r.adapter.UpdateConnector(r.ctx(ctx), id, uuid.Nil, input)
}

func (r *localRunner) DeleteConnector(ctx context.Context, id int32) error {
	return r.adapter.DeleteConnector(r.ctx(ctx), id, uuid.Nil)
}

func (r *localRunner) GetConnector(ctx context.Context, id int32) (*diagv1.Connector, error) {
	return r.adapter.GetConnector(r.ctx(ctx), id, uuid.Nil)
}

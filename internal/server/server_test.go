package server

import (
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"io/fs"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	diagv1connect "buf.build/gen/go/tldiagramcom/diagram/connectrpc/go/diag/v1/diagv1connect"
	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	assets "github.com/mertcikla/tld/v2"
	localstore "github.com/mertcikla/tld/v2/internal/store"
	"github.com/mertcikla/tld/v2/internal/tech"
	"github.com/mertcikla/tld/v2/internal/watch"
)

func TestServerReadyReportsResourceCounts(t *testing.T) {
	sqliteStore, routes := newTestServer(t, uuid.MustParse("11111111-2222-3333-4444-555555555555"), nil)
	if _, err := sqliteStore.DB().Exec(`
		INSERT INTO elements(id, name, tags, technology_connectors, created_at, updated_at)
		VALUES
			(10, 'API', '[]', '[]', 'now', 'now'),
			(11, 'DB', '[]', '[]', 'now', 'now');
		INSERT INTO connectors(view_id, source_element_id, target_element_id, direction, style, created_at, updated_at)
		VALUES (1, 10, 11, 'forward', 'solid', 'now', 'now');
	`); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/ready", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body struct {
		OK        bool `json:"ok"`
		Resources struct {
			Views      int `json:"views"`
			Elements   int `json:"elements"`
			Connectors int `json:"connectors"`
		} `json:"resources"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if !body.OK || body.Resources.Views != 1 || body.Resources.Elements != 2 || body.Resources.Connectors != 1 {
		t.Fatalf("ready body = %+v, want 1/2/1 resources", body)
	}
}

func TestServerInitializesViewNoiseGate(t *testing.T) {
	sqliteStore, routes := newTestServer(t, uuid.Nil, nil)
	if _, err := sqliteStore.DB().Exec(`
		INSERT INTO elements(id, name, tags, technology_connectors, bypass_noise_gate, created_at, updated_at)
		VALUES
			(101, 'A', '[]', '[]', 1, 'now', 'now'),
			(102, 'B', '[]', '[]', 1, 'now', 'now'),
			(103, 'C', '[]', '[]', 1, 'now', 'now'),
			(104, 'D', '[]', '[]', 1, 'now', 'now'),
			(105, 'E', '[]', '[]', 1, 'now', 'now');
		INSERT INTO placements(view_id, element_id, position_x, position_y, created_at, updated_at)
		VALUES
			(1, 101, 0, 0, 'now', 'now'),
			(1, 102, 10, 0, 'now', 'now'),
			(1, 103, 20, 0, 'now', 'now'),
			(1, 104, 30, 0, 'now', 'now'),
			(1, 105, 40, 0, 'now', 'now');
	`); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/views/1/noise-gate/initialize", strings.NewReader(`{"density_level":-1}`))
	routes.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body struct {
		ViewID           int64 `json:"view_id"`
		DensityLevel     int   `json:"density_level"`
		ElementsEnabled  int   `json:"elements_enabled"`
		OverridesCreated int   `json:"overrides_created"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.ViewID != 1 || body.DensityLevel != -1 || body.ElementsEnabled != 5 || body.OverridesCreated != 5 {
		t.Fatalf("initialize body = %+v", body)
	}

	var bypassed int
	if err := sqliteStore.DB().QueryRow(`SELECT COUNT(*) FROM elements WHERE id BETWEEN 101 AND 105 AND bypass_noise_gate = 1`).Scan(&bypassed); err != nil {
		t.Fatal(err)
	}
	if bypassed != 0 {
		t.Fatalf("bypassed initialized elements = %d, want 0", bypassed)
	}
	level, err := sqliteStore.ViewDensityLevel(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if level != -1 {
		t.Fatalf("density = %d, want -1", level)
	}
}

func TestServerAllowsVSCodeWebviewCORSPreflight(t *testing.T) {
	_, routes := newTestServer(t, uuid.New(), nil)
	req := httptest.NewRequest(http.MethodOptions, "/api/diag.v1.WorkspaceService/ListViews", nil)
	req.Header.Set("Origin", "vscode-webview://abc123")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "content-type,connect-protocol-version,x-user-agent")
	req.Header.Set("Access-Control-Request-Private-Network", "true")

	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "vscode-webview://abc123" {
		t.Fatalf("allow origin = %q, want vscode webview origin", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("allow credentials = %q, want true", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(strings.ToLower(got), "connect-protocol-version") {
		t.Fatalf("allow headers = %q, want connect-protocol-version", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(strings.ToLower(got), "x-user-agent") {
		t.Fatalf("allow headers = %q, want requested x-user-agent reflected", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Private-Network"); got != "true" {
		t.Fatalf("allow private network = %q, want true", got)
	}
}

func TestServerAllowsVSCodeFileOrigin(t *testing.T) {
	_, routes := newTestServer(t, uuid.New(), nil)
	req := httptest.NewRequest(http.MethodOptions, "/api/diag.v1.WorkspaceService/ListViews", nil)
	req.Header.Set("Origin", "vscode-file://vscode-app")
	req.Header.Set("Access-Control-Request-Method", "POST")

	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "vscode-file://vscode-app" {
		t.Fatalf("allow origin = %q, want vscode-file origin", got)
	}
}

func TestServerAllowsLocalhostCORSOrigin(t *testing.T) {
	_, routes := newTestServer(t, uuid.New(), nil)
	req := httptest.NewRequest(http.MethodOptions, "/api/diag.v1.WorkspaceService/ListViews", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Access-Control-Request-Method", "POST")

	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:5173" {
		t.Fatalf("allow origin = %q, want localhost origin", got)
	}
}

func TestServerAllowsConfiguredCORSOrigins(t *testing.T) {
	_, routes := newTestServerWithOptions(t, uuid.New(), nil, Options{
		PublicURL:      "https://app.example.com",
		AllowedOrigins: []string{"https://admin.example.com", "https://preview.example.com:8443"},
	})
	tests := []struct {
		name   string
		origin string
	}{
		{name: "public url origin", origin: "https://app.example.com"},
		{name: "allowed origin", origin: "https://admin.example.com"},
		{name: "allowed origin with port", origin: "https://preview.example.com:8443"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodOptions, "/api/diag.v1.WorkspaceService/ListViews", nil)
			req.Header.Set("Origin", tt.origin)
			req.Header.Set("Access-Control-Request-Method", "POST")
			req.Header.Set("Access-Control-Request-Headers", "content-type,connect-protocol-version")

			rec := httptest.NewRecorder()
			routes.ServeHTTP(rec, req)

			if rec.Code != http.StatusNoContent {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
			if got := rec.Header().Get("Access-Control-Allow-Origin"); got != tt.origin {
				t.Fatalf("allow origin = %q, want %q", got, tt.origin)
			}
			if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
				t.Fatalf("allow credentials = %q, want true", got)
			}
			if got := rec.Header().Get("Access-Control-Allow-Headers"); !strings.Contains(strings.ToLower(got), "connect-protocol-version") {
				t.Fatalf("allow headers = %q, want requested connect header", got)
			}
		})
	}
}

func TestServerRejectsNonLocalCORSOrigin(t *testing.T) {
	_, routes := newTestServer(t, uuid.New(), nil)
	req := httptest.NewRequest(http.MethodGet, "/api/ready", nil)
	req.Header.Set("Origin", "https://example.com")

	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("allow origin = %q, want empty for non-local origin", got)
	}
}

func TestServerOrgTagColorsRoundTrip(t *testing.T) {
	_, routes := newTestServer(t, uuid.MustParse("11111111-2222-3333-4444-555555555555"), nil)
	server := httptest.NewServer(routes)
	defer server.Close()

	client := diagv1connect.NewOrgServiceClient(http.DefaultClient, server.URL+"/api")
	description := "User managed color"
	if _, err := client.UpdateTag(context.Background(), connect.NewRequest(&diagv1.UpdateTagRequest{
		Tag:         "role:watch",
		Color:       "#123456",
		Description: &description,
	})); err != nil {
		t.Fatal(err)
	}
	resp, err := client.ListTagColors(context.Background(), connect.NewRequest(&diagv1.ListTagColorsRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	tag := resp.Msg.GetTags()["role:watch"]
	if tag == nil || tag.GetColor() != "#123456" || tag.Description == nil || tag.GetDescription() != description {
		t.Fatalf("tag = %+v, want persisted color and description", tag)
	}
}

func TestServerInjectsWorkspaceIDIntoConnectRPCResponses(t *testing.T) {
	workspaceID := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	sqliteStore, routes := newTestServer(t, workspaceID, nil)
	if _, err := sqliteStore.DB().Exec(`
		INSERT INTO elements(id, org_id, name, tags, technology_connectors, created_at, updated_at)
		VALUES (10, ?, 'API', '[]', '[]', 'now', 'now');
	`, workspaceID.String()); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(routes)
	t.Cleanup(srv.Close)

	client := diagv1connect.NewWorkspaceServiceClient(srv.Client(), srv.URL+"/api")
	resp, err := client.ListElements(context.Background(), connect.NewRequest(&diagv1.ListElementsRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Msg.GetElements()) != 1 {
		t.Fatalf("elements = %+v, want one element", resp.Msg.GetElements())
	}
	if got := resp.Msg.GetElements()[0].GetOrgId(); got != workspaceID.String() {
		t.Fatalf("org id = %q, want %s", got, workspaceID)
	}
}

func TestServerBroadcastsWorkspaceWritesToCollaborationView(t *testing.T) {
	_, routes := newTestServer(t, uuid.New(), nil)
	srv := httptest.NewServer(routes)
	t.Cleanup(srv.Close)

	client := diagv1connect.NewWorkspaceServiceClient(srv.Client(), srv.URL+"/api")
	view, err := client.CreateView(context.Background(), connect.NewRequest(&diagv1.CreateViewRequest{Name: "Collaboration Test"}))
	if err != nil {
		t.Fatal(err)
	}
	viewID := view.Msg.GetView().GetId()

	conn := dialCollaborationWebSocket(t, srv.URL, int(viewID), "alice")
	t.Cleanup(func() { _ = conn.Close() })
	readRealtimeFrameType(t, conn, "presence_snapshot")

	first, err := client.CreateElement(context.Background(), connect.NewRequest(&diagv1.CreateElementRequest{Name: "Collab API"}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.CreatePlacement(context.Background(), connect.NewRequest(&diagv1.CreatePlacementRequest{
		ViewId:    viewID,
		ElementId: first.Msg.GetElement().GetId(),
		PositionX: 120,
		PositionY: 140,
	})); err != nil {
		t.Fatal(err)
	}
	readRealtimeFrameType(t, conn, "placement_create")

	if _, err := client.UpdateElement(context.Background(), connect.NewRequest(&diagv1.UpdateElementRequest{
		ElementId: first.Msg.GetElement().GetId(),
		Name:      "Collab API Renamed",
	})); err != nil {
		t.Fatal(err)
	}
	readRealtimeFrameType(t, conn, "element_update")

	second, err := client.CreateElement(context.Background(), connect.NewRequest(&diagv1.CreateElementRequest{Name: "Collab DB"}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.CreatePlacement(context.Background(), connect.NewRequest(&diagv1.CreatePlacementRequest{
		ViewId:    viewID,
		ElementId: second.Msg.GetElement().GetId(),
		PositionX: 360,
		PositionY: 140,
	})); err != nil {
		t.Fatal(err)
	}
	readRealtimeFrameType(t, conn, "placement_create")

	initialLabel := "initial"
	createdConnector, err := client.CreateConnector(context.Background(), connect.NewRequest(&diagv1.CreateConnectorRequest{
		ViewId:          viewID,
		SourceElementId: first.Msg.GetElement().GetId(),
		TargetElementId: second.Msg.GetElement().GetId(),
		Label:           &initialLabel,
	}))
	if err != nil {
		t.Fatal(err)
	}
	readRealtimeFrameType(t, conn, "connector_create")

	updatedLabel := "updated"
	if _, err := client.UpdateConnector(context.Background(), connect.NewRequest(&diagv1.UpdateConnectorRequest{
		ConnectorId: createdConnector.Msg.GetConnector().GetId(),
		Label:       &updatedLabel,
	})); err != nil {
		t.Fatal(err)
	}
	readRealtimeFrameType(t, conn, "connector_update")

	if _, err := client.DeleteConnector(context.Background(), connect.NewRequest(&diagv1.DeleteConnectorRequest{
		ConnectorId: createdConnector.Msg.GetConnector().GetId(),
	})); err != nil {
		t.Fatal(err)
	}
	readRealtimeFrameType(t, conn, "connector_delete")

	if _, err := client.DeletePlacement(context.Background(), connect.NewRequest(&diagv1.DeletePlacementRequest{
		ViewId:    viewID,
		ElementId: first.Msg.GetElement().GetId(),
	})); err != nil {
		t.Fatal(err)
	}
	readRealtimeFrameType(t, conn, "placement_delete")
}

func TestWatchSessionLeaseDoesNotBlockWorkspaceWrites(t *testing.T) {
	sqliteStore, routes := newTestServer(t, uuid.New(), nil)
	repositoryID := insertWatchRepository(t, sqliteStore.DB())
	watchStore := watch.NewStore(sqliteStore.DB())
	if _, err := watchStore.AcquireLock(context.Background(), repositoryID, os.Getpid(), "session-token", watch.LockHeartbeatTimeout); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(routes)
	t.Cleanup(srv.Close)

	client := diagv1connect.NewWorkspaceServiceClient(srv.Client(), srv.URL+"/api")
	resp, err := client.CreateElement(context.Background(), connect.NewRequest(&diagv1.CreateElementRequest{Name: "Manual"}))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Msg.GetElement().GetName() != "Manual" {
		t.Fatalf("created element = %+v", resp.Msg.GetElement())
	}
}

func TestWatchApplyLeaseBlocksWorkspaceWrites(t *testing.T) {
	sqliteStore, routes := newTestServer(t, uuid.New(), nil)
	repositoryID := insertWatchRepository(t, sqliteStore.DB())
	watchStore := watch.NewStore(sqliteStore.DB())
	if err := watchStore.AcquireApplyLock(context.Background(), repositoryID, os.Getpid(), "apply-token", watch.LockHeartbeatTimeout); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(routes)
	t.Cleanup(srv.Close)

	client := diagv1connect.NewWorkspaceServiceClient(srv.Client(), srv.URL+"/api")
	_, err := client.CreateElement(context.Background(), connect.NewRequest(&diagv1.CreateElementRequest{Name: "Manual"}))
	if code := connect.CodeOf(err); code != connect.CodeFailedPrecondition {
		t.Fatalf("code = %s, want failed_precondition: %v", code, err)
	}
	if err := watchStore.ReleaseApplyLock(context.Background(), repositoryID, "apply-token"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.CreateElement(context.Background(), connect.NewRequest(&diagv1.CreateElementRequest{Name: "Manual"})); err != nil {
		t.Fatal(err)
	}
}

func TestServerRoutesThumbnailAndStaticFallback(t *testing.T) {
	_, routes := newTestServer(t, uuid.New(), fstest.MapFS{
		"frontend/dist/index.html": {Data: []byte("<html>app</html>")},
		"frontend/dist/app.js":     {Data: []byte("console.log('app')")},
	})

	tests := []struct {
		name        string
		path        string
		wantStatus  int
		wantType    string
		wantBodySub string
	}{
		{
			name:        "root thumbnail",
			path:        "/api/views/1/thumbnail.svg",
			wantStatus:  http.StatusOK,
			wantType:    "image/svg+xml; charset=utf-8",
			wantBodySub: "<svg",
		},
		{
			name:        "invalid thumbnail id",
			path:        "/api/views/not-a-number/thumbnail.svg",
			wantStatus:  http.StatusBadRequest,
			wantBodySub: "invalid view id",
		},
		{
			name:        "static file",
			path:        "/app.js",
			wantStatus:  http.StatusOK,
			wantType:    "application/javascript",
			wantBodySub: "console.log",
		},
		{
			name:        "spa fallback",
			path:        "/views/123",
			wantStatus:  http.StatusOK,
			wantType:    "text/html; charset=utf-8",
			wantBodySub: "app",
		},
		{
			name:       "unknown api route",
			path:       "/api/not-real",
			wantStatus: http.StatusNotFound,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			routes.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantType != "" && rec.Header().Get("Content-Type") != tt.wantType {
				t.Fatalf("content type = %q, want %q", rec.Header().Get("Content-Type"), tt.wantType)
			}
			if tt.wantBodySub != "" && !strings.Contains(rec.Body.String(), tt.wantBodySub) {
				t.Fatalf("body = %q, want substring %q", rec.Body.String(), tt.wantBodySub)
			}
		})
	}
}

func TestServerServesPrecompressedStaticAssets(t *testing.T) {
	_, routes := newTestServer(t, uuid.New(), fstest.MapFS{
		"frontend/dist/index.html":    {Data: []byte("<html>app</html>")},
		"frontend/dist/index.html.gz": {Data: []byte("gzip-index")},
		"frontend/dist/index.html.br": {Data: []byte("brotli-index")},
		"frontend/dist/app.js":        {Data: []byte("console.log('app')")},
		"frontend/dist/app.js.gz":     {Data: []byte("gzip-js")},
		"frontend/dist/app.js.br":     {Data: []byte("brotli-js")},
	})

	tests := []struct {
		name           string
		path           string
		acceptEncoding string
		wantEncoding   string
		wantBody       string
	}{
		{
			name:           "brotli is preferred",
			path:           "/app.js",
			acceptEncoding: "gzip, br",
			wantEncoding:   "br",
			wantBody:       "brotli-js",
		},
		{
			name:           "gzip is used when brotli is unavailable",
			path:           "/app.js",
			acceptEncoding: "gzip",
			wantEncoding:   "gzip",
			wantBody:       "gzip-js",
		},
		{
			name:     "uncompressed file is used without accepted encoding",
			path:     "/app.js",
			wantBody: "console.log('app')",
		},
		{
			name:           "spa fallback can use compressed index",
			path:           "/views/123",
			acceptEncoding: "br",
			wantEncoding:   "br",
			wantBody:       "brotli-index",
		},
		{
			name:           "q zero disables encoding",
			path:           "/app.js",
			acceptEncoding: "br;q=0, gzip;q=0",
			wantBody:       "console.log('app')",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			if tt.acceptEncoding != "" {
				req.Header.Set("Accept-Encoding", tt.acceptEncoding)
			}
			rec := httptest.NewRecorder()
			routes.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
			if got := rec.Header().Get("Content-Encoding"); got != tt.wantEncoding {
				t.Fatalf("content encoding = %q, want %q", got, tt.wantEncoding)
			}
			if got := rec.Header().Get("Content-Type"); got != "application/javascript" && tt.path == "/app.js" {
				t.Fatalf("content type = %q, want application/javascript", got)
			}
			if got := rec.Body.String(); got != tt.wantBody {
				t.Fatalf("body = %q, want %q", got, tt.wantBody)
			}
			if !strings.Contains(rec.Header().Get("Vary"), "Accept-Encoding") {
				t.Fatalf("vary = %q, want Accept-Encoding", rec.Header().Get("Vary"))
			}
		})
	}
}

func TestServerServesDynamicCustomIconAssets(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("TLD_CONFIG_DIR", configDir)
	tech.ReloadCatalog()
	t.Cleanup(tech.ReloadCatalog)

	customRoot := filepath.Join(configDir, "icons")
	customIcons := filepath.Join(customRoot, "icons")
	if err := os.MkdirAll(customIcons, 0o755); err != nil {
		t.Fatalf("mkdir custom icons: %v", err)
	}
	if err := os.WriteFile(filepath.Join(customRoot, "icons.json"), []byte(`[
  {
    "iconUrl": "/icons/live-icon.png",
    "name": "Live Icon",
    "nameShort": "Live",
    "defaultSlug": "live-icon"
  }
]`), 0o644); err != nil {
		t.Fatalf("write custom catalog: %v", err)
	}
	if err := os.WriteFile(filepath.Join(customIcons, "live-icon.png"), []byte("custom-png"), 0o644); err != nil {
		t.Fatalf("write custom icon: %v", err)
	}

	_, routes := newTestServer(t, uuid.New(), fstest.MapFS{
		"frontend/dist/index.html": {Data: []byte("<html>app</html>")},
	})

	iconRec := httptest.NewRecorder()
	routes.ServeHTTP(iconRec, httptest.NewRequest(http.MethodGet, "/icons/live-icon.png", nil))
	if iconRec.Code != http.StatusOK {
		t.Fatalf("icon status = %d, body = %s", iconRec.Code, iconRec.Body.String())
	}
	if got := iconRec.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("icon content type = %q, want image/png", got)
	}
	if got := iconRec.Body.String(); got != "custom-png" {
		t.Fatalf("icon body = %q, want custom-png", got)
	}

	catalogRec := httptest.NewRecorder()
	routes.ServeHTTP(catalogRec, httptest.NewRequest(http.MethodGet, "/icons.json", nil))
	if catalogRec.Code != http.StatusOK {
		t.Fatalf("catalog status = %d, body = %s", catalogRec.Code, catalogRec.Body.String())
	}
	if !strings.Contains(catalogRec.Body.String(), `"defaultSlug": "live-icon"`) {
		t.Fatalf("catalog body missing custom item: %s", catalogRec.Body.String())
	}
	if got := catalogRec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("catalog cache-control = %q, want no-store", got)
	}
}

func TestServerServesIconAssetPathsLocallyInDevMode(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("TLD_CONFIG_DIR", configDir)
	_, routes := newTestServer(t, uuid.New(), fstest.MapFS{
		"frontend/dist/index.html":           {Data: []byte("<html>app</html>")},
		"frontend/dist/icons/bundled.svg":    {Data: []byte("<svg>bundled</svg>")},
		"frontend/dist/icons/bundled.svg.br": {Data: []byte("compressed")},
	})
	t.Setenv("DEV", "true")

	iconRec := httptest.NewRecorder()
	routes.ServeHTTP(iconRec, httptest.NewRequest(http.MethodGet, "/icons/bundled.svg", nil))
	if iconRec.Code != http.StatusOK {
		t.Fatalf("bundled icon status = %d, body = %s", iconRec.Code, iconRec.Body.String())
	}
	if got := iconRec.Header().Get("Content-Type"); got != "image/svg+xml" {
		t.Fatalf("bundled icon content type = %q, want image/svg+xml", got)
	}
	if got := iconRec.Body.String(); got != "<svg>bundled</svg>" {
		t.Fatalf("bundled icon body = %q, want bundled svg", got)
	}

	missingRec := httptest.NewRecorder()
	routes.ServeHTTP(missingRec, httptest.NewRequest(http.MethodGet, "/icons/missing.svg", nil))
	if missingRec.Code != http.StatusNotFound {
		t.Fatalf("missing icon status = %d, body = %s", missingRec.Code, missingRec.Body.String())
	}
	if strings.Contains(missingRec.Body.String(), "<html>app</html>") {
		t.Fatalf("missing icon fell back to app shell: %s", missingRec.Body.String())
	}
}

func TestPopulateScoreGateAllowsLexicallyAnchoredJinaMatches(t *testing.T) {
	tests := []struct {
		name             string
		finalScore       float64
		lexicalPathScore float64
		want             bool
	}{
		{
			name:             "legacy high score passes",
			finalScore:       0.35,
			lexicalPathScore: 0,
			want:             true,
		},
		{
			name:             "jina lexical match passes",
			finalScore:       0.284,
			lexicalPathScore: 0.40,
			want:             true,
		},
		{
			name:             "jina semantic-only weak match stays hidden",
			finalScore:       0.265,
			lexicalPathScore: 0,
			want:             false,
		},
		{
			name:             "lexical match still needs evidence",
			finalScore:       0.249,
			lexicalPathScore: 0.40,
			want:             false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := passesPopulateScoreGate(tt.finalScore, tt.lexicalPathScore)
			if got != tt.want {
				t.Fatalf("passesPopulateScoreGate(%v, %v) = %v, want %v", tt.finalScore, tt.lexicalPathScore, got, tt.want)
			}
		})
	}
}

func TestBuildPopulateQueryPreservesRawUserQuery(t *testing.T) {
	tests := []struct {
		name      string
		viewName  string
		userQuery string
		want      string
	}{
		{name: "user query stays raw", viewName: "panel", userQuery: "panel", want: "panel"},
		{name: "blank query falls back to view name", viewName: "frontend", userQuery: "", want: "frontend"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sqliteStore, _ := newTestServer(t, uuid.New(), nil)
			if _, err := sqliteStore.DB().Exec(`
				INSERT INTO views(id, name, created_at, updated_at)
				VALUES (20, ?, 'now', 'now')`, tt.viewName); err != nil {
				t.Fatal(err)
			}
			got, err := buildPopulateQuery(context.Background(), sqliteStore.DB(), 20, tt.userQuery)
			if err != nil {
				t.Fatal(err)
			}
			if got.Base != tt.want || got.Compact != tt.want || got.Enriched != tt.want {
				t.Fatalf("populate query = %+v, want all query variants to equal %q", got, tt.want)
			}
		})
	}
}

func TestEmbedPopulateQueryIncludesRawBaseQuery(t *testing.T) {
	provider := &capturePopulateEmbeddingProvider{}
	query := populateQuery{
		Base:     "panel",
		Compact:  "panel",
		Enriched: "panel",
	}

	embeddings, err := embedPopulateQuery(context.Background(), provider, query)
	if err != nil {
		t.Fatal(err)
	}
	if len(provider.inputs) != 1 {
		t.Fatalf("inputs = %+v, want only the raw user query to be embedded", provider.inputs)
	}
	if provider.inputs[0].Text != "panel" {
		t.Fatalf("first query embedding = %q, want raw base query", provider.inputs[0].Text)
	}
	if got := embeddings.recallVectors(); len(got) != 1 {
		t.Fatalf("recall vectors = %d, want only the raw query vector", len(got))
	}
}

func TestPopulateEmbeddingScoreUsesStrongestQueryVariant(t *testing.T) {
	base := watch.Vector{1, 0}
	compact := watch.Vector{0, 1}
	score := populateEmbeddingScore([]watch.Vector{base, compact}, populateVectorBytesForTest(1, 0))
	if score < 0.99 {
		t.Fatalf("populateEmbeddingScore = %.3f, want strongest query variant to match candidate", score)
	}
}

func TestLoadPopulateCandidatesUsesOwnerEmbeddingsWhenPopulateResourceEmbeddingMissing(t *testing.T) {
	sqliteStore, _ := newTestServer(t, uuid.New(), nil)
	repoID := insertWatchRepository(t, sqliteStore.DB())
	if _, err := sqliteStore.DB().Exec(`
		INSERT INTO views(id, name, created_at, updated_at)
		VALUES (20, 'panel', 'now', 'now');
		INSERT INTO elements(id, name, kind, tags, technology_connectors, file_path, created_at, updated_at)
		VALUES (100, 'PanelHeader', 'function', '[]', '[]', 'frontend/src/components/PanelHeader.tsx', 'now', 'now');
		INSERT INTO watch_embedding_models(id, provider, model, dimension, config_hash, created_at)
		VALUES (1, 'local-deterministic-test', 'test', 2, 'cfg', 'now');
		INSERT INTO watch_materialization(repository_id, owner_type, owner_key, resource_type, resource_id, created_at, updated_at)
		VALUES (?, 'symbol', 'sym:panelheader', 'element', 100, 'now', 'now');
	`, repoID); err != nil {
		t.Fatal(err)
	}
	if _, err := sqliteStore.DB().Exec(`
		INSERT INTO watch_embeddings(model_id, owner_type, owner_key, input_hash, vector, created_at)
		VALUES (1, 'symbol', 'sym:panelheader', 'hash', ?, 'now');
	`, populateVectorBytesForTest(1, 0)); err != nil {
		t.Fatal(err)
	}

	candidates, err := loadPopulateCandidates(context.Background(), sqliteStore.DB(), repoID, 20, 1, watch.Vector{1, 0}, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 {
		t.Fatalf("candidates = %+v, want one symbol candidate", candidates)
	}
	if !candidates[0].hasEmbedding {
		t.Fatalf("candidate = %+v, want owner embedding to be loaded", candidates[0])
	}
	if candidates[0].embeddingScore < 0.99 {
		t.Fatalf("embeddingScore = %.3f, want owner embedding similarity to be preserved", candidates[0].embeddingScore)
	}
}

func TestBootstrapPopulateCandidatesKeepsLowConfidenceSemanticMatches(t *testing.T) {
	kind := "function"
	candidates := []populateCandidate{
		{
			element:          populateElementResult{ID: 1, Name: "PanelHeader", Kind: &kind},
			hasEmbedding:     true,
			embeddingScore:   0.08,
			lexicalPathScore: 0.15,
			finalScore:       0.09,
		},
		{
			element:          populateElementResult{ID: 2, Name: "Workspace", Kind: &kind},
			hasEmbedding:     true,
			embeddingScore:   0.05,
			lexicalPathScore: 0.05,
			finalScore:       0.04,
		},
	}
	if filtered := filterPopulateCandidates(candidates, false); len(filtered) != 0 {
		t.Fatalf("filtered = %+v, want score gates to reject low-confidence candidates first", filtered)
	}
	bootstrapped := bootstrapPopulateCandidates(candidates, 2)
	if len(bootstrapped) != 2 {
		t.Fatalf("bootstrapped = %+v, want low-confidence semantic candidates to survive into reranking", bootstrapped)
	}
	if bootstrapped[0].element.ID != 1 {
		t.Fatalf("bootstrapped[0] = %+v, want strongest low-confidence semantic match first", bootstrapped[0])
	}
}

func TestPopulatePrefersHighAbstractionCandidates(t *testing.T) {
	sqliteStore, routes := newTestServer(t, uuid.New(), nil)
	repoID := insertWatchRepository(t, sqliteStore.DB())
	if _, err := sqliteStore.DB().Exec(`
		INSERT INTO views(id, name, created_at, updated_at)
		VALUES (20, 'frontend', 'now', 'now');
		INSERT INTO elements(id, name, kind, tags, technology_connectors, file_path, created_at, updated_at)
		VALUES
			(100, 'frontend', 'folder', '["role:ui"]', '[]', 'frontend', 'now', 'now'),
			(101, 'index.tsx', 'file', '["role:ui"]', '[]', 'frontend/src/index.tsx', 'now', 'now'),
			(102, 'Frontend App', 'architecture-component', '["role:ui"]', '[]', 'frontend', 'now', 'now');
		INSERT INTO watch_materialization(repository_id, owner_type, owner_key, resource_type, resource_id, created_at, updated_at)
		VALUES
			(?, 'folder', 'folder:frontend', 'element', 100, 'now', 'now'),
			(?, 'file', 'file:frontend/src/index.tsx', 'element', 101, 'now', 'now'),
			(?, 'architecture-component', 'component:frontend', 'element', 102, 'now', 'now')`,
		repoID, repoID, repoID); err != nil {
		t.Fatal(err)
	}

	results := requestPopulateResults(t, routes, "/api/views/20/populate?q=frontend&limit=10")
	if len(results) == 0 {
		t.Fatal("expected populate results")
	}
	for _, result := range results {
		if result.Kind != "file" {
			return
		}
	}
	t.Fatalf("expected at least one high-abstraction result, got %+v", results)
}

func TestPopulateFallsBackToFilesAndExcludesPlaced(t *testing.T) {
	sqliteStore, routes := newTestServer(t, uuid.New(), nil)
	repoID := insertWatchRepository(t, sqliteStore.DB())
	if _, err := sqliteStore.DB().Exec(`
		INSERT INTO views(id, name, created_at, updated_at)
		VALUES (20, 'websocket', 'now', 'now');
		INSERT INTO elements(id, name, kind, tags, technology_connectors, file_path, created_at, updated_at)
		VALUES
			(100, 'websocket.go', 'file', '["role:watch"]', '[]', 'internal/watch/websocket.go', 'now', 'now'),
			(101, 'placed-websocket.go', 'file', '["role:watch"]', '[]', 'internal/watch/placed-websocket.go', 'now', 'now');
		INSERT INTO placements(view_id, element_id, position_x, position_y, created_at, updated_at)
		VALUES (20, 101, 0, 0, 'now', 'now');
		INSERT INTO watch_materialization(repository_id, owner_type, owner_key, resource_type, resource_id, created_at, updated_at)
		VALUES
			(?, 'file', 'file:internal/watch/websocket.go', 'element', 100, 'now', 'now'),
			(?, 'file', 'file:internal/watch/placed-websocket.go', 'element', 101, 'now', 'now')`,
		repoID, repoID); err != nil {
		t.Fatal(err)
	}

	results := requestPopulateResults(t, routes, "/api/views/20/populate?q=websocket&limit=10")
	if len(results) != 1 {
		t.Fatalf("results = %+v, want only unplaced file fallback", results)
	}
	if results[0].ID != 100 || results[0].Kind != "file" {
		t.Fatalf("result = %+v, want unplaced file 100", results[0])
	}
}

func TestPopulateLexicalScoreIgnoresContextHints(t *testing.T) {
	kind := "folder"
	tags := json.RawMessage(`["role:persistence"]`)
	workspacePath := "internal/workspace"
	storePath := "internal/store/sqlite"
	query := populateQuery{
		Base:     "database sqlite",
		ViewName: "Architecture",
		Hints:    []string{"workspace", "local", "repository"},
	}
	candidates := []populateCandidate{
		{
			element: populateElementResult{
				ID:       1,
				Name:     "workspace",
				Kind:     &kind,
				FilePath: &workspacePath,
				Tags:     json.RawMessage(`["role:config"]`),
			},
		},
		{
			element: populateElementResult{
				ID:       2,
				Name:     "sqlite store",
				Kind:     &kind,
				FilePath: &storePath,
				Tags:     tags,
			},
		},
	}

	scored := scorePopulateCandidates(query, candidates, nil)
	if scored[0].lexicalPathScore >= scored[1].lexicalPathScore {
		t.Fatalf("context hint contaminated lexical score: workspace=%.2f store=%.2f", scored[0].lexicalPathScore, scored[1].lexicalPathScore)
	}
}

func TestPopulatePenalizesSparseFolderMatches(t *testing.T) {
	folderKind := "folder"
	fileKind := "file"
	folderPath := "src/billing"
	filePath := "src/billing/BillingHandler.ts"
	query := populateQuery{Base: "billing api", ViewName: "Architecture"}
	children := []populateChildCandidate{
		{name: "BillingHandler", kind: "file", filePath: "src/billing/BillingHandler.ts", hasEmbedding: true, embeddingScore: 0.80},
		{name: "BillingRoutes", kind: "file", filePath: "src/billing/BillingRoutes.ts", hasEmbedding: true, embeddingScore: 0.80},
		{name: "Currency", kind: "file", filePath: "src/shared/Currency.ts"},
		{name: "Invoice", kind: "file", filePath: "src/accounting/Invoice.ts"},
		{name: "Tax", kind: "file", filePath: "src/shared/Tax.ts"},
		{name: "Discount", kind: "file", filePath: "src/promotions/Discount.ts"},
		{name: "Address", kind: "file", filePath: "src/customers/Address.ts"},
		{name: "Clock", kind: "file", filePath: "src/platform/Clock.ts"},
	}
	candidates := []populateCandidate{
		{
			element:        populateElementResult{ID: 1, Name: "billing", Kind: &folderKind, FilePath: &folderPath},
			children:       children,
			embeddingScore: 0.50,
			hasEmbedding:   true,
		},
		{
			element:        populateElementResult{ID: 2, Name: "BillingHandler", Kind: &fileKind, FilePath: &filePath},
			embeddingScore: 0.50,
			hasEmbedding:   true,
		},
	}

	scored := scorePopulateCandidates(query, candidates, nil)
	if scored[0].finalScore >= scored[1].finalScore {
		t.Fatalf("sparse folder score %.3f should be below concrete file %.3f; folder reason: %s", scored[0].finalScore, scored[1].finalScore, scored[0].element.MatchReason)
	}
}

func TestPopulateRewardsBroadFolderMatches(t *testing.T) {
	folderKind := "folder"
	fileKind := "file"
	folderPath := "src/billing/api"
	filePath := "src/billing/BillingHandler.ts"
	query := populateQuery{Base: "billing api", ViewName: "Architecture"}
	children := []populateChildCandidate{
		{name: "BillingHandler", kind: "file", filePath: "src/billing/api/BillingHandler.ts", hasEmbedding: true, embeddingScore: 0.80},
		{name: "BillingRoutes", kind: "file", filePath: "src/billing/api/BillingRoutes.ts", hasEmbedding: true, embeddingScore: 0.80},
		{name: "BillingController", kind: "file", filePath: "src/billing/api/BillingController.ts", hasEmbedding: true, embeddingScore: 0.80},
		{name: "BillingRequest", kind: "file", filePath: "src/billing/api/BillingRequest.ts", hasEmbedding: true, embeddingScore: 0.80},
		{name: "BillingResponse", kind: "file", filePath: "src/billing/api/BillingResponse.ts", hasEmbedding: true, embeddingScore: 0.80},
	}
	candidates := []populateCandidate{
		{
			element:        populateElementResult{ID: 1, Name: "billing api", Kind: &folderKind, FilePath: &folderPath},
			children:       children,
			embeddingScore: 0.50,
			hasEmbedding:   true,
		},
		{
			element:        populateElementResult{ID: 2, Name: "BillingHandler", Kind: &fileKind, FilePath: &filePath},
			embeddingScore: 0.50,
			hasEmbedding:   true,
		},
	}

	scored := scorePopulateCandidates(query, candidates, nil)
	if scored[0].finalScore <= scored[1].finalScore {
		t.Fatalf("broadly matching folder score %.3f should beat concrete file %.3f; folder reason: %s", scored[0].finalScore, scored[1].finalScore, scored[0].element.MatchReason)
	}
}

func TestPopulateRerankerReordersTopTwoXLimitAndUsesCodeContext(t *testing.T) {
	fixture := seedPopulateRerankerFixture(t)
	withPopulateRerankerEndpoint(t, "")
	baseline := requestPopulateResults(t, fixture.routes, "/api/views/20/populate?q=frontend&limit=2")
	if len(baseline) != 2 {
		t.Fatalf("baseline results = %+v, want 2 results", baseline)
	}

	payloadCh := make(chan populateRerankRequest, 1)
	reranker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() { _ = r.Body.Close() }()
		var req populateRerankRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode reranker request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		payloadCh <- req
		appIndex := -1
		adminIndex := -1
		results := make([]populateRerankResult, 0, len(req.Documents))
		for i, doc := range req.Documents {
			switch {
			case strings.Contains(doc, "frontend/src/frontend-app.tsx"):
				appIndex = i
			case strings.Contains(doc, "frontend/src/frontend-admin-panel.tsx"):
				adminIndex = i
			}
		}
		if appIndex >= 0 {
			results = append(results, populateRerankResult{Index: appIndex, RelevanceScore: 0.99})
		}
		if adminIndex >= 0 {
			results = append(results, populateRerankResult{Index: adminIndex, RelevanceScore: 0.91})
		}
		for i := range req.Documents {
			if i == appIndex || i == adminIndex {
				continue
			}
			results = append(results, populateRerankResult{Index: i, RelevanceScore: 0.20})
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(populateRerankResponse{Results: results}); err != nil {
			t.Errorf("encode reranker response: %v", err)
		}
	}))
	defer reranker.Close()
	withPopulateRerankerEndpoint(t, reranker.URL)

	reranked := requestPopulateResults(t, fixture.routes, "/api/views/20/populate?q=frontend&limit=2")
	if len(reranked) != 2 {
		t.Fatalf("reranked results = %+v, want 2 results", reranked)
	}
	if reranked[0].ID != fixture.appID || reranked[1].ID != fixture.adminID {
		t.Fatalf("reranked results = %+v, want app/admin ordering", reranked)
	}
	if baseline[0].ID == reranked[0].ID && baseline[1].ID == reranked[1].ID {
		t.Fatalf("baseline results = %+v, expected reranker to change the top ordering", baseline)
	}

	payload := <-payloadCh
	if payload.Query != "frontend" {
		t.Fatalf("reranker query = %q, want raw user query", payload.Query)
	}
	if payload.TopN != 4 {
		t.Fatalf("reranker top_n = %d, want 4", payload.TopN)
	}
	if len(payload.Documents) != 4 {
		t.Fatalf("reranker documents = %d, want 4 shortlisted candidates", len(payload.Documents))
	}
	appDoc := ""
	for _, doc := range payload.Documents {
		if strings.Contains(doc, "frontend/src/frontend-app.tsx") {
			appDoc = doc
			break
		}
	}
	if appDoc == "" {
		t.Fatalf("reranker documents missing app file context: %+v", payload.Documents)
	}
	if !strings.Contains(appDoc, "responsibility presentation") {
		t.Fatalf("app reranker doc should include architectural responsibility signals: %s", appDoc)
	}
	if !strings.Contains(appDoc, "export function App") {
		t.Fatalf("app reranker doc should include code snippet context: %s", appDoc)
	}
}

func TestPopulateFallsBackWhenRerankerFails(t *testing.T) {
	fixture := seedPopulateRerankerFixture(t)
	withPopulateRerankerEndpoint(t, "")
	baseline := requestPopulateResults(t, fixture.routes, "/api/views/20/populate?q=frontend&limit=2")

	reranker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":"reranker unavailable"}`))
	}))
	defer reranker.Close()
	withPopulateRerankerEndpoint(t, reranker.URL)

	results := requestPopulateResults(t, fixture.routes, "/api/views/20/populate?q=frontend&limit=2")
	if len(results) != len(baseline) {
		t.Fatalf("results = %+v, baseline = %+v", results, baseline)
	}
	for i := range baseline {
		if results[i] != baseline[i] {
			t.Fatalf("results = %+v, want fallback baseline %+v", results, baseline)
		}
	}
}

func TestPopulateRerankerMetricsTrackLatencyAndFallbacks(t *testing.T) {
	fixture := seedPopulateRerankerFixture(t)
	resetPopulateRerankerMetrics()
	t.Cleanup(resetPopulateRerankerMetrics)

	requestCount := 0
	reranker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		defer func() { _ = r.Body.Close() }()
		time.Sleep(15 * time.Millisecond)
		if requestCount == 2 {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"error":"reranker unavailable"}`))
			return
		}
		var req populateRerankRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode reranker request: %v", err)
		}
		results := make([]populateRerankResult, 0, len(req.Documents))
		for i := range req.Documents {
			results = append(results, populateRerankResult{Index: i, RelevanceScore: float64(len(req.Documents) - i)})
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(populateRerankResponse{Results: results}); err != nil {
			t.Fatalf("encode reranker response: %v", err)
		}
	}))
	defer reranker.Close()
	withPopulateRerankerEndpoint(t, reranker.URL)

	_ = requestPopulateResults(t, fixture.routes, "/api/views/20/populate?q=frontend&limit=2")
	_ = requestPopulateResults(t, fixture.routes, "/api/views/20/populate?q=frontend&limit=2")

	metrics := requestPopulateRerankerMetrics(t, fixture.routes)
	if metrics.RequestAttemptsTotal != 2 {
		t.Fatalf("request_attempts_total = %d, want 2", metrics.RequestAttemptsTotal)
	}
	if metrics.RequestSuccessTotal != 1 || metrics.RequestFailureTotal != 1 {
		t.Fatalf("success/failure totals = %+v, want 1 success and 1 failure", metrics)
	}
	if metrics.AppliedTotal != 1 {
		t.Fatalf("applied_total = %d, want 1", metrics.AppliedTotal)
	}
	if metrics.FallbacksTotal != 1 || metrics.FallbacksByReason["request_error"] != 1 {
		t.Fatalf("fallback metrics = %+v, want request_error fallback counted once", metrics)
	}
	if metrics.LastRequestDocumentCount != 4 {
		t.Fatalf("last_request_document_count = %d, want 4", metrics.LastRequestDocumentCount)
	}
	if metrics.RequestLatencyMS.Count != 2 {
		t.Fatalf("latency count = %d, want 2", metrics.RequestLatencyMS.Count)
	}
	if metrics.RequestLatencyMS.Avg <= 0 || metrics.RequestLatencyMS.Min <= 0 || metrics.RequestLatencyMS.Max <= 0 || metrics.RequestLatencyMS.Last <= 0 {
		t.Fatalf("latency snapshot = %+v, want positive latency values", metrics.RequestLatencyMS)
	}
	bucketTotal := metrics.RequestLatencyMS.LE100 + metrics.RequestLatencyMS.LE250 + metrics.RequestLatencyMS.LE500 + metrics.RequestLatencyMS.LE1000 + metrics.RequestLatencyMS.GT1000
	if bucketTotal != 2 {
		t.Fatalf("latency buckets total = %d, want 2", bucketTotal)
	}
}

type populateResultForTest struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}

type populateRerankerMetricsForTest = populateRerankerMetricsSnapshot

func requestPopulateResults(t *testing.T, routes http.Handler, path string) []populateResultForTest {
	t.Helper()
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Results []populateResultForTest `json:"results"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body.Results
}

func requestPopulateRerankerMetrics(t *testing.T, routes http.Handler) populateRerankerMetricsForTest {
	t.Helper()
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/debug/populate-reranker-metrics", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var body populateRerankerMetricsForTest
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body
}

func insertWatchRepository(t *testing.T, db interface {
	Exec(string, ...any) (sql.Result, error)
}) int64 {
	t.Helper()
	return insertWatchRepositoryAt(t, db, t.TempDir())
}

func insertWatchRepositoryAt(t *testing.T, db interface {
	Exec(string, ...any) (sql.Result, error)
}, repoRoot string) int64 {
	t.Helper()
	res, err := db.Exec(`
		INSERT INTO watch_repositories(remote_url, repo_root, display_name, branch, head_commit, identity_status, settings_hash, created_at, updated_at)
		VALUES ('local', ?, 'repo', 'main', '', 'clean', '', 'now', 'now')`, repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func newTestServer(t *testing.T, workspaceID uuid.UUID, static fs.FS) (*localstore.SQLiteStore, http.Handler) {
	return newTestServerWithOptions(t, workspaceID, static, Options{})
}

func newTestServerWithOptions(t *testing.T, workspaceID uuid.UUID, static fs.FS, opts Options) (*localstore.SQLiteStore, http.Handler) {
	t.Helper()
	t.Setenv("DEV", "")
	if static == nil {
		static = fstest.MapFS{"frontend/dist/index.html": {Data: []byte("<html>app</html>")}}
	}
	sqliteStore, err := localstore.Open(filepath.Join(t.TempDir(), "tld.db"), assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqliteStore.Legacy().Close() })
	srv, err := NewWithOptions(sqliteStore, static, workspaceID, opts)
	if err != nil {
		t.Fatal(err)
	}
	return sqliteStore, srv.Routes()
}

func dialCollaborationWebSocket(t *testing.T, serverURL string, viewID int, userID string) *websocket.Conn {
	t.Helper()
	query := url.Values{}
	query.Set("client_id", userID+"-client")
	query.Set("user_id", userID)
	query.Set("username", userID)
	wsURL := "ws" + strings.TrimPrefix(serverURL, "http") + "/api/views/" + strconv.Itoa(viewID) + "/ws?" + query.Encode()
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial collaboration websocket: %v", err)
	}
	return conn
}

func readRealtimeFrameType(t *testing.T, conn *websocket.Conn, want string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if err := conn.SetReadDeadline(deadline); err != nil {
			t.Fatal(err)
		}
		var frame map[string]any
		if err := conn.ReadJSON(&frame); err != nil {
			t.Fatalf("read realtime frame %q: %v", want, err)
		}
		if frame["type"] == want {
			return frame
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for realtime frame %q", want)
		}
	}
}

type populateRerankerFixture struct {
	routes          http.Handler
	architectureID  int64
	appID           int64
	adminID         int64
	secondaryFileID int64
}

func seedPopulateRerankerFixture(t *testing.T) populateRerankerFixture {
	t.Helper()
	repoRoot := t.TempDir()
	writePopulateFixtureFile(t, repoRoot, "frontend/src/frontend-app.tsx", `export function App() {
	return <main>App</main>
}
`)
	writePopulateFixtureFile(t, repoRoot, "frontend/src/frontend-admin-panel.tsx", `export function AdminPanel() {
	return <section>Admin</section>
}
`)
	writePopulateFixtureFile(t, repoRoot, "frontend/src/frontend-routes.tsx", `export const routes = ["/", "/admin"]
`)

	sqliteStore, routes := newTestServer(t, uuid.New(), nil)
	repoID := insertWatchRepositoryAt(t, sqliteStore.DB(), repoRoot)
	if _, err := sqliteStore.DB().Exec(`
		INSERT INTO views(id, name, created_at, updated_at)
		VALUES (20, 'frontend', 'now', 'now');
		INSERT INTO elements(id, name, kind, tags, technology_connectors, file_path, language, created_at, updated_at)
		VALUES
			(100, 'frontend', 'folder', '["role:ui"]', '[]', 'frontend', NULL, 'now', 'now'),
			(101, 'Frontend App', 'architecture-component', '["role:ui"]', '[]', 'frontend', NULL, 'now', 'now'),
			(102, 'frontend-app.tsx', 'file', '["role:ui"]', '[]', 'frontend/src/frontend-app.tsx', 'tsx', 'now', 'now'),
			(103, 'frontend-admin-panel.tsx', 'file', '["role:ui"]', '[]', 'frontend/src/frontend-admin-panel.tsx', 'tsx', 'now', 'now'),
			(104, 'frontend-routes.tsx', 'file', '["role:ui"]', '[]', 'frontend/src/frontend-routes.tsx', 'tsx', 'now', 'now');
		INSERT INTO views(id, name, owner_element_id, created_at, updated_at)
		VALUES (21, 'Frontend App detail', 101, 'now', 'now');
		INSERT INTO placements(view_id, element_id, position_x, position_y, created_at, updated_at)
		VALUES
			(21, 102, 0, 0, 'now', 'now'),
			(21, 103, 40, 0, 'now', 'now'),
			(21, 104, 80, 0, 'now', 'now');
		INSERT INTO watch_materialization(repository_id, owner_type, owner_key, resource_type, resource_id, created_at, updated_at)
		VALUES
			(?, 'folder', 'folder:frontend', 'element', 100, 'now', 'now'),
			(?, 'architecture-component', 'component:frontend', 'element', 101, 'now', 'now'),
			(?, 'file', 'file:frontend/src/frontend-app.tsx', 'element', 102, 'now', 'now'),
			(?, 'file', 'file:frontend/src/frontend-admin-panel.tsx', 'element', 103, 'now', 'now'),
			(?, 'file', 'file:frontend/src/frontend-routes.tsx', 'element', 104, 'now', 'now');
		INSERT INTO watch_files(id, repository_id, path, language, worktree_hash, size_bytes, mtime_unix, scan_status, created_at, updated_at)
		VALUES
			(201, ?, 'frontend/src/frontend-app.tsx', 'tsx', 'a', 10, 1, 'parsed', 'now', 'now'),
			(202, ?, 'frontend/src/frontend-admin-panel.tsx', 'tsx', 'b', 10, 1, 'parsed', 'now', 'now'),
			(203, ?, 'frontend/src/frontend-routes.tsx', 'tsx', 'c', 10, 1, 'parsed', 'now', 'now');
		INSERT INTO watch_symbols(id, repository_id, file_id, stable_key, name, qualified_name, kind, start_line, end_line, signature_hash, content_hash, raw_json, created_at, updated_at)
		VALUES
			(301, ?, 201, 'sym:app', 'App', 'App', 'function', 1, 3, 'sa', 'ca', '{}', 'now', 'now'),
			(302, ?, 202, 'sym:admin', 'AdminPanel', 'AdminPanel', 'function', 1, 3, 'sb', 'cb', '{}', 'now', 'now'),
			(303, ?, 203, 'sym:routes', 'routes', 'routes', 'const', 1, 1, 'sc', 'cc', '{}', 'now', 'now')`,
		repoID, repoID, repoID, repoID, repoID,
		repoID, repoID, repoID,
		repoID, repoID, repoID); err != nil {
		t.Fatal(err)
	}
	return populateRerankerFixture{
		routes:          routes,
		architectureID:  101,
		appID:           102,
		adminID:         103,
		secondaryFileID: 104,
	}
}

func writePopulateFixtureFile(t *testing.T, repoRoot, relativePath, content string) {
	t.Helper()
	path := filepath.Join(repoRoot, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func withPopulateRerankerEndpoint(t *testing.T, endpoint string) {
	t.Helper()
	oldEndpoint := populateRerankerEndpoint
	populateRerankerEndpoint = endpoint
	t.Cleanup(func() {
		populateRerankerEndpoint = oldEndpoint
	})
}

type capturePopulateEmbeddingProvider struct {
	inputs []watch.EmbeddingInput
}

func (p *capturePopulateEmbeddingProvider) ModelID() watch.ModelID {
	return watch.ModelID{Provider: "capture-test", Model: "capture-test", Dimension: 2}
}

func (p *capturePopulateEmbeddingProvider) Embed(_ context.Context, inputs []watch.EmbeddingInput) ([]watch.Vector, error) {
	p.inputs = append([]watch.EmbeddingInput(nil), inputs...)
	vectors := make([]watch.Vector, len(inputs))
	for i := range inputs {
		vectors[i] = watch.Vector{float32(i + 1), 0}
	}
	return vectors, nil
}

func populateVectorBytesForTest(values ...float32) []byte {
	data := make([]byte, len(values)*4)
	for i, value := range values {
		binary.LittleEndian.PutUint32(data[i*4:], math.Float32bits(value))
	}
	return data
}

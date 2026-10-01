package server

import (
	"context"
	"encoding/json"
	"io/fs"
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

func TestIsSafeDynamicIconFilenameRejectsPathSyntax(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		want     bool
	}{
		{name: "svg", filename: "live-icon.svg", want: true},
		{name: "png", filename: "live-icon.png", want: true},
		{name: "empty", filename: "", want: false},
		{name: "unsupported extension", filename: "live-icon.gif", want: false},
		{name: "slash path", filename: "nested/live-icon.svg", want: false},
		{name: "windows separator path", filename: `nested\live-icon.svg`, want: false},
		{name: "windows traversal", filename: `..\secret.svg`, want: false},
		{name: "windows drive path", filename: `C:\secret.svg`, want: false},
		{name: "windows drive relative path", filename: "C:secret.svg", want: false},
		{name: "windows alternate data stream", filename: "live-icon.svg:stream", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSafeDynamicIconFilename(tt.filename); got != tt.want {
				t.Fatalf("isSafeDynamicIconFilename(%q) = %v, want %v", tt.filename, got, tt.want)
			}
		})
	}
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

package server

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	diagv1connect "buf.build/gen/go/tldiagramcom/diagram/connectrpc/go/diag/v1/diagv1connect"
	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/google/uuid"
	assets "github.com/mertcikla/tld"
	localstore "github.com/mertcikla/tld/internal/store"
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

func TestServerInjectsWorkspaceIDIntoConnectRPCResponses(t *testing.T) {
	workspaceID := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	sqliteStore, routes := newTestServer(t, workspaceID, nil)
	if _, err := sqliteStore.DB().Exec(`
		INSERT INTO elements(id, name, tags, technology_connectors, created_at, updated_at)
		VALUES (10, 'API', '[]', '[]', 'now', 'now');
	`); err != nil {
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

func newTestServer(t *testing.T, workspaceID uuid.UUID, static fs.FS) (*localstore.SQLiteStore, http.Handler) {
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
	srv, err := New(sqliteStore, static, workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	return sqliteStore, srv.Routes()
}

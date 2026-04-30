package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"

	"buf.build/gen/go/tldiagramcom/diagram/connectrpc/go/diag/v1/diagv1connect"
	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/mertcikla/tld/internal/store"
	"github.com/mertcikla/tld/internal/watch"
	"github.com/mertcikla/tld/pkg/api"
)

type Server struct {
	handler http.Handler
}

func New(sqliteStore *store.SQLiteStore, static fs.FS, workspaceID uuid.UUID) (*Server, error) {
	apiStore := store.NewAPIAdapter(sqliteStore)
	watchStore := watch.NewStore(sqliteStore.DB())
	lockHooks := watchLockHooks{store: watchStore}
	wsSvc := &api.WorkspaceService{Store: apiStore, Hooks: lockHooks}
	depSvc := &api.DependencyService{Store: apiStore}
	importSvc := &api.ImportService{Store: apiStore}
	versionSvc := &api.WorkspaceVersionService{Store: apiStore, Hooks: lockHooks}

	mux := http.NewServeMux()
	watch.NewHandler(watchStore).Register(mux)

	mux.HandleFunc("GET /api/ready", func(w http.ResponseWriter, r *http.Request) {
		views, elements, connectors, err := apiStore.GetWorkspaceResourceCounts(r.Context(), workspaceID)
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":    false,
				"error": "resource counts unavailable",
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok": true,
			"resources": map[string]int{
				"views":      views,
				"elements":   elements,
				"connectors": connectors,
			},
		})
	})

	mux.HandleFunc("GET /api/views/{id}/thumbnail.svg", func(w http.ResponseWriter, r *http.Request) {
		viewID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || viewID <= 0 {
			http.Error(w, "invalid view id", http.StatusBadRequest)
			return
		}

		svg, err := sqliteStore.ThumbnailSVG(r.Context(), viewID)
		if err != nil {
			http.Error(w, "thumbnail not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write([]byte(svg))
	})

	wsPath, wsHandler := diagv1connect.NewWorkspaceServiceHandler(wsSvc)
	mux.Handle("/api"+wsPath, http.StripPrefix("/api", wsHandler))

	depPath, depHandler := diagv1connect.NewDependencyServiceHandler(depSvc)
	mux.Handle("/api"+depPath, http.StripPrefix("/api", depHandler))

	importPath, importHandler := diagv1connect.NewImportServiceHandler(importSvc)
	mux.Handle("/api"+importPath, http.StripPrefix("/api", importHandler))

	versionPath, versionHandler := diagv1connect.NewWorkspaceVersionServiceHandler(versionSvc)
	mux.Handle("/api"+versionPath, http.StripPrefix("/api", versionHandler))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		serveStatic(static, w, r)
	})

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(w, r.WithContext(api.WithWorkspaceID(r.Context(), workspaceID)))
	})

	return &Server{handler: handler}, nil
}

type watchLockHooks struct {
	api.NopWorkspaceHooks
	store *watch.Store
}

func (h watchLockHooks) CheckWrite(ctx context.Context, _ uuid.UUID, resourceType string) error {
	if h.store == nil {
		return nil
	}
	lock, live, err := h.store.ActiveLiveLock(ctx, watch.LockHeartbeatTimeout)
	if err != nil || !live {
		return err
	}
	return connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("workspace is locked by tld watch for repository %d; stop watch before editing %s", lock.RepositoryID, resourceType))
}

func (h watchLockHooks) CheckApplyPlan(ctx context.Context, workspaceID uuid.UUID, _ *diagv1.ApplyPlanRequest) error {
	return h.CheckWrite(ctx, workspaceID, "workspace")
}

func (s *Server) Routes() http.Handler {
	return s.handler
}

func (s *Server) Shutdown(context.Context) error {
	return nil
}

func serveStatic(static fs.FS, w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		http.NotFound(w, r)
		return
	}

	if os.Getenv("DEV") == "true" {
		target, err := url.Parse("http://localhost:5173")
		if err == nil {
			httputil.NewSingleHostReverseProxy(target).ServeHTTP(w, r)
			return
		}
	}

	cleaned := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if cleaned == "" {
		cleaned = "index.html"
	}

	tryPaths := []string{
		path.Join("frontend/dist", cleaned),
		"frontend/dist/index.html",
	}
	for _, candidate := range tryPaths {
		data, err := fs.ReadFile(static, candidate)
		if err != nil {
			continue
		}
		w.Header().Set("Content-Type", contentType(candidate))
		_, _ = w.Write(data)
		return
	}

	http.NotFound(w, r)
}

func contentType(file string) string {
	switch path.Ext(file) {
	case ".html":
		return "text/html; charset=utf-8"
	case ".css":
		return "text/css; charset=utf-8"
	case ".js":
		return "application/javascript"
	case ".svg":
		return "image/svg+xml"
	case ".png":
		return "image/png"
	case ".json":
		return "application/json"
	default:
		return "application/octet-stream"
	}
}

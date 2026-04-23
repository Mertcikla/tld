package server

import (
	"context"
	"io/fs"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path"
	"strings"

	"buf.build/gen/go/tldiagramcom/diagram/connectrpc/go/diag/v1/diagv1connect"
	"github.com/google/uuid"
	"github.com/mertcikla/diag/tld/internal/store"
	"github.com/mertcikla/diag/tld/pkg/api"
)

type Server struct {
	handler http.Handler
}

func New(sqliteStore *store.SQLiteStore, static fs.FS, workspaceID uuid.UUID) (*Server, error) {
	apiStore := store.NewAPIAdapter(sqliteStore)
	wsSvc := &api.WorkspaceService{Store: apiStore}
	depSvc := &api.DependencyService{Store: apiStore}
	importSvc := &api.ImportService{Store: apiStore}
	versionSvc := &api.WorkspaceVersionService{Store: apiStore}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/ready", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
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

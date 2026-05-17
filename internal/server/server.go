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
	"github.com/mertcikla/tld/v2/internal/store"
	"github.com/mertcikla/tld/v2/internal/watch"
	"github.com/mertcikla/tld/v2/pkg/api"
)

type Server struct {
	handler http.Handler
}

func New(sqliteStore *store.SQLiteStore, static fs.FS, workspaceID uuid.UUID) (*Server, error) {
	apiStore := store.NewAPIAdapter(sqliteStore)
	watchStore := watch.NewStore(sqliteStore.DB())
	lockHooks := watchLockHooks{store: watchStore}
	wsSvc := &api.WorkspaceService{Store: apiStore, Hooks: lockHooks}
	orgSvc := &api.OrgService{Store: apiStore, Hooks: lockHooks}
	depSvc := &api.DependencyService{Store: apiStore}
	importSvc := &api.ImportService{Store: apiStore}
	versionSvc := &api.WorkspaceVersionService{Store: apiStore, Hooks: lockHooks}

	mux := http.NewServeMux()
	watch.NewHandler(watchStore).Register(mux)
	registerEditorHandlers(mux, watchStore)
	registerDensityHandlers(mux, sqliteStore)
	registerMergeHandlers(mux, sqliteStore)

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

	orgPath, orgHandler := diagv1connect.NewOrgServiceHandler(orgSvc)
	mux.Handle("/api"+orgPath, http.StripPrefix("/api", orgHandler))

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

	return &Server{handler: localCORSMiddleware(handler)}, nil
}

type watchLockHooks struct {
	api.NopWorkspaceHooks
	store *watch.Store
}

func (h watchLockHooks) CheckWrite(ctx context.Context, _ uuid.UUID, resourceType string) error {
	if h.store == nil {
		return nil
	}
	applying, err := h.store.ActiveApplyLock(ctx, watch.LockHeartbeatTimeout)
	if err != nil || !applying {
		return err
	}
	return connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("workspace is being updated by tld watch; retry editing %s shortly", resourceType))
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

func localCORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); isAllowedLocalOrigin(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			if requestedHeaders := r.Header.Get("Access-Control-Request-Headers"); requestedHeaders != "" {
				w.Header().Set("Access-Control-Allow-Headers", requestedHeaders)
			} else {
				w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Connect-Protocol-Version, Connect-Timeout-Ms, Content-Type, X-CSRF-Token, X-Requested-With")
			}
			if r.Header.Get("Access-Control-Request-Private-Network") == "true" {
				w.Header().Set("Access-Control-Allow-Private-Network", "true")
			}
			w.Header().Set("Access-Control-Expose-Headers", "Connect-Protocol-Version, Grpc-Status, Grpc-Message")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isAllowedLocalOrigin(origin string) bool {
	if origin == "" {
		return false
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	switch u.Scheme {
	case "vscode-webview", "vscode-webview-resource", "vscode-file", "vscode-resource":
		return true
	case "http", "https":
		host := u.Hostname()
		return host == "127.0.0.1" || host == "localhost" || host == "::1"
	default:
		return false
	}
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
		data, encoding, err := readStaticAsset(static, candidate, r.Header.Get("Accept-Encoding"))
		if err != nil {
			continue
		}
		w.Header().Set("Content-Type", contentType(candidate))
		w.Header().Add("Vary", "Accept-Encoding")
		if encoding != "" {
			w.Header().Set("Content-Encoding", encoding)
		}
		_, _ = w.Write(data)
		return
	}

	http.NotFound(w, r)
}

func readStaticAsset(static fs.FS, candidate, acceptEncoding string) ([]byte, string, error) {
	if acceptsEncoding(acceptEncoding, "br") {
		if data, err := fs.ReadFile(static, candidate+".br"); err == nil {
			return data, "br", nil
		}
	}
	if acceptsEncoding(acceptEncoding, "gzip") {
		if data, err := fs.ReadFile(static, candidate+".gz"); err == nil {
			return data, "gzip", nil
		}
	}
	data, err := fs.ReadFile(static, candidate)
	return data, "", err
}

func acceptsEncoding(header, encoding string) bool {
	for _, part := range strings.Split(header, ",") {
		token, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		if !strings.EqualFold(token, encoding) {
			continue
		}
		for _, param := range strings.Split(params, ";") {
			key, value, ok := strings.Cut(strings.TrimSpace(param), "=")
			if ok && strings.EqualFold(key, "q") {
				quality, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
				if err == nil && quality <= 0 {
					return false
				}
			}
		}
		return true
	}
	return false
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

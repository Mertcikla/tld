package exec

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"

	rpc "buf.build/gen/go/tldiagramcom/diagram/connectrpc/go/diag/v1/diagv1connect"
	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/google/uuid"
	"github.com/mertcikla/codeindex/gitstate"
	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/internal/client"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/store"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/mertcikla/tld/v2/pkg/api"
	"github.com/mertcikla/tld/v2/pkg/app"
)

type handlerTransport struct{ handler http.Handler }

func (t handlerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	w := httptest.NewRecorder()
	t.handler.ServeHTTP(w, req)
	return w.Result(), nil
}

// PlanClients uses the same Connect handlers locally and remotely. The local
// transport stays in-process and never starts a server or writes YAML caches.
func PlanClients(cfg workspace.Config, target, dataDir, planID string) (rpc.PlanServiceClient, rpc.WorkspaceServiceClient, func() error, error) {
	resolved, err := ResolveTarget(cfg, target)
	if err != nil {
		return nil, nil, nil, err
	}
	closeFn := func() error { return nil }
	httpClient := &http.Client{}
	url := client.NormalizeURL(cfg.ServerURL)
	if resolved == TargetLocal {
		dir, e := workspace.ResolveDataDir(&cfg, dataDir)
		if e != nil {
			return nil, nil, nil, e
		}
		if e = os.MkdirAll(dir, 0755); e != nil {
			return nil, nil, nil, e
		}
		st, e := store.OpenLocal(context.Background(), &cfg, dir, assets.FS)
		if e != nil {
			return nil, nil, nil, e
		}
		closeFn = st.Close
		svc := &api.PlanService{Store: st, NotesDir: filepath.Join(dir, "plans"), Source: func(tx *app.Store) api.Store { return store.NewAPIAdapter(tx, dir) }}
		index := cstore.NewStore(st.DB(), st.BunDB(), st.Dialect())
		svc.RepositoryRevision = func(ctx context.Context, id string) (string, error) {
			repository, err := index.Repository(ctx, id)
			// Existing CLI plans can refer to a repository before it is indexed.
			if errors.Is(err, sql.ErrNoRows) {
				return "", nil
			}
			if err != nil {
				return "", err
			}
			if repository.Root == "" {
				return "", nil
			}
			_, revision, err := gitstate.CurrentCommit(ctx, repository.Root)
			return revision, err
		}

		mux := http.NewServeMux()
		path, h := rpc.NewPlanServiceHandler(svc)
		mux.Handle("/api"+path, http.StripPrefix("/api", h))
		wsPath, wsHandler := rpc.NewWorkspaceServiceHandler(&api.WorkspaceService{Store: svc.Source(st)})
		mux.Handle("/api"+wsPath, http.StripPrefix("/api", wsHandler))
		scope := uuid.Nil
		httpClient.Transport = handlerTransport{handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := api.WithWorkspaceID(r.Context(), scope)
			svc.PlanMiddleware(mux).ServeHTTP(w, r.WithContext(ctx))
		})}
		url = "http://local/api"
	}
	var revisionMu sync.Mutex
	var revision string
	intercept := connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			if cfg.APIKey != "" {
				req.Header().Set("Authorization", "Bearer "+cfg.APIKey)
			}
			if req.Header().Get("X-Tld-Request-Id") == "" {
				req.Header().Set("X-Tld-Request-Id", uuid.NewString())
			}
			if planID != "" {
				req.Header().Set("X-Tld-Plan", planID)
				revisionMu.Lock()
				if revision != "" {
					req.Header().Set("X-Tld-Plan-Revision", revision)
				}
				revisionMu.Unlock()
			}
			response, err := next(ctx, req)
			if err == nil && response != nil {
				if observed := response.Header().Get("X-Tld-Plan-Revision"); observed != "" {
					revisionMu.Lock()
					revision = observed
					revisionMu.Unlock()
				}
			}
			return response, err
		}
	})
	opts := connect.WithInterceptors(intercept)
	return rpc.NewPlanServiceClient(httpClient, url, opts), rpc.NewWorkspaceServiceClient(httpClient, url, opts), closeFn, nil
}
func NewPlanRunner(cfg workspace.Config, target, dataDir, id string) (Runner, error) {
	_, ws, closeFn, err := PlanClients(cfg, target, dataDir, id)
	if err != nil {
		return nil, err
	}
	resolved, _ := ResolveTarget(cfg, target)
	orgID := cfg.WorkspaceID
	if resolved == TargetLocal {
		orgID = uuid.NameSpaceOID.String()
	}
	return &remoteRunner{localDataDir: dataDir, planLocal: resolved == TargetLocal, serverURL: cfg.ServerURL, apiKey: cfg.APIKey, orgID: orgID, planClient: ws, planClose: closeFn}, nil
}

func (r *localRunner) ImplementationLocked(ctx context.Context) (bool, error) {
	return r.appStore.ImplementationLocked(ctx, uuid.Nil.String())
}
func (r *remoteRunner) ImplementationLocked(ctx context.Context) (bool, error) {
	cfg := workspace.Config{ServerURL: r.serverURL, APIKey: r.apiKey, WorkspaceID: r.orgID}
	target := TargetRemote
	if r.planLocal {
		target = TargetLocal
	}
	client, _, closeFn, err := PlanClients(cfg, target, r.localDataDir, "")
	if err != nil {
		return false, err
	}
	defer func() { _ = closeFn() }()
	response, err := client.GetImplementation(ctx, connect.NewRequest(&pb.GetImplementationRequest{}))
	if connect.CodeOf(err) == connect.CodeUnimplemented || connect.CodeOf(err) == connect.CodeNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return response.Msg.Locked, nil
}

package server

import (
	"context"
	"net/http"

	"buf.build/gen/go/tldiagramcom/diagram/connectrpc/go/codeindex/v1/codeindexv1connect"
	codeindexv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"connectrpc.com/connect"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/store"
)

// codeIndexRepositoryService exposes the repositories indexed by the in-process
// codeindex engine to the UI.
type codeIndexRepositoryService struct {
	codeindexv1connect.UnimplementedRepositoryServiceHandler
	store *cstore.Store
}

func (s *codeIndexRepositoryService) ListRepositories(ctx context.Context, _ *connect.Request[codeindexv1.ListRepositoriesRequest]) (*connect.Response[codeindexv1.ListRepositoriesResponse], error) {
	repositories, err := s.store.ListRepositories(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&codeindexv1.ListRepositoriesResponse{Repositories: repositories}), nil
}

func registerCodeIndexHandlers(mux *http.ServeMux, sqliteStore *store.SQLiteStore) {
	idx := cstore.NewStore(sqliteStore.DB(), sqliteStore.BunDB(), sqliteStore.Dialect())
	svc := &codeIndexRepositoryService{store: idx}
	path, handler := codeindexv1connect.NewRepositoryServiceHandler(svc)
	mux.Handle("/api"+path, http.StripPrefix("/api", handler))
}

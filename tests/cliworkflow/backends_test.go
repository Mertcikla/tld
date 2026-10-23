//go:build cliworkflow

package cliworkflow

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	diagv1connect "buf.build/gen/go/tldiagramcom/diagram/connectrpc/go/diag/v1/diagv1connect"
	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/google/uuid"
	assets "github.com/mertcikla/tld/v2"
	localstore "github.com/mertcikla/tld/v2/internal/store"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/mertcikla/tld/v2/pkg/api"
	"github.com/uptrace/bun/driver/pgdriver"
)

type rpcFaults struct {
	failExport           atomic.Bool
	failWrites           atomic.Bool
	failExportAfterWrite atomic.Bool
	interrupt            atomic.Bool
	entered              chan struct{}
	finished             chan struct{}
}

func (h *harness) setEnv(key, value string) {
	kept := h.env[:0]
	for _, e := range h.env {
		if !strings.HasPrefix(e, key+"=") {
			kept = append(kept, e)
		}
	}
	h.env = append(kept, key+"="+value)
}

func (h *harness) configureBackend(backend string) {
	if backend == "postgres" {
		h.configurePostgres()
		return
	}
	if backend != "remote" {
		return
	}
	s, err := localstore.OpenLocal(context.Background(), &workspace.Config{}, filepath.Join(h.dir, "data"), assets.FS)
	if err != nil {
		h.fail("open remote store: %v", err)
	}
	h.t.Cleanup(func() { _ = s.Close() })
	orgID := uuid.MustParse("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
	if _, err := s.DB().Exec("UPDATE views SET org_id = ? WHERE owner_element_id IS NULL", orgID.String()); err != nil {
		h.fail("scope remote bootstrap view: %v", err)
	}
	adapter := localstore.NewAPIAdapter(s)
	mux := http.NewServeMux()
	h.faults = &rpcFaults{entered: make(chan struct{}), finished: make(chan struct{})}
	wrapped := &interruptStore{Store: adapter, transactions: adapter, faults: h.faults}
	path, handler := diagv1connect.NewWorkspaceServiceHandler(&api.WorkspaceService{Store: wrapped})
	mux.Handle("/api"+path, http.StripPrefix("/api", handler))
	path, handler = diagv1connect.NewImportServiceHandler(&api.ImportService{Store: wrapped})
	mux.Handle("/api"+path, http.StripPrefix("/api", handler))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer workflow-test" {
			http.Error(w, "missing test credential", http.StatusUnauthorized)
			return
		}
		r = r.WithContext(api.WithWorkspaceID(r.Context(), orgID))
		export := strings.HasSuffix(r.URL.Path, "/ExportWorkspace")
		write := strings.Contains(r.URL.Path, "/Create") || strings.Contains(r.URL.Path, "/Update") || strings.Contains(r.URL.Path, "/Delete") || strings.HasSuffix(r.URL.Path, "/ImportResources") || strings.HasSuffix(r.URL.Path, "/ApplyWorkspaceBatch")
		if (export && h.faults.failExport.Load()) || (write && h.faults.failWrites.Load()) {
			http.Error(w, "injected RPC failure", http.StatusServiceUnavailable)
			return
		}
		if write && h.faults.failExportAfterWrite.Load() {
			h.faults.failExport.Store(true)
		}
		mux.ServeHTTP(w, r)
	}))
	h.t.Cleanup(server.Close)
	h.targetName = "remote"
	h.setEnv("TLD_SERVER_URL", server.URL)
	h.setEnv("TLD_API_KEY", "workflow-test")
	h.setEnv("TLD_ORG_ID", "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee")
}

// The barrier is inside the production import transaction, after a real row
// insert. Killing the CLI cancels HTTP, which must roll back every batch row.
type interruptStore struct {
	api.Store
	transactions api.TransactionalStore
	faults       *rpcFaults
}

func (s *interruptStore) RunInTransaction(ctx context.Context, fn func(context.Context, api.Store) error) error {
	err := s.transactions.RunInTransaction(ctx, func(txCtx context.Context, tx api.Store) error {
		return fn(txCtx, &interruptStore{Store: tx, faults: s.faults})
	})
	if s.faults.interrupt.Load() {
		close(s.faults.finished)
	}
	return err
}
func (s *interruptStore) ApplyBatch(ctx context.Context, id uuid.UUID, input *diagv1.ApplyWorkspaceBatchRequest) (*diagv1.ApplyWorkspaceBatchResponse, error) {
	el, err := s.Store.ApplyBatch(ctx, id, input)
	if err == nil && s.faults.interrupt.Load() {
		close(s.faults.entered)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return el, err
}

// Each test gets its own database. Never reset the caller's public schema.
func (h *harness) configurePostgres() {
	configured := os.Getenv("TLD_TEST_POSTGRES_URL")
	u, err := url.Parse(configured)
	if err != nil || configured == "" {
		h.fail("TLD_TEST_POSTGRES_URL must be a PostgreSQL URL")
	}
	admin := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(configured)))
	name := "tld_workflow_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err := admin.ExecContext(ctx, `CREATE DATABASE "`+name+`"`); err != nil {
		_ = admin.Close()
		h.fail("create isolated test database: %v", err)
	}
	h.t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if _, err := admin.ExecContext(ctx, `DROP DATABASE "`+name+`" WITH (FORCE)`); err != nil {
			h.t.Logf("drop test database: %v", err)
		}
		_ = admin.Close()
	})
	u.Path = "/" + name
	h.dsn = u.String()
	h.setEnv("TLD_DB_DRIVER", "postgres")
	h.setEnv("TLD_DATABASE_URL", h.dsn)
}

func (h *harness) writeDB() *sql.DB {
	if h.dsn != "" {
		return h.db()
	}
	db, err := sql.Open("sqlite", filepath.Join(h.dir, "data", "tld.db"))
	if err != nil {
		h.fail("open write connection: %v", err)
	}
	return db
}

func (h *harness) sqlExec(query string, args ...any) {
	db := h.writeDB()
	defer func() { _ = db.Close() }()
	if h.dsn != "" {
		for i := 1; strings.Contains(query, "?"); i++ {
			query = strings.Replace(query, "?", fmtInt(i), 1)
		}
	}
	if _, err := db.Exec(query, args...); err != nil {
		h.fail("execute fixture SQL: %v", err)
	}
}

func fmtInt(i int) string { return "$" + strconv.Itoa(i) }

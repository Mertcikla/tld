package store

import (
	"context"
	"database/sql"
	"errors"
	"math"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/google/uuid"
	"github.com/mertcikla/codeindex/graph"
	"github.com/mertcikla/codeindex/identity"
	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/pkg/app"
	"github.com/mertcikla/tld/v2/pkg/dbrepo"
	"github.com/uptrace/bun"
)

func TestConcurrentRepositoryIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identities.db")
	stores := make([]*Store, 2)
	for i := range stores {
		handle, err := dbrepo.OpenSQLite(context.Background(), dbrepo.DBOptions{SQLitePath: path, Migrations: assets.FS})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = handle.Close() })
		stores[i] = NewStoreFromHandle(handle)
	}
	testConcurrentRepositoryIdentity(t, stores[0], stores[1])
}

func testConcurrentRepositoryIdentity(t *testing.T, first, second *Store) {
	t.Helper()
	root := t.TempDir()
	for _, tc := range []struct {
		name, remoteURL string
		org             uuid.UUID
	}{
		{name: "local"},
		{name: "tenant A", org: uuid.New()},
		{name: "tenant B", org: uuid.New()},
		{name: "remote checkouts", remoteURL: "https://github.com/owner/concurrent", org: uuid.New()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(app.WithTenantOrgID(context.Background(), tc.org), 10*time.Second)
			defer cancel()
			otherRoot := root
			if tc.remoteURL != "" {
				otherRoot = t.TempDir()
			}
			lookupDone, release, sequenceStarted := make(chan struct{}), make(chan struct{}), make(chan struct{})
			// Pause after the first caller observes a missing root. Let the other
			// connection attempt a sequence update before the first can insert.
			// Without the transaction lock, both callers create distinct IDs.
			first.bun.AddQueryHook(&repositoryIdentityRaceHook{lookupDone: lookupDone, release: release})
			second.bun.AddQueryHook(&repositoryIdentityRaceHook{sequenceStarted: sequenceStarted})
			type result struct {
				resolved identity.Resolved
				err      error
			}
			results := make(chan result, 2)
			resolve := func(st *Store, path string, managed bool) {
				resolved, err := st.ResolveRepository(ctx, path, tc.remoteURL, managed, tc.org)
				results <- result{resolved, err}
			}
			go resolve(first, root, false)
			select {
			case <-lookupDone:
			case <-ctx.Done():
				t.Fatal("first lookup did not finish:", ctx.Err())
			}
			go resolve(second, otherRoot, true)
			select {
			case <-sequenceStarted:
			case <-ctx.Done():
				t.Fatal("second connection did not attempt ID allocation or locking:", ctx.Err())
			}
			close(release)
			var id string
			for range 2 {
				got := <-results
				if got.err != nil {
					t.Fatal(got.err)
				}
				if !numericID(got.resolved.ID) || (id != "" && id != got.resolved.ID) {
					t.Fatalf("same repository resolved to different IDs: %q and %q", id, got.resolved.ID)
				}
				id = got.resolved.ID
			}
			repositories, err := first.ListRepositories(ctx)
			if err != nil || len(repositories) != 1 || repositories[0].Id != id || !repositories[0].Managed {
				t.Fatalf("repository identity or managed flag lost: %v, %v", repositories, err)
			}
		})
	}
}

type repositoryIdentityRaceHook struct {
	lookupDone, release, sequenceStarted chan struct{}
	once                                 sync.Once
}

func (h *repositoryIdentityRaceHook) BeforeQuery(ctx context.Context, event *bun.QueryEvent) context.Context {
	if h.sequenceStarted != nil && strings.Contains(event.Query, "UPDATE codeindex_id_sequence") {
		h.once.Do(func() { close(h.sequenceStarted) })
	}
	return ctx
}

func (h *repositoryIdentityRaceHook) AfterQuery(ctx context.Context, event *bun.QueryEvent) {
	if h.lookupDone != nil && errors.Is(event.Err, sql.ErrNoRows) && strings.Contains(event.Query, "WHERE root=") {
		h.once.Do(func() {
			close(h.lookupDone)
			select {
			case <-h.release:
			case <-ctx.Done():
			}
		})
	}
}

func TestIntegerIDsAllocateAtomicallyAndUse64Bits(t *testing.T) {
	st, handle := openTestStore(t)
	defer func() { _ = handle.Close() }()
	ctx := context.Background()
	const callers = 12
	ids := make(chan int64, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Go(func() {
			id, err := st.AllocateIDs(ctx, 3)
			if err != nil {
				t.Error(err)
				return
			}
			ids <- id
		})
	}
	wg.Wait()
	close(ids)
	seen := map[int64]bool{}
	for first := range ids {
		for id := first; id < first+3; id++ {
			if seen[id] || id < 1 {
				t.Fatalf("duplicate or invalid ID: %d", id)
			}
			seen[id] = true
		}
	}
	if len(seen) != callers*3 {
		t.Fatalf("allocated %d IDs", len(seen))
	}
	if _, err := st.AllocateIDs(ctx, 0); err == nil {
		t.Fatal("zero count accepted")
	}
	if _, err := st.db.ExecContext(ctx, `UPDATE codeindex_id_sequence SET value=2147483647`); err != nil {
		t.Fatal(err)
	}
	if id, err := st.AllocateIDs(ctx, 1); err != nil || id != 2147483648 {
		t.Fatalf("64-bit allocation: %d %v", id, err)
	}
	if _, err := st.bun.NewRaw(`UPDATE codeindex_id_sequence SET value=?`, int64(math.MaxInt64)).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AllocateIDs(ctx, 1); err == nil {
		t.Fatal("overflow accepted")
	}
}

func TestIntegerIDPublicationRemapsReferencesAndPreservesPayloads(t *testing.T) {
	st, handle := openTestStore(t)
	defer func() { _ = handle.Close() }()
	ctx := context.Background()
	root := t.TempDir()
	repository, err := st.ResolveRepository(ctx, root, "", false, scope(ctx).orgID)
	if err != nil {
		t.Fatal(err)
	}
	snap := &pb.Snapshot{Id: graph.ID("pending"), RepositoryId: repository.ID}
	g := graph.NewGraph(snap.RepositoryId, snap.Id)
	src := &graph.Source{Path: "a.go", Text: []byte("func A(){}\nfunc B(){}\n"), Hash: "source-hash"}
	g.Sources[src.Path] = src
	parent := g.AddFact(pb.FactKind_FACT_KIND_FUNCTION, "A", "go", src.Anchor(0, 10), "A", "", nil)
	child := g.AddFact(pb.FactKind_FACT_KIND_FUNCTION, "B", "go", src.Anchor(11, 21), "B", "", nil)
	child.ParentFactId = parent.Id
	edge := g.AddEdgeFact(pb.EdgeKind_EDGE_KIND_CALLS, child.Id, parent.Id, "symbol/A", src.Anchor(12, 13))
	if err := st.Publish(ctx, root, snap, g); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{repository.ID, snap.Id, parent.Id, child.Id, edge.Id} {
		if _, err := strconv.ParseInt(id, 10, 64); err != nil {
			t.Fatalf("ID %q: %v", id, err)
		}
	}
	loaded, err := st.LoadGraph(ctx, snap.Id)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Facts[child.Id].ParentFactId != parent.Id || loaded.EdgeFacts[edge.Id].FromFactId != child.Id || loaded.EdgeFacts[edge.Id].ToFactId != parent.Id {
		t.Fatal("references were not remapped")
	}
	got := loaded.EdgeFacts[edge.Id]
	if got.Anchor != nil || len(got.Evidence) != 0 {
		t.Fatalf("edge retained provenance: %v", got)
	}
	first := snap.Id
	if err := st.Publish(ctx, root, snap, g); err != nil || snap.Id != first {
		t.Fatalf("retry changed snapshot: %s %v", snap.Id, err)
	}
	if _, err := st.Snapshot(ctx, graph.ID("old snapshot")); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("legacy snapshot lookup=%v", err)
	}
}

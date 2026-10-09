package suggest

import (
	"context"
	"strings"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
)

type fakeStore struct {
	repos     []*pb.Repository
	snapshots map[string]*pb.Snapshot
	facts     map[string][]*pb.CodeFact
}

func (f *fakeStore) ListRepositories(context.Context) ([]*pb.Repository, error) {
	return f.repos, nil
}

func (f *fakeStore) Latest(_ context.Context, repositoryID string) (string, error) {
	for _, repo := range f.repos {
		if repo.GetId() == repositoryID {
			return repo.GetLatestSnapshotId(), nil
		}
	}
	return "", nil
}

func (f *fakeStore) Snapshot(_ context.Context, id string) (*pb.Snapshot, error) {
	return f.snapshots[id], nil
}

func (f *fakeStore) Facts(_ context.Context, snapshotID string, _ pb.FactKind, _, after string, limit int) ([]*pb.CodeFact, error) {
	all := f.facts[snapshotID]
	start := 0
	if after != "" {
		for i, fact := range all {
			if fact.GetId() == after {
				start = i + 1
				break
			}
		}
	}
	var out []*pb.CodeFact
	for i := start; i < len(all) && len(out) < limit; i++ {
		out = append(out, all[i])
	}
	return out, nil
}

func fact(id string, kind pb.FactKind, path, name, qualified, language string) *pb.CodeFact {
	return &pb.CodeFact{
		Id:            id,
		Kind:          kind,
		Anchor:        &pb.SourceAnchor{Path: path},
		Name:          name,
		QualifiedName: qualified,
		Language:      language,
	}
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		repos: []*pb.Repository{
			{Id: "repo-a", Root: "/repo/app", RemoteUrl: "https://github.com/acme/app", LatestSnapshotId: "snap-a", Name: "app"},
			{Id: "repo-b", Root: "/repo/infra", RemoteUrl: "https://github.com/acme/infra", LatestSnapshotId: "snap-b", Name: "infra"},
		},
		snapshots: map[string]*pb.Snapshot{
			"snap-a": {Id: "snap-a", Sources: []*pb.SourceFile{
				{Path: "internal/payment/service.go"},
				{Path: "internal/order/handler.go"},
				{Path: "web/src/billing/BillingPanel.tsx"},
			}},
			"snap-b": {Id: "snap-b", Sources: []*pb.SourceFile{
				{Path: "terraform/main.tf"},
			}},
		},
		facts: map[string][]*pb.CodeFact{
			"snap-a": {
				fact("f-file-1", pb.FactKind_FACT_KIND_FILE, "internal/payment/service.go", "service.go", "", "go"),
				fact("f-1", pb.FactKind_FACT_KIND_FUNCTION, "internal/payment/service.go", "ProcessPayment", "github.com/acme/app/internal/payment.ProcessPayment", "go"),
				fact("f-2", pb.FactKind_FACT_KIND_METHOD, "internal/order/handler.go", "Handle", "github.com/acme/app/internal/order.Handler.Handle", "go"),
				fact("f-3", pb.FactKind_FACT_KIND_CLASS, "web/src/billing/BillingPanel.tsx", "BillingPanel", "BillingPanel", "tsx"),
			},
			"snap-b": {
				fact("f-4", pb.FactKind_FACT_KIND_FUNCTION, "terraform/main.tf", "main", "main", "hcl"),
			},
		},
	}
}

func TestForSubjectRanksMatchingPath(t *testing.T) {
	sug := New(newFakeStore())
	candidates, err := sug.ForSubject(context.Background(), Subject{Name: "Billing Panel", Kind: "component", Technology: "React"}, Options{Limit: 5})
	if err != nil {
		t.Fatalf("ForSubject: %v", err)
	}
	if len(candidates) == 0 {
		t.Fatal("expected candidates")
	}
	top := candidates[0]
	if top.Kind == KindRepo {
		t.Fatalf("expected a file/symbol candidate, got repo: %+v", top)
	}
	if !strings.Contains(strings.ToLower(top.Path), "billingpanel") && !strings.Contains(strings.ToLower(top.Symbol), "billingpanel") {
		t.Fatalf("unexpected top candidate: %+v", top)
	}
}

func TestForSubjectPinsRepository(t *testing.T) {
	sug := New(newFakeStore())
	candidates, err := sug.ForSubject(context.Background(), Subject{Name: "main", RepositoryID: "repo-b"}, Options{Limit: 5})
	if err != nil {
		t.Fatalf("ForSubject: %v", err)
	}
	for _, candidate := range candidates {
		if candidate.RepositoryID != "repo-b" {
			t.Fatalf("candidate leaked from %q: %+v", candidate.RepositoryID, candidate)
		}
	}
}

func TestSearchFiles(t *testing.T) {
	sug := New(newFakeStore())
	candidates, err := sug.SearchFiles(context.Background(), "service.go", Options{})
	if err != nil {
		t.Fatalf("SearchFiles: %v", err)
	}
	if len(candidates) != 1 || candidates[0].Path != "internal/payment/service.go" {
		t.Fatalf("candidates = %+v", candidates)
	}
}

func TestSearchFilesSuffix(t *testing.T) {
	sug := New(newFakeStore())
	candidates, err := sug.SearchFiles(context.Background(), "billing/BillingPanel.tsx", Options{})
	if err != nil {
		t.Fatalf("SearchFiles: %v", err)
	}
	if len(candidates) != 1 || candidates[0].RepositoryID != "repo-a" {
		t.Fatalf("candidates = %+v", candidates)
	}
}

func TestSearchFilesMatchesFolder(t *testing.T) {
	sug := New(newFakeStore())
	ctx := context.Background()
	candidates, err := sug.SearchFiles(ctx, "billing", Options{})
	if err != nil {
		t.Fatalf("SearchFiles: %v", err)
	}
	found := false
	for _, candidate := range candidates {
		if candidate.Path == "web/src/billing/" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected folder candidate, got %+v", candidates)
	}
	if !sug.Verify(ctx, Candidate{Kind: KindFile, RepositoryID: "repo-a", Path: "web/src/billing/"}) {
		t.Fatal("folder candidate should verify via prefix")
	}
}

func TestSearchSymbols(t *testing.T) {
	sug := New(newFakeStore())
	candidates, err := sug.SearchSymbols(context.Background(), "ProcessPayment", Options{})
	if err != nil {
		t.Fatalf("SearchSymbols: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("candidates = %+v", candidates)
	}
	got := candidates[0]
	if got.Kind != KindSymbol || got.Symbol != "ProcessPayment" || got.NodeType != "function" || got.Path != "internal/payment/service.go" {
		t.Fatalf("candidate = %+v", got)
	}
}

func TestSearchRepositories(t *testing.T) {
	sug := New(newFakeStore())
	candidates, err := sug.SearchRepositories(context.Background(), "acme/infra", Options{})
	if err != nil {
		t.Fatalf("SearchRepositories: %v", err)
	}
	if len(candidates) != 1 || candidates[0].RepositoryID != "repo-b" {
		t.Fatalf("candidates = %+v", candidates)
	}
}

func TestVerify(t *testing.T) {
	sug := New(newFakeStore())
	ctx := context.Background()

	if !sug.Verify(ctx, Candidate{Kind: KindFile, RepositoryID: "repo-a", Path: "internal/payment/service.go"}) {
		t.Fatal("expected file to verify")
	}
	if sug.Verify(ctx, Candidate{Kind: KindFile, RepositoryID: "repo-a", Path: "missing.go"}) {
		t.Fatal("missing file must not verify")
	}
	if !sug.Verify(ctx, Candidate{Kind: KindSymbol, RepositoryID: "repo-a", Path: "internal/payment/service.go", Symbol: "ProcessPayment"}) {
		t.Fatal("expected symbol to verify")
	}
	if sug.Verify(ctx, Candidate{Kind: KindSymbol, RepositoryID: "repo-a", Path: "internal/payment/service.go", Symbol: "Nope"}) {
		t.Fatal("unknown symbol must not verify")
	}
	if sug.Verify(ctx, Candidate{Kind: KindSymbol, RepositoryID: "repo-a", Path: "internal/order/handler.go", Symbol: "ProcessPayment"}) {
		t.Fatal("symbol must not verify in the wrong file")
	}
}

func TestRepoNameFallback(t *testing.T) {
	store := newFakeStore()
	store.repos = []*pb.Repository{{Id: "repo-c", Root: "/repo/mystery", RemoteUrl: "https://github.com/acme/mystery", LatestSnapshotId: "snap-a"}}
	sug := New(store)
	if err := sug.Refresh(context.Background()); err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if name := repoInfo(sug.repos[0]).name; name != "mystery" {
		t.Fatalf("name = %q, want mystery", name)
	}
}

func TestTokenize(t *testing.T) {
	got := tokenize("ProcessPayment HTTP handler")
	want := map[string]bool{"process": true, "payment": true, "http": true, "handler": true}
	if len(got) != len(want) {
		t.Fatalf("tokens = %v", got)
	}
	for _, token := range got {
		if !want[token] {
			t.Fatalf("unexpected token %q in %v", token, got)
		}
	}
}

package store

import (
	"context"
	"fmt"
	"slices"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
)

func TestFactsLiteralPathPrefix(t *testing.T) {
	st, handle := openTestStore(t)
	defer func() { _ = handle.Close() }()
	testFactsLiteralPathPrefix(t, st)
}

// Run the same contract against SQLite and the PostgreSQL integration store.
func testFactsLiteralPathPrefix(t *testing.T, st *Store) {
	t.Helper()
	ctx := context.Background()
	for _, tc := range []struct {
		name, prefix, path, otherPath string
	}{
		{"plain", "src/api/", "src/api/file.go", "src/api2/file.go"},
		{"underscore", "src/a_b/", "src/a_b/file.go", "src/axb/file.go"},
		{"percent", "src/a%b/", "src/a%b/file.go", "src/axxb/file.go"},
		{"backslash", `src/a\b/`, `src/a\b/file.go`, "src/ab/file.go"},
		{"escape character", "src/a!b/", "src/a!b/file.go", "src/ab/file.go"},
		{"combined", `src/a!_%\b/`, `src/a!_%\b/file.go`, "src/a!xyb/file.go"},
		{"file prefix", "src/a_b/file", "src/a_b/file.go", "src/a_b/other.go"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snap := &pb.Snapshot{Id: "prefix-" + tc.name, RepositoryId: "prefix-repo", IngestionStatus: "complete"}
			g := graph.NewGraph(snap.RepositoryId, snap.Id)
			for i, path := range []string{tc.path, tc.otherPath} {
				for j, kind := range []pb.FactKind{pb.FactKind_FACT_KIND_FUNCTION, pb.FactKind_FACT_KIND_TYPE, pb.FactKind_FACT_KIND_FUNCTION} {
					id := fmt.Sprintf("%s-%d-%d", snap.Id, i, j)
					g.Facts[id] = &pb.CodeFact{Id: id, RepositoryId: snap.RepositoryId, SnapshotId: snap.Id, Kind: kind, Anchor: &pb.SourceAnchor{Path: path}}
				}
			}
			if err := st.Publish(ctx, "/prefix-repo", snap, g); err != nil {
				t.Fatal(err)
			}
			assertIDs := func(prefix string, kind pb.FactKind, want []string) {
				t.Helper()
				var got []string
				after := ""
				for page := 0; page <= len(g.Facts); page++ {
					facts, err := st.Facts(ctx, snap.Id, kind, prefix, after, 1)
					if err != nil {
						t.Fatal(err)
					}
					if len(facts) == 0 {
						break
					}
					if len(facts) != 1 || facts[0].Id <= after || facts[0].SnapshotId != snap.Id {
						t.Fatalf("invalid page after %q: %v", after, facts)
					}
					after = facts[0].Id
					got = append(got, after)
				}
				if !slices.Equal(got, want) {
					t.Fatalf("prefix %q kind %v: IDs = %v, want %v", prefix, kind, got, want)
				}
			}
			ids := []string{snap.Id + "-0-0", snap.Id + "-0-1", snap.Id + "-0-2"}
			assertIDs(tc.prefix, pb.FactKind_FACT_KIND_UNSPECIFIED, ids)
			assertIDs(tc.prefix, pb.FactKind_FACT_KIND_FUNCTION, []string{ids[0], ids[2]})
			assertIDs(tc.prefix+"missing", pb.FactKind_FACT_KIND_UNSPECIFIED, nil)
			assertIDs("", pb.FactKind_FACT_KIND_UNSPECIFIED, append(ids, snap.Id+"-1-0", snap.Id+"-1-1", snap.Id+"-1-2"))
		})
	}
}

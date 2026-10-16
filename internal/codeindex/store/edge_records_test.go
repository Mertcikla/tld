package store

import (
	"context"
	"math"
	"strings"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/codeindex/graph"
	"google.golang.org/protobuf/proto"
)

func TestStoredEdgeKey(t *testing.T) {
	key := graph.EdgeLogicalKey(pb.EdgeKind_EDGE_KIND_CALLS, "a", "b")
	if raw, err := graph.DecodeEdgeLogicalKey(key); err != nil || len(raw) != 32 {
		t.Fatalf("valid key failed: %x %v", raw, err)
	}
	for _, key := range []string{"", "legacy", strings.Repeat("z", 64), strings.ToUpper(key), key[:63], key + "a"} {
		if _, err := graph.DecodeEdgeLogicalKey(key); err == nil {
			t.Fatalf("accepted invalid key %q", key)
		}
	}
}

func TestCompactEdgeQueries(t *testing.T) {
	st, handle := openTestStore(t)
	defer func() { _ = handle.Close() }()
	testCompactEdgeQueries(t, st)
}

func TestCompactEdgePublishRejectsInvalidData(t *testing.T) {
	st, handle := openTestStore(t)
	defer func() { _ = handle.Close() }()
	ctx := context.Background()
	for _, e := range []*pb.EdgeFact{
		{LogicalKey: "legacy"},
		{LogicalKey: graph.EdgeLogicalKey(0, "a", "b"), Weight: math.NaN()},
		{LogicalKey: graph.EdgeLogicalKey(0, "a", "b"), Weight: math.Inf(1)},
	} {
		g := graph.NewGraph("10", "100")
		e.Id, e.RepositoryId, e.SnapshotId = "101", "10", "100"
		g.EdgeFacts[e.Id] = e
		if err := st.Publish(ctx, "/invalid", &pb.Snapshot{Id: "100", RepositoryId: "10"}, g); err == nil {
			t.Fatal("published invalid key")
		}
		if _, err := st.Snapshot(ctx, "100"); err == nil {
			t.Fatal("failed publication retained a partial snapshot")
		}
	}
}

func testCompactEdgeQueries(t *testing.T, st *Store) {
	t.Helper()
	ctx := context.Background()
	rows, err := st.bun.QueryContext(ctx, `SELECT * FROM codeindex_edges WHERE 1=0`)
	if err != nil {
		t.Fatal(err)
	}
	columns, err := rows.Columns()
	_ = rows.Close()
	if err != nil {
		t.Fatal(err)
	}
	for _, column := range columns {
		if column == "details" || column == "anchor_json" || column == "evidence_json" {
			t.Fatalf("fresh edge schema retained provenance column %q", column)
		}
	}
	key := graph.EdgeLogicalKey(pb.EdgeKind_EDGE_KIND_CALLS, "a", "external")
	var expected []*pb.EdgeFact
	for i, snapID := range []string{"100", "200"} {
		g := graph.NewGraph("10", snapID)
		e := &pb.EdgeFact{Id: []string{"101", "201"}[i], RepositoryId: "10", SnapshotId: snapID, Kind: pb.EdgeKind_EDGE_KIND_CALLS, LogicalKey: key, TargetSymbolKey: "external", Weight: 2.5}
		g.EdgeFacts[e.Id] = e
		if err := st.Publish(ctx, "/compact-edge", &pb.Snapshot{Id: snapID, RepositoryId: "10", CreatedUnix: int64(i)}, g); err != nil {
			t.Fatal(err)
		}
		expected = append(expected, e)
	}
	got, err := st.EdgeFact(ctx, "101")
	if err != nil || !proto.Equal(got, expected[0]) {
		t.Fatalf("single edge changed: %v %v", got, err)
	}
	versions, err := st.EdgeVersions(ctx, key)
	if err != nil || len(versions) != 2 || !proto.Equal(versions[0], expected[1]) || !proto.Equal(versions[1], expected[0]) {
		t.Fatalf("versions = %v, err = %v", versions, err)
	}
	for _, tc := range []struct {
		kind  pb.EdgeKind
		key   string
		after string
		count int
	}{
		{pb.EdgeKind_EDGE_KIND_CALLS, key, "", 1},
		{pb.EdgeKind_EDGE_KIND_REFERENCES, key, "", 0},
		{pb.EdgeKind_EDGE_KIND_CALLS, graph.EdgeLogicalKey(pb.EdgeKind_EDGE_KIND_CALLS, "other", "external"), "", 0},
		{pb.EdgeKind_EDGE_KIND_CALLS, key, "201", 0},
	} {
		got, err := st.EdgeFacts(ctx, "200", tc.kind, tc.key, tc.after, 1)
		if err != nil || len(got) != tc.count {
			t.Fatalf("filtered edges = %v, err = %v", got, err)
		}
	}
	if _, err := st.EdgeVersions(ctx, "legacy"); err == nil {
		t.Fatal("accepted malformed version key")
	}
	if _, err := st.EdgeFacts(ctx, "200", 0, "legacy", "", 1); err == nil {
		t.Fatal("accepted malformed filter key")
	}
	if err := st.DeleteSnapshot(ctx, "100"); err != nil {
		t.Fatal(err)
	}
	versions, err = st.EdgeVersions(ctx, key)
	if err != nil || len(versions) != 1 || versions[0].Id != "101" {
		t.Fatalf("deletion changed remaining versions: %v %v", versions, err)
	}
}

package store

import (
	"bytes"
	"context"
	"encoding/binary"
	"strings"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/codeindex/graph"
	"google.golang.org/protobuf/proto"
)

func TestEdgeDetailsRoundTrip(t *testing.T) {
	a := &pb.SourceAnchor{Path: "src/世界.go", StartByte: 10, EndByte: 15, StartLine: 2, EndLine: 3, StartColumn: 4, EndColumn: 5, SourceHash: strings.Repeat("a", 64)}
	a.ProtoReflect().SetUnknown([]byte{0xa0, 0x06, 0x01})
	e := &pb.Evidence{Producer: "scip", Version: "1", OriginalId: "日本語", OriginalRange: "10:15", Derivation: "reference", Anchor: proto.Clone(a).(*pb.SourceAnchor)}
	e.ProtoReflect().SetUnknown([]byte{0xa0, 0x06, 0x02})
	for _, tc := range []struct {
		name     string
		anchor   *pb.SourceAnchor
		evidence []*pb.Evidence
	}{
		{"nil", nil, nil},
		{"empty", &pb.SourceAnchor{}, []*pb.Evidence{}},
		{"nil entries", nil, []*pb.Evidence{nil, {}, nil}},
		{"shared and distinct", a, []*pb.Evidence{e, nil, {}, {Anchor: &pb.SourceAnchor{Path: "another.go"}}, {Anchor: &pb.SourceAnchor{}}}},
		{"evidence only", nil, []*pb.Evidence{e}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := &pb.EdgeFact{Anchor: tc.anchor, Evidence: tc.evidence}
			copyBefore := proto.Clone(before)
			raw, err := marshalEdgeDetails(tc.anchor, tc.evidence)
			if err != nil {
				t.Fatal(err)
			}
			afterAnchor, afterEvidence, err := unmarshalEdgeDetails(raw)
			if err != nil {
				t.Fatal(err)
			}
			if !proto.Equal(before, &pb.EdgeFact{Anchor: afterAnchor, Evidence: afterEvidence}) || (tc.evidence == nil) != (afterEvidence == nil) {
				t.Fatalf("round trip changed details: %v %v", afterAnchor, afterEvidence)
			}
			for i := range tc.evidence {
				if (tc.evidence[i] == nil) != (afterEvidence[i] == nil) {
					t.Fatalf("nil entry %d changed", i)
				}
			}
			if !proto.Equal(before, copyBefore) {
				t.Fatal("encoding mutated input")
			}
			encodedAgain, err := marshalEdgeDetails(afterAnchor, afterEvidence)
			if err != nil || !bytes.Equal(raw, encodedAgain) {
				t.Fatal("encoding is not deterministic")
			}
			if afterAnchor != nil && len(afterEvidence) > 0 && afterEvidence[0] != nil && proto.Equal(afterAnchor, afterEvidence[0].Anchor) {
				afterEvidence[0].Anchor.Path = "changed"
				if afterAnchor.Path == "changed" {
					t.Fatal("decoded shared anchors alias mutable messages")
				}
			}
		})
	}
}

func TestEdgeDetailsRejectCorruption(t *testing.T) {
	raw, err := marshalEdgeDetails(&pb.SourceAnchor{Path: "file"}, []*pb.Evidence{{Producer: "scip"}})
	if err != nil {
		t.Fatal(err)
	}
	for i := range raw {
		if _, _, err := unmarshalEdgeDetails(raw[:i]); err == nil {
			t.Fatalf("accepted truncated payload at %d", i)
		}
	}
	badCount := binary.AppendUvarint([]byte{1, 0}, ^uint64(0))
	for _, data := range [][]byte{
		{2, 0, 0}, {1, 0, 0, 1}, {1, 0, 2, 4}, {1, 0, 2, evidenceSharedAnchor, 0},
		{1, 0, 2, evidenceFullAnchor, 0}, {1, 2, 0xff, 0}, {1, 0, 2, evidenceAbsentAnchor, 1, 0xff}, badCount,
	} {
		if _, _, err := unmarshalEdgeDetails(data); err == nil {
			t.Fatalf("accepted malformed payload %x", data)
		}
	}
}

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
		{LogicalKey: graph.EdgeLogicalKey(pb.EdgeKind_EDGE_KIND_CALLS, "a", "b"), Evidence: []*pb.Evidence{{Producer: string([]byte{0xff})}}},
	} {
		g := graph.NewGraph("10", "100")
		e.Id, e.RepositoryId, e.SnapshotId = "101", "10", "100"
		g.EdgeFacts[e.Id] = e
		if err := st.Publish(ctx, "/invalid", &pb.Snapshot{Id: "100", RepositoryId: "10"}, g); err == nil {
			t.Fatal("published invalid key or provenance")
		}
		if _, err := st.Snapshot(ctx, "100"); err == nil {
			t.Fatal("failed publication retained a partial snapshot")
		}
	}
}

func testCompactEdgeQueries(t *testing.T, st *Store) {
	t.Helper()
	ctx := context.Background()
	key := graph.EdgeLogicalKey(pb.EdgeKind_EDGE_KIND_CALLS, "a", "external")
	anchor := &pb.SourceAnchor{Path: "a.go", StartByte: 1, EndByte: 2}
	var expected []*pb.EdgeFact
	for i, snapID := range []string{"100", "200"} {
		g := graph.NewGraph("10", snapID)
		e := &pb.EdgeFact{Id: []string{"101", "201"}[i], RepositoryId: "10", SnapshotId: snapID, Kind: pb.EdgeKind_EDGE_KIND_CALLS, LogicalKey: key, Anchor: anchor, Evidence: []*pb.Evidence{{Producer: "scip", Anchor: anchor}}, TargetSymbolKey: "external", Weight: 2.5}
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
	if err != nil || len(versions) != 1 || versions[0].Id != "201" {
		t.Fatalf("deletion changed remaining versions: %v %v", versions, err)
	}
	if _, err := st.bun.ExecContext(ctx, `UPDATE codeindex_edges SET details = ? WHERE id = ?`, []byte{2}, "201"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.EdgeFact(ctx, "201"); err == nil {
		t.Fatal("read silently discarded corrupted details")
	}
}

func BenchmarkEdgeDetails(b *testing.B) {
	a := &pb.SourceAnchor{Path: "internal/codeindex/store/edge_details.go", StartByte: 1024, EndByte: 1032, StartLine: 42, EndLine: 42, SourceHash: strings.Repeat("a", 64)}
	evidence := []*pb.Evidence{{Producer: "tree-sitter+scip", Version: "0.1.26", OriginalId: "scip-go gomod github.com/mertcikla/tld/v2 . Store#Publish().", Anchor: a, Derivation: "call expression with resolved reference"}}
	raw, err := marshalEdgeDetails(a, evidence)
	if err != nil {
		b.Fatal(err)
	}
	b.Run("encode", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, err := marshalEdgeDetails(a, evidence); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("decode", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if _, _, err := unmarshalEdgeDetails(raw); err != nil {
				b.Fatal(err)
			}
		}
	})
}

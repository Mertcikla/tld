package store

import (
	"context"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/codeindex/graph"
	"google.golang.org/protobuf/proto"
)

func TestIncrementalPublicationRemapsChangedParents(t *testing.T) {
	idx, handle := openTestStore(t)
	defer func() { _ = handle.Close() }()
	ctx := context.Background()
	base := graph.NewGraph("repo", graph.ID("parent-base"))
	src := &graph.Source{Path: "a.go", Language: "go", Text: []byte("package p\ntype A struct{}\n"), Hash: "source"}
	base.Sources[src.Path] = src
	parent := base.AddFact(pb.FactKind_FACT_KIND_TYPE, "A", "go", src.Anchor(10, 25), "type A struct{}", "", nil)
	child := base.AddFact(pb.FactKind_FACT_KIND_METHOD, "Field", "go", src.Anchor(15, 20), "Field", "", nil)
	child.ParentFactId = parent.Id
	first := &pb.Snapshot{Id: base.SnapshotID, RepositoryId: base.RepositoryID}
	if err := idx.Publish(ctx, t.TempDir(), first, base); err != nil {
		t.Fatal(err)
	}
	originalParent, originalChild := parent.Id, child.Id
	next := graph.NewGraph(base.RepositoryID, graph.ID("parent-next"))
	next.Sources[src.Path] = src
	for id, f := range base.Facts {
		next.Facts[id] = proto.Clone(f).(*pb.CodeFact)
	}
	next.Facts[parent.Id].Documentation = "changed semantic metadata"
	second := &pb.Snapshot{Id: next.SnapshotID, RepositoryId: next.RepositoryID}
	if err := idx.PublishIncremental(ctx, t.TempDir(), second, next, base); err != nil {
		t.Fatal(err)
	}
	var newParent, newChild *pb.CodeFact
	for _, f := range next.Facts {
		if f.Name == "A" {
			newParent = f
		} else {
			newChild = f
		}
	}
	if newParent.Id == originalParent || newChild.Id == originalChild || newChild.ParentFactId != newParent.Id {
		t.Fatalf("changed parent identities: %v %v", newParent, newChild)
	}
	old, err := idx.LoadGraph(ctx, first.Id)
	if err != nil {
		t.Fatal(err)
	}
	if old.Facts[originalParent].Documentation != "" || old.Facts[originalChild].ParentFactId != originalParent {
		t.Fatal("earlier snapshot changed")
	}
}

func TestIncrementalPublicationKeepsRepeatedObservations(t *testing.T) {
	idx, handle := openTestStore(t)
	defer func() { _ = handle.Close() }()
	ctx := context.Background()
	base := graph.NewGraph("repo", graph.ID("observations-base"))
	src := &graph.Source{Path: "a.go", Language: "go", Text: []byte("package p\nfunc A() {B();B()}\nfunc B() {}\n"), Hash: "source"}
	base.Sources[src.Path] = src
	a := base.AddFact(pb.FactKind_FACT_KIND_FUNCTION, "A", "go", src.Anchor(10, 27), "func A() {B();B()}", "", nil)
	b := base.AddFact(pb.FactKind_FACT_KIND_FUNCTION, "B", "go", src.Anchor(28, 39), "func B() {}", "", nil)
	base.AddEdgeFact(pb.EdgeKind_EDGE_KIND_CALLS, a.Id, b.Id, "", src.Anchor(20, 21))
	first := &pb.Snapshot{Id: base.SnapshotID, RepositoryId: base.RepositoryID}
	if err := idx.Publish(ctx, t.TempDir(), first, base); err != nil {
		t.Fatal(err)
	}
	next := graph.NewGraph(base.RepositoryID, graph.ID("observations-next"))
	next.Sources[src.Path] = src
	for id, f := range base.Facts {
		next.Facts[id] = proto.Clone(f).(*pb.CodeFact)
	}
	for id, e := range base.EdgeFacts {
		next.EdgeFacts[id] = proto.Clone(e).(*pb.EdgeFact)
	}
	next.AddEdgeFact(pb.EdgeKind_EDGE_KIND_CALLS, a.Id, b.Id, "", src.Anchor(24, 25))
	second := &pb.Snapshot{Id: next.SnapshotID, RepositoryId: next.RepositoryID}
	if err := idx.PublishIncremental(ctx, t.TempDir(), second, next, base); err != nil {
		t.Fatal(err)
	}
	stored, err := idx.LoadGraph(ctx, second.Id)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored.EdgeFacts) != 2 || len(next.EdgeFacts) != 2 {
		t.Fatal("new observation collapsed into a carried immutable edge")
	}
}

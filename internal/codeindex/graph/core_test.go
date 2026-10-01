package graph

import (
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
)

func TestAdoptRekeysAcrossSnapshots(t *testing.T) {
	src := &Source{Path: "a.go", Text: []byte("package a\nfunc A(){}\n"), Hash: "hash"}
	base := NewGraph("repo", "snap-a")
	base.Sources[src.Path] = src
	parent := base.AddFact(pb.FactKind_FACT_KIND_TYPE, "T", "go", src.Anchor(9, 10), "T", "T", nil)
	child := base.AddFact(pb.FactKind_FACT_KIND_FUNCTION, "A", "go", src.Anchor(16, 25), "func A(){}", "func A()", &pb.Evidence{Producer: "tree-sitter"})
	child.ParentFactId = parent.Id
	child.SymbolKey = "sym-a"
	base.AddChunk(child.Id, src.Anchor(16, 25), "func A(){}", "func A()", 0, 1)
	edge := base.AddEdgeFact(pb.EdgeKind_EDGE_KIND_CALLS, child.Id, parent.Id, "", src.Anchor(20, 21), &pb.Evidence{Producer: "scip"})

	next := NewGraph("repo", "snap-b")
	adoptedParent := next.AdoptFact(parent)
	adoptedChild := next.AdoptFact(child)
	if adoptedChild.Id == child.Id || adoptedChild.Id != next.AdoptFact(child).Id {
		t.Fatalf("adopt is not snapshot-scoped or stable: %q", adoptedChild.Id)
	}
	if adoptedChild.SymbolKey != "sym-a" || adoptedChild.Code != child.Code || len(adoptedChild.Evidence) != 1 {
		t.Fatalf("adopted fact lost data: %+v", adoptedChild)
	}
	adoptedChild.ParentFactId = adoptedParent.Id
	for _, c := range base.Chunks {
		next.AdoptChunk(c, adoptedChild.Id)
	}
	if len(next.Chunks) != 1 {
		t.Fatalf("chunks = %d", len(next.Chunks))
	}
	adoptedEdge := next.AdoptEdgeFact(edge, adoptedChild.Id, adoptedParent.Id, "")
	if adoptedEdge == nil || adoptedEdge.FromFactId != adoptedChild.Id || adoptedEdge.ToFactId != adoptedParent.Id {
		t.Fatalf("adopted edge %+v", adoptedEdge)
	}
	if adoptedEdge.LogicalKey == "" || len(adoptedEdge.Evidence) != 1 {
		t.Fatalf("adopted edge lost identity/evidence: %+v", adoptedEdge)
	}
	if next.AdoptEdgeFact(edge, "", adoptedParent.Id, "") != nil {
		t.Fatal("edge without a source must be dropped")
	}
}

func TestAdoptInfraFactPreservesMetadata(t *testing.T) {
	src := &Source{Path: "Dockerfile", Text: []byte("FROM golang:1.26\n"), Hash: "hash"}
	base := NewGraph("repo", "snap-a")
	base.Sources[src.Path] = src
	f := base.AddInfraFact(pb.FactKind_FACT_KIND_DEPENDENCY, "root", "golang:1.26", "", "local", src.Anchor(0, 4), "FROM golang:1.26")

	next := NewGraph("repo", "snap-b")
	adopted := next.AdoptFact(f)
	if adopted == nil {
		t.Fatal("infra fact was not adopted")
	}
	if adopted.QualifiedName != "root" || adopted.Name != "golang:1.26" {
		t.Fatalf("adopted infra fact lost metadata: %+v", adopted)
	}
	if len(adopted.Evidence) != 1 || adopted.Evidence[0].Producer != "local" {
		t.Fatalf("adopted infra fact lost producer: %+v", adopted.Evidence)
	}
	if adopted.LogicalKey != f.LogicalKey {
		t.Fatalf("logical key changed: %q != %q", adopted.LogicalKey, f.LogicalKey)
	}
}

func TestOffsetEncodingsAndCRLF(t *testing.T) {
	src := []byte("a🚀z\r\nβ\n")
	cases := []struct {
		line, char int
		encoding   string
		want       int
	}{{0, 1, "utf-8", 1}, {0, 1, "utf-16", 1}, {0, 3, "utf-16", 5}, {0, 2, "utf-32", 5}, {1, 1, "utf-16", 10}}
	for _, c := range cases {
		got, e := Offset(src, c.line, c.char, c.encoding)
		if e != nil || got != c.want {
			t.Errorf("offset(%d,%d,%s)=%d,%v want %d", c.line, c.char, c.encoding, got, e, c.want)
		}
	}
	if _, e := Offset(src, 0, 2, "utf-16"); e == nil {
		t.Fatal("UTF-16 surrogate split accepted")
	}
}
func TestAnchorHalfOpen(t *testing.T) {
	s := &Source{Path: "file.go", Text: []byte("x\ny"), Hash: "hash"}
	a := s.Anchor(2, 3)
	if a.StartLine != 1 || a.EndLine != 1 || a.StartColumn != 0 || a.EndColumn != 1 {
		t.Fatalf("bad anchor: %+v", a)
	}
}

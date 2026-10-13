package store

import (
	"fmt"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
)

// C++ specializations and macro expansions can share a SCIP logical key.
// Input order must neither hide a changed occurrence nor pick an unrelated
// occurrence from an otherwise unchanged file.
func TestDiffFactsRepeatedLogicalKey(t *testing.T) {
	fact := func(id, body string, line uint32) *pb.CodeFact {
		return &pb.CodeFact{Id: id, LogicalKey: "macro", Name: "FMT_THROW", BodyHash: body,
			Anchor: &pb.SourceAnchor{Path: "os.cc", StartLine: line, StartByte: line}}
	}
	before := []*pb.CodeFact{fact("b1", "one", 10), fact("b2", "two", 20), fact("b3", "three", 30)}
	after := []*pb.CodeFact{fact("h1", "one", 11), fact("h2", "changed", 21), fact("h3", "three", 31), fact("h4", "added", 41)}
	for i := range 6 {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			delta := diffFacts(before, after)
			if len(delta.Modified) != 1 || delta.Modified[0].Id != "h2" || len(delta.Added) != 1 || delta.Added[0].Id != "h4" || len(delta.Removed) != 0 {
				t.Fatalf("lost or mispaired repeated declaration: %v", delta)
			}
			before[0], before[1] = before[1], before[0]
			after[0], after[2] = after[2], after[0]
		})
	}
	if delta := diffFacts(before, []*pb.CodeFact{fact("h1", "one", 11), fact("h2", "two", 21), fact("h3", "three", 31)}); len(delta.Modified)+len(delta.Added)+len(delta.Removed) != 0 {
		t.Fatalf("unchanged repeated declarations reported as changed: %v", delta)
	}
}

// A SCIP package version is not part of a declaration's identity. Snapshotting
// two commits embeds a different version (e.g. the git revision) into every
// SymbolKey/QualifiedName, so comparing those fields marked every symbol in a
// changed file as modified even when its code was untouched.
func TestDiffFactsIgnoresSCIPPackageVersion(t *testing.T) {
	base := &pb.CodeFact{
		Id:            "base",
		Kind:          pb.FactKind_FACT_KIND_METHOD,
		Name:          "Run",
		Language:      "go",
		Signature:     "func (s *Store) Run() error",
		BodyHash:      "base-body-hash",
		LogicalKey:    "fact|store.go|FACT_KIND_METHOD|Run|scope|*Store|0",
		SymbolKey:     "scip-go gomod github.com/x/y . `pkg`/Store#Run().",
		QualifiedName: "scip-go gomod github.com/x/y . `pkg`/Store#Run().",
		Anchor:        &pb.SourceAnchor{Path: "store.go"},
	}
	head := &pb.CodeFact{
		Id:            "head",
		Kind:          base.Kind,
		Name:          base.Name,
		Language:      base.Language,
		Signature:     base.Signature,
		BodyHash:      base.BodyHash,
		LogicalKey:    base.LogicalKey,
		SymbolKey:     "scip-go gomod github.com/x/y 15cd83bf4f54 `pkg`/Store#Run().",
		QualifiedName: "scip-go gomod github.com/x/y 15cd83bf4f54 `pkg`/Store#Run().",
		Anchor:        &pb.SourceAnchor{Path: "store.go"},
	}

	delta := diffFacts([]*pb.CodeFact{base}, []*pb.CodeFact{head})
	if len(delta.Modified) != 0 {
		t.Fatalf("version-only symbol key change reported as modified: %+v", delta.Modified)
	}
	if len(delta.Added) != 0 || len(delta.Removed) != 0 {
		t.Fatalf("unexpected add/remove: added=%d removed=%d", len(delta.Added), len(delta.Removed))
	}

	head.BodyHash = "changed-body-hash"
	if delta := diffFacts([]*pb.CodeFact{base}, []*pb.CodeFact{head}); len(delta.Modified) != 1 {
		t.Fatalf("real body change not reported as modified: %+v", delta.Modified)
	}
}

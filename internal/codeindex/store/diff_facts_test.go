package store

import (
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
)

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
		Code:          "func (s *Store) Run() error { return nil }",
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
		Code:          base.Code,
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

	head.Code = "func (s *Store) Run() error { return err }"
	if delta := diffFacts([]*pb.CodeFact{base}, []*pb.CodeFact{head}); len(delta.Modified) != 1 {
		t.Fatalf("real body change not reported as modified: %+v", delta.Modified)
	}
}

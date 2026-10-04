package maprun

import (
	"testing"

	codeindexv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	cgraph "github.com/mertcikla/tld/v2/internal/codeindex/graph"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
)

func TestBuildFileInputsUsesStableKeys(t *testing.T) {
	const keyA = "fact|a.go|FACT_KIND_FILE|a.go"
	const keyB = "fact|b.go|FACT_KIND_FILE|b.go"
	facts := []*codeindexv1.CodeFact{
		{Id: "snap-1-file-a", LogicalKey: keyA, Kind: codeindexv1.FactKind_FACT_KIND_FILE, Name: "a.go", Anchor: &codeindexv1.SourceAnchor{Path: "a.go"}},
		{Id: "snap-1-file-b", LogicalKey: keyB, Kind: codeindexv1.FactKind_FACT_KIND_FILE, Name: "b.go", Anchor: &codeindexv1.SourceAnchor{Path: "b.go"}},
	}
	edges := []cstore.FileEdge{{FromFactID: "snap-1-file-a", ToFactID: "snap-1-file-b", Weight: 2}}
	imports := []cstore.FileImport{{FileFactID: "snap-1-file-a", Import: "fmt"}}

	files, mapEdges, mapImports := buildFileInputs(facts, edges, imports)

	if len(files) != 2 || files[0].ID != keyA || files[1].ID != keyB {
		t.Fatalf("files = %+v, want logical keys", files)
	}
	if len(mapEdges) != 1 || mapEdges[0].FromFactID != keyA || mapEdges[0].ToFactID != keyB {
		t.Fatalf("edges = %+v, want logical keys", mapEdges)
	}
	if len(mapImports) != 1 || mapImports[0].FileFactID != keyA {
		t.Fatalf("imports = %+v, want logical keys", mapImports)
	}
}

func TestStableFactKeyFallsBackToStructuralKey(t *testing.T) {
	fact := &codeindexv1.CodeFact{
		Id:     "snap-1-file-a",
		Kind:   codeindexv1.FactKind_FACT_KIND_FILE,
		Name:   "a.go",
		Anchor: &codeindexv1.SourceAnchor{Path: "a.go"},
	}
	want := cgraph.LogicalFactKey(fact.GetKind(), "a.go", "a.go")
	if got := stableFactKey(fact); got != want {
		t.Fatalf("stableFactKey = %q, want %q", got, want)
	}
}

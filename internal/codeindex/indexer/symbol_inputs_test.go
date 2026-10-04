package indexer

import (
	"os"
	"path/filepath"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
)

func TestSymbolInputsInvalidatesSwappedSources(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module sample\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	projects := []*pb.Project{{Root: ".", Language: "go", ConfigPath: "go.mod"}}
	sources := map[string]*graph.Source{
		"a.go": {Language: "go", Hash: "hash-a"},
		"b.go": {Language: "go", Hash: "hash-b"},
	}
	before, err := symbolInputs(root, projects, sources, "config")
	if err != nil {
		t.Fatal(err)
	}
	for range 5 {
		same, err := symbolInputs(root, projects, sources, "config")
		if err != nil {
			t.Fatal(err)
		}
		if same["go|."] != before["go|."] {
			t.Fatal("fingerprint depends on map iteration order")
		}
	}
	sources["a.go"].Hash, sources["b.go"].Hash = sources["b.go"].Hash, sources["a.go"].Hash
	after, err := symbolInputs(root, projects, sources, "config")
	if err != nil {
		t.Fatal(err)
	}
	if before["go|."] == after["go|."] {
		t.Fatal("swapping source contents must invalidate the project artifact")
	}
}

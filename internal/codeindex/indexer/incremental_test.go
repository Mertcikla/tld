package indexer

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/config"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
	scip "github.com/scip-code/scip/bindings/go/scip"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func normalizeGraph(t *testing.T, g *graph.Graph) []string {
	t.Helper()
	out := []string{}
	key := func(id string) string {
		if fact := g.Facts[id]; fact != nil {
			return fact.LogicalKey
		}
		return id
	}
	encode := func(msg proto.Message) {
		raw, err := protojson.Marshal(msg)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, string(raw))
	}
	for _, fact := range g.Facts {
		f := proto.Clone(fact).(*pb.CodeFact)
		f.Id = ""
		f.SnapshotId = ""
		f.RepositoryId = ""
		f.ParentFactId = key(f.ParentFactId)
		encode(f)
	}
	for _, chunk := range g.Chunks {
		c := proto.Clone(chunk).(*pb.Chunk)
		c.Id = ""
		c.SnapshotId = ""
		c.FactId = key(c.FactId)
		encode(c)
	}
	for _, edge := range g.EdgeFacts {
		e := proto.Clone(edge).(*pb.EdgeFact)
		e.Id = ""
		e.RepositoryId = ""
		e.SnapshotId = ""
		e.FromFactId = key(e.FromFactId)
		e.ToFactId = key(e.ToFactId)
		encode(e)
	}
	sort.Strings(out)
	return out
}
func baseFor(snapshot *pb.Snapshot, g *graph.Graph) *IncrementalBase {
	sources := map[string]string{}
	for path, src := range g.Sources {
		sources[path] = src.Hash
	}
	return &IncrementalBase{Snapshot: snapshot, Graph: g, Sources: sources}
}
func TestIncrementalMatchesFreshAndRetainsChunks(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	put := func(name, code string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, name), []byte(code), 0600); err != nil {
			t.Fatal(err)
		}
	}
	put("a.go", "package a\nfunc Changed() int { return 1 }\n")
	put("b.go", "package a\nfunc Stable() int { return 3 }\n")
	put("compose.yaml", "services:\n  api:\n    image: api:v1\n")
	pipeline := Pipeline{Config: config.Default()}
	req := &pb.IndexRequest{Directory: root}
	snap, g, err := pipeline.Build(ctx, req, nil)
	if err != nil {
		t.Fatal(err)
	}
	stableCache := g.Sources["b.go"].SyntaxCache
	for _, code := range []string{"package a\nfunc Changed() int { return 2 }\n", "package a\nfunc Renamed() int { return 4 }\n"} {
		put("a.go", code)
		next, incremental, reused, err := pipeline.BuildIncremental(ctx, req, nil, baseFor(snap, g))
		if err != nil || reused {
			t.Fatalf("incremental: %v reused=%v", err, reused)
		}
		_, fresh, err := pipeline.Build(ctx, req, nil)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(normalizeGraph(t, incremental), normalizeGraph(t, fresh)) {
			t.Fatalf("incremental differs from fresh; chunks=%d/%d", len(incremental.Chunks), len(fresh.Chunks))
		}
		if incremental.Sources["b.go"].SyntaxCache != stableCache {
			t.Fatal("unchanged syntax was reparsed")
		}
		snap, g = next, incremental
	}
	if err := os.Remove(filepath.Join(root, "a.go")); err != nil {
		t.Fatal(err)
	}
	put("compose.yaml", "services:\n  db:\n    image: postgres:17\n")
	_, incremental, _, err := pipeline.BuildIncremental(ctx, req, nil, baseFor(snap, g))
	if err != nil {
		t.Fatal(err)
	}
	_, fresh, err := pipeline.Build(ctx, req, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(normalizeGraph(t, incremental), normalizeGraph(t, fresh)) {
		t.Fatal("deletion or infrastructure differs from fresh")
	}
}
func TestIncrementalRebindsUnchangedCallers(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	artifact := filepath.Join(t.TempDir(), "index.scip")
	caller := "export function Caller() { return Target() }\n"
	symbol := "scip-typescript npm fixture 1 Target()."
	write := func(define bool) {
		t.Helper()
		target := "export function Target() { return 1 }\n"
		if !define {
			target = "export function Other() { return 2 }\n"
		}
		files := map[string]string{"tsconfig.json": "{}", "caller.ts": caller, "target.ts": target}
		for name, code := range files {
			if err := os.WriteFile(filepath.Join(root, name), []byte(code), 0600); err != nil {
				t.Fatal(err)
			}
		}
		reference := strings.Index(caller, "Target")
		docs := []*scip.Document{{RelativePath: "caller.ts", Text: caller, PositionEncoding: scip.PositionEncoding_UTF8CodeUnitOffsetFromLineStart, Occurrences: []*scip.Occurrence{{Range: []int32{0, int32(reference), int32(reference + 6)}, Symbol: symbol}}}}
		if define {
			start := strings.Index(target, "Target")
			docs = append(docs, &scip.Document{RelativePath: "target.ts", Text: target, PositionEncoding: scip.PositionEncoding_UTF8CodeUnitOffsetFromLineStart, Occurrences: []*scip.Occurrence{{Range: []int32{0, int32(start), int32(start + 6)}, Symbol: symbol, SymbolRoles: int32(scip.SymbolRole_Definition)}}})
		}
		raw, err := proto.Marshal(&scip.Index{Metadata: &scip.Metadata{ToolInfo: &scip.ToolInfo{Name: "fixture", Version: "1"}}, Documents: docs})
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(artifact, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(true)
	pipeline := Pipeline{Config: config.Default()}
	req := &pb.IndexRequest{Directory: root, ScipArtifacts: map[string]string{".": artifact}}
	snap, g, err := pipeline.Build(ctx, req, nil)
	if err != nil {
		t.Fatal(err)
	}
	write(false)
	_, incremental, _, err := pipeline.BuildIncremental(ctx, req, nil, baseFor(snap, g))
	if err != nil {
		t.Fatal(err)
	}
	_, fresh, err := pipeline.Build(ctx, req, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(normalizeGraph(t, incremental), normalizeGraph(t, fresh)) {
		t.Fatal("unchanged caller retains stale bindings")
	}
	if len(incremental.EdgeFacts) == 0 {
		t.Fatal("caller relationships disappeared")
	}
	for _, edge := range incremental.EdgeFacts {
		if edge.ToFactId != "" {
			t.Fatal("deleted target remained resolved")
		}
	}
}

func TestSCIPArtifactReuseAcrossUnaffectedFamilies(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	toolDir := t.TempDir()
	artifact := filepath.Join(toolDir, "fixture.scip")
	counter := filepath.Join(toolDir, "runs")
	t.Setenv("TLD_SCIP_TEST_ARTIFACT", artifact)
	t.Setenv("TLD_SCIP_TEST_COUNTER", counter)
	tool := filepath.Join(toolDir, "scip-fixture")
	script := `#!/bin/sh
if [ "$1" = "--version" ]; then echo 1; exit 0; fi
cp "$TLD_SCIP_TEST_ARTIFACT" "$2"
echo indexed >> "$TLD_SCIP_TEST_COUNTER"
`
	if err := os.WriteFile(tool, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module fixture\n\ngo 1.26\n"), 0600); err != nil {
		t.Fatal(err)
	}
	update := func(code string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(root, "a.go"), []byte(code), 0600); err != nil {
			t.Fatal(err)
		}
		data, err := proto.Marshal(&scip.Index{Metadata: &scip.Metadata{ToolInfo: &scip.ToolInfo{Name: "fixture", Version: "1"}}, Documents: []*scip.Document{{RelativePath: "a.go", Text: code}}})
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(artifact, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	update("package a\nfunc A() { return }\n")
	cfg := config.Default()
	cfg.Tools.SCIPGo = tool
	pipeline := Pipeline{Config: cfg}
	req := &pb.IndexRequest{Directory: root}
	snap, g, err := pipeline.Build(ctx, req, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "other.py"), []byte("def other():\n    pass\n"), 0600); err != nil {
		t.Fatal(err)
	}
	next, incremental, _, err := pipeline.BuildIncremental(ctx, req, nil, baseFor(snap, g))
	if err != nil {
		t.Fatal(err)
	}
	runs, err := os.ReadFile(counter)
	if err != nil || strings.Count(string(runs), "indexed") != 1 {
		t.Fatalf("unaffected project reindexed: %s %v", runs, err)
	}
	if len(incremental.ProjectArtifacts) != 1 {
		t.Fatal("SCIP artifact was lost")
	}
	update("package a\nfunc A() { panic(1) }\n")
	_, incremental, _, err = pipeline.BuildIncremental(ctx, req, nil, baseFor(next, incremental))
	if err != nil {
		t.Fatal(err)
	}
	runs, err = os.ReadFile(counter)
	if err != nil || strings.Count(string(runs), "indexed") != 2 {
		t.Fatalf("changed project did not rerun: %s %v", runs, err)
	}
	_, fresh, err := pipeline.Build(ctx, req, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(normalizeGraph(t, incremental), normalizeGraph(t, fresh)) {
		t.Fatal("cached project output differs from fresh")
	}
}

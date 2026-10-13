package impact

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/codeindex/graph"
	assets "github.com/mertcikla/tld/v2"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/core"
	localstore "github.com/mertcikla/tld/v2/internal/store"
	"github.com/mertcikla/tld/v2/pkg/app"
)

func pipelineStore(t *testing.T) (context.Context, *app.Store, *cstore.Store) {
	t.Helper()
	ctx := context.Background()
	ws, err := localstore.Open(filepath.Join(t.TempDir(), "tld.db"), assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ws.Close() })
	return ctx, ws, cstore.NewStore(ws.DB(), ws.BunDB(), ws.Dialect())
}

func publishPipeline(t *testing.T, ctx context.Context, idx *cstore.Store, id string, files map[string]string, edges [][2]string) {
	t.Helper()
	g := graph.NewGraph("repo", id)
	snap := &pb.Snapshot{Id: id, RepositoryId: "repo", IngestionStatus: "complete", Manifest: &pb.SnapshotManifest{Complete: true}}
	facts := map[string]*pb.CodeFact{}
	for _, path := range sortedKeys(files) {
		text := files[path]
		snap.Manifest.Files = append(snap.Manifest.Files, &pb.SourceFile{Path: path, Hash: graph.Hash([]byte(text)), Mode: "100644"})
		if strings.HasSuffix(path, ".bin") {
			continue
		}
		source := &graph.Source{Path: path, Language: "go", Text: []byte(text), Hash: graph.Hash([]byte(text))}
		g.Sources[path] = source
		snap.Sources = append(snap.Sources, &pb.SourceFile{Path: path, Hash: source.Hash})
		facts[path] = g.AddFact(pb.FactKind_FACT_KIND_FUNCTION, "First", "go", source.Anchor(0, len(text)), text, "", nil)
	}
	for _, edge := range edges {
		g.AddEdgeFact(pb.EdgeKind_EDGE_KIND_CALLS, facts[edge[0]].Id, facts[edge[1]].Id, "", facts[edge[0]].Anchor, nil)
	}
	if err := idx.Publish(ctx, "/repo", snap, g); err != nil {
		t.Fatal(err)
	}
}

func addPipelineElement(t *testing.T, ctx context.Context, ws core.Store, viewID int64, name, path string) int64 {
	t.Helper()
	input := core.LibraryElement{Name: name}
	if path != "" {
		input.FilePath = &path
	}
	element, err := ws.CreateElement(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ws.AddPlacement(ctx, viewID, element.ID, 17, 29); err != nil {
		t.Fatal(err)
	}
	return element.ID
}

func TestSymbolLinkedAuthoredElementCoversFileChanges(t *testing.T) {
	for _, classChanged := range []bool{false, true} {
		for _, mapped := range []bool{false, true} {
			for _, radius := range []uint32{0, 1} {
				t.Run(fmt.Sprintf("classChanged=%t/mapped=%t/radius=%d", classChanged, mapped, radius), func(t *testing.T) {
					ctx, ws, idx := pipelineStore(t)
					const path = "src/Formatter.php"
					publish := func(id, value string) {
						text := "class Formatter {\n function normalize() { return " + value + "; }\n}\n"
						g := graph.NewGraph("repo", id)
						source := &graph.Source{Path: path, Language: "php", Text: []byte(text), Hash: graph.Hash([]byte(text))}
						g.Sources[path] = source
						end := strings.Index(text, " {")
						if classChanged {
							end = len(text)
						}
						g.AddFact(pb.FactKind_FACT_KIND_TYPE, "Formatter", "php", source.Anchor(0, end), text[:end], "", nil)
						start := strings.Index(text, "function normalize")
						g.AddFact(pb.FactKind_FACT_KIND_METHOD, "normalize", "php", source.Anchor(start, len(text)), text[start:], "", nil)
						file := &pb.SourceFile{Path: path, Hash: source.Hash, Mode: "100644"}
						snap := &pb.Snapshot{Id: id, RepositoryId: "repo", IngestionStatus: "complete", Manifest: &pb.SnapshotManifest{Complete: true, Files: []*pb.SourceFile{file}}, Sources: []*pb.SourceFile{file}}
						if err := idx.Publish(ctx, "/repo", snap, g); err != nil {
							t.Fatal(err)
						}
					}
					publish("base", "1")
					publish("head", "2")
					view, err := ws.CreateView(ctx, "Authored pipeline", nil, nil)
					if err != nil {
						t.Fatal(err)
					}
					link := path + "#symbol:Formatter"
					id := addPipelineElement(t, ctx, ws, view.ID, "JSON formatter", link)
					if mapped {
						mapView, err := ws.CreateView(ctx, "Code map", nil, nil)
						if err != nil {
							t.Fatal(err)
						}
						mapID := addPipelineElement(t, ctx, ws, mapView.ID, "Formatter.php", path)
						if err := idx.SaveMappings(ctx, []cstore.ResourceMapping{
							{LogicalKey: "map|file|formatter", Kind: cstore.MappingElement, ResourceID: mapID, RepositoryID: "repo", SnapshotID: "head"},
							{LogicalKey: "map|view|repo", Kind: cstore.MappingView, ResourceID: mapView.ID, RepositoryID: "repo", SnapshotID: "head"},
						}); err != nil {
							t.Fatal(err)
						}
					}
					diagram, err := (Service{Workspace: ws, Index: idx}).CompareSnapshots(ctx, "repo", "base", "head", radius)
					if err != nil {
						t.Fatal(err)
					}
					scene := diagram.Scene
					if err := ValidateCoverage(scene); err != nil {
						t.Fatal(err)
					}
					if len(scene.Files) != 1 || len(scene.Files[0].Representatives) != 1 || int64(scene.Files[0].Representatives[0].ElementId) != id || int64(scene.Files[0].Representatives[0].ViewId) != view.ID {
						t.Fatalf("expected authored file coverage: %+v", scene.Files)
					}
					placements := scene.Views[itoa(view.ID)].Placements
					if len(placements) != 1 {
						t.Fatalf("duplicate placement: %+v", placements)
					}
					placement := placements[0]
					if placement.Element.FilePath == nil || *placement.Element.FilePath != link || placement.Element.PositionX != 17 || placement.Element.PositionY != 29 {
						t.Fatalf("authored placement changed: %+v", placement.Element)
					}
					if placement.Overlay == nil || placement.Overlay.LinesAdded == nil || placement.Overlay.LinesRemoved == nil || len(placement.Overlay.Symbols) == 0 {
						t.Fatalf("missing file overlay: %+v", placement.Overlay)
					}
					if len(scene.Matches) == 0 || scene.Matches[0].Kind != "symbol" || scene.Matches[0].Partial || scene.Matches[0].SourceLink != link {
						t.Fatalf("symbol navigation and file coverage missing from audit: %+v", scene.Matches)
					}
					for _, p := range scene.Provenance {
						if p.ObjectKind == "placement" && p.Key != PlacementKey(view.ID, id) {
							t.Fatalf("redundant representative: %+v", p)
						}
					}
				})
			}
		}
	}
}

func TestPipelineMixesSourcesAndPreservesCoverage(t *testing.T) {
	ctx, ws, idx := pipelineStore(t)
	base := map[string]string{"a.go": "old a", "b.go": "old b", "s.go": "old s", "d.go": "stable d", "e.go": "stable e", "asset.bin": "old"}
	head := map[string]string{"a.go": "new a", "b.go": "new b", "s.go": "new s", "d.go": "stable d", "e.go": "stable e", "asset.bin": "new"}
	edges := [][2]string{{"a.go", "d.go"}, {"d.go", "e.go"}}
	publishPipeline(t, ctx, idx, "base", base, edges)
	publishPipeline(t, ctx, idx, "head", head, edges)
	authored, err := ws.CreateView(ctx, "Architecture", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	mappedView, err := ws.CreateView(ctx, "Code map", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	linked := addPipelineElement(t, ctx, ws, authored.ID, "Service A", "a.go")
	concept := addPipelineElement(t, ctx, ws, authored.ID, "Conceptual queue", "")
	symbol := addPipelineElement(t, ctx, ws, authored.ID, "First symbol", "s.go#function:First")
	mapped := addPipelineElement(t, ctx, ws, mappedView.ID, "Mapped B", "b.go")
	if err := idx.SaveMappings(ctx, []cstore.ResourceMapping{{LogicalKey: "map|file|b", Kind: cstore.MappingElement, ResourceID: mapped, RepositoryID: "repo", SnapshotID: "head"}, {LogicalKey: "map|view|repo", Kind: cstore.MappingView, ResourceID: mappedView.ID, RepositoryID: "repo", SnapshotID: "head"}}); err != nil {
		t.Fatal(err)
	}
	service := Service{Workspace: ws, Index: idx}
	diagram, err := service.CompareSnapshots(ctx, "repo", "base", "head", 1)
	if err != nil {
		t.Fatal(err)
	}
	scene := diagram.Scene
	if !scene.ManifestComplete || len(scene.Files) != 4 {
		t.Fatalf("coverage = %+v", scene.Files)
	}
	if err := ValidateCoverage(scene); err != nil {
		t.Fatal(err)
	}
	if p := scene.Provenance[PlacementKey(authored.ID, linked)]; p == nil || p.Origin != pb.ImpactOrigin_IMPACT_ORIGIN_AUTHORED {
		t.Fatalf("authored origin = %+v", p)
	}
	if p := scene.Provenance[PlacementKey(mappedView.ID, mapped)]; p == nil || p.Origin != pb.ImpactOrigin_IMPACT_ORIGIN_MAPPED {
		t.Fatalf("mapped origin = %+v", p)
	}
	if p := scene.Provenance[PlacementKey(authored.ID, concept)]; p == nil || p.Grounding != pb.ImpactGrounding_IMPACT_GROUNDING_UNLINKED {
		t.Fatalf("concept lost = %+v", p)
	}
	if p := scene.Provenance[PlacementKey(authored.ID, symbol)]; p == nil || p.Partial {
		t.Fatalf("file-level symbol coverage = %+v", p)
	}
	for _, coverage := range scene.Files {
		if coverage.Path == "s.go" {
			if len(coverage.Representatives) != 1 || int64(coverage.Representatives[0].ElementId) != symbol || int64(coverage.Representatives[0].ViewId) != authored.ID {
				t.Fatalf("symbol-linked authored placement did not cover the file: %+v", coverage.Representatives)
			}
		}
		if coverage.Path == "asset.bin" && coverage.HeadAnalysis != "manifest_only" {
			t.Fatal("missing facts were hidden")
		}
	}
	foundNeighbour := false
	for _, p := range scene.Provenance {
		if p.ObjectKind != "placement" {
			continue
		}
		if len(p.Paths) == 1 && p.Paths[0] == "d.go" {
			foundNeighbour = p.Origin == pb.ImpactOrigin_IMPACT_ORIGIN_RAW && p.Distance == 1 && len(p.Witnesses) == 2
		}
		for _, path := range p.Paths {
			if path == "e.go" {
				t.Fatal("second-hop neighbour leaked into radius 1")
			}
		}
	}
	if !foundNeighbour {
		t.Fatal("unmapped first-hop neighbour was lost")
	}
	for _, placement := range scene.Views[itoa(authored.ID)].Placements {
		if int64(placement.Element.ElementId) == linked && (placement.Element.PositionX != 17 || placement.Element.PositionY != 29) {
			t.Fatal("authored coordinates changed")
		}
	}
	plain := SelectScene(scene, 1, true)
	if plain.Provenance[PlacementKey(authored.ID, concept)] != nil {
		t.Fatal("plain retained ungrounded context")
	}
	if err := ValidateCoverage(plain); err != nil {
		t.Fatal(err)
	}
	if PublicScene(scene, false).Provenance != nil || PublicScene(scene, true).Provenance == nil {
		t.Fatal("debug opt-in not respected")
	}
	cached, err := service.CompareSnapshots(ctx, "repo", "base", "head", 1)
	expectedJSON, _ := json.Marshal(diagram)
	cachedJSON, _ := json.Marshal(cached)
	if err != nil || !bytes.Equal(cachedJSON, expectedJSON) {
		t.Fatalf("immutable cache mismatch: %v", err)
	}
	if _, err := ws.UpdateElement(ctx, linked, core.LibraryElement{Name: "Renamed service", FilePath: stringPtr("a.go")}); err != nil {
		t.Fatal(err)
	}
	fresh, err := service.CompareSnapshots(ctx, "repo", "base", "head", 1)
	if err != nil {
		t.Fatal(err)
	}
	analysis, err := idx.Impact(ctx, "repo", ComparisonKey("base", "head", 1, "source-analysis"))
	if err != nil || analysis.Scene != nil || analysis.WorkspaceFingerprint != "" {
		t.Fatalf("source analysis must be independent of workspace: %v", err)
	}
	if fresh.ComparisonKey == diagram.ComparisonKey || fresh.WorkspaceFingerprint == diagram.WorkspaceFingerprint {
		t.Fatal("workspace edit did not invalidate assembly")
	}
	oldScene, err := service.Scene(ctx, diagram)
	if err != nil || oldScene.Provenance[PlacementKey(authored.ID, linked)].Name != "Service A" {
		t.Fatal("frozen scene reread live workspace")
	}
}

func TestPipelineDoesNotTraverseHybridSnapshotPaths(t *testing.T) {
	ctx, ws, idx := pipelineStore(t)
	publishPipeline(t, ctx, idx, "base", map[string]string{"a.go": "old", "b.go": "b", "c.go": "c"}, [][2]string{{"a.go", "b.go"}})
	publishPipeline(t, ctx, idx, "head", map[string]string{"a.go": "new", "b.go": "b", "c.go": "c"}, [][2]string{{"b.go", "c.go"}})
	diagram, err := (Service{Workspace: ws, Index: idx}).CompareSnapshots(ctx, "repo", "base", "head", 2)
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range diagram.Nodes {
		if node.Path == "c.go" {
			t.Fatal("a BASE edge was combined with a HEAD edge")
		}
	}
	public := PublicScene(diagram.Scene, true)
	for _, witness := range public.Provenance.Witnesses {
		if witness.Side != "base" || strings.Join(witness.Paths, ",") != "a.go,b.go" {
			t.Fatalf("hybrid public witness: %v", witness)
		}
	}
	for _, dependency := range public.Provenance.Dependencies {
		if dependency.Side != "base" || dependency.FromPath != "a.go" || dependency.ToPath != "b.go" {
			t.Fatalf("hybrid public dependency: %v", dependency)
		}
	}

	if len(diagram.Nodes) != 2 || diagram.GetRequestedRadius() != 2 {
		t.Fatalf("unexpected scope: %+v", diagram.Nodes)
	}
}

func TestPipelineViewFilterKeepsUngroundedSkeletonAndRawCoverage(t *testing.T) {
	ctx, ws, idx := pipelineStore(t)
	publishPipeline(t, ctx, idx, "base", map[string]string{"a.go": "old"}, nil)
	publishPipeline(t, ctx, idx, "head", map[string]string{"a.go": "new"}, nil)
	view, err := ws.CreateView(ctx, "Conceptual architecture", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	id := addPipelineElement(t, ctx, ws, view.ID, "Ungrounded service", "")
	diagram, err := (Service{Workspace: ws, Index: idx}).CompareSnapshots(ctx, "repo", "base", "head", 0, "Conceptual")
	if err != nil {
		t.Fatal(err)
	}
	if diagram.Scene.Provenance[PlacementKey(view.ID, id)] == nil || len(diagram.Scene.Files) != 1 {
		t.Fatal("explicit skeleton or uncovered change lost")
	}
}

func TestFileLedgerCarriesChangeIdentityAndRepresentatives(t *testing.T) {
	ctx, ws, idx := pipelineStore(t)
	publishPipeline(t, ctx, idx, "base", map[string]string{"a.go": "before"}, nil)
	publishPipeline(t, ctx, idx, "head", map[string]string{"a.go": "after"}, nil)
	diagram, err := (Service{Workspace: ws, Index: idx}).CompareSnapshots(ctx, "repo", "base", "head", 0)
	if err != nil {
		t.Fatal(err)
	}
	source, covered := diagram.Diff.Sources[0], diagram.Scene.Files[0]
	if covered.Path != source.Path || covered.Change != source.Change || len(covered.Representatives) == 0 {
		t.Fatalf("file ledger lost change identity or representatives: source=%+v covered=%+v", source, covered)
	}
}

func itoa(value int64) string { return fmt.Sprint(value) }

func TestPipelineDoesNotClaimAmbiguousRepositoryAnchors(t *testing.T) {
	ctx, ws, idx := pipelineStore(t)
	publishPipeline(t, ctx, idx, "base", map[string]string{"a.go": "old"}, nil)
	publishPipeline(t, ctx, idx, "head", map[string]string{"a.go": "new"}, nil)
	view, err := ws.CreateView(ctx, "Architecture", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	element := addPipelineElement(t, ctx, ws, view.ID, "Service", "a.go")
	service := Service{Workspace: ws, Index: idx}
	original, err := service.CompareSnapshots(ctx, "repo", "base", "head", 0, "Architecture")
	if err != nil {
		t.Fatal(err)
	}
	other := &pb.Snapshot{Id: "other-head", RepositoryId: "other", Manifest: &pb.SnapshotManifest{Complete: true, Files: []*pb.SourceFile{{Path: "a.go", Hash: "other", Mode: "100644"}}}}
	if err := idx.Publish(ctx, "/other", other, graph.NewGraph("other", "other-head")); err != nil {
		t.Fatal(err)
	}
	ambiguous, err := service.CompareSnapshots(ctx, "repo", "base", "head", 0, "Architecture")
	if err != nil {
		t.Fatal(err)
	}
	if ambiguous.ComparisonKey == original.ComparisonKey {
		t.Fatal("repository identity edit did not invalidate scene")
	}
	key := PlacementKey(view.ID, element)
	if p := ambiguous.Scene.Provenance[key]; p == nil || p.Grounding != pb.ImpactGrounding_IMPACT_GROUNDING_AMBIGUOUS {
		t.Fatalf("ambiguous anchor incorrectly grounded: %+v", p)
	}
	if ambiguous.Scene.Provenance[PlacementKey(int64(ambiguous.Scene.Files[0].Representatives[0].ViewId), int64(ambiguous.Scene.Files[0].Representatives[0].ElementId))].Origin != pb.ImpactOrigin_IMPACT_ORIGIN_RAW {
		t.Fatal("ambiguous anchor consumed direct coverage")
	}
	if err := idx.SetRepositoryRemoteURL(ctx, "repo", "https://github.com/owner/repo"); err != nil {
		t.Fatal(err)
	}
	if _, err := ws.UpdateElement(ctx, element, core.LibraryElement{Name: "Service", FilePath: stringPtr("a.go"), Repo: stringPtr("git@github.com:owner/repo.git")}); err != nil {
		t.Fatal(err)
	}
	grounded, err := service.CompareSnapshots(ctx, "repo", "base", "head", 0, "Architecture")
	if err != nil {
		t.Fatal(err)
	}
	if p := grounded.Scene.Provenance[key]; p == nil || p.Grounding != pb.ImpactGrounding_IMPACT_GROUNDING_RESOLVED {
		t.Fatalf("remote anchor not resolved: %+v", p)
	}
}

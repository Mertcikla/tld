package impact

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/codeindex/graph"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/core"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

func evidenceGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %s: %v", args, output, err)
	}
	return strings.TrimSpace(string(output))
}

func publishEvidence(t *testing.T, ctx context.Context, idx *cstore.Store, root, id, revision, provenance string, files map[string]string) {
	t.Helper()
	g := graph.NewGraph("repo", id)
	snap := &pb.Snapshot{Id: id, RepositoryId: "repo", GitRevision: revision, Provenance: provenance, IngestionStatus: "complete", Manifest: &pb.SnapshotManifest{Complete: true}}
	for _, path := range sortedKeys(files) {
		text := files[path]
		mode := "100644"
		info, err := os.Stat(filepath.Join(root, path))
		if err == nil && info.Mode()&0o111 != 0 {
			mode = "100755"
		}
		snap.Manifest.Files = append(snap.Manifest.Files, &pb.SourceFile{Path: path, Hash: graph.Hash([]byte(text)), Mode: mode})
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			continue
		}
		src := &graph.Source{Path: path, Language: "go", Text: []byte(text), Hash: graph.Hash([]byte(text))}
		g.Sources[path] = src
		snap.Sources = append(snap.Sources, &pb.SourceFile{Path: path, Hash: src.Hash})
		g.AddFact(pb.FactKind_FACT_KIND_FUNCTION, path, "go", src.Anchor(0, len(text)), text, "", nil)
	}
	if err := idx.Publish(ctx, root, snap, g); err != nil {
		t.Fatal(err)
	}
}

func TestCapturedGitEvidenceSurvivesCheckoutAndRepositoryLoss(t *testing.T) {
	ctx, ws, idx := pipelineStore(t)
	root := t.TempDir()
	evidenceGit(t, root, "init", "-q")
	evidenceGit(t, root, "config", "user.name", "Evidence Test")
	evidenceGit(t, root, "config", "user.email", "evidence@example.test")
	write := func(files map[string]string) {
		t.Helper()
		for path, text := range files {
			if err := os.MkdirAll(filepath.Join(root, filepath.Dir(path)), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, path), []byte(text), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	base := map[string]string{"cmd/a.go": "old\n", "cmd/a_test.go": "before\n", "cmd/old.txt": "same\n", "cmd/asset.bin": "\x00old", "cmd/run.sh": "#!/bin/sh\n"}
	write(base)
	evidenceGit(t, root, "add", ".")
	evidenceGit(t, root, "commit", "-qm", "base")
	baseSHA := evidenceGit(t, root, "rev-parse", "HEAD")
	publishEvidence(t, ctx, idx, root, "base", baseSHA, "commit", base)
	head := map[string]string{"cmd/a.go": "new\nsecond\n", "cmd/a_test.go": "after\nmore\n", "cmd/new.txt": "same\n", "cmd/asset.bin": "\x00new", "cmd/run.sh": "#!/bin/sh\n"}
	if err := os.Remove(filepath.Join(root, "cmd/old.txt")); err != nil {
		t.Fatal(err)
	}
	write(head)
	if err := os.Chmod(filepath.Join(root, "cmd/run.sh"), 0o755); err != nil {
		t.Fatal(err)
	}
	evidenceGit(t, root, "add", ".")
	evidenceGit(t, root, "commit", "-qm", "head")
	headSHA := evidenceGit(t, root, "rev-parse", "HEAD")
	publishEvidence(t, ctx, idx, root, "head", headSHA, "commit", head)
	view, err := ws.CreateView(ctx, "Commands", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	element := addPipelineElement(t, ctx, ws, view.ID, "Command Layer", "./cmd")
	service := Service{Workspace: ws, Index: idx}
	diagram, err := service.CompareSnapshots(ctx, "repo", "base", "head", 0)
	if err != nil {
		t.Fatal(err)
	}
	scene := PublicScene(diagram.Scene, true)
	if len(scene.Files) != 6 || scene.Summary.KnownLinesAdded != 5 || scene.Summary.KnownLinesRemoved != 3 || !scene.Summary.LineCountsComplete || scene.Summary.BinaryFiles != 1 || scene.Summary.LineCountsBasis != "net_tree" {
		t.Fatalf("summary: %v", scene.Summary)
	}
	if len(scene.Comparison.Commits) != 1 || scene.Comparison.Commits[0].Sha != headSHA {
		t.Fatalf("history: %v", scene.Comparison)
	}
	files := map[string]*pb.ImpactFile{}
	for _, file := range scene.Files {
		files[file.Path] = file
		if len(file.Representatives) != 1 || int64(file.Representatives[0].ElementId) != element {
			t.Fatalf("wrong representative: %v", file)
		}
	}
	if files["cmd/a_test.go"].BaseAnalysis != "manifest_only" || files["cmd/a_test.go"].HeadAnalysis != "manifest_only" {
		t.Fatal("test analysis misrepresented")
	}
	if files["cmd/asset.bin"].LinesAdded != nil || !files["cmd/asset.bin"].Binary {
		t.Fatal("binary count must be unknown")
	}
	if !files["cmd/run.sh"].MetadataOnly || files["cmd/run.sh"].LinesAdded == nil || files["cmd/run.sh"].GetLinesAdded() != 0 {
		t.Fatalf("mode-only: %v", files["cmd/run.sh"])
	}
	if files["cmd/old.txt"].Change != pb.ChangeKind_CHANGE_KIND_REMOVED || files["cmd/new.txt"].Change != pb.ChangeKind_CHANGE_KIND_ADDED {
		t.Fatal("rename was not remove+add")
	}
	if scene.Views[strconv.FormatInt(view.ID, 10)].Placements[0].Overlay.LinesAdded != nil {
		t.Fatal("folder with binary churn cannot claim complete line totals")
	}
	frozen, err := protojson.Marshal(scene)
	if err != nil {
		t.Fatal(err)
	}
	evidenceGit(t, root, "checkout", "--detach", baseSHA)
	if err := os.RemoveAll(filepath.Join(root, ".git")); err != nil {
		t.Fatal(err)
	}
	addPipelineElement(t, ctx, ws, view.ID, "Later workspace element", "cmd/a.go")
	cached, err := idx.Impact(ctx, "repo", diagram.ComparisonKey)
	if err != nil {
		t.Fatal(err)
	}
	assembly, err := (Service{}).Scene(ctx, cached)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := protojson.Marshal(PublicScene(assembly, true))
	if err != nil {
		t.Fatal(err)
	}
	if string(actual) != string(frozen) {
		t.Fatal("cached historical result changed after checkout/workspace/repository loss")
	}
}

func TestOfflineAndWorkingTreeEvidencePreservesUnknownCounts(t *testing.T) {
	for _, working := range []bool{false, true} {
		t.Run(map[bool]string{false: "offline commit", true: "historical working tree"}[working], func(t *testing.T) {
			ctx, ws, idx := pipelineStore(t)
			provenance := "commit"
			if working {
				provenance = "working_tree"
			}
			publishEvidence(t, ctx, idx, "/missing-checkout", "base", strings.Repeat("a", 40), provenance, map[string]string{"scope/a.go": "old", "scope/a_test.go": "before"})
			publishEvidence(t, ctx, idx, "/missing-checkout", "head", strings.Repeat("b", 40), provenance, map[string]string{"scope/a.go": "new", "scope/a_test.go": "after"})
			view, err := ws.CreateView(ctx, "Scope", nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			addPipelineElement(t, ctx, ws, view.ID, "Scope", "scope/")
			diagram, err := (Service{Workspace: ws, Index: idx}).CompareSnapshots(ctx, "repo", "base", "head", 0)
			if err != nil {
				t.Fatal(err)
			}
			scene := PublicScene(diagram.Scene, true)
			if scene.Summary.LineCountsComplete || scene.Files[1].LinesAdded != nil || scene.Files[1].LinesRemoved != nil {
				t.Fatal("unknown analysis became zero")
			}
			if scene.Views[strconv.FormatInt(view.ID, 10)].Placements[0].Overlay.LinesAdded != nil || scene.Views[strconv.FormatInt(view.ID, 10)].Placements[0].Overlay.LinesRemoved != nil {
				t.Fatal("mixed folder totals became complete")
			}
			if scene.Comparison.GitUnavailableReason == "" || len(scene.Comparison.Commits) != 0 {
				t.Fatal("unavailable Git evidence not explicit")
			}
			if working && scene.Comparison.GitStatus != "not_applicable" || !working && scene.Comparison.GitStatus != "unavailable" {
				t.Fatalf("unexpected status %q", scene.Comparison.GitStatus)
			}
		})
	}
}

func TestPublicScenePreservesSelectionAndCanvasInputs(t *testing.T) {
	ctx, ws, idx := pipelineStore(t)
	edges := [][2]string{{"a.go", "b.go"}, {"b.go", "c.go"}}
	publishPipeline(t, ctx, idx, "base", map[string]string{"a.go": "before", "b.go": "stable", "c.go": "stable"}, edges)
	publishPipeline(t, ctx, idx, "head", map[string]string{"a.go": "after", "b.go": "stable", "c.go": "stable"}, edges)
	view, err := ws.CreateView(ctx, "Architecture", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	addPipelineElement(t, ctx, ws, view.ID, "Authored", "a.go")
	addPipelineElement(t, ctx, ws, view.ID, "Context", "")
	diagram, err := (Service{Workspace: ws, Index: idx}).CompareSnapshots(ctx, "repo", "base", "head", 2)
	if err != nil {
		t.Fatal(err)
	}
	for _, plain := range []bool{false, true} {
		for radius := uint32(0); radius <= 2; radius++ {
			selected := SelectScene(diagram.Scene, radius, plain)
			normal, debug := PublicScene(selected, false), PublicScene(selected, true)
			debug.Provenance = nil
			if !proto.Equal(normal, debug) {
				t.Fatal("debug changes canvas inputs")
			}
			if len(normal.Files) != 1 {
				t.Fatal("selection removed file ledger")
			}
			for key, content := range selected.Views {
				target := normal.Views[key]
				if len(content.Placements) != len(target.Placements) || len(content.Connectors) != len(target.Connectors) {
					t.Fatal("projection changed membership")
				}
				for i, p := range content.Placements {
					if !proto.Equal(p.Element, target.Placements[i].Element) {
						t.Fatal("projection changed element rendering data")
					}
				}
				for i, c := range content.Connectors {
					if !proto.Equal(c, target.Connectors[i]) {
						t.Fatal("projection changed connector rendering data")
					}
				}
			}
		}
	}
}

func TestAuditPreservesPartialAnchorsAndCrossViewRelationships(t *testing.T) {
	ctx, ws, idx := pipelineStore(t)
	publishPipeline(t, ctx, idx, "base", map[string]string{"a.go": "first\nold\nthird\n", "b.go": "before\n"}, nil)
	publishPipeline(t, ctx, idx, "head", map[string]string{"a.go": "first\nnew\nthird\n", "b.go": "after\n"}, [][2]string{{"a.go", "b.go"}})
	aView, err := ws.CreateView(ctx, "A", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	bView, err := ws.CreateView(ctx, "B", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	a := addPipelineElement(t, ctx, ws, aView.ID, "A", "a.go")
	b := addPipelineElement(t, ctx, ws, bView.ID, "B", "b.go")
	line := addPipelineElement(t, ctx, ws, aView.ID, "Changed lines", "a.go#L2-L3")
	concept := addPipelineElement(t, ctx, ws, aView.ID, "Declared context", "")
	declared, err := ws.CreateConnector(ctx, core.Connector{ViewID: aView.ID, SourceElementID: a, TargetElementID: concept, Label: proto.String("declared"), Direction: "both", Style: "step", SourceHandle: proto.String("right"), TargetHandle: proto.String("left")})
	if err != nil {
		t.Fatal(err)
	}
	diagram, err := (Service{Workspace: ws, Index: idx}).CompareSnapshots(ctx, "repo", "base", "head", 0)
	if err != nil {
		t.Fatal(err)
	}
	scene := PublicScene(diagram.Scene, true)
	content := scene.Views[strconv.FormatInt(aView.ID, 10)]
	foundPartial, foundCross, foundDeclared := false, false, false
	for _, match := range scene.Provenance.Matches {
		if int64(match.Placement.GetElementId()) == line {
			foundPartial = match.Partial && match.Kind == "line" && match.SourceLink == "a.go#L2-L3" && match.Scope == "a.go" && len(match.Representatives) == 1 && int64(match.Representatives[0].ElementId) == a
		}
	}
	for _, p := range content.Placements {
		if int64(p.Element.ElementId) == line && (p.Overlay.LinesAdded != nil || p.Overlay.LinesRemoved != nil) {
			t.Fatal("partial anchor claimed whole-file churn")
		}
	}
	for _, c := range content.Connectors {
		if int64(c.Id) == declared.ID {
			foundDeclared = c.Direction == "both" && c.Style == "step" && c.GetSourceHandle() == "right" && c.GetTargetHandle() == "left" && c.GetLabel() == "declared"
		}
		if int64(c.TargetElementId) == b {
			target := content.ConnectorTargets[strconv.Itoa(int(c.Id))]
			foundCross = int64(target.GetViewId()) == bView.ID && int64(target.GetElementId()) == b
		}
	}
	if !foundPartial || !foundCross || !foundDeclared {
		t.Fatalf("partial=%t cross=%t declared=%t", foundPartial, foundCross, foundDeclared)
	}
	if len(scene.Provenance.DeclaredConnectors) != 1 || scene.Provenance.DeclaredConnectors[0] != shortRef(ConnectorKey(aView.ID, declared.ID)) {
		t.Fatal("declared relationship not distinguished")
	}
	if len(scene.Provenance.Dependencies) != 1 || scene.Provenance.Dependencies[0].Side != "head" || scene.Provenance.Dependencies[0].FromPath != "a.go" || scene.Provenance.Dependencies[0].ToPath != "b.go" {
		t.Fatal("observed dependency direction or snapshot was lost")
	}
}

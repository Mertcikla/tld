package git

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/impact"
	"google.golang.org/protobuf/encoding/protojson"
)

func TestProvenanceSidecarDoesNotMutateOrPollutePayload(t *testing.T) {
	scene := &pb.ImpactScene{SchemaVersion: impact.PipelineVersion, ManifestComplete: true, Views: map[string]*pb.SceneViewContent{"1": {}}, Coverage: []*pb.SourceChange{{Path: "asset.bin", PlacementKeys: []string{"placement|1|-1"}, FactsUnavailable: true}}, Stages: []*pb.ImpactStage{{Id: "coverage", Name: "Guarantee direct coverage", Added: 1}}, Decisions: []*pb.ImpactDecision{{Stage: "coverage", Action: "add", Key: "placement|1|-1", Reason: "No indexed facts"}}}
	var stderr bytes.Buffer
	path := filepath.Join(t.TempDir(), "provenance.json")
	if err := writeProvenance(compareOptions{}, scene, &stderr); err != nil || stderr.Len() != 0 {
		t.Fatalf("default trace leaked: %v", err)
	}
	if err := writeProvenance(compareOptions{debugProvenance: true, provenanceJSON: path}, scene, &stderr); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(stderr.Bytes(), []byte("[coverage] add placement|1|-1")) {
		t.Fatal(stderr.String())
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	report := &pb.ImpactScene{}
	if err := protojson.Unmarshal(raw, report); err != nil {
		t.Fatal(err)
	}
	if len(report.Views) != 0 || len(report.Coverage) != 1 || len(report.Stages) == 0 {
		t.Fatalf("invalid provenance sidecar: %s", raw)
	}
	if len(scene.Views) != 1 {
		t.Fatal("sidecar mutated canonical scene")
	}
}

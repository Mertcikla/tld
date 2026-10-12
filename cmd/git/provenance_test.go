package git

import (
	"bytes"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/impact"
)

func TestProvenanceDiagnosticsAreOptIn(t *testing.T) {
	scene := &pb.ImpactScene{SchemaVersion: impact.PipelineVersion, ManifestComplete: true, Views: map[string]*pb.SceneViewContent{"1": {}}, Coverage: []*pb.SourceChange{{Path: "asset.bin", PlacementKeys: []string{"placement|1|-1"}, FactsUnavailable: true}}, Stages: []*pb.ImpactStage{{Id: "coverage", Name: "Guarantee direct coverage", Added: 1}}, Decisions: []*pb.ImpactDecision{{Stage: "coverage", Action: "add", Key: "placement|1|-1", Reason: "No indexed facts"}}}
	var stderr bytes.Buffer
	if err := writeProvenance(compareOptions{}, scene, &stderr); err != nil || stderr.Len() != 0 {
		t.Fatalf("default trace leaked: %v", err)
	}
	if err := writeProvenance(compareOptions{debugProvenance: true}, scene, &stderr); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(stderr.Bytes(), []byte("[coverage] add placement|1|-1")) {
		t.Fatal(stderr.String())
	}
}

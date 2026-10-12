package git

import (
	"bytes"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/impact"
	"github.com/mertcikla/tld/v2/internal/codeindex/model"
)

func TestProvenanceDiagnosticsAreOptIn(t *testing.T) {
	scene := &model.Assembly{SchemaVersion: impact.PipelineVersion, ManifestComplete: true, Views: map[string]*model.ViewContent{"1": {}}, Files: []*pb.ImpactFile{{Path: "asset.bin", Representatives: []*pb.ImpactPlacementRef{{ViewId: 1, ElementId: -1}}, BaseAnalysis: "manifest_only", HeadAnalysis: "manifest_only"}}, Decisions: []*model.Decision{{Stage: "coverage", Action: "suppress", Key: "placement|1|-1", Reason: "No indexed facts"}}}
	var stderr bytes.Buffer
	if err := writeProvenance(compareOptions{}, scene, &stderr); err != nil || stderr.Len() != 0 {
		t.Fatalf("default trace leaked: %v", err)
	}
	if err := writeProvenance(compareOptions{debugProvenance: true}, scene, &stderr); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(stderr.Bytes(), []byte("[coverage] suppress v1/e-1")) {
		t.Fatal(stderr.String())
	}
}

package mermaid

import (
	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/sceneurl"
	"google.golang.org/protobuf/encoding/protojson"
	"os"
	"testing"
)

// The frontend consumes these same fixtures to prevent the offline exporter
// from drifting from the canonical Go presentation rules.
func TestSceneURLSharedFixture(t *testing.T) {
	raw, err := os.ReadFile("fixtures/scene-url.json")
	if err != nil {
		t.Fatal(err)
	}
	scene := &pb.ImpactScene{}
	if err := protojson.Unmarshal(raw, scene); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("fixtures/scene-url.mmd")
	if err != nil {
		t.Fatal(err)
	}
	if got := ExportImpactScene(scene, SceneExportOptions{IncludeMetadata: true}); got != string(want) {
		t.Fatalf("Mermaid differs:\n%s", got)
	}
	link, err := sceneurl.Encode(raw, false)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := os.ReadFile("fixtures/scene-url.txt")
	if err != nil {
		t.Fatal(err)
	}
	if link+"\n" != string(expected) {
		t.Fatal("Go transport differs from frontend's shared link fixture")
	}
}

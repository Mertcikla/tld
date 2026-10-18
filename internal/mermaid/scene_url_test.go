package mermaid

import (
	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/sceneurl"
	"google.golang.org/protobuf/encoding/protojson"
	"os"
	"testing"
)

// The frontend consumes the same JSON and URL fixtures to verify transport
// compatibility between Go encoding and browser decoding.
func TestSceneURLSharedFixture(t *testing.T) {
	raw, err := os.ReadFile("fixtures/scene-url.json")
	if err != nil {
		t.Fatal(err)
	}
	scene := &pb.ImpactScene{}
	if err := protojson.Unmarshal(raw, scene); err != nil {
		t.Fatal(err)
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

package git

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/impact"
	"github.com/mertcikla/tld/v2/internal/codeindex/model"
	"google.golang.org/protobuf/proto"
)

func TestCompareMermaidRetainsAncestorWrappers(t *testing.T) {
	diagram := autoCompareFixture(2, 0)
	autoCompareEdge(diagram, 1, 2)
	scene := diagram.Scene
	scene.Views["2"] = &model.ViewContent{Placements: []*model.Placement{{Element: &diagv1.PlacedElement{ViewId: 2, ElementId: 3, Name: "Container"}}}}
	scene.Provenance[impact.PlacementKey(2, 3)] = &model.Metadata{Key: impact.PlacementKey(2, 3), ViewId: 2, ResourceId: 3, Role: pb.ImpactRole_IMPACT_ROLE_CONTAINER}
	scene.Views["3"] = &model.ViewContent{}
	scene.Tree = []*diagv1.View{{Id: 2, Name: "Workspace", Children: []*diagv1.View{{Id: 1, Name: "Changes", OwnerElementId: proto.Int32(3)}}}, {Id: 3, Name: "Unrelated"}}
	before, err := json.Marshal(scene)
	if err != nil {
		t.Fatal(err)
	}
	for _, auto := range []bool{false, true} {
		for _, plain := range []bool{false, true} {
			opts := compareOptions{mermaid: true, auto: auto, plain: plain}
			render := compareRenderer{ctx: context.Background(), opts: opts}
			selected := opts.scope(diagram, 0)
			text, _, err := render.build(selected, 0)
			if auto {
				result, _, reason, autoErr := selectAutoCompare(diagram, opts, render.build, autoFixtureLoader(diagram))
				if reason != "" {
					t.Fatal(reason)
				}
				text, err = result.text, autoErr
			}
			if err != nil {
				t.Fatal(err)
			}
			if plain {
				if strings.Contains(text, "subgraph") {
					t.Fatalf("plain output retained hierarchy (auto=%t):\n%s", auto, text)
				}
			} else {
				for _, want := range []string{`subgraph view_2["Workspace"]`, `  subgraph view_1["Changes"]`} {
					if !strings.Contains(text, want) {
						t.Fatalf("missing %q (auto=%t):\n%s", want, auto, text)
					}
				}
			}
			for _, want := range []string{"file-01.go", "file-02.go", "el_1_1 --> el_1_2"} {
				if !strings.Contains(text, want) {
					t.Fatalf("lost evidence %q (auto=%t plain=%t):\n%s", want, auto, plain, text)
				}
			}
			if strings.Contains(text, "Unrelated") || strings.Contains(text, "el_2_3") {
				t.Fatalf("restored suppressed content:\n%s", text)
			}
		}
	}
	after, err := json.Marshal(scene)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("mutated canonical scene")
	}
}

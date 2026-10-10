package mermaid

import (
	"strings"
	"testing"

	codeindexv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"google.golang.org/protobuf/proto"
)

func sceneNode(elementID int32, viewID int32, name string, overlay *codeindexv1.ImpactSceneOverlay) *codeindexv1.ScenePlacement {
	return &codeindexv1.ScenePlacement{
		Element: &diagv1.PlacedElement{Id: elementID, ElementId: elementID, ViewId: viewID, Name: name},
		Overlay: overlay,
	}
}

func sceneOverlay(change codeindexv1.ChangeKind, path string, distance uint32, added, removed uint32) *codeindexv1.ImpactSceneOverlay {
	return &codeindexv1.ImpactSceneOverlay{
		Change: change, Path: path, Distance: distance,
		LinesAdded: &added, LinesRemoved: &removed,
	}
}

func sceneFixture() *codeindexv1.ImpactScene {
	return &codeindexv1.ImpactScene{
		RepositoryId:    "r1",
		ComparisonKey:   "k1",
		FallbackViewId:  -1,
		AuthoredViewIds: []int64{51},
		Tree: []*diagv1.View{
			{Id: 51, Name: "Mine"},
			{Id: -1, Name: "Changes"},
		},
		Views: map[string]*codeindexv1.SceneViewContent{
			"51": {
				Placements: []*codeindexv1.ScenePlacement{
					sceneNode(7, 51, "svc/auth", sceneOverlay(codeindexv1.ChangeKind_CHANGE_KIND_MODIFIED, "svc/auth", 0, 3, 1)),
					sceneNode(8, 51, "svc/db", nil),
				},
				Connectors: []*diagv1.Connector{
					{Id: 1, ViewId: 51, SourceElementId: 7, TargetElementId: 8, Label: proto.String("calls"), Tags: []string{"change:added"}},
				},
			},
			"-1": {
				Placements: []*codeindexv1.ScenePlacement{
					sceneNode(-1, -1, "new.go", sceneOverlay(codeindexv1.ChangeKind_CHANGE_KIND_ADDED, "new.go", 0, 1, 0)),
				},
			},
		},
	}
}

func TestExportImpactSceneGroundedMatchesCanvas(t *testing.T) {
	t.Parallel()

	got := ExportImpactScene(sceneFixture(), SceneExportOptions{IncludeMetadata: true})
	for _, want := range []string{
		"flowchart LR",
		"%% tld-scene repo=r1 key=k1",
		`subgraph view_51["Mine"]`,
		"el_51_7[\"svc/auth<br/>modified +3 \u22121\"]",
		`el_51_8["svc/db"]`,
		`el_51_7 ==>|"calls"| el_51_8`,
		"linkStyle 0 stroke:#48bb78",
		`subgraph view_neg1["Changes"]`,
		"el_neg1_neg1[\"◇ new.go<br/>added +1 \u22120\"]",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("ExportImpactScene() missing %q in:\n%s", want, got)
		}
	}
}

func TestExportImpactSceneDegradesToMappedWithoutAuthoredViews(t *testing.T) {
	t.Parallel()

	fixture := sceneFixture()
	fixture.AuthoredViewIds = nil
	got := ExportImpactScene(fixture, SceneExportOptions{})
	for _, want := range []string{
		`subgraph view_51["Mine"]`,
		"el_51_7[\"▦ svc/auth<br/>modified +3 −1\"]",
		`subgraph view_neg1["Changes"]`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("degraded scene missing %q in:\n%s", want, got)
		}
	}
}

func TestExportImpactSceneRemovedConnector(t *testing.T) {
	t.Parallel()

	fixture := sceneFixture()
	fixture.Views["51"].Connectors[0].Tags = []string{"change:removed"}
	got := ExportImpactScene(fixture, SceneExportOptions{})
	for _, want := range []string{
		`el_51_7--x|"calls"|el_51_8`,
		"linkStyle 0 stroke:#fc8181,stroke-dasharray:5 5",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("ExportImpactScene() missing %q in:\n%s", want, got)
		}
	}
}

func TestExportImpactSceneNilIsEmptyFlowchart(t *testing.T) {
	t.Parallel()

	if got := ExportImpactScene(nil, SceneExportOptions{}); got != "flowchart LR\n" {
		t.Fatalf("ExportImpactScene(nil) = %q", got)
	}
}

func TestExportImpactScenePlainHidesUnimpacted(t *testing.T) {
	t.Parallel()

	fixture := sceneFixture()
	got := ExportImpactScene(fixture, SceneExportOptions{Plain: true})
	for _, want := range []string{
		`el_51_7["svc/auth<br/>modified +3 −1"]`,
		`el_neg1_neg1["◇ new.go<br/>added +1 −0"]`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("plain scene missing %q in:\n%s", want, got)
		}
	}
	// The unimpacted member drops out, taking its connector with it.
	if strings.Contains(got, `el_51_8["svc/db"]`) || strings.Contains(got, "linkStyle") {
		t.Fatalf("plain scene kept unimpacted content:\n%s", got)
	}
}

package mermaid

import (
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/mertcikla/tld/v2/internal/codeindex/impact"
	"github.com/mertcikla/tld/v2/internal/codeindex/model"

	codeindexv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"google.golang.org/protobuf/proto"
)

func TestExportImpactSceneConnectorDirections(t *testing.T) {
	raw, err := os.ReadFile("fixtures/connector-directions.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Direction   string
		Change      string
		Label       string
		TargetView  int32
		Line        string
		NoThemeLine string
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	for _, test := range cases {
		t.Run(test.Direction+"/"+test.Change, func(t *testing.T) {
			content := &codeindexv1.SceneViewContent{
				Placements: []*codeindexv1.ScenePlacement{{Element: &diagv1.PlacedElement{ElementId: 10, ViewId: 1, Name: "Source"}}},
				Connectors: []*diagv1.Connector{{Id: 1, ViewId: 1, SourceElementId: 10, TargetElementId: 20, Direction: test.Direction, Label: proto.String(test.Label)}},
			}
			if test.Change != "" {
				content.Connectors[0].Tags = []string{"change:" + test.Change}
			}
			scene := &codeindexv1.ImpactScene{Views: map[string]*codeindexv1.SceneViewContent{"1": content}}
			target := &codeindexv1.ScenePlacement{Element: &diagv1.PlacedElement{ElementId: 20, ViewId: test.TargetView, Name: "Target"}}
			if test.TargetView == 1 {
				content.Placements = append(content.Placements, target)
			} else {
				scene.Views[strconv.Itoa(int(test.TargetView))] = &codeindexv1.SceneViewContent{Placements: []*codeindexv1.ScenePlacement{target}}
				content.ConnectorTargets = map[string]*codeindexv1.ImpactPlacementRef{"1": {ViewId: test.TargetView, ElementId: 20}}
			}
			got := ExportImpactScene(scene, SceneExportOptions{})
			if !strings.Contains(got, "\n  "+test.Line+"\n") {
				t.Fatalf("missing connector %q:\n%s", test.Line, got)
			}
			parsed, err := Parse(got)
			if err != nil {
				t.Fatalf("invalid Mermaid: %v\n%s", err, got)
			}
			if len(parsed.Warnings) > 0 {
				t.Fatalf("invalid Mermaid connectors: %v\n%s", parsed.Warnings, got)
			}
			unstyled := ExportImpactScene(scene, SceneExportOptions{NoTheme: true})
			if !strings.Contains(unstyled, "\n  "+test.NoThemeLine+"\n") || strings.Contains(unstyled, "linkStyle") {
				t.Fatalf("unstyled connector lost change status or direction, want %q:\n%s", test.NoThemeLine, unstyled)
			}
			parsed, err = Parse(unstyled)
			if err != nil || len(parsed.Warnings) > 0 {
				t.Fatalf("invalid unstyled Mermaid: %v\n%s", err, unstyled)
			}
		})
	}
}

func sceneNode(elementID int32, viewID int32, name string, overlay *model.Overlay) *model.Placement {
	return &model.Placement{
		Element: &diagv1.PlacedElement{Id: elementID, ElementId: elementID, ViewId: viewID, Name: name},
		Overlay: overlay,
	}
}

func sceneOverlay(change codeindexv1.ChangeKind, path string, added, removed uint32) *model.Overlay {
	return &model.Overlay{
		Change: change, Path: path,
		LinesAdded: &added, LinesRemoved: &removed,
	}
}

func sceneFixture() *model.Assembly {
	return &model.Assembly{
		RepositoryId:    "r1",
		ComparisonKey:   "k1",
		SchemaVersion:   "2",
		Provenance:      map[string]*model.Metadata{"placement|-1|-1": {Origin: codeindexv1.ImpactOrigin_IMPACT_ORIGIN_RAW}},
		FallbackViewId:  -1,
		AuthoredViewIds: []int64{51},
		Tree: []*diagv1.View{
			{Id: 51, Name: "Mine"},
			{Id: -1, Name: "Changes"},
		},
		Views: map[string]*model.ViewContent{
			"51": {
				Placements: []*model.Placement{
					sceneNode(7, 51, "svc/auth", sceneOverlay(codeindexv1.ChangeKind_CHANGE_KIND_MODIFIED, "svc/auth", 3, 1)),
					sceneNode(8, 51, "svc/db", nil),
				},
				Connectors: []*diagv1.Connector{
					{Id: 1, ViewId: 51, SourceElementId: 7, TargetElementId: 8, Label: proto.String("calls"), Tags: []string{"change:added"}},
				},
			},
			"-1": {
				Placements: []*model.Placement{
					sceneNode(-1, -1, "new.go", sceneOverlay(codeindexv1.ChangeKind_CHANGE_KIND_ADDED, "new.go", 1, 0)),
				},
			},
		},
	}
}

func TestExportImpactSceneGroundedMatchesCanvas(t *testing.T) {
	t.Parallel()

	got := ExportImpactScene(impact.PublicScene(sceneFixture(), true), SceneExportOptions{IncludeMetadata: true})
	for _, want := range []string{
		"flowchart LR",
		"%% tld-scene repo=r1 key=k1",
		`subgraph view_51["Mine"]`,
		"el_51_7(\"svc/auth<br/>modified +3 \u22121\")",
		`el_51_8("svc/db")`,
		`el_51_7 ==>|"calls"| el_51_8`,
		"linkStyle 0 stroke:#48bb78",
		`subgraph view_neg1["Changes"]`,
		"el_neg1_neg1(\"new.go<br/>added +1\")",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("ExportImpactScene() missing %q in:\n%s", want, got)
		}
	}
}

func TestExportImpactSceneMappedOriginKeepsPlainLabel(t *testing.T) {
	t.Parallel()

	fixture := sceneFixture()
	fixture.AuthoredViewIds = nil
	fixture.Provenance["placement|51|7"] = &model.Metadata{Origin: codeindexv1.ImpactOrigin_IMPACT_ORIGIN_MAPPED}
	got := ExportImpactScene(impact.PublicScene(fixture, true), SceneExportOptions{})
	for _, want := range []string{
		`subgraph view_51["Mine"]`,
		"el_51_7(\"svc/auth<br/>modified +3 −1\")",
		`subgraph view_neg1["Changes"]`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("degraded scene missing %q in:\n%s", want, got)
		}
	}
}

func TestExportImpactSceneUntaggedPlacementInDegradedViewStaysPlain(t *testing.T) {
	t.Parallel()

	// User-authored placements carry no provenance tag: even when their view
	// has no change overlay, they must not read as map-generated.
	fixture := sceneFixture()
	fixture.AuthoredViewIds = nil
	got := ExportImpactScene(impact.PublicScene(fixture, true), SceneExportOptions{})
	if !strings.Contains(got, `el_51_7("svc/auth<br/>modified +3 −1")`) {
		t.Fatalf("authored placement mislabeled in degraded view:\n%s", got)
	}
}

func TestExportImpactSceneRemovedConnector(t *testing.T) {
	t.Parallel()

	fixture := sceneFixture()
	fixture.Views["51"].Connectors[0].Tags = []string{"change:removed"}
	got := ExportImpactScene(impact.PublicScene(fixture, true), SceneExportOptions{})
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

func TestExportImpactSceneCrossViewTargetDoesNotRewriteSourceOrLabel(t *testing.T) {
	t.Parallel()
	for _, change := range []string{"", "added", "removed", "modified"} {
		t.Run(change, func(t *testing.T) {
			var tags []string
			if change != "" {
				tags = []string{"change:" + change}
			}
			scene := &codeindexv1.ImpactScene{
				Tree: []*diagv1.View{{Id: 1, Name: "Caller"}, {Id: 2, Name: "Callee"}},
				Views: map[string]*codeindexv1.SceneViewContent{
					"1": {
						Placements:       []*codeindexv1.ScenePlacement{{Element: &diagv1.PlacedElement{ElementId: 70, ViewId: 1, Name: "Source"}}},
						Connectors:       []*diagv1.Connector{{Id: 1, ViewId: 1, SourceElementId: 70, TargetElementId: 7, Label: proto.String("calls el_1_7"), Tags: tags}},
						ConnectorTargets: map[string]*codeindexv1.ImpactPlacementRef{"1": {ViewId: 2, ElementId: 7}},
					},
					"2": {Placements: []*codeindexv1.ScenePlacement{{Element: &diagv1.PlacedElement{ElementId: 7, ViewId: 2, Name: "Target"}}}},
				},
			}
			got := ExportImpactScene(scene, SceneExportOptions{})
			if strings.Contains(got, "el_2_70") || !strings.Contains(got, `"calls el_1_7"`) {
				t.Fatalf("cross-view target rewrote source or user label:\n%s", got)
			}
			var edge string
			for _, line := range strings.Split(got, "\n") {
				if strings.Contains(line, `"calls el_1_7"`) {
					edge = strings.TrimSpace(line)
				}
			}
			if !strings.HasPrefix(edge, "el_1_70") || !strings.HasSuffix(edge, "el_2_7") {
				t.Fatalf("wrong cross-view endpoints: %q", edge)
			}
		})
	}
}

func TestExportImpactSceneRendersBackendPlainSelection(t *testing.T) {
	t.Parallel()

	fixture := sceneFixture()
	// The backend has already suppressed authored context and its connectors.
	fixture.Views["51"].Placements = fixture.Views["51"].Placements[:1]
	fixture.Views["51"].Connectors = nil
	got := ExportImpactScene(impact.PublicScene(fixture, true), SceneExportOptions{})
	for _, want := range []string{
		`el_51_7("svc/auth<br/>modified +3 −1")`,
		`el_neg1_neg1("new.go<br/>added +1")`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("plain scene missing %q in:\n%s", want, got)
		}
	}
	// The unimpacted member drops out, taking its connector with it.
	if strings.Contains(got, `el_51_8("svc/db")`) || strings.Contains(got, "linkStyle") {
		t.Fatalf("plain scene kept unimpacted content:\n%s", got)
	}
}

func TestExportImpactSceneChangedLinkAfterUnchanged(t *testing.T) {
	t.Parallel()

	fixture := sceneFixture()
	fixture.Views["51"].Connectors = []*diagv1.Connector{
		{Id: 9, ViewId: 51, SourceElementId: 7, TargetElementId: 8, Label: proto.String("uses")},
		{Id: 1, ViewId: 51, SourceElementId: 7, TargetElementId: 8, Label: proto.String("calls"), Tags: []string{"change:modified"}},
	}
	got := ExportImpactScene(impact.PublicScene(fixture, true), SceneExportOptions{})
	if !strings.Contains(got, "linkStyle 1 stroke:#ecc94b") {
		t.Fatalf("changed second link must address index 1, got:\n%s", got)
	}
	if strings.Contains(got, "linkStyle 0") {
		t.Fatalf("unchanged first link must not be styled, got:\n%s", got)
	}
}

func TestExportImpactSceneKeepsUnknownLineCountsUnknown(t *testing.T) {
	scene := sceneFixture()
	scene.Views["51"].Placements[0].Overlay.LinesAdded = nil
	scene.Views["51"].Placements[0].Overlay.LinesRemoved = nil
	got := ExportImpactScene(impact.PublicScene(scene, true), SceneExportOptions{})
	if strings.Contains(got, "modified +0") || !strings.Contains(got, "svc/auth<br/>modified") {
		t.Fatalf("unknown counts invented: %s", got)
	}
}

func containerSceneFixture() *codeindexv1.ImpactScene {
	return &codeindexv1.ImpactScene{
		Tree: []*diagv1.View{{Id: 1, Name: "Workspace", Children: []*diagv1.View{
			{Id: 2, Name: "tld", OwnerElementId: proto.Int32(1)},
		}}},
		Views: map[string]*codeindexv1.SceneViewContent{
			"1": {Placements: []*codeindexv1.ScenePlacement{
				{Element: &diagv1.PlacedElement{ElementId: 1, ViewId: 1, Name: "tld"},
					Origin: codeindexv1.ImpactOrigin_IMPACT_ORIGIN_MAPPED, Role: codeindexv1.ImpactRole_IMPACT_ROLE_CONTAINER},
			}},
			"2": {Placements: []*codeindexv1.ScenePlacement{
				{Element: &diagv1.PlacedElement{ElementId: 2, ViewId: 2, Name: "main.go"}},
			}},
		},
	}
}

func TestExportImpactSceneFoldsIsolatedContainerIntoChildSubgraph(t *testing.T) {
	t.Parallel()
	scene := containerSceneFixture()
	before := proto.Clone(scene)
	got := ExportImpactScene(scene, SceneExportOptions{})
	if strings.Contains(got, "el_1_1") {
		t.Fatalf("redundant container node remains:\n%s", got)
	}
	for _, want := range []string{`subgraph view_1["Workspace"]`, `  subgraph view_2["tld"]`, `el_2_2("main.go")`} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q:\n%s", want, got)
		}
	}
	if !proto.Equal(scene, before) {
		t.Fatal("export mutated the canonical scene")
	}
}

func TestExportImpactSceneKeepsOnlyRelevantOwners(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		modify func(*codeindexv1.ImpactScene)
		want   string
	}{
		{"change overlay", func(scene *codeindexv1.ImpactScene) {
			scene.Views["1"].Placements[0].Overlay = &codeindexv1.ImpactSceneOverlay{Change: codeindexv1.ChangeKind_CHANGE_KIND_MODIFIED}
		}, "modified"},
		{"dependency context", func(scene *codeindexv1.ImpactScene) {
			scene.Views["1"].Placements[0].Role = codeindexv1.ImpactRole_IMPACT_ROLE_DEPENDENCY_CONTEXT
		}, ""},
		{"authored context", func(scene *codeindexv1.ImpactScene) {
			scene.Views["1"].Placements[0].Role = codeindexv1.ImpactRole_IMPACT_ROLE_AUTHORED_CONTEXT
		}, ""},
		{"child absent", func(scene *codeindexv1.ImpactScene) {
			scene.Tree[0].Children = nil
			delete(scene.Views, "2")
		}, ""},
		{"child without content", func(scene *codeindexv1.ImpactScene) {
			delete(scene.Views, "2")
		}, ""},
		{"same name different owner", func(scene *codeindexv1.ImpactScene) {
			scene.Tree[0].Children[0].OwnerElementId = proto.Int32(99)
		}, ""},
		{"outgoing connector", func(scene *codeindexv1.ImpactScene) {
			scene.Views["1"].Placements = append(scene.Views["1"].Placements,
				&codeindexv1.ScenePlacement{Element: &diagv1.PlacedElement{ElementId: 3, ViewId: 1, Name: "other"}})
			scene.Views["1"].Connectors = []*diagv1.Connector{{Id: 1, ViewId: 1, SourceElementId: 1, TargetElementId: 3, Tags: []string{"change:added"}}}
		}, "el_1_1 ==> el_1_3\nend\n\nlinkStyle 0 stroke:#48bb78"},
		{"incoming connector", func(scene *codeindexv1.ImpactScene) {
			scene.Views["1"].Placements = append(scene.Views["1"].Placements,
				&codeindexv1.ScenePlacement{Element: &diagv1.PlacedElement{ElementId: 3, ViewId: 1, Name: "other"}})
			scene.Views["1"].Connectors = []*diagv1.Connector{{Id: 1, ViewId: 1, SourceElementId: 3, TargetElementId: 1}}
		}, "el_1_3 --> el_1_1"},
		{"incoming cross-view connector", func(scene *codeindexv1.ImpactScene) {
			scene.Views["2"].Connectors = []*diagv1.Connector{{Id: 1, ViewId: 2, SourceElementId: 2, TargetElementId: 1}}
			scene.Views["2"].ConnectorTargets = map[string]*codeindexv1.ImpactPlacementRef{"1": {ViewId: 1, ElementId: 1}}
		}, "el_2_2 --> el_1_1"},
	} {
		t.Run(test.name, func(t *testing.T) {
			scene := containerSceneFixture()
			test.modify(scene)
			got := ExportImpactScene(scene, SceneExportOptions{})
			keep := test.name == "change overlay" || test.name == "dependency context" || test.name == "outgoing connector"
			if strings.Contains(got, `el_1_1("tld`) != keep || (keep && !strings.Contains(got, test.want)) {
				t.Fatalf("owner or independent information missing:\n%s", got)
			}
		})
	}
}

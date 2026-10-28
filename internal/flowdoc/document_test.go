package flowdoc

import (
	"os"
	"strings"
	"testing"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"google.golang.org/protobuf/proto"
	"gopkg.in/yaml.v3"
)

func TestCompleteRoundTrip(t *testing.T) {
	source, err := os.ReadFile("fixtures/checkout.flow.yaml")
	if err != nil {
		t.Fatal(err)
	}
	doc, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	diagram := PreviewResources(doc.Resources, 7)
	for i, e := range doc.Resources.Elements {
		p := diagram.Placements[i]
		p.Kind = proto.String(e.Kind)
		p.Description = proto.String(e.Description)
		p.Technology = proto.String(e.Technology)
		p.Tags = e.Tags
		p.PositionX = *e.X
		p.PositionY = *e.Y
		for _, link := range e.TechnologyConnectors {
			p.TechnologyLinks = append(p.TechnologyLinks, &diagv1.TechnologyLink{Type: link.Type, Label: link.Label, IsPrimaryIcon: link.Primary})
		}
	}
	f, err := Import(doc, 7, diagram, Catalog{})
	if err != nil {
		t.Fatal(err)
	}
	diagram.Connectors[0].Direction = "forward"
	diagram.Connectors[0].Style = "bezier"
	diagram.Connectors[0].Tags = []string{"storage"}
	encoded, err := Export(f, diagram, Catalog{}, true)
	if err != nil {
		t.Fatal(err)
	}
	exported, err := Parse(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if exported.Resources.Elements[0].TechnologyConnectors[0].Primary != true || exported.Resources.Connectors[0].Tags[0] != "storage" {
		t.Fatalf("lost resource metadata: %s", encoded)
	}
	again, err := Import(exported, 7, diagram, Catalog{})
	if err != nil {
		t.Fatal(err)
	}
	if !proto.Equal(f, again) {
		t.Fatalf("round trip changed tree\n%v\n%v", f, again)
	}
}
func TestStrictParsing(t *testing.T) {
	base := "format: tld-flow\nversion: 1\nname: Example\nsteps: []\n"
	cases := map[string]string{
		"unknown":            base + "surprise: true\n",
		"alias":              "format: tld-flow\nversion: 1\nname: &name Example\nsteps: []\n",
		"duplicate":          base + "name: Again\n",
		"multiple":           base + "---\nname: Second\n",
		"null":               strings.Replace(base, "steps: []", "steps: null", 1),
		"coercion":           strings.Replace(base, "Example", "123", 1),
		"version":            strings.Replace(base, "version: 1", "version: 2", 1),
		"bad enum":           "format: tld-flow\nversion: 1\nname: Example\nsteps:\n - branches:\n    mode: {}\n    paths: []\n",
		"both controls":      "format: tld-flow\nversion: 1\nname: Example\nsteps:\n - goToFlow: {name: Other}\n   branches: {mode: alternate, paths: [{name: A, steps: []}, {name: B, steps: []}]}\n",
		"duplicate IDs":      "format: tld-flow\nversion: 1\nname: Example\nsteps: [{id: same}, {id: same}]\n",
		"oversize":           base + strings.Repeat("#", MaxBytes),
		"utf8":               base + string([]byte{0xff}),
		"unpaired positions": base + "resources: {elements: [{name: API, position_x: 1}], connectors: []}\n",
	}
	for name, source := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]byte(source)); err == nil {
				t.Fatal("accepted invalid document")
			}
		})
	}
	if _, err := Parse([]byte(base)); err != nil {
		t.Fatal(err)
	}
}
func TestReferenceResolution(t *testing.T) {
	label := "writes"
	diagram := &diagv1.ViewContent{Placements: []*diagv1.PlacedElement{{ElementId: 1, Name: "API"}, {ElementId: 2, Name: "DB"}}, Connectors: []*diagv1.Connector{{Id: 3, SourceElementId: 1, TargetElementId: 2, Label: &label}, {Id: 4, SourceElementId: 1, TargetElementId: 2, Label: proto.String("reads")}}}
	doc := &Document{Name: "Example", Steps: []Step{{Connectors: []Connection{{Source: "API", Target: "DB"}}}}}
	if _, err := Import(doc, 1, diagram, Catalog{}); err == nil {
		t.Fatal("ambiguous connector accepted")
	}
	doc.Steps[0].Connectors[0].Label = &label
	f, err := Import(doc, 1, diagram, Catalog{})
	if err != nil {
		t.Fatal(err)
	}
	if f.Content.Steps[0].ConnectorIds[0] != 3 {
		t.Fatal("wrong connector")
	}
	diagram.Placements = append(diagram.Placements, &diagv1.PlacedElement{ElementId: 5, Name: "API"})
	if _, err = Import(doc, 1, diagram, Catalog{}); err == nil {
		t.Fatal("ambiguous element accepted")
	}
	if _, err = Export(f, diagram, Catalog{}, false); err == nil {
		t.Fatal("ambiguous export accepted")
	}
}

func TestPortableLimitsAndCrossViewLinks(t *testing.T) {
	// Check cumulative nested limits rather than only each individual array.
	steps := make([]Step, 999)
	for i := range steps {
		steps[i] = Step{}
	}
	steps = append(steps, Step{Branches: &Branches{Mode: "alternate", Paths: []Path{{Name: "A", Steps: []Step{{}}}, {Name: "B", Steps: []Step{}}}}})
	doc := Document{Format: "tld-flow", Version: 1, Name: "Limits", Steps: steps}
	data, err := yaml.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Parse(data); err == nil {
		t.Fatal("accepted more than 1000 total steps")
	}
	nested := []Step{{}}
	for range 9 {
		nested = []Step{{Branches: &Branches{Mode: "parallel", Paths: []Path{{Name: "A", Steps: nested}, {Name: "B", Steps: []Step{}}}}}}
	}
	doc.Steps = nested
	data, err = yaml.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = Parse(data); err == nil {
		t.Fatal("accepted excessive depth")
	}
	catalog := Catalog{Views: []*diagv1.View{{Id: 2, Name: "Payments"}}, Flows: []*diagv1.Flow{{Id: "linked-id", ViewId: 2, Content: &diagv1.FlowContent{Name: "Payment"}}}}
	doc.Steps = []Step{{ID: "link", GoToFlow: &Link{Name: "Payment", View: "Payments"}}}
	f, err := Import(&doc, 1, &diagv1.ViewContent{}, catalog)
	if err != nil {
		t.Fatal(err)
	}
	if f.Content.Steps[0].GetGoToFlowId() != "linked-id" {
		t.Fatal("cross-view link did not resolve")
	}
	data, err = Export(f, &diagv1.ViewContent{}, catalog, false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "linked-id") || !strings.Contains(string(data), "Payments") {
		t.Fatalf("export leaked identity or lost view: %s", data)
	}
}

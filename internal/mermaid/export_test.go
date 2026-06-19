package mermaid

import (
	"strings"
	"testing"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
)

func TestExportViewWithAndWithoutMetadata(t *testing.T) {
	t.Parallel()

	content := &diagv1.ViewContent{
		Placements: []*diagv1.PlacedElement{
			{
				ElementId:       2,
				ViewId:          42,
				PositionX:       320,
				PositionY:       80,
				Name:            `Database "Primary" & Cache`,
				Kind:            ptrString("database"),
				Description:     ptrString("stores\nrecords"),
				Technology:      ptrString("Postgres"),
				Tags:            []string{"backend", "data,tier"},
				HasView:         true,
				ViewLabel:       ptrString("Container"),
				BypassNoiseGate: true,
				TechnologyLinks: []*diagv1.TechnologyLink{{Type: "vendor", Slug: ptrString("postgres"), Label: "PostgreSQL", IsPrimaryIcon: true}},
			},
			{ElementId: 1, ViewId: 42, PositionX: 120, PositionY: 80, Name: "API"},
		},
		Connectors: []*diagv1.Connector{
			{
				Id:              9,
				ViewId:          42,
				SourceElementId: 1,
				TargetElementId: 2,
				Label:           ptrString(`reads "writes"`),
				Relationship:    ptrString("query"),
				Direction:       "forward",
				Style:           "straight",
				SourceHandle:    ptrString("right"),
				TargetHandle:    ptrString("left"),
			},
		},
	}

	withMetadata := ExportView(content, 42, true)
	for _, want := range []string{
		"flowchart LR",
		"%% tld/v1 view=42",
		`node_1["API"]`,
		`node_2["Database &quot;Primary&quot; &amp; Cache"]`,
		"%% tld-element ref=node_2 x=320 y=80 kind=database desc=stores\\nrecords tech=Postgres tags=backend,data\\,tier techLinks=vendor:postgres:PostgreSQL:1 bypass=1 hasView=1 viewLabel=Container",
		`node_1 -- "reads &quot;writes&quot;" --> node_2`,
		`%% tld-connector ref=9 source=node_1 target=node_2 label=reads "writes" rel=query style=straight`,
	} {
		if !strings.Contains(withMetadata, want) {
			t.Fatalf("ExportView() missing %q in:\n%s", want, withMetadata)
		}
	}

	withoutMetadata := ExportView(content, 42, false)
	if strings.Contains(withoutMetadata, "%% tld-") || strings.Contains(withoutMetadata, "%% tld/v1") {
		t.Fatalf("ExportView(include metadata=false) contains metadata:\n%s", withoutMetadata)
	}
}

func TestExportedMetadataParsesBack(t *testing.T) {
	t.Parallel()

	content := &diagv1.ViewContent{
		Placements: []*diagv1.PlacedElement{
			{ElementId: 1, ViewId: 42, PositionX: 120, PositionY: 80, Name: "API", Kind: ptrString("service"), Tags: []string{"backend"}},
			{ElementId: 2, ViewId: 42, PositionX: 320, PositionY: 80, Name: "DB", Kind: ptrString("database")},
		},
		Connectors: []*diagv1.Connector{
			{Id: 9, ViewId: 42, SourceElementId: 1, TargetElementId: 2, Label: ptrString("reads"), Relationship: ptrString("query"), Style: "straight"},
		},
	}
	parsed, err := Parse(ExportView(content, 42, true))
	if err != nil {
		t.Fatalf("Parse(exported) error = %v", err)
	}
	if len(parsed.Warnings) > 0 {
		t.Fatalf("Parse(exported) warnings = %v", parsed.Warnings)
	}
	if len(parsed.Elements) != 2 || len(parsed.Connectors) != 1 {
		t.Fatalf("Parse(exported) got %d elements/%d connectors", len(parsed.Elements), len(parsed.Connectors))
	}
	if got := parsed.Elements[0].GetRef(); got != "node_1" {
		t.Fatalf("first element ref = %q", got)
	}
	if got := parsed.Elements[0].GetKind(); got != "service" {
		t.Fatalf("first element kind = %q", got)
	}
	if got := parsed.Elements[0].GetTags(); len(got) != 1 || got[0] != "backend" {
		t.Fatalf("first element tags = %v", got)
	}
	if got := parsed.Elements[0].GetPlacements()[0].GetPositionX(); got != 120 {
		t.Fatalf("first element x = %v", got)
	}
	if got := parsed.Connectors[0].GetRelationship(); got != "query" {
		t.Fatalf("connector relationship = %q", got)
	}
}

package importer

import (
	"reflect"
	"testing"
)

func TestParseStructurizrHierarchicalModelResolvesScopedRelationships(t *testing.T) {
	input := `
workspace "Payments" {
  !identifiers hierarchical

  model {
    customer = person "Customer" "Buys things"

    payments = softwareSystem "Payments" {
      db = container "Database" "Stores data" "PostgreSQL"
      api = container "API" "Serves requests" "Go" {
        description "Public payments API"
        technology "Go / PostgreSQL"
        -> db "reads from" "SQL"
      }
    }

    customer -> payments.api "uses"
  }
}`

	got, err := ParseStructurizr(input)
	if err != nil {
		t.Fatalf("ParseStructurizr returned error: %v", err)
	}

	wantElements := []ParsedElement{
		{ID: "customer", Name: "Customer", Kind: "person", Description: "Buys things"},
		{ID: "payments", Name: "Payments", Kind: "system"},
		{ID: "payments.db", Name: "Database", Kind: "container", Description: "Stores data", Technology: "PostgreSQL"},
		{ID: "payments.api", Name: "API", Kind: "container", Description: "Public payments API", Technology: "Go / PostgreSQL"},
	}
	if !reflect.DeepEqual(got.Elements, wantElements) {
		t.Fatalf("elements mismatch\n got: %#v\nwant: %#v", got.Elements, wantElements)
	}

	wantConnectors := []ParsedConnector{
		{SourceID: "payments.api", TargetID: "payments.db", Label: "reads from", Technology: "SQL"},
		{SourceID: "customer", TargetID: "payments.api", Label: "uses"},
	}
	if !reflect.DeepEqual(got.Connectors, wantConnectors) {
		t.Fatalf("connectors mismatch\n got: %#v\nwant: %#v", got.Connectors, wantConnectors)
	}
	if len(got.Warnings) != 0 {
		t.Fatalf("unexpected warnings: %#v", got.Warnings)
	}
}

func TestParseStructurizrHandlesContinuationsCommentsAndArchetypeWarnings(t *testing.T) {
	input := `
workspace {
  model {
    // Relationship label is continued onto the next line.
    user = person "User"
    service = queueConsumer "Queue Consumer" "Processes jobs" {
      technology "Ruby"
    }
    user \
      -> service "submits jobs"
    /* unknown blocks should be ignored */
    views { systemContext service { include * } }
  }
}`

	got, err := ParseStructurizr(input)
	if err != nil {
		t.Fatalf("ParseStructurizr returned error: %v", err)
	}

	if len(got.Elements) != 2 {
		t.Fatalf("expected 2 elements, got %#v", got.Elements)
	}
	if got.Elements[1].ID != "service" || got.Elements[1].Kind != "element" || got.Elements[1].Technology != "Ruby" {
		t.Fatalf("archetype element was not preserved as generic element with body fields: %#v", got.Elements[1])
	}
	if len(got.Warnings) != 1 || got.Warnings[0] != `unknown element type "queueConsumer" (archetype instance), treating as element` {
		t.Fatalf("unexpected warnings: %#v", got.Warnings)
	}

	wantConnectors := []ParsedConnector{{SourceID: "user", TargetID: "service", Label: "submits jobs"}}
	if !reflect.DeepEqual(got.Connectors, wantConnectors) {
		t.Fatalf("connectors mismatch\n got: %#v\nwant: %#v", got.Connectors, wantConnectors)
	}
}

package planner_test

import (
	"testing"

	"github.com/mertcikla/tld/internal/planner"
	"github.com/mertcikla/tld/internal/workspace"
)

func TestProtoMapping_ElementFields(t *testing.T) {
	plan, err := planner.Build(&workspace.Workspace{
		Config: workspace.Config{WorkspaceID: "org-1"},
		Elements: map[string]*workspace.Element{
			"api": {
				Name:        "API",
				Kind:        "service",
				Description: "desc",
				Technology:  "Go",
				URL:         "https://example.com",
				LogoURL:     "https://example.com/logo.svg",
				Repo:        "repo",
				Branch:      "main",
				Language:    "go",
				FilePath:    "backend/main.go",
				HasView:     true,
				ViewLabel:   "Container",
				Placements: []workspace.ViewPlacement{{
					ParentRef: "root",
					PositionX: 42,
					PositionY: 21,
				}},
			},
		},
	}, false)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	element := plan.Request.Elements[0]
	if plan.Request.OrgId != "org-1" || element.Ref != "api" || element.Name != "API" {
		t.Fatalf("unexpected element mapping: %+v", element)
	}
	if element.Kind == nil || *element.Kind != "service" {
		t.Fatalf("Kind = %v", element.Kind)
	}
	if element.Url == nil || *element.Url != "https://example.com" {
		t.Fatalf("Url = %v", element.Url)
	}
	if element.LogoUrl == nil || *element.LogoUrl != "https://example.com/logo.svg" {
		t.Fatalf("LogoUrl = %v", element.LogoUrl)
	}
	if !element.HasView {
		t.Fatal("expected HasDiagram to be true")
	}
	if element.ViewLabel == nil || *element.ViewLabel != "Container" {
		t.Fatalf("ViewLabel = %v", element.ViewLabel)
	}
	if len(element.Placements) != 1 {
		t.Fatalf("Placements = %d", len(element.Placements))
	}
	if element.Placements[0].PositionX == nil || *element.Placements[0].PositionX != 42 {
		t.Fatalf("PositionX = %v", element.Placements[0].PositionX)
	}
}

func TestProtoMapping_ConnectorFields(t *testing.T) {
	plan, err := planner.Build(&workspace.Workspace{
		Elements: map[string]*workspace.Element{
			"a": {Name: "A", Kind: "service"},
			"b": {Name: "B", Kind: "database"},
		},
		Connectors: map[string]*workspace.Connector{
			"a:b": {
				View:         "root",
				Source:       "a",
				Target:       "b",
				Label:        "reads",
				Description:  "primary path",
				Relationship: "sync",
				Direction:    "forward",
				Style:        "solid",
				URL:          "https://example.com/flow",
				SourceHandle: "right",
				TargetHandle: "left",
			},
		},
	}, false)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	connector := plan.Request.Connectors[0]
	if connector.ViewRef != "root" || connector.SourceElementRef != "a" || connector.TargetElementRef != "b" {
		t.Fatalf("unexpected connector mapping: %+v", connector)
	}
	if connector.Label == nil || *connector.Label != "reads" {
		t.Fatalf("Label = %v", connector.Label)
	}
	if connector.Url == nil || *connector.Url != "https://example.com/flow" {
		t.Fatalf("Url = %v", connector.Url)
	}
}

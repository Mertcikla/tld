package planner_test

import (
	"testing"
	"time"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/mertcikla/tld/internal/planner"
	"github.com/mertcikla/tld/internal/workspace"
)

func elementWorkspace() *workspace.Workspace {
	return &workspace.Workspace{
		Config: workspace.Config{WorkspaceID: "test-org-id"},
		Elements: map[string]*workspace.Element{
			"platform": {
				Name:      "Platform",
				Kind:      "workspace",
				HasView:   true,
				ViewLabel: "System",
			},
			"api": {
				Name:        "API",
				Kind:        "service",
				Description: "Handles traffic",
				Technology:  "Go",
				URL:         "https://example.com/api",
				HasView:     true,
				Placements: []workspace.ViewPlacement{{
					ParentRef: "platform",
					PositionX: 120,
					PositionY: 240,
				}},
			},
			"db": {
				Name:       "DB",
				Kind:       "database",
				Placements: []workspace.ViewPlacement{{ParentRef: "platform"}},
			},
		},
		Connectors: map[string]*workspace.Connector{
			"platform:api:db:reads": {
				View:         "platform",
				Source:       "api",
				Target:       "db",
				Label:        "reads",
				Direction:    "forward",
				Relationship: "sync",
			},
		},
	}
}

func TestBuild_EmptyWorkspace(t *testing.T) {
	plan, err := planner.Build(&workspace.Workspace{Config: workspace.Config{WorkspaceID: "test-org-id"}}, false)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if plan.Request.OrgId != "test-org-id" {
		t.Fatalf("OrgId = %q", plan.Request.OrgId)
	}
	if len(plan.Request.Elements) != 0 || len(plan.Request.Connectors) != 0 {
		t.Fatalf("expected empty workspace request, got %+v", plan.Request)
	}
	if plan.Model != "workspace" {
		t.Fatalf("Model = %q", plan.Model)
	}
}

func TestBuild_MapsElementsAndConnectors(t *testing.T) {
	plan, err := planner.Build(elementWorkspace(), false)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(plan.Request.Elements) != 3 {
		t.Fatalf("Elements = %d, want 3", len(plan.Request.Elements))
	}
	if len(plan.Request.Connectors) != 1 {
		t.Fatalf("Connectors = %d, want 1", len(plan.Request.Connectors))
	}

	var apiFound bool
	for _, element := range plan.Request.Elements {
		if element.Ref != "api" {
			continue
		}
		apiFound = true
		if element.Kind == nil || *element.Kind != "service" {
			t.Fatalf("api kind = %v", element.Kind)
		}
		if element.ViewId != nil {
			t.Fatalf("unexpected view id on fresh workspace")
		}
		if len(element.Placements) != 1 {
			t.Fatalf("api placements = %d", len(element.Placements))
		}
		placement := element.Placements[0]
		if placement.ParentRef != "platform" {
			t.Fatalf("ParentRef = %q", placement.ParentRef)
		}
		if placement.PositionX == nil || *placement.PositionX != 120 {
			t.Fatalf("PositionX = %v", placement.PositionX)
		}
	}
	if !apiFound {
		t.Fatal("api element missing")
	}

	connector := plan.Request.Connectors[0]
	if connector.ViewRef != "platform" {
		t.Fatalf("ViewRef = %q", connector.ViewRef)
	}
	if connector.SourceElementRef != "api" || connector.TargetElementRef != "db" {
		t.Fatalf("unexpected connector endpoints: %+v", connector)
	}
}

func TestBuild_PromotesPlacementParentsToViews(t *testing.T) {
	ws := &workspace.Workspace{
		Config: workspace.Config{WorkspaceID: "test-org-id"},
		Elements: map[string]*workspace.Element{
			"cmd": {
				Name: "cmd",
				Kind: "folder",
			},
			"analyze-go": {
				Name:       "analyze-go",
				Kind:       "service",
				Placements: []workspace.ViewPlacement{{ParentRef: "cmd"}},
			},
		},
	}

	plan, err := planner.Build(ws, false)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	var cmd *diagv1.PlanElement
	for _, element := range plan.Request.Elements {
		if element.Ref == "cmd" {
			cmd = element
			break
		}
	}
	if cmd == nil {
		t.Fatal("cmd element missing")
	}
	if !cmd.HasView {
		t.Fatal("expected placement parent to be promoted to a canonical view")
	}
}

func TestBuild_ReusesMetadataIDs(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	ws := elementWorkspace()
	ws.Meta = &workspace.Meta{
		Elements: map[string]*workspace.ResourceMetadata{
			"api": {ID: 101, UpdatedAt: now},
		},
		Views: map[string]*workspace.ResourceMetadata{
			"api": {ID: 202, UpdatedAt: now},
		},
		Connectors: map[string]*workspace.ResourceMetadata{
			"platform:api:db:reads": {ID: 303, UpdatedAt: now},
		},
	}

	plan, err := planner.Build(ws, false)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	var api *diagv1.PlanElement
	for _, element := range plan.Request.Elements {
		if element.Ref == "api" {
			api = element
			break
		}
	}
	if api == nil {
		t.Fatal("api element missing")
	}
	if api.Id == nil || *api.Id != 101 {
		t.Fatalf("api id = %v", api.Id)
	}
	if api.ViewId == nil || *api.ViewId != 202 {
		t.Fatalf("api view id = %v", api.ViewId)
	}
	if plan.Request.Connectors[0].Id == nil || *plan.Request.Connectors[0].Id != 303 {
		t.Fatalf("connector id = %v", plan.Request.Connectors[0].Id)
	}
}

func TestBuild_FiltersByOwnerForActiveRepo(t *testing.T) {
	ws := elementWorkspace()
	ws.ActiveRepo = "frontend"
	ws.Elements["api"].Owner = "frontend"
	ws.Elements["db"].Owner = "backend"
	ws.Connectors["platform:api:db:reads"].Source = "api"
	ws.Connectors["platform:api:db:reads"].Target = "db"

	plan, err := planner.Build(ws, false)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(plan.Request.Elements) != 2 {
		t.Fatalf("elements = %d, want 2", len(plan.Request.Elements))
	}
	if len(plan.Request.Connectors) != 0 {
		t.Fatalf("connectors = %d, want 0 when target owner is excluded", len(plan.Request.Connectors))
	}
}

func TestBuild_RejectsMissingConfiguredRepositoryRoot(t *testing.T) {
	ws := elementWorkspace()
	ws.ActiveRepo = "frontend"
	ws.WorkspaceConfig = &workspace.WorkspaceConfig{
		Repositories: map[string]workspace.Repository{
			"frontend": {Root: "missing-root"},
		},
	}

	_, err := planner.Build(ws, false)
	if err == nil {
		t.Fatal("expected build to fail for missing configured repository root")
	}
	if got := err.Error(); got == "" || got != "repository \"frontend\" root \"missing-root\" not found in elements" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBuild_SynthesizesRepositoryRootWhenUnset(t *testing.T) {
	ws := &workspace.Workspace{
		Config:     workspace.Config{WorkspaceID: "test-org-id"},
		ActiveRepo: "frontend",
		WorkspaceConfig: &workspace.WorkspaceConfig{
			Repositories: map[string]workspace.Repository{
				"frontend": {},
			},
		},
	}

	plan, err := planner.Build(ws, false)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(plan.Request.Elements) != 1 {
		t.Fatalf("Elements = %d, want 1", len(plan.Request.Elements))
	}
	root := plan.Request.Elements[0]
	if root.Ref != "frontend" {
		t.Fatalf("root ref = %q, want frontend", root.Ref)
	}
	if root.Kind == nil || *root.Kind != "repository" {
		t.Fatalf("root kind = %v, want repository", root.Kind)
	}
	if !root.HasView {
		t.Fatal("expected synthesized repository root to own a view")
	}
	if len(root.Placements) != 1 || root.Placements[0].ParentRef != "root" {
		t.Fatalf("root placements = %+v, want parent root", root.Placements)
	}
}

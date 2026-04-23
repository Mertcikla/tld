package workspace_test

import (
	"strings"
	"testing"

	"github.com/mertcikla/tld/internal/workspace"
)

func buildWorkspace(elements map[string]*workspace.Element, connectors map[string]*workspace.Connector) *workspace.Workspace {
	if elements == nil {
		elements = map[string]*workspace.Element{}
	}
	if connectors == nil {
		connectors = map[string]*workspace.Connector{}
	}
	return &workspace.Workspace{Elements: elements, Connectors: connectors}
}

func containsValidationError(errs []workspace.ValidationError, location, message string) bool {
	for _, err := range errs {
		if err.Location == location && strings.Contains(err.Message, message) {
			return true
		}
	}
	return false
}

func containsValidationMessage(errs []workspace.ValidationError, message string) bool {
	for _, err := range errs {
		if strings.Contains(err.Message, message) {
			return true
		}
	}
	return false
}

func TestValidate_EmptyWorkspace(t *testing.T) {
	if errs := buildWorkspace(nil, nil).Validate(); len(errs) != 0 {
		t.Fatalf("expected 0 errors, got %v", errs)
	}
}

func TestValidate_ElementNameRequired(t *testing.T) {
	err := buildWorkspace(map[string]*workspace.Element{"api": {Kind: "service"}}, nil).Validate()
	if !containsValidationError(err, "elements.yaml[api]", "name is required") {
		t.Fatalf("expected missing name error, got %v", err)
	}
}

func TestValidate_ElementKindRequired(t *testing.T) {
	err := buildWorkspace(map[string]*workspace.Element{"api": {Name: "API"}}, nil).Validate()
	if !containsValidationError(err, "elements.yaml[api]", "kind is required") {
		t.Fatalf("expected missing kind error, got %v", err)
	}
}

func TestValidate_DuplicateElementNames(t *testing.T) {
	err := buildWorkspace(map[string]*workspace.Element{
		"api":     {Name: "API", Kind: "service"},
		"api-dup": {Name: "API", Kind: "service"},
	}, nil).Validate()
	if !containsValidationMessage(err, "duplicate element name") {
		t.Fatalf("expected duplicate name error, got %v", err)
	}
}

func TestValidate_ElementOwnerMustBeRegisteredRepository(t *testing.T) {
	ws := buildWorkspace(map[string]*workspace.Element{
		"api": {Name: "API", Kind: "service", Owner: "frontend"},
	}, nil)
	ws.WorkspaceConfig = &workspace.WorkspaceConfig{Repositories: map[string]workspace.Repository{}}
	err := ws.Validate()
	if !containsValidationMessage(err, "owner \"frontend\" is not a registered repository") {
		t.Fatalf("expected owner validation error, got %v", err)
	}
}

func TestValidate_ElementPlacementParentRequired(t *testing.T) {
	err := buildWorkspace(map[string]*workspace.Element{
		"api": {Name: "API", Kind: "service", Placements: []workspace.ViewPlacement{{ParentRef: ""}}},
	}, nil).Validate()
	if !containsValidationError(err, "elements.yaml[api][placements][0]", "parent is required") {
		t.Fatalf("expected missing parent error, got %v", err)
	}
}

func TestValidate_ElementPlacementParentRefNotFound(t *testing.T) {
	err := buildWorkspace(map[string]*workspace.Element{
		"api": {Name: "API", Kind: "service", Placements: []workspace.ViewPlacement{{ParentRef: "missing"}}},
	}, nil).Validate()
	if !containsValidationMessage(err, "parent ref \"missing\" not found") {
		t.Fatalf("expected missing parent ref error, got %v", err)
	}
}

func TestValidate_ConnectorViewSourceAndTargetIntegrity(t *testing.T) {
	ws := buildWorkspace(map[string]*workspace.Element{
		"api": {Name: "API", Kind: "service"},
	}, map[string]*workspace.Connector{
		"bad": {View: "missing-view", Source: "missing-source", Target: "missing-target"},
	})
	err := ws.Validate()
	if !containsValidationMessage(err, "view ref \"missing-view\" not found") ||
		!containsValidationMessage(err, "source ref \"missing-source\" not found") ||
		!containsValidationMessage(err, "target ref \"missing-target\" not found") {
		t.Fatalf("expected connector integrity errors, got %v", err)
	}
}

func TestValidate_ConnectorRequiredFields(t *testing.T) {
	err := buildWorkspace(map[string]*workspace.Element{
		"api": {Name: "API", Kind: "service"},
	}, map[string]*workspace.Connector{
		"bad": {},
	}).Validate()
	if !containsValidationError(err, "connectors.yaml[bad]", "view is required") ||
		!containsValidationError(err, "connectors.yaml[bad]", "source is required") ||
		!containsValidationError(err, "connectors.yaml[bad]", "target is required") {
		t.Fatalf("expected connector required-field errors, got %v", err)
	}
}

func TestValidate_ConflictMarkers(t *testing.T) {
	err := buildWorkspace(map[string]*workspace.Element{
		"api": {Name: "<<< LOCAL", Kind: "service"},
	}, map[string]*workspace.Connector{
		"root:api:db:reads": {View: "root", Source: "api", Target: "db", Label: ">>> SERVER"},
	}).Validate()
	if !containsValidationMessage(err, "unresolved merge conflict") {
		t.Fatalf("expected conflict marker error, got %v", err)
	}
}

func TestValidate_RootPlacementsAndRootViewAreAllowed(t *testing.T) {
	ws := buildWorkspace(map[string]*workspace.Element{
		"api": {Name: "API", Kind: "service", Placements: []workspace.ViewPlacement{{ParentRef: "root"}}},
		"db":  {Name: "DB", Kind: "database", Placements: []workspace.ViewPlacement{{ParentRef: "root"}}},
	}, map[string]*workspace.Connector{
		"root:api:db:reads": {View: "root", Source: "api", Target: "db", Label: "reads"},
	})
	if errs := ws.Validate(); len(errs) != 0 {
		t.Fatalf("expected valid workspace, got %v", errs)
	}
}

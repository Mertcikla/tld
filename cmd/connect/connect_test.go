package connect_test

import (
	"github.com/mertcikla/diag/tld/cmd"
	"os"
	"path/filepath"
	"testing"

	"github.com/mertcikla/diag/tld/internal/workspace"
	"gopkg.in/yaml.v3"
)

// setupWorkspaceForLinks creates an element workspace with two children on the same parent diagram.
func setupWorkspaceForLinks(t *testing.T, dir string) {
	t.Helper()
	cmd.MustInitWorkspace(t, dir)
	cmd.MustRunCmd(t, dir, "add", "Platform", "--ref", "platform", "--kind", "workspace")
	cmd.MustRunCmd(t, dir, "add", "API", "--ref", "api", "--parent", "platform", "--kind", "service")
	cmd.MustRunCmd(t, dir, "add", "DB", "--ref", "db", "--parent", "platform", "--kind", "database")
}

func TestConnectCmd_AppendsConnector(t *testing.T) {
	dir := t.TempDir()
	setupWorkspaceForLinks(t, dir)

	_, _, err := cmd.RunCmd(t, dir, "connect", "--from", "api", "--to", "db")
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatalf("load workspace: %v", err)
	}
	connectors := ws.Connectors
	if len(connectors) != 1 {
		t.Fatalf("len(connectors) = %d, want 1", len(connectors))
	}
	connector := connectors["platform:api:db:"]
	if connector == nil || connector.View != "platform" || connector.Source != "api" || connector.Target != "db" {
		t.Errorf("unexpected connector: %+v", connector)
	}
}

func TestConnectCmd_RootElementsInferRootView(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.MustRunCmd(t, dir, "add", "API", "--ref", "api", "--kind", "service")
	cmd.MustRunCmd(t, dir, "add", "DB", "--ref", "db", "--kind", "database")

	_, _, err := cmd.RunCmd(t, dir, "connect", "--from", "api", "--to", "db")
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatalf("load workspace: %v", err)
	}
	connectors := ws.Connectors
	if len(connectors) != 1 {
		t.Fatalf("len(connectors) = %d, want 1", len(connectors))
	}
	connector := connectors["root:api:db:"]
	if connector == nil || connector.View != "root" {
		t.Errorf("unexpected connector: %+v", connector)
	}
}

func TestConnectCmd_TwoCallsTwoEntries(t *testing.T) {
	dir := t.TempDir()
	setupWorkspaceForLinks(t, dir)

	_, _, err := cmd.RunCmd(t, dir, "connect", "--from", "api", "--to", "db")
	if err != nil {
		t.Fatalf("first connect: %v", err)
	}
	_, _, err = cmd.RunCmd(t, dir, "connect", "--from", "db", "--to", "api")
	if err != nil {
		t.Fatalf("second connect: %v", err)
	}

	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatalf("load workspace: %v", err)
	}
	connectors := ws.Connectors

	if len(connectors) != 2 {
		t.Fatalf("len(connectors) = %d, want 2", len(connectors))
	}
}

func TestConnectCmd_ElementsInDifferentViewsSucceeds(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.MustRunCmd(t, dir, "add", "Parent1", "--ref", "parent1", "--kind", "workspace")
	cmd.MustRunCmd(t, dir, "add", "Parent2", "--ref", "parent2", "--kind", "workspace")
	cmd.MustRunCmd(t, dir, "add", "API", "--ref", "api", "--parent", "parent1", "--kind", "service")
	cmd.MustRunCmd(t, dir, "add", "DB", "--ref", "db", "--parent", "parent2", "--kind", "database")

	_, _, err := cmd.RunCmd(t, dir, "connect", "--from", "api", "--to", "db")
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatalf("load workspace: %v", err)
	}
	connectors := ws.Connectors
	connector := connectors["root:api:db:"]
	if connector == nil || connector.View != "root" {
		t.Errorf("expected connector in root view, got %+v", connector)
	}
}

func TestConnectCmd_ElementsWithMultiplePlacementsSucceeds(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	// Create an element with 2 placements manually in elements.yaml
	elements := map[string]*workspace.Element{
		"api": {
			Name: "API", Kind: "service",
			Placements: []workspace.ViewPlacement{
				{ParentRef: "root"},
				{ParentRef: "other"},
			},
		},
		"db": {
			Name: "DB", Kind: "database",
			Placements: []workspace.ViewPlacement{
				{ParentRef: "other"},
			},
		},
	}
	data, err := yaml.Marshal(elements)
	if err != nil {
		t.Fatalf("marshal elements: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "elements.yaml"), data, 0600); err != nil {
		t.Fatalf("write elements.yaml: %v", err)
	}

	_, _, err = cmd.RunCmd(t, dir, "connect", "--from", "api", "--to", "db")
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatalf("load workspace: %v", err)
	}
	connectors := ws.Connectors
	connector := connectors["other:api:db:"]
	if connector == nil || connector.View != "other" {
		t.Errorf("expected connector in 'other' view (shared parent), got %+v", connector)
	}
}

func TestConnectCmd_MissingFromFlag(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)

	_, _, err := cmd.RunCmd(t, dir, "connect", "--to", "db")
	if err == nil {
		t.Fatal("expected error for missing --from")
	}
}

func TestConnectCmd_MissingToFlag(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)

	_, _, err := cmd.RunCmd(t, dir, "connect", "--from", "api")
	if err == nil {
		t.Fatal("expected error for missing --to")
	}
}

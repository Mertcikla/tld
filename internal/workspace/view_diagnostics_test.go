package workspace_test

import (
	"testing"

	"github.com/mertcikla/tld/v2/internal/workspace"
)

func TestUnplacedPersistedEndpointHasNoImplicitRootPlacement(t *testing.T) {
	ws := &workspace.Workspace{
		Elements: map[string]*workspace.Element{"pending": {Name: "Pending"}, "unplaced": {Name: "Unplaced"}},
		Meta:     &workspace.Meta{Elements: map[string]*workspace.ResourceMetadata{"unplaced": {ID: 1}}},
	}
	if !ws.ElementPlacedInView("pending", "root") || ws.ElementPlacedInView("unplaced", "root") {
		t.Fatal("implicit root placement must apply only to unapplied YAML elements")
	}
	d := ws.DiagnoseConnector(&workspace.Connector{View: "root", Source: "pending", Target: "unplaced"})
	if d.Mode != "off-view" || len(d.EndpointViews["unplaced"]) != 0 || len(d.OffViewEndpoints) != 1 {
		t.Fatalf("unplaced endpoint reported as directly placed: %+v", d)
	}
	if got := d.Description(); got != "off-view (unplaced unplaced)" {
		t.Fatalf("incorrect unplaced endpoint description: %q", got)
	}
}

func TestConnectorVisibilitySeparatesOffViewFromUnresolved(t *testing.T) {
	ws := &workspace.Workspace{Elements: map[string]*workspace.Element{
		"runtime": {Name: "Runtime", HasView: true},
		"api":     {Name: "API", Placements: []workspace.ViewPlacement{{ParentRef: "runtime"}}},
		"db":      {Name: "Database", Placements: []workspace.ViewPlacement{{ParentRef: "root"}, {ParentRef: "storage"}}},
	}, Connectors: map[string]*workspace.Connector{}}
	for _, tt := range []struct{ source, target, mode string }{
		{"api", "api", "on-view"},
		{"api", "db", "off-view"},
		{"db", "runtime", "off-view"},
		{"api", "missing", "unresolved"},
	} {
		c := &workspace.Connector{View: "runtime", Source: tt.source, Target: tt.target}
		d := ws.DiagnoseConnector(c)
		if d.Mode != tt.mode {
			t.Fatalf("%s -> %s: %+v", tt.source, tt.target, d)
		}
		ws.Connectors[workspace.ConnectorKey(c)] = c
	}
	d := ws.DiagnoseView("runtime")
	if d.RenderedConnectors != 1 || len(d.OffViewConnectors) != 2 || len(d.UnresolvedConnectors) != 1 || len(d.OmittedConnectors) != 3 {
		t.Fatalf("incorrect export breakdown: %+v", d)
	}
}

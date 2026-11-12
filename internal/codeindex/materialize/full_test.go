package materialize

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/mertcikla/codeindex/community"
	cgraph "github.com/mertcikla/codeindex/graph"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/core"
)

func TestFullMapStopsAtFiles(t *testing.T) {
	ws, idx := openGroupMapStore(t)
	ctx := context.Background()
	files := []community.File{{ID: "a", Path: "src/a.go", DisplayName: "src/a.go", Language: "go"}}
	edges := []MapEdge{}
	for i := 0; i < 45; i++ {
		id := fmt.Sprintf("b-%02d", i)
		files = append(files, community.File{ID: id, Path: "lib/" + id + ".go", Language: "go"})
		edges = append(edges, MapEdge{FromFactID: "a", ToFactID: id, Kind: "calls"})
	}
	edges = append(edges, edges[0], MapEdge{FromFactID: "a", ToFactID: "b-00", Kind: "references"}, MapEdge{FromFactID: "b-00", ToFactID: "a", Kind: "calls"}, MapEdge{FromFactID: "a", ToFactID: "a", Kind: "calls"}, MapEdge{FromFactID: "a", ToFactID: "unresolved", Kind: "references"})
	input := GroupMapInput{Full: true, RepositoryID: "repo", RepositoryName: "demo", SnapshotID: "one", Files: files, Edges: edges}
	result, err := ApplyGroupMap(ctx, ws, idx, input, MapOptions{MaxConnectorsPerView: 1})
	if err != nil {
		t.Fatal(err)
	}
	if result.Connectors != 48 {
		t.Fatalf("connectors=%d want 48", result.Connectors)
	}
	mappings, err := idx.MappingsByRepository(ctx, "repo")
	if err != nil {
		t.Fatal(err)
	}
	resources := map[string]int64{}
	for _, m := range mappings {
		resources[m.LogicalKey] = m.ResourceID
		switch m.Kind {
		case cstore.MappingElement:
			e, err := ws.ElementByID(ctx, m.ResourceID)
			if err != nil {
				t.Fatal(err)
			}
			if e.Kind == nil || (*e.Kind != "repository" && *e.Kind != "directory" && *e.Kind != "file") {
				t.Fatalf("non-file leaf materialized: %+v", e)
			}
			if *e.Kind == "file" && e.HasView {
				t.Fatalf("file got a child view: %+v", e)
			}
		case cstore.MappingView:
			if strings.HasPrefix(m.LogicalKey, "map|fact|") {
				t.Fatalf("file child view materialized: %s", m.LogicalKey)
			}
		case cstore.MappingConnector:
			c, err := ws.ConnectorByID(ctx, m.ResourceID)
			if err != nil {
				t.Fatal(err)
			}
			if (c.Label != nil && *c.Label != "") || (c.Relationship != nil && *c.Relationship != "") {
				t.Fatalf("dependency still labeled: %+v", c)
			}
			if c.Direction != "forward" || c.SourceElementID == c.TargetElementID {
				t.Fatalf("incorrect file connector: %+v", c)
			}
		}
	}
	file, err := ws.ElementByID(ctx, resources["map|fact|repo|a"])
	if err != nil {
		t.Fatal(err)
	}
	if file.Name != "a.go" || file.FilePath == nil || *file.FilePath != "src/a.go" {
		t.Fatalf("file presentation=%+v", file)
	}
	placements, err := ws.ElementPlacements(ctx, resources["map|directory|repo|src|view"])
	if err != nil {
		t.Fatal(err)
	}
	if len(placements) != 1 || placements[0].ElementID != file.ID {
		t.Fatalf("file lost its folder: %+v", placements)
	}
	input.SnapshotID = "two"
	if _, err := ApplyGroupMap(ctx, ws, idx, input, MapOptions{}); err != nil {
		t.Fatal(err)
	}
	after, err := idx.MappingsByRepository(ctx, "repo")
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(mappings) {
		t.Fatalf("rerun added resources: %d -> %d", len(mappings), len(after))
	}
	for _, m := range after {
		if resources[m.LogicalKey] != m.ResourceID {
			t.Fatalf("replaced %s", m.LogicalKey)
		}
	}
}

func TestFullFileMapRetiresOldSymbolProjection(t *testing.T) {
	ws, idx := openGroupMapStore(t)
	ctx := context.Background()
	input := GroupMapInput{Full: true, RepositoryID: "repo", RepositoryName: "demo", SnapshotID: "one", Files: []community.File{{ID: "file", Path: "src/a.go", DisplayName: "src/a.go"}}}
	if _, err := ApplyGroupMap(ctx, ws, idx, input, MapOptions{}); err != nil {
		t.Fatal(err)
	}
	mapping, _, err := idx.MappingByLogicalKey(ctx, "map|fact|repo|file")
	if err != nil {
		t.Fatal(err)
	}
	fileView, err := ws.CreateView(ctx, "a.go", optionalStr("Map"), &mapping.ResourceID)
	if err != nil {
		t.Fatal(err)
	}
	symbol, err := ws.CreateElement(ctx, core.LibraryElement{Name: "Run", Kind: optionalStr("function")})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ws.AddPlacement(ctx, fileView.ID, symbol.ID, 0, 0); err != nil {
		t.Fatal(err)
	}
	symbolView, err := ws.CreateView(ctx, "Run", optionalStr("Map"), &symbol.ID)
	if err != nil {
		t.Fatal(err)
	}
	connector, err := ws.CreateConnector(ctx, core.Connector{ViewID: fileView.ID, SourceElementID: symbol.ID, TargetElementID: symbol.ID, Direction: "forward", Style: "bezier"})
	if err != nil {
		t.Fatal(err)
	}
	authored, err := ws.CreateView(ctx, "Authored", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ws.AddPlacement(ctx, authored.ID, symbol.ID, 10, 20); err != nil {
		t.Fatal(err)
	}
	// An authored child view must survive even if its symbol leaves the map.
	description := "Authored documentation"
	if _, err := ws.UpdateView(ctx, symbolView.ID, nil, &description, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := idx.SaveMappings(ctx, []cstore.ResourceMapping{
		{LogicalKey: "map|fact|repo|file|view", Kind: cstore.MappingView, ResourceID: fileView.ID, RepositoryID: "repo", SnapshotID: "one"},
		{LogicalKey: "map|fact|repo|symbol", Kind: cstore.MappingElement, ResourceID: symbol.ID, RepositoryID: "repo", SnapshotID: "one"},
		{LogicalKey: "map|fact|repo|symbol|view", Kind: cstore.MappingView, ResourceID: symbolView.ID, RepositoryID: "repo", SnapshotID: "one"},
		{LogicalKey: "map|symbol-edge|repo|old", Kind: cstore.MappingConnector, ResourceID: connector.ID, RepositoryID: "repo", SnapshotID: "one"},
	}); err != nil {
		t.Fatal(err)
	}
	// Older projections may have touched timestamps and stale status already.
	if _, err := ws.UpdateView(ctx, fileView.ID, nil, nil, nil, []string{StaleTag}); err != nil {
		t.Fatal(err)
	}
	input.SnapshotID = "two"
	if _, err := ApplyGroupMap(ctx, ws, idx, input, MapOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := ws.ViewByID(ctx, fileView.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("file child view retained: %v", err)
	}
	if _, err := ws.ConnectorByID(ctx, connector.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("symbol connector retained: %v", err)
	}
	if _, err := ws.ElementByID(ctx, symbol.ID); err != nil {
		t.Fatalf("library symbol deleted: %v", err)
	}
	view, err := ws.ViewByID(ctx, symbolView.ID)
	if err != nil || view.Description == nil || *view.Description != description {
		t.Fatalf("authored view changed: %+v %v", view, err)
	}
	placements, err := ws.ElementPlacements(ctx, authored.ID)
	if err != nil || len(placements) != 1 || placements[0].ElementID != symbol.ID {
		t.Fatalf("authored placement changed: %+v %v", placements, err)
	}
	if _, err := ApplyGroupMap(ctx, ws, idx, input, MapOptions{}); err != nil {
		t.Fatalf("retry with retained mappings: %v", err)
	}
}

func TestFullMapMergesExistingCallsAndReferences(t *testing.T) {
	ws, idx := openGroupMapStore(t)
	ctx := context.Background()
	input := GroupMapInput{Full: true, RepositoryID: "repo", RepositoryName: "demo", SnapshotID: "one", Files: []community.File{{ID: "a", Path: "src/a.go"}, {ID: "b", Path: "src/b.go"}}}
	if _, err := ApplyGroupMap(ctx, ws, idx, input, MapOptions{}); err != nil {
		t.Fatal(err)
	}
	a, _, err := idx.MappingByLogicalKey(ctx, "map|fact|repo|a")
	if err != nil {
		t.Fatal(err)
	}
	b, _, err := idx.MappingByLogicalKey(ctx, "map|fact|repo|b")
	if err != nil {
		t.Fatal(err)
	}
	view, _, err := idx.MappingByLogicalKey(ctx, "map|directory|repo|src|view")
	if err != nil {
		t.Fatal(err)
	}
	old := map[string]int64{}
	for _, kind := range []string{"calls", "references"} {
		description := "Existing route documentation"
		c, err := ws.CreateConnector(ctx, core.Connector{ViewID: view.ResourceID, SourceElementID: a.ResourceID, TargetElementID: b.ResourceID, Direction: "forward", Style: "bezier", Label: &kind, Relationship: &kind, Description: &description})
		if err != nil {
			t.Fatal(err)
		}
		old[kind] = c.ID
		if err := idx.SaveMappings(ctx, []cstore.ResourceMapping{{LogicalKey: "map|file-edge|repo|" + cgraph.ID("a", "b", kind), Kind: cstore.MappingConnector, ResourceID: c.ID, RepositoryID: "repo", SnapshotID: "one"}}); err != nil {
			t.Fatal(err)
		}
	}
	authoredLabel := "Manually authored connection"
	authored, err := ws.CreateConnector(ctx, core.Connector{ViewID: view.ResourceID, SourceElementID: a.ResourceID, TargetElementID: b.ResourceID, Direction: "forward", Style: "bezier", Label: &authoredLabel})
	if err != nil {
		t.Fatal(err)
	}
	input.Edges = []MapEdge{{FromFactID: "a", ToFactID: "b", Kind: "references", SourceLines: []uint32{30, 12}, TargetLines: []uint32{8}}, {FromFactID: "a", ToFactID: "b", Kind: "calls", SourceLines: []uint32{12}, TargetLines: []uint32{6, 8}}, {FromFactID: "a", ToFactID: "b", Kind: "implements"}, {FromFactID: "b", ToFactID: "a", Kind: "references"}}
	for i := 0; i < 3; i++ {
		input.SnapshotID = fmt.Sprint(i + 2)
		result, err := ApplyGroupMap(ctx, ws, idx, input, MapOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if result.Connectors != 3 {
			t.Fatalf("connectors=%d want 3", result.Connectors)
		}
		merged, _, err := idx.MappingByLogicalKey(ctx, "map|file-edge|repo|"+cgraph.ID("a", "b", ""))
		if err != nil {
			t.Fatal(err)
		}
		if merged.ResourceID != old["calls"] {
			t.Fatalf("existing connector replaced: %d -> %d", old["calls"], merged.ResourceID)
		}
		c, err := ws.ConnectorByID(ctx, merged.ResourceID)
		if err != nil {
			t.Fatal(err)
		}
		if (c.Label != nil && *c.Label != "") || (c.Relationship != nil && *c.Relationship != "") {
			t.Fatalf("generated labels retained: %+v", c)
		}
		if c.SourceHandle == nil || *c.SourceHandle != "right-line-12,30" || c.TargetHandle == nil || *c.TargetHandle != "left-line-6,8" {
			t.Fatalf("existing connector did not receive all symbol anchors: %+v", c)
		}
		if c.Description == nil || *c.Description != "Existing route documentation" {
			t.Fatal("connector documentation lost")
		}
		for _, tag := range c.Tags {
			if tag == StaleTag {
				t.Fatal("reused connector marked stale by old ownership key")
			}
		}
		if _, err := ws.ConnectorByID(ctx, old["references"]); !errors.Is(err, sql.ErrNoRows) {
			t.Fatalf("duplicate connector retained: %v", err)
		}
		manual, err := ws.ConnectorByID(ctx, authored.ID)
		if err != nil || manual.Label == nil || *manual.Label != authoredLabel {
			t.Fatalf("authored connector changed: %+v %v", manual, err)
		}
		all, err := ws.Connectors(ctx, view.ResourceID)
		if err != nil || len(all) != 4 {
			t.Fatalf("view connectors=%d want 4: %v", len(all), err)
		}
	}
}

func TestFullMapLayoutPreservesSymbolAnchors(t *testing.T) {
	ws, idx := openGroupMapStore(t)
	ctx := context.Background()
	input := GroupMapInput{Full: true, RepositoryID: "repo", RepositoryName: "demo", SnapshotID: "one", Files: []community.File{{ID: "a", Path: "src/a.go"}, {ID: "b", Path: "src/b.go"}}, Edges: []MapEdge{{FromFactID: "a", ToFactID: "b", Kind: "calls", SourceLines: []uint32{3, 8}, TargetLines: []uint32{20, 30}}}}
	if _, err := ApplyGroupMap(ctx, ws, idx, input, MapOptions{}); err != nil {
		t.Fatal(err)
	}
	view, _, err := idx.MappingByLogicalKey(ctx, "map|directory|repo|src|view")
	if err != nil {
		t.Fatal(err)
	}
	connectors, err := ws.Connectors(ctx, view.ResourceID)
	if err != nil || len(connectors) != 1 {
		t.Fatalf("connectors=%v %v", connectors, err)
	}
	c := connectors[0]
	if c.SourceHandle == nil || !strings.HasSuffix(*c.SourceHandle, "-line-3,8") || (!strings.HasPrefix(*c.SourceHandle, "left-") && !strings.HasPrefix(*c.SourceHandle, "right-")) {
		t.Fatalf("layout discarded source symbols: %+v", c)
	}
	if c.TargetHandle == nil || !strings.HasSuffix(*c.TargetHandle, "-line-20,30") || (!strings.HasPrefix(*c.TargetHandle, "left-") && !strings.HasPrefix(*c.TargetHandle, "right-")) {
		t.Fatalf("layout discarded target symbols: %+v", c)
	}
	if _, err := ApplyGroupMap(ctx, ws, idx, input, MapOptions{}); err != nil {
		t.Fatal(err)
	}
	again, err := ws.ConnectorByID(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if again.SourceHandle == nil || *again.SourceHandle != *c.SourceHandle || again.TargetHandle == nil || *again.TargetHandle != *c.TargetHandle {
		t.Fatalf("rerun moved symbol ports: %+v -> %+v", c, again)
	}
	for _, side := range []string{"top-2", "bottom-0"} {
		if got := preserveFullLineHandle(optionalStr("right-line-3,8"), side); got != "right-line-3,8" {
			t.Fatalf("invalid code port: %s", got)
		}
	}
}

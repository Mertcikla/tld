package materialize

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/mertcikla/codeindex/community"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/core"
)

func TestUpsertProtectsAuthoredDiagramsAcrossRemovalAndRetry(t *testing.T) {
	ctx := context.Background()
	ws, idx := openGroupMapStore(t)
	input := GroupMapInput{RepositoryID: "repo", SnapshotID: "a", Files: groupMapFiles(),
		Groups: []*community.Group{{Key: "old", Name: "old", Files: 2, Members: []int{0, 1}}},
		Edges:  []MapEdge{{FromFactID: "id-a", ToFactID: "id-b", Weight: 1}},
	}
	apply := func() {
		t.Helper()
		if _, err := ApplyGroupMap(ctx, ws, idx, input, MapOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	apply()
	mapped := func(key string) cstore.ResourceMapping {
		t.Helper()
		m, ok, err := idx.MappingByLogicalKey(ctx, key)
		if err != nil || !ok {
			t.Fatalf("mapping %s: %v %v", key, ok, err)
		}
		return m
	}
	file := mapped(fileKey("repo", "id-a"))
	group := mapped(groupElementKey("repo", "old"))
	groupView := mapped(groupViewKey("repo", "old"))
	authored, err := ws.CreateView(ctx, "Authored", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	note, err := ws.CreateElement(ctx, core.LibraryElement{Name: "User note"})
	if err != nil {
		t.Fatal(err)
	}
	for _, viewID := range []int64{authored.ID, groupView.ResourceID} {
		for _, id := range []int64{note.ID, file.ResourceID} {
			if _, err := ws.AddPlacement(ctx, viewID, id, 123, 456); err != nil {
				t.Fatal(err)
			}
		}
	}
	custom, err := ws.CreateConnector(ctx, core.Connector{ViewID: authored.ID, SourceElementID: note.ID, TargetElementID: file.ResourceID, Label: strPtr("authored"), SourceHandle: strPtr("left"), Style: "dashed"})
	if err != nil {
		t.Fatal(err)
	}
	nested, err := ws.CreateView(ctx, "Authored child", nil, &file.ResourceID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ws.AddPlacement(ctx, nested.ID, note.ID, 11, 22); err != nil {
		t.Fatal(err)
	}
	layer, err := ws.CreateLayer(ctx, groupView.ResourceID, "Authored layer", []string{"mine"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	check := func(stale bool) {
		t.Helper()
		element, err := ws.ElementByID(ctx, file.ResourceID)
		if err != nil || slices.Contains(element.Tags, StaleTag) != stale {
			t.Fatalf("file lost/wrong status: %+v %v", element, err)
		}
		for _, id := range []int64{group.ResourceID, note.ID} {
			if _, err := ws.ElementByID(ctx, id); err != nil {
				t.Fatal(err)
			}
		}
		for _, id := range []int64{authored.ID, groupView.ResourceID, nested.ID} {
			if _, err := ws.ViewByID(ctx, id); err != nil {
				t.Fatal(err)
			}
		}
		placements, err := ws.ElementPlacements(ctx, authored.ID)
		if err != nil || len(placements) != 2 {
			t.Fatalf("authored placements lost: %v %v", placements, err)
		}
		for _, p := range placements {
			if p.PositionX != 123 || p.PositionY != 456 {
				t.Fatalf("authored position changed: %+v", p)
			}
		}
		got, err := ws.ConnectorByID(ctx, custom.ID)
		if err != nil || !reflect.DeepEqual(got, custom) {
			t.Fatalf("authored connector changed: %+v %v", got, err)
		}
		gotLayer, err := ws.LayerByID(ctx, layer.ID)
		if err != nil || !reflect.DeepEqual(gotLayer, layer) {
			t.Fatalf("authored layer changed: %+v %v", gotLayer, err)
		}
	}
	// The old files and group disappear from the generated hierarchy entirely.
	input.SnapshotID = "b"
	input.Groups = []*community.Group{{Key: "new", Name: "new", Files: 2, Members: []int{2, 3}}}
	input.Edges = nil
	cancelCtx, cancel := context.WithCancel(ctx)
	wrapped := &cancelMapStore{Store: ws, cancel: cancel, remaining: 1}
	_, err = ApplyGroupMap(cancelCtx, wrapped, idx, input, MapOptions{})
	cancel()
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected interrupted remap: %v", err)
	}
	check(false)
	apply()
	check(true)
	old, ok, err := idx.MappingByLogicalKey(ctx, file.LogicalKey)
	if err != nil || !ok || old.ResourceID != file.ResourceID || old.SnapshotID != "a" {
		t.Fatalf("lost retained identity/provenance: %+v %v", old, err)
	}
	before, err := idx.MappingsByRepository(ctx, "repo")
	if err != nil {
		t.Fatal(err)
	}
	apply()
	after, err := idx.MappingsByRepository(ctx, "repo")
	if err != nil || len(before) != len(after) {
		t.Fatalf("retry duplicated resources: %v", err)
	}
	// Reappearance reuses the original element and removes only the status tag.
	input.SnapshotID = "a"
	input.Groups = []*community.Group{{Key: "old", Name: "old", Files: 2, Members: []int{0, 1}}}
	apply()
	check(false)
	if mapped(file.LogicalKey).ResourceID != file.ResourceID {
		t.Fatal("reappearance replaced element")
	}
}

func TestUpsertPreservesEditedGeneratedResources(t *testing.T) {
	ctx := context.Background()
	ws, idx := openGroupMapStore(t)
	input := GroupMapInput{RepositoryID: "repo", SnapshotID: "a", Files: groupMapFiles(),
		Groups: []*community.Group{{Key: "all", Name: "all", Files: 4, Members: []int{0, 1, 2, 3}}},
		Edges:  []MapEdge{{FromFactID: "id-a", ToFactID: "id-b", Weight: 1}},
	}
	if _, err := ApplyGroupMap(ctx, ws, idx, input, MapOptions{}); err != nil {
		t.Fatal(err)
	}
	viewID := viewIDByName(t, ws, "all")
	connectors, err := ws.Connectors(ctx, viewID)
	if err != nil || len(connectors) != 1 {
		t.Fatalf("connectors: %v %v", connectors, err)
	}
	note, err := ws.CreateElement(ctx, core.LibraryElement{Name: "custom endpoint"})
	if err != nil {
		t.Fatal(err)
	}
	edited, err := ws.UpdateConnector(ctx, connectors[0].ID, core.Connector{SourceElementID: note.ID, Direction: "both", Label: strPtr("custom"), SourceHandle: strPtr("bottom"), TargetHandle: strPtr("top"), Style: "dashed"})
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a legacy view whose ownership no longer matches the generated owner.
	if _, err := ws.DB().ExecContext(ctx, `UPDATE views SET owner_element_id = NULL WHERE id = ?`, viewID); err != nil {
		t.Fatal(err)
	}
	if _, err := ws.AddPlacement(ctx, viewID, note.ID, 500, 600); err != nil {
		t.Fatal(err)
	}
	file, ok, err := idx.MappingByLogicalKey(ctx, fileKey("repo", "id-a"))
	if err != nil || !ok {
		t.Fatal(err)
	}
	if _, err := ws.UpdateElement(ctx, file.ResourceID, core.LibraryElement{Kind: strPtr("component"), Name: "renamed", Description: strPtr("authored")}); err != nil {
		t.Fatal(err)
	}
	// Layer reuse must not move/delete it or replace edited membership selectors.
	layer, err := ws.CreateLayer(ctx, viewID, "Mine", []string{"user-selection"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	layerKey := groupLayerKey("repo", "all")
	if err := idx.SaveMappings(ctx, []cstore.ResourceMapping{{LogicalKey: layerKey, Kind: cstore.MappingLayer, ResourceID: layer.ID, RepositoryID: "repo", SnapshotID: "a"}}); err != nil {
		t.Fatal(err)
	}
	input.SnapshotID = "b"
	if _, err := ApplyGroupMap(ctx, ws, idx, input, MapOptions{}); err != nil {
		t.Fatal(err)
	}
	got, err := ws.ConnectorByID(ctx, edited.ID)
	if err != nil || !reflect.DeepEqual(got, edited) {
		t.Fatalf("generated connector edit overwritten: %+v %v", got, err)
	}
	view, err := ws.ViewByID(ctx, viewID)
	if err != nil || view.OwnerElementID != nil {
		t.Fatalf("view replaced/reparented: %+v %v", view, err)
	}
	gotFile, err := ws.ElementByID(ctx, file.ResourceID)
	if err != nil || gotFile.Name != "renamed" || *gotFile.Kind != "component" || *gotFile.Description != "authored" {
		t.Fatalf("element presentation overwritten: %+v %v", gotFile, err)
	}
	m := &mapMaterializer{ctx: ctx, ws: ws, input: MapInput{RepositoryID: "repo", SnapshotID: "b"}, kept: map[string]bool{}, byKey: map[string]cstore.ResourceMapping{layerKey: {Kind: cstore.MappingLayer, ResourceID: layer.ID}}}
	if err := m.upsertLayer(layerKey, viewID+1000, "Generated", []string{"generated"}); err != nil {
		t.Fatal(err)
	}
	gotLayer, err := ws.LayerByID(ctx, layer.ID)
	if err != nil || !reflect.DeepEqual(gotLayer, layer) {
		t.Fatalf("generated layer edit overwritten: %+v %v", gotLayer, err)
	}
}

// A transient store failure must not be mistaken for a missing element: doing
// so replaces its mapping and silently disconnects the authored diagram.
type failingElementStore struct {
	core.Store
	failure error
	onRead  bool
}

func (s failingElementStore) ElementByID(ctx context.Context, id int64) (core.LibraryElement, error) {
	if s.onRead {
		return core.LibraryElement{}, s.failure
	}
	return s.Store.ElementByID(ctx, id)
}

func (s failingElementStore) UpdateElement(context.Context, int64, core.LibraryElement) (core.LibraryElement, error) {
	return core.LibraryElement{}, s.failure
}

func TestUpsertStoreFailureDoesNotReplaceOwnership(t *testing.T) {
	for _, onRead := range []bool{true, false} {
		name := "write"
		if onRead {
			name = "read"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			ws, idx := openGroupMapStore(t)
			input := GroupMapInput{RepositoryID: "repo", SnapshotID: "a", Files: groupMapFiles(), Groups: []*community.Group{{Key: "all", Name: "all", Files: 4, Members: []int{0, 1, 2, 3}}}}
			if _, err := ApplyGroupMap(ctx, ws, idx, input, MapOptions{}); err != nil {
				t.Fatal(err)
			}
			before, err := idx.MappingsByRepository(ctx, "repo")
			if err != nil {
				t.Fatal(err)
			}
			var countBefore, countAfter int
			if err := ws.DB().QueryRow(`SELECT COUNT(*) FROM elements`).Scan(&countBefore); err != nil {
				t.Fatal(err)
			}
			failure := errors.New("storage unavailable")
			input.SnapshotID = "b"
			if _, err := ApplyGroupMap(ctx, failingElementStore{Store: ws, failure: failure, onRead: onRead}, idx, input, MapOptions{}); !errors.Is(err, failure) {
				t.Fatalf("storage failure swallowed: %v", err)
			}
			after, err := idx.MappingsByRepository(ctx, "repo")
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, after) {
				t.Fatal("failure replaced ownership")
			}
			if err := ws.DB().QueryRow(`SELECT COUNT(*) FROM elements`).Scan(&countAfter); err != nil {
				t.Fatal(err)
			}
			if countBefore != countAfter {
				t.Fatal("failure created replacement elements")
			}
		})
	}
}

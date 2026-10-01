package store

import (
	"context"
	"testing"
)

func TestResourceMappings(t *testing.T) {
	ctx := context.Background()
	st, handle := openTestStore(t)
	defer func() { _ = handle.Close() }()

	mappings := []ResourceMapping{
		{LogicalKey: "fact|a.go|FACT_KIND_FUNCTION|A", Kind: MappingElement, ResourceID: 11, RepositoryID: "repo", SnapshotID: "snap-1"},
		{LogicalKey: "edge|CALLS|fact|a.go|FACT_KIND_FUNCTION|A|fact|b.go|FACT_KIND_FUNCTION|B", Kind: MappingConnector, ResourceID: 22, RepositoryID: "repo", SnapshotID: "snap-1"},
	}
	if err := st.SaveMappings(ctx, mappings); err != nil {
		t.Fatalf("save mappings: %v", err)
	}

	got, ok, err := st.MappingByLogicalKey(ctx, mappings[0].LogicalKey)
	if err != nil || !ok {
		t.Fatalf("lookup: ok=%v err=%v", ok, err)
	}
	if got.ResourceID != 11 || got.Kind != MappingElement {
		t.Fatalf("unexpected mapping: %+v", got)
	}

	// Upsert replaces the resource id for an existing logical key.
	if err := st.SaveMappings(ctx, []ResourceMapping{{LogicalKey: mappings[0].LogicalKey, Kind: MappingElement, ResourceID: 99, RepositoryID: "repo", SnapshotID: "snap-2"}}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, _, err = st.MappingByLogicalKey(ctx, mappings[0].LogicalKey)
	if err != nil {
		t.Fatalf("lookup after upsert: %v", err)
	}
	if got.ResourceID != 99 || got.SnapshotID != "snap-2" {
		t.Fatalf("upsert did not replace: %+v", got)
	}

	bySnapshot, err := st.MappingsBySnapshot(ctx, "snap-2")
	if err != nil {
		t.Fatalf("by snapshot: %v", err)
	}
	if len(bySnapshot) != 1 || bySnapshot[0].ResourceID != 99 {
		t.Fatalf("by snapshot = %+v", bySnapshot)
	}

	// The connector mapping remains under its original snapshot.
	if _, ok, _ := st.MappingByLogicalKey(ctx, mappings[1].LogicalKey); !ok {
		t.Fatal("connector mapping missing")
	}

	if err := st.DeleteMappingsForSnapshot(ctx, "snap-2"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, ok, _ := st.MappingByLogicalKey(ctx, mappings[0].LogicalKey); ok {
		t.Fatal("mapping not deleted")
	}
}

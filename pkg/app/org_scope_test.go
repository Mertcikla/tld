package app

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// seedOrgTenant creates a view plus an element so child rows have a parent.
func seedOrgTenant(t *testing.T, store *Store, ctx context.Context, name string) (ViewSummary, LibraryElement) {
	t.Helper()
	view, err := store.CreateView(ctx, name, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	element, err := store.CreateElement(ctx, LibraryElement{Name: name + "-element"})
	if err != nil {
		t.Fatal(err)
	}
	return view, element
}

func TestStoreTenantScopeInjectsOrgIDIntoPlacementsAndLayers(t *testing.T) {
	store := openAppStore(t)
	ctx := context.Background()

	orgA := uuid.New()
	orgB := uuid.New()
	ctxA := WithTenantOrgID(ctx, orgA)
	ctxB := WithTenantOrgID(ctx, orgB)

	viewA, elementA := seedOrgTenant(t, store, ctxA, "A")
	viewB, elementB := seedOrgTenant(t, store, ctxB, "B")

	placementA, err := store.AddPlacement(ctxA, viewA.ID, elementA.ID, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddPlacement(ctxB, viewB.ID, elementB.ID, 3, 4); err != nil {
		t.Fatal(err)
	}
	layerA, err := store.CreateLayer(ctxA, viewA.ID, "layer-a", []string{"shared"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	layerB, err := store.CreateLayer(ctxB, viewB.ID, "layer-b", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetVisibilityOverride(ctxA, viewA.ID, "element", elementA.ID, -1); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SetVisibilityOverride(ctxB, viewB.ID, "element", elementB.ID, -1); err != nil {
		t.Fatal(err)
	}

	assertOrgID := func(table string, column string, value int64, want uuid.UUID) {
		t.Helper()
		var got string
		if err := store.DB().QueryRowContext(ctx,
			`SELECT org_id FROM `+table+` WHERE `+column+` = ?`, value).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want.String() {
			t.Fatalf("%s(%s = %d) org_id = %q, want %q", table, column, value, got, want)
		}
	}
	assertOrgID("placements", "id", placementA.ID, orgA)
	assertOrgID("view_layers", "id", layerA.ID, orgA)
	assertOrgID("view_visibility_overrides", "view_id", viewA.ID, orgA)

	// Reads stay inside the owning tenant.
	placementsA, err := store.ElementPlacements(ctxA, viewA.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(placementsA) != 1 || placementsA[0].ElementID != elementA.ID {
		t.Fatalf("tenant A placements = %+v, want only its own", placementsA)
	}
	overridesB, err := store.VisibilityOverrides(ctxB, viewB.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(overridesB) != 1 || overridesB[0].ResourceID != elementB.ID {
		t.Fatalf("tenant B overrides = %+v, want only its own", overridesB)
	}

	// A cross-tenant delete must not reach another tenant's row.
	if err := store.DeleteLayer(ctxA, layerB.ID); err != nil {
		t.Fatal(err)
	}
	layersB, err := store.Layers(ctxB, viewB.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(layersB) != 1 {
		t.Fatalf("tenant B layers = %d, want 1 after cross-tenant delete", len(layersB))
	}
}

func TestStoreTagsAreScopedPerOrg(t *testing.T) {
	store := openAppStore(t)
	ctx := context.Background()

	orgA := uuid.New()
	orgB := uuid.New()
	ctxA := WithTenantOrgID(ctx, orgA)
	ctxB := WithTenantOrgID(ctx, orgB)

	if err := store.UpdateTag(ctxA, "shared", "#111111", nil); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateTag(ctxB, "shared", "#222222", nil); err != nil {
		t.Fatal(err)
	}

	tagsA, err := store.Tags(ctxA)
	if err != nil {
		t.Fatal(err)
	}
	if got := tagsA["shared"].Color; got != "#111111" {
		t.Fatalf("tenant A shared tag colour = %q, want #111111", got)
	}
	tagsB, err := store.Tags(ctxB)
	if err != nil {
		t.Fatal(err)
	}
	if got := tagsB["shared"].Color; got != "#222222" {
		t.Fatalf("tenant B shared tag colour = %q, want #222222", got)
	}

	if err := store.DeleteTag(ctxA, "shared"); err != nil {
		t.Fatal(err)
	}
	tagsB, err = store.Tags(ctxB)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := tagsB["shared"]; !ok {
		t.Fatal("tenant B lost its shared tag after tenant A deleted theirs")
	}
}

func TestStoreSelfHostedTagsUseNilOrgSentinel(t *testing.T) {
	store := openAppStore(t)
	ctx := context.Background()

	if err := store.UpdateTag(ctx, "local", "#333333", nil); err != nil {
		t.Fatal(err)
	}

	// Self-hosted mode omits org_id, so the column default has to supply the nil
	// organisation. A NULL would make the (org_id, name) primary key treat every
	// row as distinct and silently allow duplicate tag names.
	var orgID string
	if err := store.DB().QueryRowContext(ctx, `SELECT org_id FROM tags WHERE name = ?`, "local").Scan(&orgID); err != nil {
		t.Fatal(err)
	}
	if orgID != uuid.Nil.String() {
		t.Fatalf("self-hosted tag org_id = %q, want %q", orgID, uuid.Nil)
	}

	if err := store.UpdateTag(ctx, "local", "#444444", nil); err != nil {
		t.Fatal(err)
	}
	var got string
	if err := store.DB().QueryRowContext(ctx, `SELECT color FROM tags WHERE name = ?`, "local").Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != "#444444" {
		t.Fatalf("self-hosted tag colour = %q, want #444444 (upsert must hit the composite key)", got)
	}
	var count int
	if err := store.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM tags WHERE name = ?`, "local").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("self-hosted tag rows = %d, want 1", count)
	}
}
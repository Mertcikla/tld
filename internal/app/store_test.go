package app

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	assets "github.com/mertcikla/tld"
)

func TestConfigureSQLiteDBEnablesBusyTimeoutAndWAL(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "tld.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	if err := configureSQLiteDB(db); err != nil {
		t.Fatal(err)
	}

	var busyTimeout int
	if err := db.QueryRow(`PRAGMA busy_timeout;`).Scan(&busyTimeout); err != nil {
		t.Fatal(err)
	}
	if busyTimeout != 5000 {
		t.Fatalf("busy_timeout = %d, want 5000", busyTimeout)
	}

	var journalMode string
	if err := db.QueryRow(`PRAGMA journal_mode;`).Scan(&journalMode); err != nil {
		t.Fatal(err)
	}
	if strings.ToLower(journalMode) != "wal" {
		t.Fatalf("journal_mode = %q, want wal", journalMode)
	}
}

func TestStoreElementsSearchPaginationAndViewMetadata(t *testing.T) {
	store := openAppStore(t)
	ctx := context.Background()

	serviceKind := "service"
	api, err := store.CreateElement(ctx, LibraryElement{Name: "API", Kind: &serviceKind, Description: strPtr("Public runtime API"), Tags: []string{"runtime"}})
	if err != nil {
		t.Fatal(err)
	}
	worker, err := store.CreateElement(ctx, LibraryElement{Name: "Worker", Kind: &serviceKind, Description: strPtr("Background jobs"), Tags: []string{"runtime"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateView(ctx, "API detail", strPtr("Service"), &api.ID); err != nil {
		t.Fatal(err)
	}

	results, total, err := store.Elements(ctx, 1, 0, "runtime")
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(results) != 1 || results[0].ID != api.ID {
		t.Fatalf("search results = total:%d elements:%+v, want only API", total, results)
	}
	if !results[0].HasView || results[0].ViewLabel == nil || *results[0].ViewLabel != "Service" {
		t.Fatalf("view metadata = has:%v label:%v, want Service child view", results[0].HasView, results[0].ViewLabel)
	}

	results, total, err = store.Elements(ctx, 1, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(results) != 1 || results[0].ID != api.ID {
		t.Fatalf("paginated results = total:%d elements:%+v, want second inserted API after Worker", total, results)
	}

	tags, err := store.Tags(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := tags["runtime"]; !ok {
		t.Fatalf("tags = %+v, want runtime tag color created with element", tags)
	}
	_ = worker
}

func TestStoreConnectorsPreserveHandlesAndPatchDefaults(t *testing.T) {
	store := openAppStore(t)
	ctx := context.Background()

	source, err := store.CreateElement(ctx, LibraryElement{Name: "API"})
	if err != nil {
		t.Fatal(err)
	}
	target, err := store.CreateElement(ctx, LibraryElement{Name: "DB"})
	if err != nil {
		t.Fatal(err)
	}
	label := "reads"
	sourceHandle := "right"
	targetHandle := "left"
	connector, err := store.CreateConnector(ctx, Connector{
		ViewID:          1,
		SourceElementID: source.ID,
		TargetElementID: target.ID,
		Label:           &label,
		Style:           "bezier",
		SourceHandle:    &sourceHandle,
		TargetHandle:    &targetHandle,
	})
	if err != nil {
		t.Fatal(err)
	}
	if connector.Direction != "forward" {
		t.Fatalf("direction = %q, want forward default", connector.Direction)
	}
	if connector.SourceHandle == nil || *connector.SourceHandle != "right" || connector.TargetHandle == nil || *connector.TargetHandle != "left" {
		t.Fatalf("handles = %v/%v, want right/left", connector.SourceHandle, connector.TargetHandle)
	}

	updatedLabel := "streams"
	updated, err := store.UpdateConnector(ctx, connector.ID, Connector{Label: &updatedLabel})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Label == nil || *updated.Label != "streams" {
		t.Fatalf("label = %v, want streams", updated.Label)
	}
	if updated.SourceElementID != source.ID || updated.TargetElementID != target.ID || updated.Style != "bezier" || updated.Direction != "forward" {
		t.Fatalf("patched connector lost defaults or endpoints: %+v", updated)
	}
	if updated.SourceHandle == nil || *updated.SourceHandle != "right" || updated.TargetHandle == nil || *updated.TargetHandle != "left" {
		t.Fatalf("patched handles = %v/%v, want right/left", updated.SourceHandle, updated.TargetHandle)
	}
}

func TestStoreLayersPersistTagsColorsAndUpdates(t *testing.T) {
	store := openAppStore(t)
	ctx := context.Background()

	layer, err := store.CreateLayer(ctx, 1, "Runtime", []string{"api", "db"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if layer.Color == nil || *layer.Color == "" {
		t.Fatalf("layer color = %v, want generated color", layer.Color)
	}
	if strings.Join(layer.Tags, ",") != "api,db" {
		t.Fatalf("layer tags = %+v, want api,db", layer.Tags)
	}

	color := "#123456"
	updated, err := store.UpdateLayer(ctx, layer.ID, ViewLayer{Name: "Data", Tags: []string{"db"}, Color: &color})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "Data" || updated.Color == nil || *updated.Color != color || strings.Join(updated.Tags, ",") != "db" {
		t.Fatalf("updated layer = %+v, want Data/db/%s", updated, color)
	}

	tags, err := store.Tags(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := tags["api"]; !ok {
		t.Fatalf("tags = %+v, want api tag retained", tags)
	}
	if _, ok := tags["db"]; !ok {
		t.Fatalf("tags = %+v, want db tag retained", tags)
	}
}

func openAppStore(t *testing.T) *Store {
	t.Helper()
	store, err := OpenStore(filepath.Join(t.TempDir(), "tld.db"), assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func strPtr(value string) *string {
	return &value
}

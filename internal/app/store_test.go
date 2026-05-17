package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/internal/tagcolors"
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
	if maxOpen := db.Stats().MaxOpenConnections; maxOpen != 1 {
		t.Fatalf("max open connections = %d, want 1 for shared SQLite locking", maxOpen)
	}
}

func TestStoreElementsSearchPaginationAndViewMetadata(t *testing.T) {
	store := openAppStore(t)
	ctx := context.Background()

	serviceKind := "service"
	api, err := store.CreateElement(ctx, LibraryElement{Name: "API", Kind: &serviceKind, Description: new("Public runtime API"), Tags: []string{"runtime"}})
	if err != nil {
		t.Fatal(err)
	}
	worker, err := store.CreateElement(ctx, LibraryElement{Name: "Worker", Kind: &serviceKind, Description: new("Background jobs"), Tags: []string{"runtime"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateView(ctx, "API detail", new("Service"), &api.ID); err != nil {
		t.Fatal(err)
	}

	results, total, err := store.Elements(ctx, 1, 0, "api")
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

	if _, err := store.UpdateConnector(ctx, 999999, Connector{Label: &updatedLabel}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing connector update error = %v, want sql.ErrNoRows", err)
	}
}

func TestStoreUndoableElementUpdatesPreserveAndRestoreFields(t *testing.T) {
	store := openAppStore(t)
	ctx := context.Background()

	kind := "service"
	technology := "Go"
	url := "https://example.com/api"
	logo := "https://example.com/go.svg"
	repo := "repo"
	branch := "main"
	filePath := "cmd/api/main.go"
	language := "go"
	original, err := store.CreateElement(ctx, LibraryElement{
		Name:        "API",
		Kind:        &kind,
		Description: new("original description"),
		Technology:  &technology,
		URL:         &url,
		LogoURL:     &logo,
		TechnologyConnectors: []TechnologyConnector{{
			Type:          "catalog",
			Slug:          "go",
			Label:         "Go",
			IsPrimaryIcon: true,
		}},
		Tags:     []string{"runtime", "public"},
		Repo:     &repo,
		Branch:   &branch,
		FilePath: &filePath,
		Language: &language,
	})
	if err != nil {
		t.Fatal(err)
	}

	changedKind := "database"
	changedTech := "PostgreSQL"
	changedURL := "https://example.com/db"
	changedLogo := "https://example.com/postgres.svg"
	changed, err := store.UpdateElement(ctx, original.ID, LibraryElement{
		Name:        "DB",
		Kind:        &changedKind,
		Description: new("changed description"),
		Technology:  &changedTech,
		URL:         &changedURL,
		LogoURL:     &changedLogo,
		TechnologyConnectors: []TechnologyConnector{{
			Type:          "catalog",
			Slug:          "postgresql",
			Label:         "PostgreSQL",
			IsPrimaryIcon: true,
		}},
		Tags: []string{"data"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if changed.Repo == nil || *changed.Repo != repo || changed.FilePath == nil || *changed.FilePath != filePath {
		t.Fatalf("partial update lost source metadata: %+v", changed)
	}
	if len(changed.TechnologyConnectors) != 1 || changed.TechnologyConnectors[0].Slug != "postgresql" || strings.Join(changed.Tags, ",") != "data" {
		t.Fatalf("changed technology/tags = links:%+v tags:%+v, want postgres/data", changed.TechnologyConnectors, changed.Tags)
	}

	nameOnly, err := store.UpdateElement(ctx, original.ID, LibraryElement{Name: "Database"})
	if err != nil {
		t.Fatal(err)
	}
	if nameOnly.Name != "Database" || nameOnly.Kind == nil || *nameOnly.Kind != changedKind || len(nameOnly.TechnologyConnectors) != 1 || nameOnly.TechnologyConnectors[0].Slug != "postgresql" || strings.Join(nameOnly.Tags, ",") != "data" {
		t.Fatalf("name-only update = %+v, want previous nullable/list fields preserved", nameOnly)
	}

	restored, err := store.UpdateElement(ctx, original.ID, original)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Name != original.Name || restored.Kind == nil || *restored.Kind != kind || restored.Description == nil || *restored.Description != "original description" {
		t.Fatalf("restored element basics = %+v, want original", restored)
	}
	if restored.LogoURL == nil || *restored.LogoURL != logo || len(restored.TechnologyConnectors) != 1 || restored.TechnologyConnectors[0].Slug != "go" {
		t.Fatalf("restored technology = logo:%v links:%+v, want original", restored.LogoURL, restored.TechnologyConnectors)
	}
	if strings.Join(restored.Tags, ",") != "runtime,public" || restored.Repo == nil || *restored.Repo != repo || restored.Branch == nil || *restored.Branch != branch || restored.FilePath == nil || *restored.FilePath != filePath || restored.Language == nil || *restored.Language != language {
		t.Fatalf("restored metadata = %+v, want original tags/source fields", restored)
	}

	if _, err := store.UpdateElement(ctx, 999999, LibraryElement{Name: "Missing"}); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing element update error = %v, want sql.ErrNoRows", err)
	}
}

func TestStoreUndoablePlacementRemoveAndRestorePreservesElement(t *testing.T) {
	store := openAppStore(t)
	ctx := context.Background()

	element, err := store.CreateElement(ctx, LibraryElement{Name: "API", Tags: []string{"runtime"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddPlacement(ctx, 1, element.ID, 10, 20); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdatePlacement(ctx, 1, element.ID, 30, 40); err != nil {
		t.Fatal(err)
	}
	if err := store.DeletePlacement(ctx, 1, element.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ElementByID(ctx, element.ID); err != nil {
		t.Fatalf("element after placement removal: %v", err)
	}
	restored, err := store.AddPlacement(ctx, 1, element.ID, 30, 40)
	if err != nil {
		t.Fatal(err)
	}
	if restored.ViewID != 1 || restored.ElementID != element.ID || restored.PositionX != 30 || restored.PositionY != 40 {
		t.Fatalf("restored placement = %+v, want view/element/position restored", restored)
	}
}

func TestStoreUndoableConnectorDeleteRecreateAndRestoreFields(t *testing.T) {
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
	original, err := store.CreateConnector(ctx, Connector{
		ViewID:          1,
		SourceElementID: source.ID,
		TargetElementID: target.ID,
		Label:           new("reads"),
		Description:     new("primary query path"),
		Relationship:    new("SQL"),
		Direction:       "both",
		Style:           "smoothstep",
		URL:             new("https://example.com/runbook"),
		SourceHandle:    new("right"),
		TargetHandle:    new("left"),
	})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := store.UpdateConnector(ctx, original.ID, Connector{
		Label:        new("streams"),
		SourceHandle: new("bottom"),
		TargetHandle: new("top"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Description == nil || *updated.Description != "primary query path" || updated.Relationship == nil || *updated.Relationship != "SQL" || updated.Direction != "both" || updated.Style != "smoothstep" || updated.URL == nil || *updated.URL != "https://example.com/runbook" {
		t.Fatalf("connector patch lost fields: %+v", updated)
	}
	restored, err := store.UpdateConnector(ctx, original.ID, original)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Label == nil || *restored.Label != "reads" || restored.SourceHandle == nil || *restored.SourceHandle != "right" || restored.TargetHandle == nil || *restored.TargetHandle != "left" {
		t.Fatalf("restored connector = %+v, want original label/handles", restored)
	}
	if err := store.DeleteConnector(ctx, restored.ID); err != nil {
		t.Fatal(err)
	}
	recreated, err := store.CreateConnector(ctx, Connector{
		ViewID:          restored.ViewID,
		SourceElementID: restored.SourceElementID,
		TargetElementID: restored.TargetElementID,
		Label:           restored.Label,
		Description:     restored.Description,
		Relationship:    restored.Relationship,
		Direction:       restored.Direction,
		Style:           restored.Style,
		URL:             restored.URL,
		SourceHandle:    restored.SourceHandle,
		TargetHandle:    restored.TargetHandle,
	})
	if err != nil {
		t.Fatal(err)
	}
	if recreated.ID == restored.ID || recreated.ViewID != restored.ViewID || recreated.SourceElementID != restored.SourceElementID || recreated.TargetElementID != restored.TargetElementID || recreated.Label == nil || *recreated.Label != "reads" || recreated.Description == nil || *recreated.Description != "primary query path" || recreated.Relationship == nil || *recreated.Relationship != "SQL" || recreated.Direction != "both" || recreated.Style != "smoothstep" || recreated.URL == nil || *recreated.URL != "https://example.com/runbook" || recreated.SourceHandle == nil || *recreated.SourceHandle != "right" || recreated.TargetHandle == nil || *recreated.TargetHandle != "left" {
		t.Fatalf("recreated connector = %+v, want equivalent payload with new id", recreated)
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

	updated, err = store.UpdateLayer(ctx, layer.ID, ViewLayer{Name: "Data", Tags: []string{"queue"}, Color: &color})
	if err != nil {
		t.Fatal(err)
	}
	tags, err = store.Tags(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if tag, ok := tags["queue"]; !ok || tag.Color == "" {
		t.Fatalf("tags = %+v, want queue tag with generated color after layer update", tags)
	}
	_ = updated
}

func TestExploreLoadsWorkspaceDataInBatches(t *testing.T) {
	store := openAppStore(t)
	ctx := context.Background()
	db := store.DB()
	if _, err := db.Exec(`
		INSERT INTO elements(id, name, tags, technology_connectors, created_at, updated_at)
		VALUES
			(100, 'API', '["runtime"]', '[]', 'now', 'now'),
			(101, 'DB', '["data"]', '[]', 'now', 'now');
		INSERT INTO views(id, owner_element_id, name, description, level_label, level, created_at, updated_at)
		VALUES
			(100, NULL, 'System', NULL, 'System', 1, 'now', 'now'),
			(101, 100, 'API detail', NULL, 'Service', 2, 'now', 'now');
		INSERT INTO placements(id, view_id, element_id, position_x, position_y, created_at, updated_at)
		VALUES
			(100, 100, 100, 10, 20, 'now', 'now'),
			(101, 100, 101, 30, 40, 'now', 'now'),
			(102, 101, 101, 50, 60, 'now', 'now');
		INSERT INTO connectors(id, view_id, source_element_id, target_element_id, label, direction, style, created_at, updated_at)
		VALUES (100, 100, 100, 101, 'reads', 'forward', 'solid', 'now', 'now');
	`); err != nil {
		t.Fatal(err)
	}

	explore, err := store.Explore(ctx)
	if err != nil {
		t.Fatal(err)
	}
	root := explore.Views["100"]
	if len(root.Placements) != 2 || len(root.Connectors) != 1 {
		t.Fatalf("root explore data = placements:%d connectors:%d, want 2/1", len(root.Placements), len(root.Connectors))
	}
	if !root.Placements[0].HasView || root.Placements[0].ViewLabel == nil || *root.Placements[0].ViewLabel != "Service" {
		t.Fatalf("root first placement view metadata = has:%v label:%v, want Service child view", root.Placements[0].HasView, root.Placements[0].ViewLabel)
	}
	child := explore.Views["101"]
	if len(child.Placements) != 1 || child.Placements[0].ElementID != 101 {
		t.Fatalf("child explore placements = %+v, want DB placement", child.Placements)
	}
	if len(explore.Navigations) != 1 {
		t.Fatalf("navigations = %+v, want one child navigation", explore.Navigations)
	}
	nav := explore.Navigations[0]
	if nav.ElementID == nil || *nav.ElementID != 100 || nav.FromViewID != 100 || nav.ToViewID != 101 || nav.ToViewName != "API detail" || nav.RelationType != "child" {
		t.Fatalf("navigation = %+v, want API child navigation from root to detail", nav)
	}
}

func TestStoreAutoTagColorsPreserveUserMetadata(t *testing.T) {
	store := openAppStore(t)
	ctx := context.Background()

	description := "User chosen tag"
	if err := store.UpdateTag(ctx, "runtime", "#123456", &description); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateElement(ctx, LibraryElement{Name: "API", Tags: []string{"runtime", "worker", "api"}}); err != nil {
		t.Fatal(err)
	}

	tags, err := store.Tags(ctx)
	if err != nil {
		t.Fatal(err)
	}
	runtime := tags["runtime"]
	if runtime.Color != "#123456" || runtime.Description == nil || *runtime.Description != description {
		t.Fatalf("runtime tag = %+v, want user metadata preserved", runtime)
	}
	if tags["worker"].Color == "" || tags["api"].Color == "" {
		t.Fatalf("tags = %+v, want generated colors for new tags", tags)
	}
	if tags["worker"].Color == tags["api"].Color {
		t.Fatalf("worker/api colors both %q, want unused colors preferred", tags["worker"].Color)
	}
}

func TestStoreAutoTagColorsGenerateUnusedColorsAfterSwatchesAreExhausted(t *testing.T) {
	store := openAppStore(t)
	ctx := context.Background()

	for i, color := range tagcolors.SwatchColors {
		if err := store.UpdateTag(ctx, fmt.Sprintf("existing-%d", i), color, nil); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.CreateElement(ctx, LibraryElement{Name: "Worker", Tags: []string{"generated-a", "generated-b", "generated-c"}}); err != nil {
		t.Fatal(err)
	}

	tags, err := store.Tags(ctx)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]string{}
	for _, name := range []string{"generated-a", "generated-b", "generated-c"} {
		tag := tags[name]
		if tag.Color == "" {
			t.Fatalf("%s color is empty", name)
		}
		if existing := seen[tag.Color]; existing != "" {
			t.Fatalf("%s and %s both use %s, want generated fallback colors to stay unused", existing, name, tag.Color)
		}
		seen[tag.Color] = name
		for _, swatch := range tagcolors.SwatchColors {
			if strings.EqualFold(tag.Color, swatch) {
				t.Fatalf("%s color = %s, want non-swatch fallback after swatches exhausted", name, tag.Color)
			}
		}
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

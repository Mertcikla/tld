package store

import (
	"context"
	"testing"

	"github.com/mertcikla/tld/internal/app"
)

func seedDensityView(t *testing.T, sqliteStore *SQLiteStore) {
	t.Helper()
	if _, err := sqliteStore.DB().Exec(`
		INSERT INTO elements(id, name, tags, technology_connectors, created_at, updated_at)
		VALUES
			(101, 'A', '[]', '[]', 'now', 'now'),
			(102, 'B', '[]', '[]', 'now', 'now'),
			(103, 'C', '[]', '[]', 'now', 'now'),
			(104, 'D', '[]', '[]', 'now', 'now'),
			(105, 'E', '[]', '[]', 'now', 'now'),
			(106, 'F', '[]', '[]', 'now', 'now');
		INSERT INTO placements(view_id, element_id, position_x, position_y, created_at, updated_at)
		VALUES
			(1, 101, 0, 0, 'now', 'now'),
			(1, 102, 10, 0, 'now', 'now'),
			(1, 103, 20, 0, 'now', 'now'),
			(1, 104, 30, 0, 'now', 'now'),
			(1, 105, 40, 0, 'now', 'now'),
			(1, 106, 50, 0, 'now', 'now');
		INSERT INTO connectors(id, view_id, source_element_id, target_element_id, label, direction, style, created_at, updated_at)
		VALUES
			(201, 1, 101, 102, 'important', 'forward', 'solid', 'now', 'now'),
			(202, 1, 105, 106, NULL, 'forward', 'solid', 'now', 'now'),
			(203, 1, 103, 104, 'important', 'forward', 'solid', 'now', 'now');
	`); err != nil {
		t.Fatal(err)
	}
}

func TestDensityValidationAndOverrideClamping(t *testing.T) {
	sqliteStore := openAdapterTestStore(t)
	ctx := context.Background()

	if err := sqliteStore.SetViewDensityLevel(ctx, 1, -3); err == nil {
		t.Fatal("expected invalid density to fail")
	}
	if err := sqliteStore.SetViewDensityLevel(ctx, 1, 2); err != nil {
		t.Fatal(err)
	}
	level, err := sqliteStore.ViewDensityLevel(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if level != 2 {
		t.Fatalf("density = %d, want 2", level)
	}

	override, err := sqliteStore.SetVisibilityOverride(ctx, 1, "element", 1, 99)
	if err != nil {
		t.Fatal(err)
	}
	if override.LevelDelta != 4 {
		t.Fatalf("delta = %d, want clamp to 4", override.LevelDelta)
	}
	override, err = sqliteStore.SetVisibilityOverride(ctx, 1, "element", 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if override.LevelDelta != 0 {
		t.Fatalf("reset delta = %d, want 0", override.LevelDelta)
	}
	overrides, err := sqliteStore.VisibilityOverrides(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(overrides) != 0 {
		t.Fatalf("overrides after reset = %v, want none", overrides)
	}
}

func TestDensityProjectionPromotedConnectorPullsEndpoints(t *testing.T) {
	sqliteStore := openAdapterTestStore(t)
	seedDensityView(t, sqliteStore)
	ctx := context.Background()

	if err := sqliteStore.SetViewDensityLevel(ctx, 1, -2); err != nil {
		t.Fatal(err)
	}
	content, err := sqliteStore.ProjectedViewContent(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(content.Placements) != 4 {
		t.Fatalf("compact placements = %d, want soft cap 4", len(content.Placements))
	}
	if containsConnector(content.Connectors, 202) {
		t.Fatal("connector 202 should be outside the compact projection before override")
	}

	if _, err := sqliteStore.AdjustVisibilityOverride(ctx, 1, "connector", 202, 1); err != nil {
		t.Fatal(err)
	}
	content, err = sqliteStore.ProjectedViewContent(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !containsConnector(content.Connectors, 202) || !containsPlacement(content.Placements, 105) || !containsPlacement(content.Placements, 106) {
		t.Fatalf("promoted connector did not pull endpoint: placements=%v connectors=%v", placementIDs(content.Placements), connectorIDs(content.Connectors))
	}
}

func TestFullDensityKeepsAllExceptExplicitDemotions(t *testing.T) {
	sqliteStore := openAdapterTestStore(t)
	seedDensityView(t, sqliteStore)
	ctx := context.Background()

	if err := sqliteStore.SetViewDensityLevel(ctx, 1, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := sqliteStore.AdjustVisibilityOverride(ctx, 1, "element", 102, -1); err != nil {
		t.Fatal(err)
	}
	content, err := sqliteStore.ProjectedViewContent(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if containsPlacement(content.Placements, 102) {
		t.Fatal("demoted element should be hidden at full density")
	}
	if containsConnector(content.Connectors, 201) {
		t.Fatal("connector incident to hidden element should be hidden")
	}
}

func containsPlacement(items []app.PlacedElement, elementID int64) bool {
	for _, item := range items {
		if item.ElementID == elementID {
			return true
		}
	}
	return false
}

func containsConnector(items []app.Connector, connectorID int64) bool {
	for _, item := range items {
		if item.ID == connectorID {
			return true
		}
	}
	return false
}

func placementIDs(items []app.PlacedElement) []int64 {
	out := make([]int64, 0, len(items))
	for _, item := range items {
		out = append(out, item.ElementID)
	}
	return out
}

func connectorIDs(items []app.Connector) []int64 {
	out := make([]int64, 0, len(items))
	for _, item := range items {
		out = append(out, item.ID)
	}
	return out
}

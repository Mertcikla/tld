package store

import (
	"context"
	"testing"

	"github.com/mertcikla/tld/v2/pkg/app"
)

func seedDensityView(t *testing.T, appStore *app.Store) {
	t.Helper()
	if _, err := appStore.DB().Exec(`
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
			(201, 1, 101, 102, 'important', 'forward', 'bezier', 'now', 'now'),
			(202, 1, 105, 106, NULL, 'forward', 'bezier', 'now', 'now'),
			(203, 1, 103, 104, 'important', 'forward', 'bezier', 'now', 'now');
	`); err != nil {
		t.Fatal(err)
	}
}

func TestDensityValidationAndOverrideClamping(t *testing.T) {
	appStore := openAdapterTestStore(t)
	ctx := context.Background()

	if err := appStore.SetViewDensityLevel(ctx, 1, -3); err == nil {
		t.Fatal("expected invalid density to fail")
	}
	if err := appStore.SetViewDensityLevel(ctx, 1, 2); err != nil {
		t.Fatal(err)
	}
	level, err := appStore.ViewDensityLevel(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if level != 2 {
		t.Fatalf("density = %d, want 2", level)
	}

	override, err := appStore.SetVisibilityOverride(ctx, 1, "element", 1, 99)
	if err != nil {
		t.Fatal(err)
	}
	if override.LevelDelta != 2 {
		t.Fatalf("delta = %d, want clamp to 2", override.LevelDelta)
	}
	override, err = appStore.SetVisibilityOverride(ctx, 1, "element", 1, -99)
	if err != nil {
		t.Fatal(err)
	}
	if override.LevelDelta != -2 {
		t.Fatalf("delta = %d, want clamp to -2", override.LevelDelta)
	}
	override, err = appStore.SetVisibilityOverride(ctx, 1, "element", 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if override.LevelDelta != 0 {
		t.Fatalf("normal gate delta = %d, want 0", override.LevelDelta)
	}
	overrides, err := appStore.VisibilityOverrides(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(overrides) != 1 {
		t.Fatalf("overrides after normal gate = %v, want one explicit override", overrides)
	}
	if err := appStore.DeleteVisibilityOverride(ctx, 1, "element", 1); err != nil {
		t.Fatal(err)
	}
	overrides, err = appStore.VisibilityOverrides(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(overrides) != 0 {
		t.Fatalf("overrides after reset = %v, want none", overrides)
	}
}

func TestInitializeViewNoiseGatePreservesConfiguredBypassesAndCreatesMissingOverrides(t *testing.T) {
	appStore := openAdapterTestStore(t)
	seedDensityView(t, appStore)
	ctx := context.Background()

	if _, err := appStore.DB().ExecContext(ctx, `UPDATE elements SET bypass_noise_gate = 1 WHERE id IN (105, 106)`); err != nil {
		t.Fatal(err)
	}
	if _, err := appStore.SetVisibilityOverride(ctx, 1, "element", 102, -2); err != nil {
		t.Fatal(err)
	}
	level := 0
	result, err := appStore.InitializeViewNoiseGate(ctx, 1, &level)
	if err != nil {
		t.Fatal(err)
	}
	if result.DensityLevel != 0 || result.ElementsEnabled != 0 || result.OverridesCreated != 5 {
		t.Fatalf("initialization result = %+v, want density 0, 0 enabled, 5 new overrides", result)
	}

	var bypassed int
	if err := appStore.DB().QueryRowContext(ctx, `SELECT COUNT(*) FROM elements WHERE id BETWEEN 101 AND 106 AND bypass_noise_gate = 1`).Scan(&bypassed); err != nil {
		t.Fatal(err)
	}
	if bypassed != 2 {
		t.Fatalf("bypassed placed elements = %d, want 2", bypassed)
	}
	storedLevel, err := appStore.ViewDensityLevel(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if storedLevel != 0 {
		t.Fatalf("stored density = %d, want 0", storedLevel)
	}

	overrides, err := appStore.VisibilityOverrides(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(overrides) != 6 {
		t.Fatalf("overrides = %v, want one per placed element", overrides)
	}
	if delta := densityOverrideDelta(overrides, 102); delta != -2 {
		t.Fatalf("existing override delta for 102 = %d, want preserved -2", delta)
	}
	if delta := densityOverrideDelta(overrides, 106); delta != 1 {
		t.Fatalf("inferred override delta for 106 = %d, want 1", delta)
	}

	level = -1
	second, err := appStore.InitializeViewNoiseGate(ctx, 1, &level)
	if err != nil {
		t.Fatal(err)
	}
	if second.OverridesCreated != 0 || second.DensityLevel != -1 {
		t.Fatalf("second initialization result = %+v, want idempotent overrides and density -1", second)
	}
}

func TestInitializeViewNoiseGatePreservesExplicitBypassOnReenable(t *testing.T) {
	appStore := openAdapterTestStore(t)
	seedDensityView(t, appStore)
	ctx := context.Background()

	level := 0
	first, err := appStore.InitializeViewNoiseGate(ctx, 1, &level)
	if err != nil {
		t.Fatal(err)
	}
	if first.ElementsEnabled != 6 || first.OverridesCreated != 6 {
		t.Fatalf("first initialization result = %+v, want 6 enabled and 6 new overrides", first)
	}
	if _, err := appStore.DB().ExecContext(ctx, `UPDATE elements SET bypass_noise_gate = 1 WHERE id = 106`); err != nil {
		t.Fatal(err)
	}
	if err := appStore.SetViewDensityLevel(ctx, 1, 2); err != nil {
		t.Fatal(err)
	}
	second, err := appStore.InitializeViewNoiseGate(ctx, 1, &level)
	if err != nil {
		t.Fatal(err)
	}
	if second.ElementsEnabled != 0 || second.OverridesCreated != 0 {
		t.Fatalf("second initialization result = %+v, want no element enablement or new overrides", second)
	}

	var bypassed bool
	if err := appStore.DB().QueryRowContext(ctx, `SELECT bypass_noise_gate FROM elements WHERE id = 106`).Scan(&bypassed); err != nil {
		t.Fatal(err)
	}
	if !bypassed {
		t.Fatal("element 106 bypass_noise_gate was reset after re-enabling the view noise gate")
	}
}

func TestInitializeViewNoiseGateProjectionUsesInferredLevels(t *testing.T) {
	appStore := openAdapterTestStore(t)
	seedDensityView(t, appStore)
	ctx := context.Background()

	level := -2
	if _, err := appStore.InitializeViewNoiseGate(ctx, 1, &level); err != nil {
		t.Fatal(err)
	}
	content, err := appStore.GetProjectedViewContent(ctx, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if containsPlacement(content.Placements, 106) {
		t.Fatal("element 106 should be hidden at quiet density after initialization")
	}

	if err := appStore.SetViewDensityLevel(ctx, 1, -1); err != nil {
		t.Fatal(err)
	}
	content, err = appStore.GetProjectedViewContent(ctx, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !containsPlacement(content.Placements, 106) {
		t.Fatal("element 106 should be visible at its inferred lean density")
	}
}

func TestDensityProjectionPromotedConnectorPullsEndpoints(t *testing.T) {
	appStore := openAdapterTestStore(t)
	seedDensityView(t, appStore)
	ctx := context.Background()

	if err := appStore.SetViewDensityLevel(ctx, 1, -2); err != nil {
		t.Fatal(err)
	}
	content, err := appStore.GetProjectedViewContent(ctx, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(content.Placements) != 4 {
		t.Fatalf("compact placements = %d, want soft cap 4", len(content.Placements))
	}
	if containsConnector(content.Connectors, 202) {
		t.Fatal("connector 202 should be outside the compact projection before override")
	}

	if _, err := appStore.AdjustVisibilityOverride(ctx, 1, "connector", 202, 1); err != nil {
		t.Fatal(err)
	}
	content, err = appStore.GetProjectedViewContent(ctx, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !containsConnector(content.Connectors, 202) || !containsPlacement(content.Placements, 105) || !containsPlacement(content.Placements, 106) {
		t.Fatalf("promoted connector did not pull endpoint: placements=%v connectors=%v", placementIDs(content.Placements), connectorIDs(content.Connectors))
	}
}

func TestDensityProjectionBypassNoiseGateDoesNotConsumeElementCap(t *testing.T) {
	appStore := openAdapterTestStore(t)
	seedDensityView(t, appStore)
	ctx := context.Background()

	if _, err := appStore.DB().ExecContext(ctx, `UPDATE elements SET bypass_noise_gate = 1 WHERE id = 106`); err != nil {
		t.Fatal(err)
	}
	if err := appStore.SetViewDensityLevel(ctx, 1, -2); err != nil {
		t.Fatal(err)
	}
	content, err := appStore.GetProjectedViewContent(ctx, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(content.Placements) != 5 {
		t.Fatalf("compact placements = %d, want cap 4 plus bypass element; ids=%v", len(content.Placements), placementIDs(content.Placements))
	}
	if !containsPlacement(content.Placements, 106) {
		t.Fatalf("bypass element missing from compact projection: ids=%v", placementIDs(content.Placements))
	}
}

func TestDensityProjectionBypassNoiseGateIgnoresOverrideUntilDisabled(t *testing.T) {
	appStore := openAdapterTestStore(t)
	seedDensityView(t, appStore)
	ctx := context.Background()

	if _, err := appStore.DB().ExecContext(ctx, `UPDATE elements SET bypass_noise_gate = 1 WHERE id = 106`); err != nil {
		t.Fatal(err)
	}
	if _, err := appStore.SetVisibilityOverride(ctx, 1, "element", 106, -2); err != nil {
		t.Fatal(err)
	}
	if err := appStore.SetViewDensityLevel(ctx, 1, -2); err != nil {
		t.Fatal(err)
	}

	content, err := appStore.GetProjectedViewContent(ctx, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !containsPlacement(content.Placements, 106) {
		t.Fatal("bypass element should ignore its element visibility override")
	}

	if _, err := appStore.DB().ExecContext(ctx, `UPDATE elements SET bypass_noise_gate = 0 WHERE id = 106`); err != nil {
		t.Fatal(err)
	}
	content, err = appStore.GetProjectedViewContent(ctx, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if containsPlacement(content.Placements, 106) {
		t.Fatal("element visibility override should apply again when bypass is disabled")
	}
}

func TestRichNoiseGateRemainsVisibleAtRichAndFullDensity(t *testing.T) {
	appStore := openAdapterTestStore(t)
	seedDensityView(t, appStore)
	ctx := context.Background()

	if err := appStore.SetViewDensityLevel(ctx, 1, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := appStore.SetVisibilityOverride(ctx, 1, "element", 102, -1); err != nil {
		t.Fatal(err)
	}
	content, err := appStore.GetProjectedViewContent(ctx, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !containsPlacement(content.Placements, 102) {
		t.Fatal("rich-gated element should be visible at full density")
	}
	if !containsConnector(content.Connectors, 201) {
		t.Fatal("connector incident to a visible rich-gated element should remain visible at full density")
	}

	if err := appStore.SetViewDensityLevel(ctx, 1, 1); err != nil {
		t.Fatal(err)
	}
	content, err = appStore.GetProjectedViewContent(ctx, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !containsPlacement(content.Placements, 102) {
		t.Fatal("rich-gated element should be visible at rich density")
	}
	if !containsConnector(content.Connectors, 201) {
		t.Fatal("connector incident to a visible rich-gated element should remain visible at rich density")
	}
}

func TestFullNoiseGateHidesUntilFullDensity(t *testing.T) {
	appStore := openAdapterTestStore(t)
	seedDensityView(t, appStore)
	ctx := context.Background()

	override, err := appStore.SetVisibilityOverride(ctx, 1, "element", 102, -4)
	if err != nil {
		t.Fatal(err)
	}
	if override.LevelDelta != -2 {
		t.Fatalf("delta = %d, want clamp to -2", override.LevelDelta)
	}

	if err := appStore.SetViewDensityLevel(ctx, 1, 2); err != nil {
		t.Fatal(err)
	}
	content, err := appStore.GetProjectedViewContent(ctx, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !containsPlacement(content.Placements, 102) {
		t.Fatal("full-gated element should be visible at full density")
	}
	if !containsConnector(content.Connectors, 201) {
		t.Fatal("connector incident to full-gated element should be visible at full density")
	}

	if err := appStore.SetViewDensityLevel(ctx, 1, 1); err != nil {
		t.Fatal(err)
	}
	content, err = appStore.GetProjectedViewContent(ctx, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if containsPlacement(content.Placements, 102) {
		t.Fatal("full-gated element should be hidden before full density")
	}
	if containsConnector(content.Connectors, 201) {
		t.Fatal("connector incident to full-gated element should be hidden before full density")
	}
}

func TestElementNoiseGateThresholdForcesVisibilityAtSelectedDensity(t *testing.T) {
	appStore := openAdapterTestStore(t)
	seedDensityView(t, appStore)
	ctx := context.Background()

	if _, err := appStore.SetVisibilityOverride(ctx, 1, "element", 106, 1); err != nil {
		t.Fatal(err)
	}

	if err := appStore.SetViewDensityLevel(ctx, 1, -2); err != nil {
		t.Fatal(err)
	}
	content, err := appStore.GetProjectedViewContent(ctx, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if containsPlacement(content.Placements, 106) {
		t.Fatal("lean-gated element should be hidden at quiet density")
	}

	if err := appStore.SetViewDensityLevel(ctx, 1, -1); err != nil {
		t.Fatal(err)
	}
	content, err = appStore.GetProjectedViewContent(ctx, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !containsPlacement(content.Placements, 106) {
		t.Fatal("lean-gated element should be visible at lean density")
	}
}

func TestElementNormalNoiseGateIsExplicitOverride(t *testing.T) {
	appStore := openAdapterTestStore(t)
	seedDensityView(t, appStore)
	ctx := context.Background()

	if _, err := appStore.SetVisibilityOverride(ctx, 1, "element", 106, 0); err != nil {
		t.Fatal(err)
	}
	overrides, err := appStore.VisibilityOverrides(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(overrides) != 1 || overrides[0].LevelDelta != 0 {
		t.Fatalf("normal gate override = %v, want one level_delta=0 row", overrides)
	}

	if err := appStore.SetViewDensityLevel(ctx, 1, -1); err != nil {
		t.Fatal(err)
	}
	content, err := appStore.GetProjectedViewContent(ctx, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if containsPlacement(content.Placements, 106) {
		t.Fatal("normal-gated element should be hidden before normal density")
	}

	if err := appStore.SetViewDensityLevel(ctx, 1, 0); err != nil {
		t.Fatal(err)
	}
	content, err = appStore.GetProjectedViewContent(ctx, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !containsPlacement(content.Placements, 106) {
		t.Fatal("normal-gated element should be visible at normal density")
	}

	if err := appStore.DeleteVisibilityOverride(ctx, 1, "element", 106); err != nil {
		t.Fatal(err)
	}
	overrides, err = appStore.VisibilityOverrides(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(overrides) != 0 {
		t.Fatalf("overrides after reset = %v, want none", overrides)
	}
}

func densityOverrideDelta(overrides []app.VisibilityOverride, elementID int64) int {
	for _, override := range overrides {
		if override.ResourceType == "element" && override.ResourceID == elementID {
			return override.LevelDelta
		}
	}
	return 999
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

func TestDensityProjectionDependencyGroup(t *testing.T) {
	appStore := openAdapterTestStore(t)
	ctx := context.Background()

	// Seed one regular element (id 301, kind "component") and one dependency-group element (id 302, kind "dependency-group")
	// and a connector (id 401) between them.
	if _, err := appStore.DB().Exec(`
		INSERT INTO views(id, name, created_at, updated_at)
		VALUES (5, 'Test View 5', 'now', 'now');
		INSERT INTO elements(id, name, kind, tags, technology_connectors, created_at, updated_at)
		VALUES
			(301, 'Regular Component', 'component', '[]', '[]', 'now', 'now'),
			(302, 'Dep Group', 'dependency-group', '[]', '[]', 'now', 'now');
		INSERT INTO placements(view_id, element_id, position_x, position_y, created_at, updated_at)
		VALUES
			(5, 301, 100, 100, 'now', 'now'),
			(5, 302, 200, 200, 'now', 'now');
		INSERT INTO connectors(id, view_id, source_element_id, target_element_id, label, direction, style, created_at, updated_at)
		VALUES
			(401, 5, 301, 302, 'depends', 'forward', 'bezier', 'now', 'now');
	`); err != nil {
		t.Fatal(err)
	}

	// 1. Check at density level 2 (max density) - both elements and connector should be visible
	if err := appStore.SetViewDensityLevel(ctx, 5, 2); err != nil {
		t.Fatal(err)
	}
	content, err := appStore.GetProjectedViewContent(ctx, 5, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !containsPlacement(content.Placements, 301) {
		t.Fatal("expected regular element 301 to be visible at density 2")
	}
	if !containsPlacement(content.Placements, 302) {
		t.Fatal("expected dependency-group element 302 to be visible at density 2")
	}
	if !containsConnector(content.Connectors, 401) {
		t.Fatal("expected connector 401 to be visible at density 2")
	}

	// 2. Check at density level 1 - dependency-group element and connector should be pruned
	if err := appStore.SetViewDensityLevel(ctx, 5, 1); err != nil {
		t.Fatal(err)
	}
	content, err = appStore.GetProjectedViewContent(ctx, 5, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !containsPlacement(content.Placements, 301) {
		t.Fatal("expected regular element 301 to be visible at density 1")
	}
	if containsPlacement(content.Placements, 302) {
		t.Fatal("expected dependency-group element 302 to be pruned at density 1")
	}
	if containsConnector(content.Connectors, 401) {
		t.Fatal("expected connector 401 to be pruned at density 1")
	}

	// 3. Check that a positive visibility override does NOT force dependency-group element to be shown at density level 1
	if _, err := appStore.SetVisibilityOverride(ctx, 5, "element", 302, 1); err != nil {
		t.Fatal(err)
	}
	content, err = appStore.GetProjectedViewContent(ctx, 5, nil)
	if err != nil {
		t.Fatal(err)
	}
	if containsPlacement(content.Placements, 302) {
		t.Fatal("expected dependency-group element 302 to remain pruned at density 1 even with positive override")
	}
	if containsConnector(content.Connectors, 401) {
		t.Fatal("expected connector 401 to remain pruned at density 1 even with positive override")
	}
}

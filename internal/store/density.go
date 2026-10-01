package store

import (
	"context"
	"time"

	"github.com/mertcikla/tld/v2/pkg/app"
	"github.com/uptrace/bun"
)

type VisibilityOverride = app.VisibilityOverride

type ProjectedViewContent = app.ProjectedViewContent

type NoiseGateInitialization struct {
	ViewID           int64 `json:"view_id"`
	DensityLevel     int   `json:"density_level"`
	ElementsEnabled  int   `json:"elements_enabled"`
	OverridesCreated int   `json:"overrides_created"`
}

const (
	MinDensityLevel  = app.MinDensityLevel
	MaxDensityLevel  = app.MaxDensityLevel
	MinOverrideDelta = app.MinOverrideDelta
	MaxOverrideDelta = app.MaxOverrideDelta
)

func ValidateDensityLevel(level int) error { return app.ValidateDensityLevel(level) }

func ValidateResourceType(resourceType string) error { return app.ValidateResourceType(resourceType) }

func (s *SQLiteStore) ViewDensityLevel(ctx context.Context, viewID int64) (int, error) {
	return s.legacy.ViewDensityLevel(ctx, viewID)
}

func (s *SQLiteStore) SetViewDensityLevel(ctx context.Context, viewID int64, level int) error {
	return s.legacy.SetViewDensityLevel(ctx, viewID, level)
}

func (s *SQLiteStore) VisibilityOverrides(ctx context.Context, viewID int64) ([]VisibilityOverride, error) {
	return s.legacy.VisibilityOverrides(ctx, viewID)
}

func (s *SQLiteStore) SetVisibilityOverride(ctx context.Context, viewID int64, resourceType string, resourceID int64, delta int) (VisibilityOverride, error) {
	return s.legacy.SetVisibilityOverride(ctx, viewID, resourceType, resourceID, delta)
}

func (s *SQLiteStore) AdjustVisibilityOverride(ctx context.Context, viewID int64, resourceType string, resourceID int64, step int) (VisibilityOverride, error) {
	return s.legacy.AdjustVisibilityOverride(ctx, viewID, resourceType, resourceID, step)
}

func (s *SQLiteStore) DeleteVisibilityOverride(ctx context.Context, viewID int64, resourceType string, resourceID int64) error {
	return s.legacy.DeleteVisibilityOverride(ctx, viewID, resourceType, resourceID)
}

func (s *SQLiteStore) DeleteResourceVisibilityOverrides(ctx context.Context, resourceType string, resourceID int64) error {
	return s.legacy.DeleteResourceVisibilityOverrides(ctx, resourceType, resourceID)
}

func (s *SQLiteStore) InitializeViewNoiseGate(ctx context.Context, viewID int64, densityLevel *int) (NoiseGateInitialization, error) {
	currentLevel, err := s.legacy.ViewDensityLevel(ctx, viewID)
	if err != nil {
		return NoiseGateInitialization{}, err
	}
	targetLevel := currentLevel
	if densityLevel != nil {
		if err := app.ValidateDensityLevel(*densityLevel); err != nil {
			return NoiseGateInitialization{}, err
		}
		targetLevel = *densityLevel
	}

	placements, err := s.legacy.Placements(ctx, viewID)
	if err != nil {
		return NoiseGateInitialization{}, err
	}
	connectors, err := s.legacy.Connectors(ctx, viewID)
	if err != nil {
		return NoiseGateInitialization{}, err
	}
	overrides, err := s.legacy.VisibilityOverrides(ctx, viewID)
	if err != nil {
		return NoiseGateInitialization{}, err
	}

	signals := app.EmptyDensitySignals()
	if len(placements) > 0 {
		signals, err = s.densitySignals(ctx, placements, connectors)
		if err != nil {
			return NoiseGateInitialization{}, err
		}
	}
	levels := app.InferElementGateLevels(placements, connectors, overrides, signals)

	elementIDs := make([]int64, 0, len(placements))
	seenElementIDs := make(map[int64]struct{}, len(placements))
	for _, placement := range placements {
		if _, ok := seenElementIDs[placement.ElementID]; ok {
			continue
		}
		seenElementIDs[placement.ElementID] = struct{}{}
		elementIDs = append(elementIDs, placement.ElementID)
	}

	existingElementOverrides := make(map[int64]struct{})
	for _, override := range overrides {
		if override.ResourceType == "element" {
			existingElementOverrides[override.ResourceID] = struct{}{}
		}
	}
	enableElements := len(overrides) == 0
	elementsEnabled := 0
	if enableElements {
		elementsEnabled = len(elementIDs)
	}

	now := nowString()
	newOverrides := make([]visibilityOverrideModel, 0, len(elementIDs))
	for _, elementID := range elementIDs {
		if _, exists := existingElementOverrides[elementID]; exists {
			continue
		}
		level, ok := levels[elementID]
		if !ok {
			level = app.MaxDensityLevel
		}
		newOverrides = append(newOverrides, visibilityOverrideModel{
			ViewID:       viewID,
			ResourceType: "element",
			ResourceID:   elementID,
			LevelDelta:   -level,
			CreatedAt:    now,
			UpdatedAt:    now,
		})
	}

	if err := s.legacy.BunDB().RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		if _, err := tx.NewUpdate().
			Table("views").
			Set("density_level = ?", targetLevel).
			Set("updated_at = ?", now).
			Where("id = ?", viewID).
			Exec(ctx); err != nil {
			return err
		}
		if enableElements && len(elementIDs) > 0 {
			if _, err := tx.NewUpdate().
				Table("elements").
				Set("bypass_noise_gate = ?", false).
				Set("updated_at = ?", now).
				Where("id IN (?)", bun.List(elementIDs)).
				Exec(ctx); err != nil {
				return err
			}
		}
		if len(newOverrides) > 0 {
			if _, err := tx.NewInsert().
				Model(&newOverrides).
				On("CONFLICT(view_id, resource_type, resource_id) DO NOTHING").
				Exec(ctx); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return NoiseGateInitialization{}, err
	}

	return NoiseGateInitialization{
		ViewID:           viewID,
		DensityLevel:     targetLevel,
		ElementsEnabled:  elementsEnabled,
		OverridesCreated: len(newOverrides),
	}, nil
}

func (s *SQLiteStore) ExportDensityState(ctx context.Context) (map[int64]int, []VisibilityOverride, error) {
	levels := map[int64]int{}
	var levelRows []struct {
		ID           int64 `bun:"id"`
		DensityLevel int   `bun:"density_level"`
	}
	if err := s.legacy.BunDB().NewSelect().
		Table("views").
		Column("id", "density_level").
		Order("id").
		Scan(ctx, &levelRows); err != nil {
		return nil, nil, err
	}
	for _, row := range levelRows {
		if row.DensityLevel != 0 {
			levels[row.ID] = row.DensityLevel
		}
	}

	var overrideRows []visibilityOverrideModel
	if err := s.legacy.BunDB().NewSelect().
		Model(&overrideRows).
		Order("view_id").
		Order("resource_type").
		Order("resource_id").
		Scan(ctx); err != nil {
		return nil, nil, err
	}
	overrides := make([]VisibilityOverride, 0, len(overrideRows))
	for _, row := range overrideRows {
		overrides = append(overrides, visibilityOverrideFromModel(row))
	}
	return levels, overrides, nil
}

func (s *SQLiteStore) ProjectedViewContent(ctx context.Context, viewID int64, densityOverride *int) (ProjectedViewContent, error) {
	level, err := s.legacy.ViewDensityLevel(ctx, viewID)
	if err != nil {
		return ProjectedViewContent{}, err
	}
	if densityOverride != nil {
		if err := app.ValidateDensityLevel(*densityOverride); err != nil {
			return ProjectedViewContent{}, err
		}
		level = *densityOverride
	}
	placements, err := s.legacy.Placements(ctx, viewID)
	if err != nil {
		return ProjectedViewContent{}, err
	}
	connectors, err := s.legacy.Connectors(ctx, viewID)
	if err != nil {
		return ProjectedViewContent{}, err
	}
	if len(placements) == 0 {
		return ProjectedViewContent{Placements: placements, Connectors: connectors}, nil
	}
	caps := app.CapsForDensity(level)
	overrides, err := s.legacy.VisibilityOverrides(ctx, viewID)
	if err != nil {
		return ProjectedViewContent{}, err
	}

	signals := app.EmptyDensitySignals()
	if !caps.Full {
		var err error
		signals, err = s.densitySignals(ctx, placements, connectors)
		if err != nil {
			return ProjectedViewContent{}, err
		}
	}
	return app.ProjectViewContent(placements, connectors, overrides, level, signals), nil
}

// densitySignals returns the per-resource ranking signals used by the density
// engine. The legacy watch filter/architecture signals were removed with the
// watch pipeline; codeindex visibility now drives each generated element's
// noise-gate bypass instead, so no extra signals are loaded here.
func (s *SQLiteStore) densitySignals(context.Context, []app.PlacedElement, []app.Connector) (app.DensitySignals, error) {
	return app.EmptyDensitySignals(), nil
}

func nowString() string {
	return time.Now().UTC().Format(time.RFC3339)
}

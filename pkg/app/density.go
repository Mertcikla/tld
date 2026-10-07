package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"sort"

	"github.com/uptrace/bun"
)

const (
	MinDensityLevel  = -2
	MaxDensityLevel  = 2
	MinOverrideDelta = MinDensityLevel
	MaxOverrideDelta = MaxDensityLevel
)

type DensitySignalKey struct {
	ResourceType string
	ResourceID   int64
}

type DensitySignals struct {
	FilterScore            map[DensitySignalKey]float64
	FilterTier             map[DensitySignalKey]int
	ArchitectureConfidence map[DensitySignalKey]float64
}

type DensityCaps struct {
	Elements   int
	Connectors int
	Full       bool
}

type VisibilityOverride struct {
	ViewID       int64  `json:"view_id"`
	ResourceType string `json:"resource_type"`
	ResourceID   int64  `json:"resource_id"`
	LevelDelta   int    `json:"level_delta"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

type ProjectedViewContent struct {
	Placements []PlacedElement `json:"placements"`
	Connectors []Connector     `json:"connectors"`
}

func ValidateDensityLevel(level int) error {
	if level < MinDensityLevel || level > MaxDensityLevel {
		return fmt.Errorf("density_level must be between %d and %d", MinDensityLevel, MaxDensityLevel)
	}
	return nil
}

func ValidateResourceType(resourceType string) error {
	if resourceType != "element" && resourceType != "connector" {
		return errors.New("resource_type must be element or connector")
	}
	return nil
}

func ClampOverrideDelta(delta int) int {
	return min(MaxOverrideDelta, max(MinOverrideDelta, delta))
}

func noiseGateLevelForDelta(delta int) int {
	return -ClampOverrideDelta(delta)
}

func EmptyDensitySignals() DensitySignals {
	return DensitySignals{
		FilterScore:            map[DensitySignalKey]float64{},
		FilterTier:             map[DensitySignalKey]int{},
		ArchitectureConfidence: map[DensitySignalKey]float64{},
	}
}

func CapsForDensity(level int) DensityCaps {
	switch level {
	case -2:
		return DensityCaps{Elements: 4, Connectors: 8}
	case -1:
		return DensityCaps{Elements: 8, Connectors: 16}
	case 1:
		return DensityCaps{Elements: 32, Connectors: 64}
	case 2:
		return DensityCaps{Full: true}
	default:
		return DensityCaps{Elements: 12, Connectors: 24}
	}
}

type rankedElement struct {
	item         PlacedElement
	score        float64
	forceVisible bool
}

type rankedConnector struct {
	item  Connector
	score float64
	boost int
}

func ProjectViewContent(placements []PlacedElement, connectors []Connector, overrides []VisibilityOverride, level int, signals DensitySignals) ProjectedViewContent {
	minLevel := MinDensityLevel
	if level < minLevel {
		minLevel = level
	}
	visibleElements := make(map[int64]struct{})
	visibleConnectors := make(map[int64]struct{})
	for currentLevel := minLevel; currentLevel <= level; currentLevel++ {
		content := projectViewContentAtLevel(placements, connectors, overrides, currentLevel, signals)
		for _, placement := range content.Placements {
			visibleElements[placement.ElementID] = struct{}{}
		}
		for _, connector := range content.Connectors {
			visibleConnectors[connector.ID] = struct{}{}
		}
	}

	outPlacements := make([]PlacedElement, 0, len(visibleElements))
	for _, placement := range placements {
		if _, ok := visibleElements[placement.ElementID]; ok {
			outPlacements = append(outPlacements, placement)
		}
	}
	outConnectors := make([]Connector, 0, len(visibleConnectors))
	for _, connector := range connectors {
		if _, ok := visibleConnectors[connector.ID]; ok {
			outConnectors = append(outConnectors, connector)
		}
	}
	return ProjectedViewContent{Placements: outPlacements, Connectors: outConnectors}
}

func InferElementGateLevels(placements []PlacedElement, connectors []Connector, overrides []VisibilityOverride, signals DensitySignals) map[int64]int {
	normalizedPlacements := make([]PlacedElement, len(placements))
	copy(normalizedPlacements, placements)
	for index := range normalizedPlacements {
		normalizedPlacements[index].BypassNoiseGate = false
	}

	levels := make(map[int64]int, len(normalizedPlacements))
	for level := MinDensityLevel; level <= MaxDensityLevel; level++ {
		content := ProjectViewContent(normalizedPlacements, connectors, overrides, level, signals)
		for _, placement := range content.Placements {
			if _, ok := levels[placement.ElementID]; !ok {
				levels[placement.ElementID] = level
			}
		}
	}
	for _, placement := range normalizedPlacements {
		if _, ok := levels[placement.ElementID]; !ok {
			levels[placement.ElementID] = MaxDensityLevel
		}
	}
	return levels
}

func projectViewContentAtLevel(placements []PlacedElement, connectors []Connector, overrides []VisibilityOverride, level int, signals DensitySignals) ProjectedViewContent {
	caps := CapsForDensity(level)
	elementGateLevels := make(map[int64]int)
	connectorGateLevels := make(map[int64]int)
	for _, override := range overrides {
		gateLevel := noiseGateLevelForDelta(override.LevelDelta)
		switch override.ResourceType {
		case "element":
			elementGateLevels[override.ResourceID] = gateLevel
		case "connector":
			connectorGateLevels[override.ResourceID] = gateLevel
		}
	}

	elementKinds := make(map[int64]string)
	for _, placement := range placements {
		if placement.Kind != nil {
			elementKinds[placement.ElementID] = *placement.Kind
		}
	}

	degree := make(map[int64]int)
	for _, connector := range connectors {
		degree[connector.SourceElementID]++
		degree[connector.TargetElementID]++
	}

	rankedElements := make([]rankedElement, 0, len(placements))
	gatedElements := make(map[int64]struct{})
	for _, placement := range placements {
		if placement.BypassNoiseGate {
			rankedElements = append(rankedElements, rankedElement{
				item:         placement,
				score:        baseElementScore(placement, degree[placement.ElementID], signals),
				forceVisible: true,
			})
			continue
		}
		if level < 2 && placement.Kind != nil && *placement.Kind == "dependency-group" {
			continue
		}
		gateLevel, hasGate := elementGateLevels[placement.ElementID]
		if hasGate && level < gateLevel {
			gatedElements[placement.ElementID] = struct{}{}
			continue
		}
		boost := -gateLevel
		rankedElements = append(rankedElements, rankedElement{
			item:         placement,
			score:        baseElementScore(placement, degree[placement.ElementID], signals) + float64(boost)*100,
			forceVisible: hasGate,
		})
	}
	sort.SliceStable(rankedElements, func(i, j int) bool {
		if rankedElements[i].score == rankedElements[j].score {
			return rankedElements[i].item.ID < rankedElements[j].item.ID
		}
		return rankedElements[i].score > rankedElements[j].score
	})

	visibleElements := make(map[int64]struct{})
	elementLimit := caps.Elements
	if caps.Full {
		elementLimit = len(rankedElements)
	}
	cappedElementCount := 0
	for _, ranked := range rankedElements {
		if !caps.Full && !ranked.forceVisible {
			if cappedElementCount >= elementLimit {
				continue
			}
			cappedElementCount++
		}
		visibleElements[ranked.item.ElementID] = struct{}{}
	}

	rankedConnectors := make([]rankedConnector, 0, len(connectors))
	for _, connector := range connectors {
		if level < 2 && (elementKinds[connector.SourceElementID] == "dependency-group" || elementKinds[connector.TargetElementID] == "dependency-group") {
			continue
		}
		if _, ok := gatedElements[connector.SourceElementID]; ok {
			continue
		}
		if _, ok := gatedElements[connector.TargetElementID]; ok {
			continue
		}
		boost := -connectorGateLevels[connector.ID]
		rankedConnectors = append(rankedConnectors, rankedConnector{
			item:  connector,
			score: baseConnectorScore(connector, signals) + float64(boost)*100,
			boost: boost,
		})
	}
	sort.SliceStable(rankedConnectors, func(i, j int) bool {
		if rankedConnectors[i].score == rankedConnectors[j].score {
			return rankedConnectors[i].item.ID < rankedConnectors[j].item.ID
		}
		return rankedConnectors[i].score > rankedConnectors[j].score
	})

	visibleConnectors := make(map[int64]struct{})
	connectorLimit := caps.Connectors
	if caps.Full {
		connectorLimit = len(rankedConnectors)
	}
	cappedConnectorCount := 0
	for _, ranked := range rankedConnectors {
		connector := ranked.item
		if ranked.boost > 0 {
			visibleElements[connector.SourceElementID] = struct{}{}
			visibleElements[connector.TargetElementID] = struct{}{}
		}
		_, sourceVisible := visibleElements[connector.SourceElementID]
		_, targetVisible := visibleElements[connector.TargetElementID]
		if !sourceVisible || !targetVisible {
			continue
		}
		if !caps.Full && ranked.boost <= 0 {
			if cappedConnectorCount >= connectorLimit {
				continue
			}
			cappedConnectorCount++
		}
		visibleConnectors[connector.ID] = struct{}{}
	}

	outPlacements := make([]PlacedElement, 0, len(visibleElements))
	for _, placement := range placements {
		if _, ok := visibleElements[placement.ElementID]; ok {
			outPlacements = append(outPlacements, placement)
		}
	}
	outConnectors := make([]Connector, 0, len(visibleConnectors))
	for _, connector := range connectors {
		if _, ok := visibleConnectors[connector.ID]; ok {
			outConnectors = append(outConnectors, connector)
		}
	}
	return ProjectedViewContent{Placements: outPlacements, Connectors: outConnectors}
}

func baseElementScore(placement PlacedElement, degree int, signals DensitySignals) float64 {
	if placement.Kind != nil && *placement.Kind == "dependency-group" {
		return -1000.0
	}
	score := float64(degree) * 12
	key := DensitySignalKey{ResourceType: "element", ResourceID: placement.ElementID}
	score += signals.FilterScore[key] * 30
	if tier, ok := signals.FilterTier[key]; ok {
		score += float64(max(0, 10-tier)) * 5
	}
	score += signals.ArchitectureConfidence[key] * 20
	if placement.HasView {
		score += 20
	}
	if placement.Description != nil && *placement.Description != "" {
		score += 4
	}
	if len(placement.Tags) > 0 {
		score += 3
	}
	if placement.FilePath != nil && *placement.FilePath != "" {
		score += 2
	}
	return score - math.Log1p(float64(max(0, placement.ID)))*0.001
}

func baseConnectorScore(connector Connector, signals DensitySignals) float64 {
	score := 0.0
	key := DensitySignalKey{ResourceType: "connector", ResourceID: connector.ID}
	score += signals.FilterScore[key] * 30
	if tier, ok := signals.FilterTier[key]; ok {
		score += float64(max(0, 10-tier)) * 5
	}
	score += signals.ArchitectureConfidence[key] * 20
	if connector.Relationship != nil && *connector.Relationship != "" {
		score += 10
	}
	if connector.Label != nil && *connector.Label != "" {
		score += 6
	}
	if connector.Description != nil && *connector.Description != "" {
		score += 3
	}
	return score - math.Log1p(float64(max(0, connector.ID)))*0.001
}

func (s *Store) ViewDensityLevel(ctx context.Context, viewID int64) (int, error) {
	var row struct {
		DensityLevel int `bun:"density_level"`
	}
	err := s.bun.NewSelect().
		Model((*viewModel)(nil)).
		Column("density_level").
		Where("id = ?", viewID).
		Scan(ctx, &row)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	return row.DensityLevel, err
}

func (s *Store) SetViewDensityLevel(ctx context.Context, viewID int64, level int) error {
	if err := ValidateDensityLevel(level); err != nil {
		return err
	}
	res, err := s.bun.NewUpdate().
		Model((*viewModel)(nil)).
		Set("density_level = ?", level).
		Set("updated_at = ?", nowString()).
		Where("id = ?", viewID).
		Exec(ctx)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) VisibilityOverrides(ctx context.Context, viewID int64) ([]VisibilityOverride, error) {
	var rows []visibilityOverrideModel
	if err := s.bun.NewSelect().
		Model(&rows).
		Where("view_id = ?", viewID).
		Order("resource_type").
		Order("resource_id").
		Scan(ctx); err != nil {
		return nil, err
	}
	out := make([]VisibilityOverride, 0, len(rows))
	for _, row := range rows {
		out = append(out, VisibilityOverride{
			ViewID:       row.ViewID,
			ResourceType: row.ResourceType,
			ResourceID:   row.ResourceID,
			LevelDelta:   ClampOverrideDelta(row.LevelDelta),
			CreatedAt:    row.CreatedAt,
			UpdatedAt:    row.UpdatedAt,
		})
	}
	return out, nil
}

func (s *Store) SetVisibilityOverride(ctx context.Context, viewID int64, resourceType string, resourceID int64, delta int) (VisibilityOverride, error) {
	if err := ValidateResourceType(resourceType); err != nil {
		return VisibilityOverride{}, err
	}
	delta = ClampOverrideDelta(delta)
	now := nowString()
	row := &visibilityOverrideModel{ViewID: viewID, ResourceType: resourceType, ResourceID: resourceID, LevelDelta: delta, CreatedAt: now, UpdatedAt: now}
	_, err := s.bun.NewInsert().
		Model(row).
		On("CONFLICT(view_id, resource_type, resource_id) DO UPDATE").
		Set("level_delta = excluded.level_delta").
		Set("updated_at = excluded.updated_at").
		Exec(ctx)
	if err != nil {
		return VisibilityOverride{}, err
	}
	return s.visibilityOverride(ctx, viewID, resourceType, resourceID)
}

func (s *Store) AdjustVisibilityOverride(ctx context.Context, viewID int64, resourceType string, resourceID int64, step int) (VisibilityOverride, error) {
	if err := ValidateResourceType(resourceType); err != nil {
		return VisibilityOverride{}, err
	}
	var row visibilityOverrideModel
	err := s.bun.NewSelect().
		Model(&row).
		Column("level_delta").
		Where("view_id = ?", viewID).
		Where("resource_type = ?", resourceType).
		Where("resource_id = ?", resourceID).
		Scan(ctx)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return VisibilityOverride{}, err
	}
	return s.SetVisibilityOverride(ctx, viewID, resourceType, resourceID, row.LevelDelta+step)
}

func (s *Store) DeleteVisibilityOverride(ctx context.Context, viewID int64, resourceType string, resourceID int64) error {
	if err := ValidateResourceType(resourceType); err != nil {
		return err
	}
	_, err := s.bun.NewDelete().
		Model((*visibilityOverrideModel)(nil)).
		Where("view_id = ?", viewID).
		Where("resource_type = ?", resourceType).
		Where("resource_id = ?", resourceID).
		Exec(ctx)
	return err
}

func (s *Store) DeleteResourceVisibilityOverrides(ctx context.Context, resourceType string, resourceID int64) error {
	if err := ValidateResourceType(resourceType); err != nil {
		return err
	}
	_, err := s.bun.NewDelete().
		Model((*visibilityOverrideModel)(nil)).
		Where("resource_type = ?", resourceType).
		Where("resource_id = ?", resourceID).
		Exec(ctx)
	return err
}

func (s *Store) visibilityOverride(ctx context.Context, viewID int64, resourceType string, resourceID int64) (VisibilityOverride, error) {
	var row visibilityOverrideModel
	err := s.bun.NewSelect().
		Model(&row).
		Where("view_id = ?", viewID).
		Where("resource_type = ?", resourceType).
		Where("resource_id = ?", resourceID).
		Scan(ctx)
	if err != nil {
		return VisibilityOverride{}, err
	}
	return VisibilityOverride{
		ViewID:       row.ViewID,
		ResourceType: row.ResourceType,
		ResourceID:   row.ResourceID,
		LevelDelta:   ClampOverrideDelta(row.LevelDelta),
		CreatedAt:    row.CreatedAt,
		UpdatedAt:    row.UpdatedAt,
	}, nil
}

func (s *Store) GetProjectedViewContent(ctx context.Context, viewID int64, densityOverride *int) (ProjectedViewContent, error) {
	level, err := s.ViewDensityLevel(ctx, viewID)
	if err != nil {
		return ProjectedViewContent{}, err
	}
	if densityOverride != nil {
		if err := ValidateDensityLevel(*densityOverride); err != nil {
			return ProjectedViewContent{}, err
		}
		level = *densityOverride
	}
	placements, err := s.Placements(ctx, viewID)
	if err != nil {
		return ProjectedViewContent{}, err
	}
	connectors, err := s.Connectors(ctx, viewID)
	if err != nil {
		return ProjectedViewContent{}, err
	}
	if len(placements) == 0 {
		return ProjectedViewContent{Placements: placements, Connectors: connectors}, nil
	}
	overrides, err := s.VisibilityOverrides(ctx, viewID)
	if err != nil {
		return ProjectedViewContent{}, err
	}
	return ProjectViewContent(placements, connectors, overrides, level, EmptyDensitySignals()), nil
}

// NoiseGateInitialization reports the outcome of InitializeViewNoiseGate.
type NoiseGateInitialization struct {
	ViewID           int64 `json:"view_id"`
	DensityLevel     int   `json:"density_level"`
	ElementsEnabled  int   `json:"elements_enabled"`
	OverridesCreated int   `json:"overrides_created"`
}

// InitializeViewNoiseGate applies the density level to a view and creates the
// per-element visibility overrides the gate needs, preserving existing
// overrides and bypasses.
func (s *Store) InitializeViewNoiseGate(ctx context.Context, viewID int64, densityLevel *int) (NoiseGateInitialization, error) {
	currentLevel, err := s.ViewDensityLevel(ctx, viewID)
	if err != nil {
		return NoiseGateInitialization{}, err
	}
	targetLevel := currentLevel
	if densityLevel != nil {
		if err := ValidateDensityLevel(*densityLevel); err != nil {
			return NoiseGateInitialization{}, err
		}
		targetLevel = *densityLevel
	}

	placements, err := s.Placements(ctx, viewID)
	if err != nil {
		return NoiseGateInitialization{}, err
	}
	connectors, err := s.Connectors(ctx, viewID)
	if err != nil {
		return NoiseGateInitialization{}, err
	}
	overrides, err := s.VisibilityOverrides(ctx, viewID)
	if err != nil {
		return NoiseGateInitialization{}, err
	}

	signals := EmptyDensitySignals()
	levels := InferElementGateLevels(placements, connectors, overrides, signals)

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
			level = MaxDensityLevel
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

	if err := s.QueryDB().RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
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

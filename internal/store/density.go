package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/mertcikla/tld/internal/app"
)

const (
	MinDensityLevel  = -2
	MaxDensityLevel  = 2
	MinOverrideDelta = -4
	MaxOverrideDelta = 4
)

type VisibilityOverride struct {
	ViewID       int64  `json:"view_id"`
	ResourceType string `json:"resource_type"`
	ResourceID   int64  `json:"resource_id"`
	LevelDelta   int    `json:"level_delta"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

type ProjectedViewContent struct {
	Placements []app.PlacedElement
	Connectors []app.Connector
}

type densitySignals struct {
	filterScore            map[string]float64
	filterTier             map[string]int
	architectureConfidence map[string]float64
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

func clampOverrideDelta(delta int) int {
	return min(MaxOverrideDelta, max(MinOverrideDelta, delta))
}

func (s *SQLiteStore) ViewDensityLevel(ctx context.Context, viewID int64) (int, error) {
	var level int
	err := s.DB().QueryRowContext(ctx, `SELECT density_level FROM views WHERE id = ?`, viewID).Scan(&level)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	return level, err
}

func (s *SQLiteStore) SetViewDensityLevel(ctx context.Context, viewID int64, level int) error {
	if err := ValidateDensityLevel(level); err != nil {
		return err
	}
	res, err := s.DB().ExecContext(ctx, `UPDATE views SET density_level = ?, updated_at = ? WHERE id = ?`, level, nowString(), viewID)
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

func (s *SQLiteStore) VisibilityOverrides(ctx context.Context, viewID int64) ([]VisibilityOverride, error) {
	rows, err := s.DB().QueryContext(ctx, `
		SELECT view_id, resource_type, resource_id, level_delta, created_at, updated_at
		FROM view_visibility_overrides
		WHERE view_id = ?
		ORDER BY resource_type, resource_id`, viewID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make([]VisibilityOverride, 0)
	for rows.Next() {
		var item VisibilityOverride
		if err := rows.Scan(&item.ViewID, &item.ResourceType, &item.ResourceID, &item.LevelDelta, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) SetVisibilityOverride(ctx context.Context, viewID int64, resourceType string, resourceID int64, delta int) (VisibilityOverride, error) {
	if err := ValidateResourceType(resourceType); err != nil {
		return VisibilityOverride{}, err
	}
	delta = clampOverrideDelta(delta)
	if delta == 0 {
		if err := s.DeleteVisibilityOverride(ctx, viewID, resourceType, resourceID); err != nil {
			return VisibilityOverride{}, err
		}
		return VisibilityOverride{ViewID: viewID, ResourceType: resourceType, ResourceID: resourceID, LevelDelta: 0}, nil
	}
	now := nowString()
	_, err := s.DB().ExecContext(ctx, `
		INSERT INTO view_visibility_overrides(view_id, resource_type, resource_id, level_delta, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(view_id, resource_type, resource_id) DO UPDATE SET
		  level_delta = excluded.level_delta,
		  updated_at = excluded.updated_at`,
		viewID, resourceType, resourceID, delta, now, now)
	if err != nil {
		return VisibilityOverride{}, err
	}
	return s.visibilityOverride(ctx, viewID, resourceType, resourceID)
}

func (s *SQLiteStore) AdjustVisibilityOverride(ctx context.Context, viewID int64, resourceType string, resourceID int64, step int) (VisibilityOverride, error) {
	if err := ValidateResourceType(resourceType); err != nil {
		return VisibilityOverride{}, err
	}
	var current int
	err := s.DB().QueryRowContext(ctx, `
		SELECT level_delta FROM view_visibility_overrides
		WHERE view_id = ? AND resource_type = ? AND resource_id = ?`, viewID, resourceType, resourceID).Scan(&current)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return VisibilityOverride{}, err
	}
	return s.SetVisibilityOverride(ctx, viewID, resourceType, resourceID, current+step)
}

func (s *SQLiteStore) DeleteVisibilityOverride(ctx context.Context, viewID int64, resourceType string, resourceID int64) error {
	if err := ValidateResourceType(resourceType); err != nil {
		return err
	}
	_, err := s.DB().ExecContext(ctx, `
		DELETE FROM view_visibility_overrides
		WHERE view_id = ? AND resource_type = ? AND resource_id = ?`, viewID, resourceType, resourceID)
	return err
}

func (s *SQLiteStore) DeleteResourceVisibilityOverrides(ctx context.Context, resourceType string, resourceID int64) error {
	if err := ValidateResourceType(resourceType); err != nil {
		return err
	}
	_, err := s.DB().ExecContext(ctx, `DELETE FROM view_visibility_overrides WHERE resource_type = ? AND resource_id = ?`, resourceType, resourceID)
	return err
}

func (s *SQLiteStore) visibilityOverride(ctx context.Context, viewID int64, resourceType string, resourceID int64) (VisibilityOverride, error) {
	var item VisibilityOverride
	err := s.DB().QueryRowContext(ctx, `
		SELECT view_id, resource_type, resource_id, level_delta, created_at, updated_at
		FROM view_visibility_overrides
		WHERE view_id = ? AND resource_type = ? AND resource_id = ?`, viewID, resourceType, resourceID).Scan(
		&item.ViewID, &item.ResourceType, &item.ResourceID, &item.LevelDelta, &item.CreatedAt, &item.UpdatedAt,
	)
	return item, err
}

func (s *SQLiteStore) ProjectedViewContent(ctx context.Context, viewID int64) (ProjectedViewContent, error) {
	level, err := s.ViewDensityLevel(ctx, viewID)
	if err != nil {
		return ProjectedViewContent{}, err
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
	overrides, err := s.VisibilityOverrides(ctx, viewID)
	if err != nil {
		return ProjectedViewContent{}, err
	}
	signals, err := s.densitySignals(ctx)
	if err != nil {
		return ProjectedViewContent{}, err
	}
	return projectViewContent(placements, connectors, overrides, level, signals), nil
}

func (s *SQLiteStore) densitySignals(ctx context.Context) (densitySignals, error) {
	signals := densitySignals{
		filterScore:            make(map[string]float64),
		filterTier:             make(map[string]int),
		architectureConfidence: make(map[string]float64),
	}

	rows, err := s.DB().QueryContext(ctx, `
		SELECT wm.resource_type, wm.resource_id, MAX(wfd.score), MIN(wfd.tier)
		FROM watch_materialization wm
		JOIN watch_filter_decisions wfd
		  ON wfd.owner_type = wm.owner_type
		 AND (wfd.owner_key = wm.owner_key OR (wfd.owner_id IS NOT NULL AND CAST(wfd.owner_id AS TEXT) = wm.owner_key))
		WHERE wm.resource_type IN ('element', 'connector')
		GROUP BY wm.resource_type, wm.resource_id`)
	if err != nil {
		return densitySignals{}, err
	}
	for rows.Next() {
		var resourceType string
		var resourceID int64
		var score sql.NullFloat64
		var tier sql.NullInt64
		if err := rows.Scan(&resourceType, &resourceID, &score, &tier); err != nil {
			_ = rows.Close()
			return densitySignals{}, err
		}
		key := densitySignalKey(resourceType, resourceID)
		if score.Valid {
			signals.filterScore[key] = score.Float64
		}
		if tier.Valid {
			signals.filterTier[key] = int(tier.Int64)
		}
	}
	if err := rows.Close(); err != nil {
		return densitySignals{}, err
	}
	if err := rows.Err(); err != nil {
		return densitySignals{}, err
	}

	rows, err = s.DB().QueryContext(ctx, `
		SELECT target_resource_type, target_resource_id, MAX(confidence)
		FROM watch_architecture_links
		WHERE target_resource_type IN ('element', 'connector') AND target_resource_id IS NOT NULL
		GROUP BY target_resource_type, target_resource_id`)
	if err != nil {
		return densitySignals{}, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var resourceType string
		var resourceID int64
		var confidence sql.NullFloat64
		if err := rows.Scan(&resourceType, &resourceID, &confidence); err != nil {
			return densitySignals{}, err
		}
		if confidence.Valid {
			signals.architectureConfidence[densitySignalKey(resourceType, resourceID)] = confidence.Float64
		}
	}
	return signals, rows.Err()
}

type densityCaps struct {
	elements   int
	connectors int
	full       bool
}

func capsForDensity(level int) densityCaps {
	switch level {
	case -2:
		return densityCaps{elements: 4, connectors: 8}
	case -1:
		return densityCaps{elements: 8, connectors: 16}
	case 1:
		return densityCaps{elements: 32, connectors: 64}
	case 2:
		return densityCaps{full: true}
	default:
		return densityCaps{elements: 12, connectors: 24}
	}
}

type rankedElement struct {
	item  app.PlacedElement
	score float64
	delta int
}

type rankedConnector struct {
	item  app.Connector
	score float64
	delta int
}

func projectViewContent(placements []app.PlacedElement, connectors []app.Connector, overrides []VisibilityOverride, level int, signals densitySignals) ProjectedViewContent {
	caps := capsForDensity(level)
	elementDeltas := make(map[int64]int)
	connectorDeltas := make(map[int64]int)
	for _, override := range overrides {
		switch override.ResourceType {
		case "element":
			elementDeltas[override.ResourceID] = override.LevelDelta
		case "connector":
			connectorDeltas[override.ResourceID] = override.LevelDelta
		}
	}

	degree := make(map[int64]int)
	for _, connector := range connectors {
		degree[connector.SourceElementID]++
		degree[connector.TargetElementID]++
	}

	rankedElements := make([]rankedElement, 0, len(placements))
	for _, placement := range placements {
		delta := elementDeltas[placement.ElementID]
		rankedElements = append(rankedElements, rankedElement{
			item:  placement,
			score: baseElementScore(placement, degree[placement.ElementID], signals) + float64(delta)*100,
			delta: delta,
		})
	}
	sort.SliceStable(rankedElements, func(i, j int) bool {
		if rankedElements[i].score == rankedElements[j].score {
			return rankedElements[i].item.ID < rankedElements[j].item.ID
		}
		return rankedElements[i].score > rankedElements[j].score
	})

	visibleElements := make(map[int64]struct{})
	elementLimit := caps.elements
	if caps.full {
		elementLimit = len(rankedElements)
	}
	for _, ranked := range rankedElements {
		if ranked.delta <= -4 || (caps.full && ranked.delta < 0) {
			continue
		}
		if !caps.full && len(visibleElements) >= elementLimit && ranked.delta <= 0 {
			continue
		}
		visibleElements[ranked.item.ElementID] = struct{}{}
	}

	rankedConnectors := make([]rankedConnector, 0, len(connectors))
	for _, connector := range connectors {
		delta := connectorDeltas[connector.ID]
		rankedConnectors = append(rankedConnectors, rankedConnector{
			item:  connector,
			score: baseConnectorScore(connector, signals) + float64(delta)*100,
			delta: delta,
		})
	}
	sort.SliceStable(rankedConnectors, func(i, j int) bool {
		if rankedConnectors[i].score == rankedConnectors[j].score {
			return rankedConnectors[i].item.ID < rankedConnectors[j].item.ID
		}
		return rankedConnectors[i].score > rankedConnectors[j].score
	})

	visibleConnectors := make(map[int64]struct{})
	connectorLimit := caps.connectors
	if caps.full {
		connectorLimit = len(rankedConnectors)
	}
	for _, ranked := range rankedConnectors {
		connector := ranked.item
		if ranked.delta <= -4 || (caps.full && ranked.delta < 0) {
			continue
		}
		if ranked.delta > 0 {
			visibleElements[connector.SourceElementID] = struct{}{}
			visibleElements[connector.TargetElementID] = struct{}{}
		}
		_, sourceVisible := visibleElements[connector.SourceElementID]
		_, targetVisible := visibleElements[connector.TargetElementID]
		if !sourceVisible || !targetVisible {
			continue
		}
		if !caps.full && len(visibleConnectors) >= connectorLimit && ranked.delta <= 0 {
			continue
		}
		visibleConnectors[connector.ID] = struct{}{}
	}

	outPlacements := make([]app.PlacedElement, 0, len(visibleElements))
	for _, placement := range placements {
		if _, ok := visibleElements[placement.ElementID]; ok {
			outPlacements = append(outPlacements, placement)
		}
	}
	outConnectors := make([]app.Connector, 0, len(visibleConnectors))
	for _, connector := range connectors {
		if _, ok := visibleConnectors[connector.ID]; ok {
			outConnectors = append(outConnectors, connector)
		}
	}
	return ProjectedViewContent{Placements: outPlacements, Connectors: outConnectors}
}

func baseElementScore(placement app.PlacedElement, degree int, signals densitySignals) float64 {
	score := float64(degree) * 12
	key := densitySignalKey("element", placement.ElementID)
	score += signals.filterScore[key] * 30
	if tier, ok := signals.filterTier[key]; ok {
		score += float64(max(0, 10-tier)) * 5
	}
	score += signals.architectureConfidence[key] * 20
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

func baseConnectorScore(connector app.Connector, signals densitySignals) float64 {
	score := 0.0
	key := densitySignalKey("connector", connector.ID)
	score += signals.filterScore[key] * 30
	if tier, ok := signals.filterTier[key]; ok {
		score += float64(max(0, 10-tier)) * 5
	}
	score += signals.architectureConfidence[key] * 20
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

func densitySignalKey(resourceType string, resourceID int64) string {
	return fmt.Sprintf("%s:%d", resourceType, resourceID)
}

func nowString() string {
	return time.Now().UTC().Format(time.RFC3339)
}

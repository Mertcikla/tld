package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/uptrace/bun"
)

type MergeResolved struct {
	Kind         *string `json:"kind,omitempty"`
	Description  *string `json:"description,omitempty"`
	Repo         *string `json:"repo,omitempty"`
	RepositoryID *string `json:"repository_id,omitempty"`
	Branch       *string `json:"branch,omitempty"`
	FilePath     *string `json:"file_path,omitempty"`
	Language     *string `json:"language,omitempty"`
}

type MergeResult struct {
	Survivor  LibraryElement `json:"survivor"`
	DeletedID int64          `json:"deleted_id"`
}

func (s *Store) MergeElements(ctx context.Context, sourceID, survivorID int64, resolved MergeResolved) (MergeResult, error) {
	if sourceID == survivorID {
		return MergeResult{}, errors.New("cannot merge an element into itself")
	}

	source, err := s.ElementByID(ctx, sourceID)
	if err != nil {
		return MergeResult{}, fmt.Errorf("load source element: %w", err)
	}
	survivor, err := s.ElementByID(ctx, survivorID)
	if err != nil {
		return MergeResult{}, fmt.Errorf("load survivor element: %w", err)
	}

	if err := s.bun.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		// Reassign connectors: source_element_id -> survivor, target_element_id -> survivor.
		if _, err := tx.NewUpdate().
			Model((*connectorModel)(nil)).
			Set("source_element_id = ?", survivorID).
			Where("source_element_id = ?", sourceID).
			Exec(ctx); err != nil {
			return fmt.Errorf("reassign source connectors: %w", err)
		}
		if _, err := tx.NewUpdate().
			Model((*connectorModel)(nil)).
			Set("target_element_id = ?", survivorID).
			Where("target_element_id = ?", sourceID).
			Exec(ctx); err != nil {
			return fmt.Errorf("reassign target connectors: %w", err)
		}

		// Deduplicate connectors that became identical after reassignment.
		if err := deduplicateConnectors(ctx, tx, survivorID); err != nil {
			return fmt.Errorf("deduplicate connectors: %w", err)
		}

		// For placements: update non-conflicting, delete conflicting (survivor position wins).
		var placements []elementPlacementModel
		if err := tx.NewSelect().
			Model(&placements).
			Column("id", "view_id").
			Where("element_id = ?", sourceID).
			Scan(ctx); err != nil {
			return fmt.Errorf("load source placements: %w", err)
		}
		for _, placement := range placements {
			exists, err := tx.NewSelect().
				Model((*elementPlacementModel)(nil)).
				Where("view_id = ?", placement.ViewID).
				Where("element_id = ?", survivorID).
				Exists(ctx)
			if err != nil {
				return fmt.Errorf("check placement conflict: %w", err)
			}
			if exists {
				if _, err := tx.NewDelete().
					Model((*elementPlacementModel)(nil)).
					Where("view_id = ?", placement.ViewID).
					Where("element_id = ?", sourceID).
					Exec(ctx); err != nil {
					return fmt.Errorf("delete conflicting placement: %w", err)
				}
				continue
			}
			if _, err := tx.NewUpdate().
				Model((*elementPlacementModel)(nil)).
				Set("element_id = ?", survivorID).
				Where("id = ?", placement.ID).
				Exec(ctx); err != nil {
				return fmt.Errorf("reassign placement: %w", err)
			}
		}

		// Reassign child view ownership if source owns a view.
		if _, err := tx.NewUpdate().
			Model((*viewModel)(nil)).
			Set("owner_element_id = ?", survivorID).
			Where("owner_element_id = ?", sourceID).
			Exec(ctx); err != nil {
			return fmt.Errorf("reassign child view: %w", err)
		}

		merged := mergeElementFields(survivor, source, resolved)
		if _, err := tx.NewUpdate().
			Model((*elementModel)(nil)).
			Set("name = ?", merged.Name).
			Set("kind = ?", merged.Kind).
			Set("description = ?", merged.Description).
			Set("technology = ?", merged.Technology).
			Set("url = ?", merged.URL).
			Set("logo_url = ?", merged.LogoURL).
			Set("technology_connectors = ?", jsonString(merged.TechnologyConnectors, "[]")).
			Set("tags = ?", jsonString(merged.Tags, "[]")).
			Set("repo = ?", merged.Repo).
			Set("repository_id = ?", merged.RepositoryID).
			Set("branch = ?", merged.Branch).
			Set("file_path = ?", merged.FilePath).
			Set("language = ?", merged.Language).
			Set("updated_at = ?", nowString()).
			Where("id = ?", survivorID).
			Exec(ctx); err != nil {
			return fmt.Errorf("update survivor: %w", err)
		}

		if _, err := tx.NewDelete().
			Model((*visibilityOverrideModel)(nil)).
			Where("resource_type = 'element'").
			Where("resource_id = ?", sourceID).
			Exec(ctx); err != nil {
			return fmt.Errorf("cleanup source visibility overrides: %w", err)
		}

		if _, err := tx.NewDelete().
			Model((*elementModel)(nil)).
			Where("id = ?", sourceID).
			Exec(ctx); err != nil {
			return fmt.Errorf("delete source element: %w", err)
		}
		return nil
	}); err != nil {
		return MergeResult{}, err
	}

	// Re-read survivor to get fresh state.
	result, err := s.ElementByID(ctx, survivorID)
	if err != nil {
		return MergeResult{}, fmt.Errorf("reload survivor: %w", err)
	}

	return MergeResult{Survivor: result, DeletedID: sourceID}, nil
}

func mergeElementFields(survivor, source LibraryElement, resolved MergeResolved) LibraryElement {
	merged := survivor

	if resolved.Kind != nil {
		merged.Kind = resolved.Kind
	}
	if resolved.Description != nil {
		merged.Description = resolved.Description
	}
	if resolved.Repo != nil {
		merged.Repo = resolved.Repo
	}
	if resolved.RepositoryID != nil {
		merged.RepositoryID = resolved.RepositoryID
	}
	if resolved.Branch != nil {
		merged.Branch = resolved.Branch
	}
	if resolved.FilePath != nil {
		merged.FilePath = resolved.FilePath
	}
	if resolved.Language != nil {
		merged.Language = resolved.Language
	}

	// Union tags, survivor's first.
	merged.Tags = unionStrings(survivor.Tags, source.Tags)

	// Union technology_connectors, max 3, survivor's first.
	merged.TechnologyConnectors = unionTechnologyConnectors(survivor.TechnologyConnectors, source.TechnologyConnectors)

	return merged
}

func unionStrings(a, b []string) []string {
	seen := make(map[string]bool, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, s := range a {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	for _, s := range b {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func unionTechnologyConnectors(a, b []TechnologyConnector) []TechnologyConnector {
	seen := make(map[string]bool, len(a)+len(b))
	out := make([]TechnologyConnector, 0, len(a)+len(b))
	for _, tc := range a {
		key := tc.Type + "|" + tc.Slug + "|" + tc.Label
		if !seen[key] {
			seen[key] = true
			out = append(out, tc)
		}
	}
	for _, tc := range b {
		key := tc.Type + "|" + tc.Slug + "|" + tc.Label
		if !seen[key] {
			seen[key] = true
			out = append(out, tc)
		}
	}
	if len(out) > 3 {
		out = out[:3]
	}
	return out
}

type duplicateGroup struct {
	ViewID          int64 `bun:"view_id"`
	SourceElementID int64 `bun:"source_element_id"`
	TargetElementID int64 `bun:"target_element_id"`
	SurvivorID      int64 `bun:"survivor_id"`
}

func deduplicateConnectors(ctx context.Context, tx bun.Tx, survivorID int64) error {
	var groups []duplicateGroup
	if err := tx.NewSelect().
		Model((*connectorModel)(nil)).
		Column("view_id", "source_element_id", "target_element_id").
		ColumnExpr("MIN(id) AS survivor_id").
		Where("source_element_id = ? OR target_element_id = ?", survivorID, survivorID).
		Group("view_id", "source_element_id", "target_element_id").
		Having("COUNT(*) > 1").
		Scan(ctx, &groups); err != nil {
		return fmt.Errorf("query duplicate connectors: %w", err)
	}

	for _, g := range groups {
		if err := mergeConnectorsInGroup(ctx, tx, g); err != nil {
			return err
		}
	}
	return nil
}

func mergeConnectorsInGroup(ctx context.Context, tx bun.Tx, g duplicateGroup) error {
	var rows []connectorModel
	if err := tx.NewSelect().
		Model(&rows).
		Column("id", "label", "description", "direction").
		Where("view_id = ?", g.ViewID).
		Where("source_element_id = ?", g.SourceElementID).
		Where("target_element_id = ?", g.TargetElementID).
		Order("id").
		Scan(ctx); err != nil {
		return fmt.Errorf("query group connectors: %w", err)
	}

	var labels []string
	var descriptions []string
	var directions []string
	var deleteIDs []int64

	for _, row := range rows {
		if row.ID != g.SurvivorID {
			deleteIDs = append(deleteIDs, row.ID)
		}
		if row.Label != nil && strings.TrimSpace(*row.Label) != "" {
			labels = append(labels, strings.TrimSpace(*row.Label))
		}
		if row.Description != nil && strings.TrimSpace(*row.Description) != "" {
			descriptions = append(descriptions, strings.TrimSpace(*row.Description))
		}
		directions = append(directions, row.Direction)
	}

	// Merge labels: unique labels joined with " / ".
	seenLabel := map[string]bool{}
	var mergedLabels []string
	for _, l := range labels {
		if !seenLabel[l] {
			seenLabel[l] = true
			mergedLabels = append(mergedLabels, l)
		}
	}
	var mergedLabel *string
	if len(mergedLabels) > 0 {
		s := strings.Join(mergedLabels, " / ")
		mergedLabel = &s
	}

	// Merge descriptions: pick first non-empty.
	var mergedDesc *string
	for _, d := range descriptions {
		s := d
		mergedDesc = &s
		break
	}

	// Merge directions: forward + backward = "both", same stays, "none" yields.
	var hasForward, hasBackward bool
	for _, d := range directions {
		switch d {
		case "forward":
			hasForward = true
		case "backward":
			hasBackward = true
		case "both":
			hasForward = true
			hasBackward = true
		}
	}
	mergedDir := "none"
	if hasForward && hasBackward {
		mergedDir = "both"
	} else if hasForward {
		mergedDir = "forward"
	} else if hasBackward {
		mergedDir = "backward"
	}

	// Update the survivor connector.
	if _, err := tx.NewUpdate().
		Model((*connectorModel)(nil)).
		Set("label = ?", mergedLabel).
		Set("description = ?", mergedDesc).
		Set("direction = ?", mergedDir).
		Set("updated_at = ?", nowString()).
		Where("id = ?", g.SurvivorID).
		Exec(ctx); err != nil {
		return fmt.Errorf("update survivor connector %d: %w", g.SurvivorID, err)
	}

	// Delete all other connectors in the group.
	if len(deleteIDs) > 0 {
		if _, err := tx.NewDelete().
			Model((*connectorModel)(nil)).
			Where("id IN (?)", bun.List(deleteIDs)).
			Exec(ctx); err != nil {
			return fmt.Errorf("delete duplicate connectors: %w", err)
		}
	}

	return nil
}

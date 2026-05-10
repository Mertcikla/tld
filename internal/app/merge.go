package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

type MergeResolved struct {
	Kind        *string `json:"kind,omitempty"`
	Description *string `json:"description,omitempty"`
	Repo        *string `json:"repo,omitempty"`
	Branch      *string `json:"branch,omitempty"`
	FilePath    *string `json:"file_path,omitempty"`
	Language    *string `json:"language,omitempty"`
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

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return MergeResult{}, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Reassign connectors: source_element_id → survivor, target_element_id → survivor.
	if _, err := tx.ExecContext(ctx,
		`UPDATE connectors SET source_element_id = ? WHERE source_element_id = ?`,
		survivorID, sourceID,
	); err != nil {
		return MergeResult{}, fmt.Errorf("reassign source connectors: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE connectors SET target_element_id = ? WHERE target_element_id = ?`,
		survivorID, sourceID,
	); err != nil {
		return MergeResult{}, fmt.Errorf("reassign target connectors: %w", err)
	}

	// Deduplicate connectors that became identical after reassignment.
	if err := deduplicateConnectors(ctx, tx, sourceID, survivorID); err != nil {
		return MergeResult{}, fmt.Errorf("deduplicate connectors: %w", err)
	}

	// For placements: update non-conflicting, delete conflicting (survivor position wins).
	placementRows, err := tx.QueryContext(ctx,
		`SELECT id, view_id FROM placements WHERE element_id = ?`, sourceID)
	if err != nil {
		return MergeResult{}, fmt.Errorf("load source placements: %w", err)
	}
	defer func() { _ = placementRows.Close() }()
	for placementRows.Next() {
		var placementID, viewID int64
		if err := placementRows.Scan(&placementID, &viewID); err != nil {
			return MergeResult{}, fmt.Errorf("scan placement: %w", err)
		}
		var exists bool
		if err := tx.QueryRowContext(ctx,
			`SELECT EXISTS(SELECT 1 FROM placements WHERE view_id = ? AND element_id = ?)`,
			viewID, survivorID,
		).Scan(&exists); err != nil {
			return MergeResult{}, fmt.Errorf("check placement conflict: %w", err)
		}
		if exists {
			if _, err := tx.ExecContext(ctx,
				`DELETE FROM placements WHERE view_id = ? AND element_id = ?`,
				viewID, sourceID,
			); err != nil {
				return MergeResult{}, fmt.Errorf("delete conflicting placement: %w", err)
			}
		} else {
			if _, err := tx.ExecContext(ctx,
				`UPDATE placements SET element_id = ? WHERE id = ?`,
				survivorID, placementID,
			); err != nil {
				return MergeResult{}, fmt.Errorf("reassign placement: %w", err)
			}
		}
	}
	if err := placementRows.Err(); err != nil {
		return MergeResult{}, fmt.Errorf("iterate placements: %w", err)
	}

	// Reassign child view ownership if source owns a view.
	if _, err := tx.ExecContext(ctx,
		`UPDATE views SET owner_element_id = ? WHERE owner_element_id = ?`,
		survivorID, sourceID,
	); err != nil {
		return MergeResult{}, fmt.Errorf("reassign child view: %w", err)
	}

	// Merge element properties.
	merged := mergeElementFields(survivor, source, resolved)

	if _, err := tx.ExecContext(ctx, `
		UPDATE elements SET
			name = ?, kind = ?, description = ?, technology = ?, url = ?, logo_url = ?,
			technology_connectors = ?, tags = ?, repo = ?, branch = ?, file_path = ?, language = ?,
			updated_at = ?
		WHERE id = ?`,
		merged.Name, merged.Kind, merged.Description, merged.Technology, merged.URL, merged.LogoURL,
		jsonString(merged.TechnologyConnectors, "[]"), jsonString(merged.Tags, "[]"),
		merged.Repo, merged.Branch, merged.FilePath, merged.Language,
		nowString(), survivorID,
	); err != nil {
		return MergeResult{}, fmt.Errorf("update survivor: %w", err)
	}

	// Clean up visibility overrides for the source element.
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM view_visibility_overrides WHERE resource_type = 'element' AND resource_id = ?`,
		sourceID,
	); err != nil {
		return MergeResult{}, fmt.Errorf("cleanup source visibility overrides: %w", err)
	}

	// Delete the source element.
	if _, err := tx.ExecContext(ctx, `DELETE FROM elements WHERE id = ?`, sourceID); err != nil {
		return MergeResult{}, fmt.Errorf("delete source element: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return MergeResult{}, fmt.Errorf("commit merge: %w", err)
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
	ViewID          int64
	SourceElementID int64
	TargetElementID int64
	SurvivorID      int64
}

func deduplicateConnectors(ctx context.Context, tx interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}, sourceID, survivorID int64) error {
	dupRows, err := tx.QueryContext(ctx, `
		SELECT view_id, source_element_id, target_element_id, MIN(id)
		FROM connectors
		WHERE source_element_id = ? OR target_element_id = ?
		GROUP BY view_id, source_element_id, target_element_id
		HAVING COUNT(*) > 1`, survivorID, survivorID)
	if err != nil {
		return fmt.Errorf("query duplicate connectors: %w", err)
	}
	defer func() { _ = dupRows.Close() }()

	var groups []duplicateGroup
	for dupRows.Next() {
		var g duplicateGroup
		if err := dupRows.Scan(&g.ViewID, &g.SourceElementID, &g.TargetElementID, &g.SurvivorID); err != nil {
			return fmt.Errorf("scan duplicate group: %w", err)
		}
		groups = append(groups, g)
	}
	if err := dupRows.Err(); err != nil {
		return fmt.Errorf("iterate duplicate groups: %w", err)
	}

	for _, g := range groups {
		if err := mergeConnectorsInGroup(ctx, tx, g); err != nil {
			return err
		}
	}
	return nil
}

func mergeConnectorsInGroup(ctx context.Context, tx interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}, g duplicateGroup) error {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, label, description, direction
		FROM connectors
		WHERE view_id = ? AND source_element_id = ? AND target_element_id = ?
		ORDER BY id`, g.ViewID, g.SourceElementID, g.TargetElementID)
	if err != nil {
		return fmt.Errorf("query group connectors: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var labels []string
	var descriptions []string
	var directions []string
	var deleteIDs []int64

	for rows.Next() {
		var id int64
		var label, description sql.NullString
		var direction string
		if err := rows.Scan(&id, &label, &description, &direction); err != nil {
			return fmt.Errorf("scan connector: %w", err)
		}
		if id != g.SurvivorID {
			deleteIDs = append(deleteIDs, id)
		}
		if label.Valid && strings.TrimSpace(label.String) != "" {
			labels = append(labels, strings.TrimSpace(label.String))
		}
		if description.Valid && strings.TrimSpace(description.String) != "" {
			descriptions = append(descriptions, strings.TrimSpace(description.String))
		}
		directions = append(directions, direction)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate group connectors: %w", err)
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
	if _, err := tx.ExecContext(ctx, `
		UPDATE connectors SET label = ?, description = ?, direction = ?, updated_at = ?
		WHERE id = ?`,
		mergedLabel, mergedDesc, mergedDir, nowString(), g.SurvivorID,
	); err != nil {
		return fmt.Errorf("update survivor connector %d: %w", g.SurvivorID, err)
	}

	// Delete all other connectors in the group.
	for _, deleteID := range deleteIDs {
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM connectors WHERE id = ?`, deleteID,
		); err != nil {
			return fmt.Errorf("delete duplicate connector %d: %w", deleteID, err)
		}
	}

	return nil
}

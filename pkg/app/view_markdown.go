package app

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

type ViewMarkdownDocument struct {
	Path       string `json:"path"`
	IsManaged  bool   `json:"is_managed"`
	UpdatedAt  string `json:"updated_at"`
	SourceKind string `json:"source_kind"`
}

func (s *Store) viewMarkdownMap(ctx context.Context) (map[int64]*ViewMarkdownDocument, error) {
	if err := s.ensureViewMarkdownTable(ctx); err != nil {
		return nil, err
	}
	var rows []viewMarkdownModel
	if err := s.bun.NewSelect().Model(&rows).Order("view_id").Scan(ctx); err != nil {
		if stringsContainsNoSuchTable(err) {
			return map[int64]*ViewMarkdownDocument{}, nil
		}
		return nil, err
	}
	out := make(map[int64]*ViewMarkdownDocument, len(rows))
	for _, row := range rows {
		out[row.ViewID] = viewMarkdownDocumentFromModel(row)
	}
	return out, nil
}

func (s *Store) ViewMarkdownByViewID(ctx context.Context, viewID int64) (*ViewMarkdownDocument, error) {
	if err := s.ensureViewMarkdownTable(ctx); err != nil {
		return nil, err
	}
	var row viewMarkdownModel
	if err := s.bun.NewSelect().
		Model(&row).
		Where("view_id = ?", viewID).
		Scan(ctx); err != nil {
		if errors.Is(err, sql.ErrNoRows) || stringsContainsNoSuchTable(err) {
			return nil, nil
		}
		return nil, err
	}
	return viewMarkdownDocumentFromModel(row), nil
}

func (s *Store) UpsertViewMarkdown(ctx context.Context, viewID int64, path string, isManaged bool, updatedAt string, sourceKind ...string) error {
	if updatedAt == "" {
		updatedAt = nowString()
	}
	if err := s.ensureViewMarkdownTable(ctx); err != nil {
		return err
	}
	kind := ""
	if len(sourceKind) > 0 {
		kind = strings.TrimSpace(sourceKind[0])
	}
	if kind == "" {
		if isManaged {
			kind = "PRIVATE_APP"
		} else {
			kind = "ATTACHED"
		}
	}
	row := &viewMarkdownModel{
		ViewID:     viewID,
		Path:       path,
		IsManaged:  isManaged,
		SourceKind: kind,
		CreatedAt:  updatedAt,
		UpdatedAt:  updatedAt,
	}
	_, err := s.bun.NewInsert().
		Model(row).
		On("CONFLICT(view_id) DO UPDATE").
		Set("path = EXCLUDED.path").
		Set("is_managed = EXCLUDED.is_managed").
		Set("source_kind = EXCLUDED.source_kind").
		Set("updated_at = EXCLUDED.updated_at").
		Exec(ctx)
	return err
}

func (s *Store) CountViewMarkdownByPath(ctx context.Context, path string) (int, error) {
	if err := s.ensureViewMarkdownTable(ctx); err != nil {
		return 0, err
	}
	count, err := s.bun.NewSelect().
		Model((*viewMarkdownModel)(nil)).
		Where("path = ?", path).
		Count(ctx)
	return count, err
}

func (s *Store) DeleteViewMarkdown(ctx context.Context, viewID int64) error {
	if err := s.ensureViewMarkdownTable(ctx); err != nil {
		if stringsContainsNoSuchTable(err) {
			return nil
		}
		return err
	}
	_, err := s.bun.NewDelete().
		Model((*viewMarkdownModel)(nil)).
		Where("view_id = ?", viewID).
		Exec(ctx)
	return err
}

func (s *Store) ensureViewMarkdownTable(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, `
			CREATE TABLE IF NOT EXISTS view_markdown_documents (
				view_id INTEGER PRIMARY KEY,
				org_id TEXT NULL,
				path TEXT NOT NULL,
				is_managed INTEGER NOT NULL DEFAULT 0,
				source_kind TEXT NOT NULL DEFAULT '',
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL,
				FOREIGN KEY (view_id) REFERENCES views(id) ON DELETE CASCADE
			)
		`); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `ALTER TABLE view_markdown_documents ADD COLUMN org_id TEXT NULL`); err != nil && !isDuplicateColumnError(err) {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `ALTER TABLE view_markdown_documents ADD COLUMN source_kind TEXT NOT NULL DEFAULT ''`); err != nil && !isDuplicateColumnError(err) {
		return err
	}
	return nil
}

func viewMarkdownDocumentFromModel(row viewMarkdownModel) *ViewMarkdownDocument {
	sourceKind := strings.TrimSpace(row.SourceKind)
	if sourceKind == "" {
		if row.IsManaged {
			sourceKind = "PRIVATE_APP"
		} else {
			sourceKind = "ATTACHED"
		}
	}
	return &ViewMarkdownDocument{
		Path:       row.Path,
		IsManaged:  row.IsManaged,
		UpdatedAt:  row.UpdatedAt,
		SourceKind: sourceKind,
	}
}

func stringsContainsNoSuchTable(err error) bool {
	return err != nil && !errors.Is(err, sql.ErrNoRows) && containsNoSuchTable(err.Error())
}

func containsNoSuchTable(message string) bool {
	return strings.Contains(message, "no such table: view_markdown_documents") ||
		strings.Contains(message, `relation "view_markdown_documents" does not exist`)
}

func isDuplicateColumnError(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.Contains(message, "duplicate column name:") ||
		strings.Contains(message, `of relation "view_markdown_documents" already exists`)
}

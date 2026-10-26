package app

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

// ErrFlowConflict means another writer changed or deleted the story.
var ErrFlowConflict = errors.New("flow changed; reload before saving")

// FlowRecord stores the versioned protobuf JSON document without coupling the
// persistence layer to a particular generated API revision.
type FlowRecord struct {
	bun.BaseModel `bun:"table:flows,alias:flow"`
	ID            string     `bun:"id,pk"`
	ViewID        int64      `bun:"view_id"`
	OrgID         *uuid.UUID `bun:"org_id"`
	Document      string     `bun:"document"`
	Revision      int32      `bun:"revision"`
}

func (s *Store) ListFlows(ctx context.Context, viewID int64) ([]FlowRecord, error) {
	rows := []FlowRecord{}
	q := s.bun.NewSelect().Model(&rows).OrderExpr("view_id, id")
	if viewID != 0 {
		q.Where("view_id = ?", viewID)
	}
	if orgID := TenantOrgIDFromCtx(ctx); orgID != uuid.Nil {
		q.Where("org_id = ?", orgID)
	}
	err := q.Scan(ctx)
	return rows, err
}

// SaveFlow uses a compare-and-swap update so stale clients cannot silently
// replace a collaborator's steps. A flow's home view cannot be reassigned.
func (s *Store) SaveFlow(ctx context.Context, row FlowRecord) (FlowRecord, error) {
	if _, err := s.ViewByID(ctx, row.ViewID); err != nil {
		return FlowRecord{}, err
	}
	if row.ID == "" {
		row.ID = uuid.NewString()
		row.Revision = 1
		if orgID := TenantOrgIDFromCtx(ctx); orgID != uuid.Nil {
			row.OrgID = &orgID
		}
		_, err := s.bun.NewInsert().Model(&row).Exec(ctx)
		return row, err
	}
	q := s.bun.NewUpdate().Model((*FlowRecord)(nil)).
		Set("document = ?", row.Document).Set("revision = revision + 1").
		Where("id = ? AND view_id = ? AND revision = ?", row.ID, row.ViewID, row.Revision)
	if orgID := TenantOrgIDFromCtx(ctx); orgID != uuid.Nil {
		q.Where("org_id = ?", orgID)
	}
	result, err := q.Exec(ctx)
	if err != nil {
		return FlowRecord{}, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return FlowRecord{}, err
	}
	if count != 1 {
		return FlowRecord{}, ErrFlowConflict
	}
	row.Revision++
	return row, nil
}

func (s *Store) DeleteFlow(ctx context.Context, id string, viewID int64, revision int32) error {
	if _, err := s.ViewByID(ctx, viewID); err != nil {
		return err
	}
	q := s.bun.NewDelete().Model((*FlowRecord)(nil)).Where("id = ? AND view_id = ? AND revision = ?", id, viewID, revision)
	if orgID := TenantOrgIDFromCtx(ctx); orgID != uuid.Nil {
		q.Where("org_id = ?", orgID)
	}
	result, err := q.Exec(ctx)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrFlowConflict
	}
	return nil
}

package app

import (
	"context"
	"database/sql"
	"errors"
)

// PlanHandoff is an immutable workspace snapshot captured at implementation start.
// Callers must resolve the plan in their workspace before accessing its handoff.
func (s *Store) PlanHandoff(ctx context.Context, id string, revision uint32) (string, error) {
	var payload string
	err := s.bun.NewRaw("SELECT payload FROM plan_handoffs WHERE plan_id=? AND revision=?", id, revision).Scan(ctx, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return payload, err
}

func (s *Store) SavePlanHandoff(ctx context.Context, id string, revision uint32, payload string) error {
	_, err := s.bun.ExecContext(ctx, "INSERT INTO plan_handoffs (plan_id,revision,payload) VALUES (?,?,?)", id, revision, payload)
	return err
}

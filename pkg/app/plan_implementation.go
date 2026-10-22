package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
)

// Implementation is separate from the reviewed plan and its human-authored history.
func (s *Store) Implementation(ctx context.Context, scope string) (string, error) {
	var payload string
	err := s.bun.NewRaw("SELECT payload FROM plan_implementations WHERE workspace_id=?", scope).Scan(ctx, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return payload, err
}
func (s *Store) SaveImplementation(ctx context.Context, scope, plan, payload string, revision, expected uint32) error {
	if expected == 0 {
		_, err := s.bun.ExecContext(ctx, "INSERT INTO plan_implementations (workspace_id,plan_id,revision,payload) VALUES (?,?,?,?)", scope, plan, revision, payload)
		return err
	}
	result, err := s.bun.ExecContext(ctx, "UPDATE plan_implementations SET plan_id=?,revision=?,payload=? WHERE workspace_id=? AND revision=?", plan, revision, payload, scope, expected)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n != 1 {
		return ErrPlanConflict
	}
	return err
}

// SimulateWorkspace runs the normal database handlers in a savepoint that is
// always rolled back. Filesystem-writing handlers must never enter this path.
func (s *Store) SimulateWorkspace(ctx context.Context, fn func() error) error {
	if _, err := s.bun.ExecContext(ctx, "SAVEPOINT agent_workspace"); err != nil {
		return err
	}
	err := fn()
	_, rollback := s.bun.ExecContext(ctx, "ROLLBACK TO SAVEPOINT agent_workspace")
	_, release := s.bun.ExecContext(ctx, "RELEASE SAVEPOINT agent_workspace")
	return errors.Join(err, rollback, release)
}

func (s *Store) ApplyImplementationStep(ctx context.Context, fn func() error) error {
	if _, err := s.bun.ExecContext(ctx, "SAVEPOINT implementation_step"); err != nil {
		return err
	}
	err := fn()
	if err != nil {
		_, rollback := s.bun.ExecContext(ctx, "ROLLBACK TO SAVEPOINT implementation_step")
		err = errors.Join(err, rollback)
	}
	_, release := s.bun.ExecContext(ctx, "RELEASE SAVEPOINT implementation_step")
	return errors.Join(err, release)
}

func (s *Store) ImplementationLocked(ctx context.Context, scope string) (bool, error) {
	raw, err := s.Implementation(ctx, scope)
	if err != nil || raw == "" {
		return false, err
	}
	var state struct{ Locked bool }
	err = json.Unmarshal([]byte(raw), &state)
	return state.Locked, err
}

// Used by materializers which cannot produce individual agent proof-of-work edits.
func (s *Store) CheckImplementationUnlocked(ctx context.Context) error {
	var rows []string
	if err := s.bun.NewRaw("SELECT payload FROM plan_implementations").Scan(ctx, &rows); err != nil {
		return err
	}
	for _, raw := range rows {
		var v struct{ Locked bool }
		if err := json.Unmarshal([]byte(raw), &v); err != nil {
			return err
		}
		if v.Locked {
			return errors.New("workspace is locked for implementation; automatic materialization is disabled")
		}
	}
	return nil
}

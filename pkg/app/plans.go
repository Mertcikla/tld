package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/mertcikla/tld/v2/pkg/dbrepo"
	"github.com/mertcikla/tld/v2/pkg/plans"
	"github.com/uptrace/bun"
	"google.golang.org/protobuf/encoding/protojson"
)

var ErrPlanConflict = errors.New("plan changed; reload before saving")

func (s *Store) RunPlanTransaction(ctx context.Context, fn func(*Store) error) error {
	opts := &sql.TxOptions{}
	if s.dialect == dbrepo.DialectPostgres {
		opts.Isolation = sql.LevelSerializable
	}
	return s.bun.RunInTx(ctx, opts, func(_ context.Context, tx bun.Tx) error { c := *s; c.bun = tx; return fn(&c) })
}
func (s *Store) CreatePlan(ctx context.Context, scope string, p *pb.Plan) error {
	data, err := protojson.Marshal(p)
	if err != nil {
		return err
	}
	history, _ := json.Marshal(map[string]string{strconv.FormatUint(uint64(p.Revision), 10): string(data)})
	_, err = s.bun.ExecContext(ctx, "INSERT INTO plans (id,workspace_id,repository_id,status,revision,payload,history,updated_at) VALUES (?,?,?,?,?,?,?,?)", p.Id, scope, p.RepositoryId, p.Status, p.Revision, string(data), string(history), p.UpdatedAt)
	return err
}
func (s *Store) Plan(ctx context.Context, scope, id string) (*pb.Plan, error) {
	var data string
	err := s.bun.NewRaw("SELECT payload FROM plans WHERE workspace_id = ? AND id = ?", scope, id).Scan(ctx, &data)
	if errors.Is(err, sql.ErrNoRows) {
		candidates, e := s.Plans(ctx, scope, "", true)
		if e != nil {
			return nil, e
		}
		var match *pb.Plan
		for _, candidate := range candidates {
			if candidate.Name == id {
				if match != nil {
					return nil, fmt.Errorf("plan name %q is ambiguous; give plans unique names or use an ID from tld plan list", id)
				}
				match = candidate
			}
		}
		if match != nil {
			return match, nil
		}
	}
	if err != nil {
		return nil, err
	}
	p := &pb.Plan{}
	err = protojson.Unmarshal([]byte(data), p)
	return p, err
}
func (s *Store) Plans(ctx context.Context, scope, repo string, archived bool) ([]*pb.Plan, error) {
	q := "SELECT payload FROM plans WHERE workspace_id = ?"
	args := []any{scope}
	if repo != "" {
		q += " AND repository_id = ?"
		args = append(args, repo)
	}
	if !archived {
		q += " AND status <> 'archived'"
	}
	q += " ORDER BY updated_at DESC, id"
	var rows []string
	if err := s.bun.NewRaw(q, args...).Scan(ctx, &rows); err != nil {
		return nil, err
	}
	out := []*pb.Plan{}
	for _, data := range rows {
		p := &pb.Plan{}
		if err := protojson.Unmarshal([]byte(data), p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}
func (s *Store) SavePlan(ctx context.Context, scope string, p *pb.Plan, expected uint32) error {
	data, err := protojson.Marshal(p)
	if err != nil {
		return err
	}
	var raw string
	if err = s.bun.NewRaw("SELECT history FROM plans WHERE workspace_id=? AND id=?", scope, p.Id).Scan(ctx, &raw); err != nil {
		return err
	}
	history := map[string]string{}
	if err = json.Unmarshal([]byte(raw), &history); err != nil {
		return err
	}
	history[strconv.FormatUint(uint64(p.Revision), 10)] = string(data)
	encoded, err := json.Marshal(history)
	if err != nil {
		return err
	}
	result, err := s.bun.ExecContext(ctx, "UPDATE plans SET status=?, revision=?, payload=?, history=?, updated_at=? WHERE workspace_id=? AND id=? AND revision=?", p.Status, p.Revision, string(data), string(encoded), p.UpdatedAt, scope, p.Id, expected)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n != 1 {
		return ErrPlanConflict
	}
	return err
}
func (s *Store) PlanOperations(ctx context.Context, id string, revision uint32) ([]*pb.PlanOperation, error) {
	var rows []string
	if err := s.bun.NewRaw("SELECT payload FROM plan_operations WHERE plan_id=? AND revision<=? ORDER BY revision, operation_index", id, revision).Scan(ctx, &rows); err != nil {
		return nil, err
	}
	out := []*pb.PlanOperation{}
	for _, data := range rows {
		op := &pb.PlanOperation{}
		if err := protojson.Unmarshal([]byte(data), op); err != nil {
			return nil, err
		}
		out = append(out, op)
	}
	return out, nil
}
func (s *Store) PlanRequestExists(ctx context.Context, id, request, hash string) (bool, error) {
	var hashes []string
	err := s.bun.NewRaw("SELECT request_hash FROM plan_operations WHERE plan_id=? AND request_id=?", id, request).Scan(ctx, &hashes)
	if err != nil {
		return false, err
	}
	for _, existing := range hashes {
		if existing != hash {
			return false, errors.New("idempotency key was already used for different operations")
		}
	}
	return len(hashes) > 0, nil
}
func (s *Store) SavePlanOperation(ctx context.Context, id, request, hash string, op *pb.PlanOperation) error {
	data, err := protojson.Marshal(op)
	if err != nil {
		return err
	}
	_, err = s.bun.ExecContext(ctx, "INSERT INTO plan_operations (plan_id,revision,operation_index,operation_id,request_id,request_hash,payload) VALUES (?,?,?,?,?,?,?)", id, op.Revision, op.Index, op.Id, request, hash, string(data))
	if err != nil {
		return err
	}
	for _, p := range op.Preconditions {
		b, e := protojson.Marshal(p)
		if e != nil {
			return e
		}
		if _, e = s.bun.ExecContext(ctx, "INSERT INTO plan_operation_preconditions (plan_id,operation_id,resource_key,expected_hash,payload) VALUES (?,?,?,?,?)", id, op.Id, p.ResourceKey, p.Hash, string(b)); e != nil {
			return e
		}
	}
	return nil
}
func (s *Store) SavePlanState(ctx context.Context, id string, state *plans.State) error {
	for _, table := range []string{"plan_members", "plan_checklist_targets", "plan_checklist_items", "plan_notes"} {
		if _, err := s.bun.ExecContext(ctx, "DELETE FROM "+table+" WHERE plan_id=?", id); err != nil {
			return err
		}
	}
	for k, v := range state.Members {
		data, err := protojson.Marshal(v)
		if err != nil {
			return err
		}
		if _, err = s.bun.ExecContext(ctx, "INSERT INTO plan_members (plan_id,resource_key,payload) VALUES (?,?,?)", id, k, string(data)); err != nil {
			return err
		}
	}
	for k, v := range state.Checklist {
		data, err := protojson.Marshal(v)
		if err != nil {
			return err
		}
		if _, err = s.bun.ExecContext(ctx, "INSERT INTO plan_checklist_items (plan_id,item_id,payload) VALUES (?,?,?)", id, k, string(data)); err != nil {
			return err
		}
		for _, target := range v.ResourceKeys {
			if _, err = s.bun.ExecContext(ctx, "INSERT INTO plan_checklist_targets (plan_id,item_id,resource_key) VALUES (?,?,?)", id, k, target); err != nil {
				return err
			}
		}
	}
	for k, v := range state.Notes {
		data, err := protojson.Marshal(v)
		if err != nil {
			return err
		}
		if _, err = s.bun.ExecContext(ctx, "INSERT INTO plan_notes (plan_id,view_key,payload) VALUES (?,?,?)", id, k, string(data)); err != nil {
			return err
		}
	}
	return nil
}

// PlanRevision retains only plan metadata, never a workspace snapshot.
func (s *Store) PlanRevision(ctx context.Context, scope, id string, revision uint32) (*pb.Plan, error) {
	current, resolveErr := s.Plan(ctx, scope, id)
	if resolveErr != nil {
		return nil, resolveErr
	}
	id = current.Id
	var raw string
	if err := s.bun.NewRaw("SELECT history FROM plans WHERE workspace_id=? AND id=?", scope, id).Scan(ctx, &raw); err != nil {
		return nil, err
	}
	history := map[string]string{}
	if err := json.Unmarshal([]byte(raw), &history); err != nil {
		return nil, err
	}
	data, ok := history[strconv.FormatUint(uint64(revision), 10)]
	if !ok {
		return nil, fmt.Errorf("plan revision not found")
	}
	p := &pb.Plan{}
	err := protojson.Unmarshal([]byte(data), p)
	return p, err
}

func (s *Store) PlanWorkspaceRequest(ctx context.Context, id, request, hash string) (string, error) {
	var rows []struct {
		RequestHash string
		Response    string
	}
	err := s.bun.NewRaw("SELECT request_hash, response FROM plan_workspace_requests WHERE plan_id=? AND request_id=?", id, request).Scan(ctx, &rows)
	if err != nil || len(rows) == 0 {
		return "", err
	}
	if rows[0].RequestHash != hash {
		return "", errors.New("idempotency key was already used for a different workspace request")
	}
	return rows[0].Response, nil
}
func (s *Store) SavePlanWorkspaceRequest(ctx context.Context, id, request, hash, response string) error {
	_, err := s.bun.ExecContext(ctx, "INSERT INTO plan_workspace_requests (plan_id,request_id,request_hash,response) VALUES (?,?,?,?)", id, request, hash, response)
	return err
}

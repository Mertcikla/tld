package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/google/uuid"
	"google.golang.org/protobuf/encoding/protojson"
)

func (s *Store) SaveImpact(ctx context.Context, diagram *pb.ImpactDiagram) error {
	raw, err := protojson.Marshal(diagram)
	if err != nil {
		return err
	}
	_, err = s.bun.NewRaw(`INSERT INTO codeindex_impacts (repository_id, comparison_key, result_json, org_id) VALUES (?, ?, ?, ?)
 ON CONFLICT(org_id, repository_id, comparison_key) DO UPDATE SET result_json = excluded.result_json`, diagram.RepositoryId, diagram.ComparisonKey, string(raw), scope(ctx).value()).Exec(ctx)
	return err
}

func (s *Store) Impact(ctx context.Context, repositoryID, key string) (*pb.ImpactDiagram, error) {
	where, scopeArgs := scope(ctx).clause("org_id")
	var raw string
	if err := s.bun.NewRaw(`SELECT result_json FROM codeindex_impacts WHERE repository_id = ? AND comparison_key = ?`+where, append([]any{repositoryID, key}, scopeArgs...)...).Scan(ctx, &raw); err != nil {
		return nil, err
	}
	diagram := &pb.ImpactDiagram{}
	if err := protojson.Unmarshal([]byte(raw), diagram); err != nil {
		return nil, err
	}
	// Older saved diagrams predate line counts; derive them from their immutable
	// captured sources without modifying a potentially newer watcher result.
	if err := s.sourceLineStats(ctx, diagram.GetDiff().GetSources()); err != nil {
		return nil, err
	}
	return diagram, nil
}

func (s *Store) LiveImpact(ctx context.Context, repositoryID string) (*pb.LiveImpact, error) {
	result := &pb.LiveImpact{}
	diagram, err := s.Impact(ctx, repositoryID, "live")
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	result.Diagram = diagram
	state, ok, err := s.WatchState(ctx, repositoryID)
	if err != nil {
		return nil, err
	}
	if ok {
		result.Error = state.Error
		result.GitBranch = state.GitBranch
		result.GitRevision = state.GitRevision
		result.Watching = state.Live(time.Now())
	}
	return result, nil
}

var ErrBusy = errors.New("repository indexing is already running")

// AcquireLease serializes indexing/materialization across server and CLI
// processes. Renewals cancel the operation if ownership is lost.
func (s *Store) AcquireLease(ctx context.Context, repositoryID string) (context.Context, func(), error) {
	where, scopeArgs := scope(ctx).clause("org_id")
	owner := uuid.NewString()
	now := time.Now().Unix()
	res, err := s.bun.NewRaw(`INSERT INTO codeindex_leases (repository_id, owner, expires_unix, org_id) VALUES (?, ?, ?, ?)
 ON CONFLICT(org_id, repository_id) DO UPDATE SET owner = excluded.owner, expires_unix = excluded.expires_unix WHERE codeindex_leases.expires_unix <= ?`, repositoryID, owner, now+30, scope(ctx).value(), now).Exec(ctx)
	if err != nil {
		return nil, nil, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return nil, nil, err
	}
	if n == 0 {
		return nil, nil, ErrBusy
	}
	leased, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-leased.Done():
				return
			case <-ticker.C:
				res, e := s.bun.NewRaw(`UPDATE codeindex_leases SET expires_unix = ? WHERE repository_id = ? AND owner = ? AND expires_unix > ?`+where, append([]any{time.Now().Unix() + 30, repositoryID, owner, time.Now().Unix()}, scopeArgs...)...).Exec(leased)
				if e != nil {
					cancel()
					return
				}
				n, e := res.RowsAffected()
				if e != nil || n != 1 {
					cancel()
					return
				}
			}
		}
	}()
	release := func() {
		cancel()
		<-done
		cleanup, end := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer end()
		_, _ = s.bun.NewRaw(`DELETE FROM codeindex_leases WHERE repository_id = ? AND owner = ?`+where, append([]any{repositoryID, owner}, scopeArgs...)...).Exec(cleanup)
	}
	return leased, release, nil
}

func (s *Store) AdvanceLatest(ctx context.Context, repositoryID, snapshotID string) error {
	repoWhere, repoScopeArgs := scope(ctx).clause("org_id")
	snapWhere, snapScopeArgs := scope(ctx).clause("org_id")
	res, err := s.bun.NewRaw(`UPDATE codeindex_repositories SET latest_snapshot_id = ? WHERE id = ? AND EXISTS (SELECT 1 FROM codeindex_snapshots WHERE id = ? AND repository_id = ?`+snapWhere+`)`+repoWhere,
		append(append([]any{snapshotID, repositoryID, snapshotID, repositoryID}, snapScopeArgs...), repoScopeArgs...)...).Exec(ctx)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("snapshot does not belong to repository")
	}
	return nil
}

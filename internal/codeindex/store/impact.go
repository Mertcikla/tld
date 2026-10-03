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
	_, err = s.bun.NewRaw(`INSERT INTO codeindex_impacts (repository_id, comparison_key, result_json) VALUES (?, ?, ?)
 ON CONFLICT(repository_id, comparison_key) DO UPDATE SET result_json = excluded.result_json`, diagram.RepositoryId, diagram.ComparisonKey, string(raw)).Exec(ctx)
	return err
}

func (s *Store) Impact(ctx context.Context, repositoryID, key string) (*pb.ImpactDiagram, error) {
	var raw string
	if err := s.bun.NewRaw(`SELECT result_json FROM codeindex_impacts WHERE repository_id = ? AND comparison_key = ?`, repositoryID, key).Scan(ctx, &raw); err != nil {
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

func (s *Store) WatchHeartbeat(ctx context.Context, repositoryID, branch, revision, message string, running bool) error {
	stamp := int64(0)
	if running {
		stamp = time.Now().Unix()
	}
	_, err := s.bun.NewRaw(`INSERT INTO codeindex_watch_state (repository_id, heartbeat_unix, error, git_branch, git_revision) VALUES (?, ?, ?, ?, ?)
 ON CONFLICT(repository_id) DO UPDATE SET heartbeat_unix = excluded.heartbeat_unix, error = excluded.error, git_branch = excluded.git_branch, git_revision = excluded.git_revision`, repositoryID, stamp, message, branch, revision).Exec(ctx)
	return err
}

func (s *Store) LiveImpact(ctx context.Context, repositoryID string) (*pb.LiveImpact, error) {
	result := &pb.LiveImpact{}
	diagram, err := s.Impact(ctx, repositoryID, "live")
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	result.Diagram = diagram
	var heartbeat int64
	err = s.bun.NewRaw(`SELECT heartbeat_unix, error, git_branch, git_revision FROM codeindex_watch_state WHERE repository_id = ?`, repositoryID).Scan(ctx, &heartbeat, &result.Error, &result.GitBranch, &result.GitRevision)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	result.Watching = heartbeat > time.Now().Add(-30*time.Second).Unix()
	return result, nil
}

var ErrBusy = errors.New("repository indexing is already running")

// AcquireLease serializes indexing/materialization across server and CLI
// processes. Renewals cancel the operation if ownership is lost.
func (s *Store) AcquireLease(ctx context.Context, repositoryID string) (context.Context, func(), error) {
	owner := uuid.NewString()
	now := time.Now().Unix()
	res, err := s.bun.NewRaw(`INSERT INTO codeindex_leases (repository_id, owner, expires_unix) VALUES (?, ?, ?)
 ON CONFLICT(repository_id) DO UPDATE SET owner = excluded.owner, expires_unix = excluded.expires_unix WHERE codeindex_leases.expires_unix <= ?`, repositoryID, owner, now+30, now).Exec(ctx)
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
				res, e := s.bun.NewRaw(`UPDATE codeindex_leases SET expires_unix = ? WHERE repository_id = ? AND owner = ? AND expires_unix > ?`, time.Now().Unix()+30, repositoryID, owner, time.Now().Unix()).Exec(leased)
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
		_, _ = s.bun.NewRaw(`DELETE FROM codeindex_leases WHERE repository_id = ? AND owner = ?`, repositoryID, owner).Exec(cleanup)
	}
	return leased, release, nil
}

func (s *Store) AdvanceLatest(ctx context.Context, repositoryID, snapshotID string) error {
	res, err := s.bun.NewRaw(`UPDATE codeindex_repositories SET latest_snapshot_id = ? WHERE id = ? AND EXISTS (SELECT 1 FROM codeindex_snapshots WHERE id = ? AND repository_id = ?)`, snapshotID, repositoryID, snapshotID, repositoryID).Exec(ctx)
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

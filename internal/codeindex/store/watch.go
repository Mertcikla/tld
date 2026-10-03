package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// WatchHeartbeatFreshWindow is how long a heartbeat remains "running" after the
// watcher's last update. The UI and CLI use it to distinguish a live watcher
// from a crashed or stopped one.
const WatchHeartbeatFreshWindow = 30 * time.Second

// WatchState is the shared control and status record for a repository's
// watcher. The CLI and the server both read and write the same row so either
// side can observe, start, or stop the other's watcher.
type WatchState struct {
	RepositoryID       string
	OwnerKind          string
	OwnerPID           int
	OwnerID            string
	State              string
	Stage              string
	Error              string
	GitBranch          string
	GitRevision        string
	RepoRoot           string
	SnapshotID         string
	ContentFingerprint string
	ChangedFiles       int
	PendingFiles       int
	StartedUnix        int64
	LastScanUnix       int64
	LastScanMS         int64
	HeartbeatUnix      int64
	StopRequested      bool
	PollIntervalMS     int64
	DebounceMS         int64
}

// Fresh reports whether the heartbeat is recent enough to consider the watcher
// running.
func (w WatchState) Fresh(now time.Time) bool {
	return w.HeartbeatUnix > now.Add(-WatchHeartbeatFreshWindow).Unix()
}

const watchColumns = `repository_id, owner_kind, owner_pid, owner_id, state, stage, error,
	git_branch, git_revision, repo_root, snapshot_id, content_fingerprint,
	changed_files, pending_files, started_unix, last_scan_unix, last_scan_ms,
	heartbeat_unix, stop_requested, poll_interval_ms, debounce_ms`

func scanWatchState(scan func(dest ...any) error) (WatchState, error) {
	var st WatchState
	err := scan(&st.RepositoryID, &st.OwnerKind, &st.OwnerPID, &st.OwnerID, &st.State, &st.Stage, &st.Error,
		&st.GitBranch, &st.GitRevision, &st.RepoRoot, &st.SnapshotID, &st.ContentFingerprint,
		&st.ChangedFiles, &st.PendingFiles, &st.StartedUnix, &st.LastScanUnix, &st.LastScanMS,
		&st.HeartbeatUnix, &st.StopRequested, &st.PollIntervalMS, &st.DebounceMS)
	return st, err
}

// WatchState reads a repository's watcher record.
func (s *Store) WatchState(ctx context.Context, repositoryID string) (WatchState, bool, error) {
	st, err := scanWatchState(func(dest ...any) error {
		return s.bun.NewRaw(`SELECT `+watchColumns+` FROM codeindex_watch_state WHERE repository_id = ?`, repositoryID).Scan(ctx, dest...)
	})
	if errors.Is(err, sql.ErrNoRows) {
		return WatchState{}, false, nil
	}
	if err != nil {
		return WatchState{}, false, err
	}
	return st, true, nil
}

// ListWatchStates returns every repository's watcher record.
func (s *Store) ListWatchStates(ctx context.Context) ([]WatchState, error) {
	rows, err := s.bun.QueryContext(ctx, `SELECT `+watchColumns+` FROM codeindex_watch_state ORDER BY repository_id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []WatchState
	for rows.Next() {
		st, err := scanWatchState(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}

// UpsertWatchState writes the watcher's status. HeartbeatUnix is set to now when
// the watcher is running so freshness reflects liveness. stop_requested is
// owned by external controllers (the CLI stop command or the server) and is
// deliberately not overwritten, so a heartbeat can never clobber a stop.
func (s *Store) UpsertWatchState(ctx context.Context, st WatchState) error {
	if st.HeartbeatUnix == 0 {
		st.HeartbeatUnix = time.Now().Unix()
	}
	_, err := s.bun.NewRaw(`INSERT INTO codeindex_watch_state
		(`+watchColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(repository_id) DO UPDATE SET
			owner_kind = excluded.owner_kind, owner_pid = excluded.owner_pid, owner_id = excluded.owner_id,
			state = excluded.state, stage = excluded.stage, error = excluded.error,
			git_branch = excluded.git_branch, git_revision = excluded.git_revision,
			repo_root = excluded.repo_root, snapshot_id = excluded.snapshot_id,
			content_fingerprint = excluded.content_fingerprint,
			changed_files = excluded.changed_files, pending_files = excluded.pending_files,
			started_unix = excluded.started_unix, last_scan_unix = excluded.last_scan_unix,
			last_scan_ms = excluded.last_scan_ms, heartbeat_unix = excluded.heartbeat_unix,
			poll_interval_ms = excluded.poll_interval_ms,
			debounce_ms = excluded.debounce_ms`,
		st.RepositoryID, st.OwnerKind, st.OwnerPID, st.OwnerID, st.State, st.Stage, st.Error,
		st.GitBranch, st.GitRevision, st.RepoRoot, st.SnapshotID, st.ContentFingerprint,
		st.ChangedFiles, st.PendingFiles, st.StartedUnix, st.LastScanUnix, st.LastScanMS,
		st.HeartbeatUnix, st.StopRequested, st.PollIntervalMS, st.DebounceMS).Exec(ctx)
	if err != nil {
		return fmt.Errorf("upsert watch state: %w", err)
	}
	return nil
}

// ErrWatchActive is returned when another watcher owns a repository.
var ErrWatchActive = errors.New("a watcher is already running for this repository")

// ClaimWatch atomically claims ownership of a repository's watcher. It succeeds
// when no fresh watcher exists, when the previous watcher requested a stop, or
// when the same owner is re-claiming. A successful claim also seeds the status
// fields so other readers can see the new watcher immediately.
func (s *Store) ClaimWatch(ctx context.Context, st WatchState) error {
	now := time.Now()
	st.HeartbeatUnix = now.Unix()
	res, err := s.bun.NewRaw(`INSERT INTO codeindex_watch_state
		(`+watchColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(repository_id) DO UPDATE SET
			owner_kind = excluded.owner_kind, owner_pid = excluded.owner_pid, owner_id = excluded.owner_id,
			state = excluded.state, stage = excluded.stage, error = excluded.error,
			git_branch = excluded.git_branch, git_revision = excluded.git_revision,
			repo_root = excluded.repo_root, snapshot_id = excluded.snapshot_id,
			content_fingerprint = excluded.content_fingerprint,
			changed_files = excluded.changed_files, pending_files = excluded.pending_files,
			started_unix = excluded.started_unix, last_scan_unix = excluded.last_scan_unix,
			last_scan_ms = excluded.last_scan_ms, heartbeat_unix = excluded.heartbeat_unix,
			stop_requested = excluded.stop_requested, poll_interval_ms = excluded.poll_interval_ms,
			debounce_ms = excluded.debounce_ms
		WHERE codeindex_watch_state.heartbeat_unix <= ?
		   OR codeindex_watch_state.stop_requested = TRUE
		   OR codeindex_watch_state.owner_id = excluded.owner_id`,
		st.RepositoryID, st.OwnerKind, st.OwnerPID, st.OwnerID, st.State, st.Stage, st.Error,
		st.GitBranch, st.GitRevision, st.RepoRoot, st.SnapshotID, st.ContentFingerprint,
		st.ChangedFiles, st.PendingFiles, st.StartedUnix, st.LastScanUnix, st.LastScanMS,
		st.HeartbeatUnix, st.StopRequested, st.PollIntervalMS, st.DebounceMS,
		now.Add(-WatchHeartbeatFreshWindow).Unix()).Exec(ctx)
	if err != nil {
		return fmt.Errorf("claim watch: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrWatchActive
	}
	return nil
}

// RequestWatchStop marks the repository's watcher for cooperative shutdown.
func (s *Store) RequestWatchStop(ctx context.Context, repositoryID string) error {
	if _, err := s.bun.NewRaw(`UPDATE codeindex_watch_state SET stop_requested = TRUE WHERE repository_id = ?`, repositoryID).Exec(ctx); err != nil {
		return fmt.Errorf("request watch stop: %w", err)
	}
	return nil
}

// WatchStopRequested reports whether a stop has been requested.
func (s *Store) WatchStopRequested(ctx context.Context, repositoryID string) (bool, error) {
	var requested bool
	err := s.bun.NewRaw(`SELECT stop_requested FROM codeindex_watch_state WHERE repository_id = ?`, repositoryID).Scan(ctx, &requested)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return requested, err
}

// ReleaseWatch clears a watcher's heartbeat and stop flag, but only when the
// caller still owns the record, so a new watcher's claim is never clobbered.
func (s *Store) ReleaseWatch(ctx context.Context, repositoryID, ownerID string) error {
	_, err := s.bun.NewRaw(`UPDATE codeindex_watch_state
		SET heartbeat_unix = 0, stop_requested = FALSE, state = 'stopped', stage = ''
		WHERE repository_id = ? AND owner_id = ?`, repositoryID, ownerID).Exec(ctx)
	return err
}

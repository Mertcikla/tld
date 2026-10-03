package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
)

// WatchHeartbeatFreshWindow is how long a heartbeat remains "running" after the
// watcher's last update. The UI and CLI use it to distinguish a live watcher
// from a crashed or stopped one.
const WatchHeartbeatFreshWindow = 30 * time.Second

// WatchStopDeadline is how long a stop request may remain unhonored before a
// controller escalates to killing the owning process. It bounds "stuck in
// stopping" states from crashed or non-cooperative watchers.
const WatchStopDeadline = 15 * time.Second

// PIDAlive reports whether a process with the given pid currently exists. Signal
// 0 performs an existence check without delivering a signal.
func PIDAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		// EPERM means the process exists but is owned by another user.
		return errors.Is(err, syscall.EPERM)
	}
	return true
}

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
	StopRequestedUnix  int64
	PollIntervalMS     int64
	DebounceMS         int64
}

// Fresh reports whether the heartbeat is recent enough to consider the watcher
// running. It does not verify the owning process; use Live for that.
func (w WatchState) Fresh(now time.Time) bool {
	return w.HeartbeatUnix > now.Add(-WatchHeartbeatFreshWindow).Unix()
}

// Live reports whether the watcher should be treated as running: a fresh
// heartbeat whose owning process still exists. Legacy rows written without an
// owner pid rely on freshness alone.
func (w WatchState) Live(now time.Time) bool {
	if !w.Fresh(now) {
		return false
	}
	if w.OwnerPID == 0 {
		return true
	}
	return PIDAlive(w.OwnerPID)
}

// StopExpired reports whether a stop has been requested and the watcher has
// failed to honor it within the deadline, so a controller should escalate.
func (w WatchState) StopExpired(now time.Time, requestedUnix int64) bool {
	return w.StopRequested && requestedUnix > 0 && now.Unix()-requestedUnix > int64(WatchStopDeadline.Seconds())
}

const watchColumns = `repository_id, owner_kind, owner_pid, owner_id, state, stage, error,
	git_branch, git_revision, repo_root, snapshot_id, content_fingerprint,
	changed_files, pending_files, started_unix, last_scan_unix, last_scan_ms,
	heartbeat_unix, stop_requested, stop_requested_unix, poll_interval_ms, debounce_ms`

const watchPlaceholders = `?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?`

func scanWatchState(scan func(dest ...any) error) (WatchState, error) {
	var st WatchState
	err := scan(&st.RepositoryID, &st.OwnerKind, &st.OwnerPID, &st.OwnerID, &st.State, &st.Stage, &st.Error,
		&st.GitBranch, &st.GitRevision, &st.RepoRoot, &st.SnapshotID, &st.ContentFingerprint,
		&st.ChangedFiles, &st.PendingFiles, &st.StartedUnix, &st.LastScanUnix, &st.LastScanMS,
		&st.HeartbeatUnix, &st.StopRequested, &st.StopRequestedUnix, &st.PollIntervalMS, &st.DebounceMS)
	return st, err
}

func watchArgs(st WatchState) []any {
	return []any{
		st.RepositoryID, st.OwnerKind, st.OwnerPID, st.OwnerID, st.State, st.Stage, st.Error,
		st.GitBranch, st.GitRevision, st.RepoRoot, st.SnapshotID, st.ContentFingerprint,
		st.ChangedFiles, st.PendingFiles, st.StartedUnix, st.LastScanUnix, st.LastScanMS,
		st.HeartbeatUnix, st.StopRequested, st.StopRequestedUnix, st.PollIntervalMS, st.DebounceMS,
	}
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

// UpsertWatchState writes the watcher's status. Prefer WatchHeartbeat in the
// running loop: this unconditional form is retained for seeding and tests.
// stop_requested is owned by external controllers and is not overwritten.
func (s *Store) UpsertWatchState(ctx context.Context, st WatchState) error {
	if st.HeartbeatUnix == 0 {
		st.HeartbeatUnix = time.Now().Unix()
	}
	_, err := s.bun.NewRaw(`INSERT INTO codeindex_watch_state
		(`+watchColumns+`)
		VALUES (`+watchPlaceholders+`)
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
		watchArgs(st)...).Exec(ctx)
	if err != nil {
		return fmt.Errorf("upsert watch state: %w", err)
	}
	return nil
}

// ErrWatchOwnershipLost is returned when a heartbeat's owner id no longer
// matches the row, meaning another watcher took over.
var ErrWatchOwnershipLost = errors.New("watcher ownership was taken over")

// WatchHeartbeat updates a running watcher's status only while it still owns the
// row. It returns ErrWatchOwnershipLost when another owner has claimed the
// repository, so the caller can stop to avoid two concurrent watchers. It never
// clears stop_requested, so a stop can never be clobbered by a heartbeat.
func (s *Store) WatchHeartbeat(ctx context.Context, st WatchState) error {
	if st.OwnerID == "" {
		return fmt.Errorf("watch heartbeat requires owner id")
	}
	st.HeartbeatUnix = time.Now().Unix()
	res, err := s.bun.NewRaw(`UPDATE codeindex_watch_state SET
			owner_kind = ?, owner_pid = ?, state = ?, stage = ?, error = ?,
			git_branch = ?, git_revision = ?, repo_root = ?, snapshot_id = ?, content_fingerprint = ?,
			changed_files = ?, pending_files = ?, started_unix = ?, last_scan_unix = ?, last_scan_ms = ?,
			heartbeat_unix = ?, poll_interval_ms = ?, debounce_ms = ?
		WHERE repository_id = ? AND owner_id = ?`,
		st.OwnerKind, st.OwnerPID, st.State, st.Stage, st.Error,
		st.GitBranch, st.GitRevision, st.RepoRoot, st.SnapshotID, st.ContentFingerprint,
		st.ChangedFiles, st.PendingFiles, st.StartedUnix, st.LastScanUnix, st.LastScanMS,
		st.HeartbeatUnix, st.PollIntervalMS, st.DebounceMS,
		st.RepositoryID, st.OwnerID).Exec(ctx)
	if err != nil {
		return fmt.Errorf("watch heartbeat: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrWatchOwnershipLost
	}
	return nil
}

// ErrWatchActive is returned when another watcher owns a repository.
var ErrWatchActive = errors.New("a watcher is already running for this repository")

// ClaimWatch atomically claims ownership of a repository's watcher. It succeeds
// when the existing record is stale, its owning process is dead, a stop was
// requested, or the same owner is re-claiming. A successful claim seeds the
// status fields and resets any prior stop flag so other readers see the new
// watcher immediately.
//
// The comparison happens in SQL so two concurrent starts cannot both win. When
// the row belongs to a live process we do not recognize, the database rejects
// the claim and ErrWatchActive is returned.
func (s *Store) ClaimWatch(ctx context.Context, st WatchState) error {
	now := time.Now()
	existing, found, err := s.WatchState(ctx, st.RepositoryID)
	if err != nil {
		return err
	}
	if found && existing.OwnerID != st.OwnerID {
		if existing.Live(now) {
			// The row is live and owned by a different process. The owner id is
			// unique per process start, so a recycled pid cannot masquerade.
			return ErrWatchActive
		}
		// The recorded owner is dead or stale but its heartbeat may still be
		// fresh. Clear it so the SQL compare-and-swap below can proceed.
		if err := s.ForceClearWatchState(ctx, st.RepositoryID); err != nil {
			return err
		}
	}
	st.HeartbeatUnix = now.Unix()
	st.StopRequested = false
	st.StopRequestedUnix = 0
	res, err := s.bun.NewRaw(`INSERT INTO codeindex_watch_state
		(`+watchColumns+`)
		VALUES (`+watchPlaceholders+`)
		ON CONFLICT(repository_id) DO UPDATE SET
			owner_kind = excluded.owner_kind, owner_pid = excluded.owner_pid, owner_id = excluded.owner_id,
			state = excluded.state, stage = excluded.stage, error = excluded.error,
			git_branch = excluded.git_branch, git_revision = excluded.git_revision,
			repo_root = excluded.repo_root, snapshot_id = excluded.snapshot_id,
			content_fingerprint = excluded.content_fingerprint,
			changed_files = excluded.changed_files, pending_files = excluded.pending_files,
			started_unix = excluded.started_unix, last_scan_unix = excluded.last_scan_unix,
			last_scan_ms = excluded.last_scan_ms, heartbeat_unix = excluded.heartbeat_unix,
			stop_requested = excluded.stop_requested, stop_requested_unix = excluded.stop_requested_unix,
			poll_interval_ms = excluded.poll_interval_ms,
			debounce_ms = excluded.debounce_ms
		WHERE codeindex_watch_state.owner_id = excluded.owner_id
		   OR codeindex_watch_state.heartbeat_unix <= ?`,
		append(watchArgs(st), now.Add(-WatchHeartbeatFreshWindow).Unix())...).Exec(ctx)
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

// RequestWatchStop marks the repository's watcher for cooperative shutdown and
// records when the stop was requested so controllers can escalate if it is not
// honored within WatchStopDeadline.
func (s *Store) RequestWatchStop(ctx context.Context, repositoryID string) error {
	if _, err := s.bun.NewRaw(`UPDATE codeindex_watch_state
		SET stop_requested = TRUE,
		    stop_requested_unix = CASE WHEN stop_requested THEN stop_requested_unix ELSE ? END
		WHERE repository_id = ?`, time.Now().Unix(), repositoryID).Exec(ctx); err != nil {
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
		SET heartbeat_unix = 0, stop_requested = FALSE, stop_requested_unix = 0, state = 'stopped', stage = ''
		WHERE repository_id = ? AND owner_id = ?`, repositoryID, ownerID).Exec(ctx)
	return err
}

// ForceClearWatchState resets a repository's watcher record to stopped. It is
// used to recover from stale rows left by crashed watchers.
func (s *Store) ForceClearWatchState(ctx context.Context, repositoryID string) error {
	_, err := s.bun.NewRaw(`UPDATE codeindex_watch_state
		SET heartbeat_unix = 0, stop_requested = FALSE, stop_requested_unix = 0, state = 'stopped', stage = ''
		WHERE repository_id = ?`, repositoryID).Exec(ctx)
	return err
}

// ReapStaleWatchStates resets records whose owning process is gone or whose
// heartbeat is stale. It returns the number of records cleared. It is safe to
// call on every status read.
func (s *Store) ReapStaleWatchStates(ctx context.Context) error {
	states, err := s.ListWatchStates(ctx)
	if err != nil {
		return err
	}
	now := time.Now()
	for _, st := range states {
		if st.OwnerID == "" && st.HeartbeatUnix == 0 {
			continue
		}
		if st.Live(now) {
			continue
		}
		if err := s.ForceClearWatchState(ctx, st.RepositoryID); err != nil {
			return err
		}
	}
	return nil
}

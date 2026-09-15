-- Persists the deterministic output of `tld impact` so the last run can be
-- reloaded (UI refresh, PR comment) without recomputing. It is a read-only
-- snapshot: it never feeds back into the authored architecture.
CREATE TABLE IF NOT EXISTS impact_runs (
  id             INTEGER PRIMARY KEY AUTOINCREMENT,
  repo_ref       TEXT NOT NULL DEFAULT '',
  repo_root      TEXT NOT NULL DEFAULT '',
  remote_url     TEXT NOT NULL DEFAULT '',
  base           TEXT NOT NULL DEFAULT '',
  head           TEXT NOT NULL DEFAULT '',
  view_id        INTEGER NOT NULL DEFAULT 0,
  arch_revision  TEXT NOT NULL DEFAULT '',
  changed_files  TEXT NOT NULL DEFAULT '[]',
  changed        TEXT NOT NULL DEFAULT '[]',
  candidates     TEXT NOT NULL DEFAULT '[]',
  related        TEXT NOT NULL DEFAULT '[]',
  edges          TEXT NOT NULL DEFAULT '[]',
  unmapped       TEXT NOT NULL DEFAULT '[]',
  coverage       TEXT NOT NULL DEFAULT '{}',
  created_at     TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_impact_runs_latest
  ON impact_runs (repo_root, created_at DESC);

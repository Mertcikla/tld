CREATE TABLE flows (
  id TEXT PRIMARY KEY,
  view_id INTEGER NOT NULL REFERENCES views(id) ON DELETE CASCADE,
  org_id TEXT NULL,
  document TEXT NOT NULL,
  revision INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0)
);
CREATE INDEX idx_flows_view_id ON flows(view_id);
CREATE INDEX idx_flows_org_id ON flows(org_id);

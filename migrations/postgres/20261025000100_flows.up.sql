CREATE TABLE flows (
  id TEXT PRIMARY KEY,
  view_id BIGINT NOT NULL REFERENCES views(id) ON DELETE CASCADE,
  org_id UUID NULL,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  revision INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0)
);
CREATE INDEX idx_flows_view_id ON flows(view_id);
CREATE INDEX idx_flows_org_id ON flows(org_id);

CREATE TABLE flow_steps (
  flow_id TEXT NOT NULL REFERENCES flows(id) ON DELETE CASCADE,
  id TEXT NOT NULL,
  path_id TEXT NULL,
  position INTEGER NOT NULL CHECK (position >= 0),
  title TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  element_ids TEXT NOT NULL DEFAULT '[]',
  connector_ids TEXT NOT NULL DEFAULT '[]',
  branch_mode TEXT NULL CHECK (branch_mode IN ('alternate', 'parallel')),
  go_to_flow_id TEXT NULL,
  PRIMARY KEY (flow_id, id),
  CHECK (branch_mode IS NULL OR go_to_flow_id IS NULL),
  CHECK (id <> ''),
  CHECK (go_to_flow_id IS NULL OR go_to_flow_id <> '')
);
CREATE UNIQUE INDEX idx_flow_steps_root_position ON flow_steps(flow_id, position) WHERE path_id IS NULL;
CREATE UNIQUE INDEX idx_flow_steps_path_position ON flow_steps(flow_id, path_id, position) WHERE path_id IS NOT NULL;

CREATE TABLE flow_paths (
  flow_id TEXT NOT NULL REFERENCES flows(id) ON DELETE CASCADE,
  id TEXT NOT NULL,
  step_id TEXT NOT NULL,
  position INTEGER NOT NULL CHECK (position >= 0),
  name TEXT NOT NULL CHECK (name <> ''),
  PRIMARY KEY (flow_id, id),
  UNIQUE (flow_id, step_id, position),
  CHECK (id <> ''),
  FOREIGN KEY (flow_id, step_id) REFERENCES flow_steps(flow_id, id) ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED
);
ALTER TABLE flow_steps ADD CONSTRAINT flow_steps_path_fk
  FOREIGN KEY (flow_id, path_id) REFERENCES flow_paths(flow_id, id) ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED;

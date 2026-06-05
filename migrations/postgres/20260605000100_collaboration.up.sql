CREATE TABLE IF NOT EXISTS view_threads (
  id BIGSERIAL PRIMARY KEY,
  workspace_id TEXT NOT NULL,
  view_id BIGINT NOT NULL REFERENCES views(id) ON DELETE CASCADE,
  element_id BIGINT NULL REFERENCES elements(id) ON DELETE CASCADE,
  connector_id BIGINT NULL REFERENCES connectors(id) ON DELETE CASCADE,
  created_by TEXT NOT NULL,
  created_by_username TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'resolved')),
  created_at TEXT NOT NULL,
  resolved_at TEXT NULL,
  CHECK (((element_id IS NOT NULL)::int + (connector_id IS NOT NULL)::int) = 1)
);

CREATE INDEX IF NOT EXISTS idx_view_threads_workspace_view
  ON view_threads(workspace_id, view_id);

CREATE INDEX IF NOT EXISTS idx_view_threads_element
  ON view_threads(workspace_id, view_id, element_id)
  WHERE element_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_view_threads_connector
  ON view_threads(workspace_id, view_id, connector_id)
  WHERE connector_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS view_comments (
  id BIGSERIAL PRIMARY KEY,
  workspace_id TEXT NOT NULL,
  view_id BIGINT NOT NULL REFERENCES views(id) ON DELETE CASCADE,
  thread_id BIGINT NOT NULL REFERENCES view_threads(id) ON DELETE CASCADE,
  author_id TEXT NOT NULL,
  author_username TEXT NOT NULL,
  body TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_view_comments_thread
  ON view_comments(workspace_id, thread_id, created_at);

CREATE TABLE IF NOT EXISTS element_reactions (
  id BIGSERIAL PRIMARY KEY,
  workspace_id TEXT NOT NULL,
  view_id BIGINT NOT NULL REFERENCES views(id) ON DELETE CASCADE,
  element_id BIGINT NOT NULL REFERENCES elements(id) ON DELETE CASCADE,
  user_id TEXT NOT NULL,
  emoji TEXT NOT NULL,
  created_at TEXT NOT NULL,
  UNIQUE (workspace_id, view_id, element_id, user_id, emoji)
);

CREATE INDEX IF NOT EXISTS idx_element_reactions_view_element
  ON element_reactions(workspace_id, view_id, element_id);

CREATE TABLE IF NOT EXISTS drawings (
  id BIGSERIAL PRIMARY KEY,
  workspace_id TEXT NOT NULL,
  view_id BIGINT NOT NULL REFERENCES views(id) ON DELETE CASCADE,
  user_id TEXT NOT NULL,
  path_id TEXT NOT NULL,
  points TEXT NOT NULL,
  color TEXT NOT NULL,
  width DOUBLE PRECISION NOT NULL,
  text TEXT NOT NULL DEFAULT '',
  font_size DOUBLE PRECISION NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE (workspace_id, view_id, path_id)
);

CREATE INDEX IF NOT EXISTS idx_drawings_workspace_view
  ON drawings(workspace_id, view_id);

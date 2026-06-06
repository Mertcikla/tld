CREATE TABLE IF NOT EXISTS view_threads (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  workspace_id TEXT NOT NULL,
  view_id INTEGER NOT NULL,
  element_id INTEGER NULL,
  connector_id INTEGER NULL,
  created_by TEXT NOT NULL,
  created_by_username TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'open',
  created_at TEXT NOT NULL,
  resolved_at TEXT NULL,
  CHECK (status IN ('open', 'resolved')),
  CHECK (((element_id IS NOT NULL) + (connector_id IS NOT NULL)) = 1),
  FOREIGN KEY (view_id) REFERENCES views(id) ON DELETE CASCADE,
  FOREIGN KEY (element_id) REFERENCES elements(id) ON DELETE CASCADE,
  FOREIGN KEY (connector_id) REFERENCES connectors(id) ON DELETE CASCADE
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
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  workspace_id TEXT NOT NULL,
  view_id INTEGER NOT NULL,
  thread_id INTEGER NOT NULL,
  author_id TEXT NOT NULL,
  author_username TEXT NOT NULL,
  body TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (view_id) REFERENCES views(id) ON DELETE CASCADE,
  FOREIGN KEY (thread_id) REFERENCES view_threads(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_view_comments_thread
  ON view_comments(workspace_id, thread_id, created_at);

CREATE TABLE IF NOT EXISTS element_reactions (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  workspace_id TEXT NOT NULL,
  view_id INTEGER NOT NULL,
  element_id INTEGER NOT NULL,
  user_id TEXT NOT NULL,
  emoji TEXT NOT NULL,
  created_at TEXT NOT NULL,
  UNIQUE (workspace_id, view_id, element_id, user_id, emoji),
  FOREIGN KEY (view_id) REFERENCES views(id) ON DELETE CASCADE,
  FOREIGN KEY (element_id) REFERENCES elements(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_element_reactions_view_element
  ON element_reactions(workspace_id, view_id, element_id);

CREATE TABLE IF NOT EXISTS drawings (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  workspace_id TEXT NOT NULL,
  view_id INTEGER NOT NULL,
  user_id TEXT NOT NULL,
  path_id TEXT NOT NULL,
  points TEXT NOT NULL,
  color TEXT NOT NULL,
  width REAL NOT NULL,
  text TEXT NOT NULL DEFAULT '',
  font_size REAL NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE (workspace_id, view_id, path_id),
  FOREIGN KEY (view_id) REFERENCES views(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_drawings_workspace_view
  ON drawings(workspace_id, view_id);

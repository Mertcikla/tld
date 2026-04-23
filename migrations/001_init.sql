PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS views (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  owner_element_id INTEGER NULL,
  name TEXT NOT NULL,
  description TEXT NULL,
  level_label TEXT NULL,
  level INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (owner_element_id) REFERENCES elements(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS elements (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  kind TEXT NULL,
  description TEXT NULL,
  technology TEXT NULL,
  url TEXT NULL,
  logo_url TEXT NULL,
  technology_connectors TEXT NOT NULL DEFAULT '[]',
  tags TEXT NOT NULL DEFAULT '[]',
  repo TEXT NULL,
  branch TEXT NULL,
  file_path TEXT NULL,
  language TEXT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS placements (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  view_id INTEGER NOT NULL,
  element_id INTEGER NOT NULL,
  position_x REAL NOT NULL,
  position_y REAL NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  UNIQUE(view_id, element_id),
  FOREIGN KEY (view_id) REFERENCES views(id) ON DELETE CASCADE,
  FOREIGN KEY (element_id) REFERENCES elements(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS connectors (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  view_id INTEGER NOT NULL,
  source_element_id INTEGER NOT NULL,
  target_element_id INTEGER NOT NULL,
  label TEXT NULL,
  description TEXT NULL,
  relationship TEXT NULL,
  direction TEXT NOT NULL DEFAULT 'forward',
  style TEXT NOT NULL DEFAULT 'solid',
  url TEXT NULL,
  source_handle TEXT NULL,
  target_handle TEXT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (view_id) REFERENCES views(id) ON DELETE CASCADE,
  FOREIGN KEY (source_element_id) REFERENCES elements(id) ON DELETE CASCADE,
  FOREIGN KEY (target_element_id) REFERENCES elements(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS view_layers (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  view_id INTEGER NOT NULL,
  name TEXT NOT NULL,
  tags TEXT NOT NULL DEFAULT '[]',
  color TEXT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY (view_id) REFERENCES views(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS tags (
  name TEXT PRIMARY KEY,
  color TEXT NOT NULL,
  description TEXT NULL
);

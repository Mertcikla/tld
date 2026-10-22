CREATE TABLE plan_handoffs (
 plan_id TEXT NOT NULL REFERENCES plans(id) ON DELETE CASCADE,
 revision INTEGER NOT NULL, payload TEXT NOT NULL,
 PRIMARY KEY (plan_id, revision)
);

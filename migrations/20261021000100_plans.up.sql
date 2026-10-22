CREATE TABLE plans (
 id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL, repository_id TEXT NOT NULL,
 status TEXT NOT NULL, revision INTEGER NOT NULL DEFAULT 0, payload TEXT NOT NULL, history TEXT NOT NULL,
 updated_at TEXT NOT NULL
);
CREATE INDEX plans_repository ON plans(workspace_id, repository_id, updated_at);
CREATE TABLE plan_operations (
 plan_id TEXT NOT NULL REFERENCES plans(id) ON DELETE CASCADE,
 revision INTEGER NOT NULL, operation_index INTEGER NOT NULL,
 operation_id TEXT NOT NULL, request_id TEXT NOT NULL, request_hash TEXT NOT NULL, payload TEXT NOT NULL,
 PRIMARY KEY(plan_id, revision, operation_index), UNIQUE(plan_id, operation_id)
);
CREATE INDEX plan_operations_request ON plan_operations(plan_id, request_id);
CREATE TABLE plan_operation_preconditions (
 plan_id TEXT NOT NULL REFERENCES plans(id) ON DELETE CASCADE,
 operation_id TEXT NOT NULL, resource_key TEXT NOT NULL, expected_hash TEXT NOT NULL,
 payload TEXT NOT NULL, PRIMARY KEY(plan_id, operation_id, resource_key)
);
CREATE TABLE plan_members (
 plan_id TEXT NOT NULL REFERENCES plans(id) ON DELETE CASCADE,
 resource_key TEXT NOT NULL, payload TEXT NOT NULL, PRIMARY KEY(plan_id, resource_key)
);
CREATE TABLE plan_checklist_items (
 plan_id TEXT NOT NULL REFERENCES plans(id) ON DELETE CASCADE,
 item_id TEXT NOT NULL, payload TEXT NOT NULL, PRIMARY KEY(plan_id, item_id)
);
CREATE TABLE plan_checklist_targets (
 plan_id TEXT NOT NULL REFERENCES plans(id) ON DELETE CASCADE,
 item_id TEXT NOT NULL, resource_key TEXT NOT NULL, PRIMARY KEY(plan_id, item_id, resource_key)
);
CREATE TABLE plan_notes (
 plan_id TEXT NOT NULL REFERENCES plans(id) ON DELETE CASCADE,
 view_key TEXT NOT NULL, payload TEXT NOT NULL, PRIMARY KEY(plan_id, view_key)
);

CREATE TABLE plan_workspace_requests (
 plan_id TEXT NOT NULL REFERENCES plans(id) ON DELETE CASCADE,
 request_id TEXT NOT NULL,
 request_hash TEXT NOT NULL,
 response TEXT NOT NULL,
 PRIMARY KEY (plan_id, request_id)
);

CREATE TABLE plan_implementations (workspace_id TEXT PRIMARY KEY, plan_id TEXT NOT NULL, revision INTEGER NOT NULL, payload TEXT NOT NULL);

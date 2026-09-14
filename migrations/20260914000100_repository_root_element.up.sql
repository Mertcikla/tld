PRAGMA foreign_keys = ON;

-- Links a workspace repository to the diagram element that represents it, so
-- UI-managed repositories stay integrated with the architecture.
ALTER TABLE watch_repositories ADD COLUMN root_element_id INTEGER NULL;

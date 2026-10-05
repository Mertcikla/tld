package dbrepo

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/migrate"
)

func bootstrapLegacyMigrationState(ctx context.Context, db *bun.DB, migrator *migrate.Migrator, migrations migrate.MigrationSlice, dialect Dialect) error {
	applied, err := migrator.AppliedMigrations(ctx)
	if err != nil {
		return fmt.Errorf("read applied migrations: %w", err)
	}
	if len(applied) > 0 {
		return nil
	}

	for _, migration := range migrations {
		ok, err := legacyMigrationAlreadyApplied(ctx, db, dialect, migration.Comment)
		if err != nil {
			return fmt.Errorf("inspect legacy migration %s: %w", migration.String(), err)
		}
		if !ok {
			continue
		}
		migration.GroupID = 1
		if err := migrator.MarkApplied(ctx, &migration); err != nil {
			return fmt.Errorf("mark legacy migration %s applied: %w", migration.String(), err)
		}
	}
	return nil
}

func legacyMigrationAlreadyApplied(ctx context.Context, db *bun.DB, dialect Dialect, comment string) (bool, error) {
	switch dialect {
	case DialectSQLite:
		return legacySQLiteMigrationAlreadyApplied(ctx, db, comment)
	case DialectPostgres:
		return legacyPostgresMigrationAlreadyApplied(ctx, db, comment)
	default:
		return false, fmt.Errorf("unsupported db dialect %q", dialect)
	}
}

func legacySQLiteMigrationAlreadyApplied(ctx context.Context, db *bun.DB, comment string) (bool, error) {
	switch comment {
	case "init":
		return sqliteTablesExist(ctx, db, "elements", "views", "placements", "connectors", "view_layers", "tags")
	case "view_density_visibility_overrides":
		return sqliteColumnExists(ctx, db, "views", "density_level", "view_visibility_overrides")
	case "missing_fk_indexes":
		return sqliteIndexesExist(ctx, db, "idx_view_layers_view_id", "idx_connectors_source_element_id", "idx_connectors_target_element_id")
	case "view_connector_tags":
		viewsTags, err := sqliteColumnExists(ctx, db, "views", "tags")
		if err != nil || !viewsTags {
			return viewsTags, err
		}
		return sqliteColumnExists(ctx, db, "connectors", "tags")
	case "element_noise_gate_bypass":
		return sqliteColumnExists(ctx, db, "elements", "bypass_noise_gate")
	case "view_markdown_source_kind":
		return sqliteColumnExists(ctx, db, "view_markdown_documents", "source_kind")
	default:
		return false, nil
	}
}

func legacyPostgresMigrationAlreadyApplied(ctx context.Context, db *bun.DB, comment string) (bool, error) {
	switch comment {
	case "local_schema":
		return postgresTablesExist(ctx, db, "elements", "views", "connectors", "watch_embedding_models")
	case "element_noise_gate_bypass":
		return postgresColumnExists(ctx, db, "elements", "bypass_noise_gate")
	default:
		return false, nil
	}
}

func sqliteTablesExist(ctx context.Context, db *bun.DB, tables ...string) (bool, error) {
	for _, table := range tables {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type IN ('table', 'view') AND name = ?`, table).Scan(&count); err != nil {
			return false, err
		}
		if count == 0 {
			return false, nil
		}
	}
	return true, nil
}

func sqliteIndexesExist(ctx context.Context, db *bun.DB, indexes ...string) (bool, error) {
	for _, index := range indexes {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?`, index).Scan(&count); err != nil {
			return false, err
		}
		if count == 0 {
			return false, nil
		}
	}
	return true, nil
}

func sqliteColumnExists(ctx context.Context, db *bun.DB, table, column string, requiredTables ...string) (bool, error) {
	for _, required := range requiredTables {
		ok, err := sqliteTablesExist(ctx, db, required)
		if err != nil || !ok {
			return ok, err
		}
	}
	query, ok := sqliteTableInfoQuery(table)
	if !ok {
		return false, fmt.Errorf("unsupported sqlite table %q", table)
	}
	rows, err := db.DB.QueryContext(ctx, query)
	if err != nil {
		return false, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, pk int
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	return false, nil
}

func sqliteTableInfoQuery(table string) (string, bool) {
	switch table {
	case "elements":
		return "PRAGMA table_info(elements)", true
	case "views":
		return "PRAGMA table_info(views)", true
	case "connectors":
		return "PRAGMA table_info(connectors)", true
	case "view_markdown_documents":
		return "PRAGMA table_info(view_markdown_documents)", true
	default:
		return "", false
	}
}

func postgresTablesExist(ctx context.Context, db *bun.DB, tables ...string) (bool, error) {
	for _, table := range tables {
		var exists bool
		if err := db.QueryRowContext(ctx, `
			SELECT EXISTS (
				SELECT 1
				FROM information_schema.tables
				WHERE table_schema = current_schema() AND table_name = ?
			)`, table).Scan(&exists); err != nil {
			return false, err
		}
		if !exists {
			return false, nil
		}
	}
	return true, nil
}

func postgresColumnExists(ctx context.Context, db *bun.DB, table, column string) (bool, error) {
	var exists bool
	if err := db.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.columns
			WHERE table_schema = current_schema() AND table_name = ? AND column_name = ?
		)`, table, column).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

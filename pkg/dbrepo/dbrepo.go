// Package dbrepo contains shared database opening primitives for tld stores.
package dbrepo

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"time"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/pgdriver"
	"github.com/uptrace/bun/migrate"
	sqlitevec "github.com/viant/sqlite-vec/vec"
	_ "modernc.org/sqlite"
)

type Dialect string

const (
	DialectSQLite   Dialect = "sqlite"
	DialectPostgres Dialect = "postgres"
)

type SchemaProfile string

const (
	SchemaLocal SchemaProfile = "local"
	SchemaCloud SchemaProfile = "cloud"
)

type DBOptions struct {
	Dialect       Dialect
	SQLitePath    string
	DatabaseURL   string
	SchemaProfile SchemaProfile
	Migrations    embed.FS
}

type Handle struct {
	DB            *sql.DB
	Bun           *bun.DB
	Dialect       Dialect
	SchemaProfile SchemaProfile
}

func (h *Handle) Close() error {
	if h == nil || h.DB == nil {
		return nil
	}
	return h.DB.Close()
}

func Open(ctx context.Context, opts DBOptions) (*Handle, error) {
	if opts.Dialect == "" {
		opts.Dialect = DialectSQLite
	}
	if opts.SchemaProfile == "" {
		opts.SchemaProfile = SchemaLocal
	}
	switch opts.Dialect {
	case DialectSQLite:
		return OpenSQLite(ctx, opts)
	case DialectPostgres:
		return OpenPostgres(ctx, opts)
	default:
		return nil, fmt.Errorf("unsupported db dialect %q", opts.Dialect)
	}
}

func OpenSQLite(ctx context.Context, opts DBOptions) (*Handle, error) {
	if opts.SQLitePath == "" {
		return nil, fmt.Errorf("sqlite path is required")
	}
	db, err := sql.Open("sqlite", opts.SQLitePath)
	if err != nil {
		return nil, err
	}
	if err := sqlitevec.Register(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("register sqlite-vec: %w", err)
	}
	configureSQLitePool(db)
	if err := configureSQLite(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	bunDB := bun.NewDB(db, sqlitedialect.New())
	if err := ApplyEmbeddedMigrations(ctx, bunDB, opts.Migrations, "migrations", DialectSQLite); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Handle{DB: db, Bun: bunDB, Dialect: DialectSQLite, SchemaProfile: opts.SchemaProfile}, nil
}

func OpenPostgres(ctx context.Context, opts DBOptions) (*Handle, error) {
	if opts.DatabaseURL == "" {
		return nil, fmt.Errorf("postgres database url is required")
	}
	db := sql.OpenDB(pgdriver.NewConnector(pgdriver.WithDSN(opts.DatabaseURL)))
	configurePostgresPool(db)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	bunDB := bun.NewDB(db, pgdialect.New())
	if err := ApplyEmbeddedMigrations(ctx, bunDB, opts.Migrations, "migrations/postgres", DialectPostgres); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Handle{DB: db, Bun: bunDB, Dialect: DialectPostgres, SchemaProfile: opts.SchemaProfile}, nil
}

func ApplyEmbeddedMigrations(ctx context.Context, db *bun.DB, migrations embed.FS, dir string, dialect Dialect) error {
	migrationFS, err := fs.Sub(migrations, dir)
	if err != nil {
		return err
	}

	migrationsCollection := migrate.NewMigrations(migrate.WithMigrationsDirectory(dir))
	if err := migrationsCollection.Discover(shallowMigrationFS{fsys: migrationFS}); err != nil {
		return fmt.Errorf("discover migrations in %s: %w", dir, err)
	}

	migrator := migrate.NewMigrator(
		db,
		migrationsCollection,
		migrate.WithMarkAppliedOnSuccess(true),
		migrate.WithUpsert(true),
	)
	if err := migrator.Init(ctx); err != nil {
		return fmt.Errorf("init migrations: %w", err)
	}
	if err := bootstrapLegacyMigrationState(ctx, db, migrator, migrationsCollection.Sorted(), dialect); err != nil {
		return err
	}
	if _, err := migrator.Migrate(ctx); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}

type shallowMigrationFS struct {
	fsys fs.FS
}

func (s shallowMigrationFS) Open(name string) (fs.File, error) {
	return s.fsys.Open(name)
}

func (s shallowMigrationFS) ReadDir(name string) ([]fs.DirEntry, error) {
	entries, err := fs.ReadDir(s.fsys, name)
	if err != nil {
		return nil, err
	}
	files := entries[:0]
	for _, entry := range entries {
		if !entry.IsDir() {
			files = append(files, entry)
		}
	}
	return files, nil
}

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
	case "watch_raw_code_graph":
		return sqliteTablesExist(ctx, db, "watch_repositories", "watch_files", "watch_symbols", "watch_embeddings", "watch_materialization")
	case "view_density_visibility_overrides":
		return sqliteColumnExists(ctx, db, "views", "density_level", "view_visibility_overrides")
	case "missing_fk_indexes":
		return sqliteIndexesExist(ctx, db, "idx_view_layers_view_id", "idx_connectors_source_element_id", "idx_connectors_target_element_id")
	case "watch_materialization_resource_lookup":
		return sqliteIndexesExist(ctx, db, "idx_watch_materialization_resource_lookup")
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

func configureSQLitePool(db *sql.DB) {
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
}

func configureSQLite(ctx context.Context, db *sql.DB) error {
	pragmas := []string{
		`PRAGMA busy_timeout = 5000;`,
		`PRAGMA journal_mode = WAL;`,
		`PRAGMA synchronous = NORMAL;`,
		`PRAGMA foreign_keys = ON;`,
	}
	for _, pragma := range pragmas {
		if _, err := db.ExecContext(ctx, pragma); err != nil {
			return fmt.Errorf("configure sqlite %s: %w", pragma, err)
		}
	}
	return nil
}

func configurePostgresPool(db *sql.DB) {
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(30 * time.Minute)
	db.SetConnMaxIdleTime(5 * time.Minute)
}

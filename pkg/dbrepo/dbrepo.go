// Package dbrepo contains shared database opening primitives for tld stores.
package dbrepo

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"time"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/pgdriver"
	"github.com/uptrace/bun/migrate"
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
	configureSQLitePool(db)
	if err := configureSQLite(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	bunDB := bun.NewDB(db, sqlitedialect.New())
	if err := ApplyEmbeddedMigrations(ctx, bunDB, opts.Migrations, "migrations"); err != nil {
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
	if err := ApplyEmbeddedMigrations(ctx, bunDB, opts.Migrations, "migrations/postgres"); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Handle{DB: db, Bun: bunDB, Dialect: DialectPostgres, SchemaProfile: opts.SchemaProfile}, nil
}

func ApplyEmbeddedMigrations(ctx context.Context, db *bun.DB, migrations embed.FS, dir string) error {
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
	dbDialect := DialectSQLite
	if db.Dialect().Name() == dialect.PG {
		dbDialect = DialectPostgres
	}
	if err := bootstrapLegacyMigrationState(ctx, db, migrator, migrationsCollection.Sorted(), dbDialect); err != nil {
		return err
	}
	if dbDialect == DialectSQLite {
		return applySQLiteMigrations(ctx, db, migrator, migrationFS)
	}
	if _, err := migrator.Migrate(ctx); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}

// SQLite DDL and migration history must commit together. Otherwise an
// interrupted upgrade can delete data or commit an ALTER TABLE without its
// history record, causing the next startup to repeat a non-repeatable change.
func applySQLiteMigrations(ctx context.Context, db *bun.DB, migrator *migrate.Migrator, migrationFS fs.FS) error {
	migrations, err := migrator.MigrationsWithStatus(ctx)
	if err != nil {
		return fmt.Errorf("read migration status: %w", err)
	}
	pending := migrations.Unapplied()
	if len(pending) == 0 {
		return nil
	}
	applied, err := migrator.AppliedMigrations(ctx)
	if err != nil {
		return fmt.Errorf("read applied migrations: %w", err)
	}
	groupID := applied.LastGroupID() + 1
	err = db.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		for _, migration := range pending {
			filename := migration.String() + ".up.sql"
			raw, err := fs.ReadFile(migrationFS, filename)
			if errors.Is(err, fs.ErrNotExist) {
				filename = migration.String() + ".tx.up.sql"
				raw, err = fs.ReadFile(migrationFS, filename)
			}
			if err != nil {
				return fmt.Errorf("read migration %s: %w", filename, err)
			}
			if _, err := tx.Tx.ExecContext(ctx, string(raw)); err != nil {
				return fmt.Errorf("migration %s: %w", migration.Name, err)
			}
			migration.GroupID = groupID
			if _, err := tx.NewInsert().Model(&migration).ModelTableExpr("bun_migrations").Exec(ctx); err != nil {
				return fmt.Errorf("record migration %s: %w", migration.Name, err)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("apply sqlite migrations: %w", err)
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

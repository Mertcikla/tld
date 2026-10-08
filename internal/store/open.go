package store

import (
	"context"
	"embed"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/mertcikla/tld/v2/pkg/app"
	"github.com/mertcikla/tld/v2/pkg/dbrepo"
)

func Open(dbPath string, migrations embed.FS) (*app.Store, error) {
	return app.OpenStore(dbPath, migrations)
}

func OpenWithOptions(ctx context.Context, opts dbrepo.DBOptions) (*app.Store, error) {
	return app.OpenStoreWithOptions(ctx, opts)
}

func OpenLocal(ctx context.Context, cfg *workspace.Config, dataDir string, migrations embed.FS) (*app.Store, error) {
	opts, err := LocalDBOptions(cfg, dataDir, migrations)
	if err != nil {
		return nil, err
	}
	return OpenWithOptions(ctx, opts)
}

func LocalDBOptions(cfg *workspace.Config, dataDir string, migrations embed.FS) (dbrepo.DBOptions, error) {
	driver := "sqlite"
	databaseURL := ""
	if cfg != nil {
		if strings.TrimSpace(cfg.Database.Driver) != "" {
			driver = strings.ToLower(strings.TrimSpace(cfg.Database.Driver))
		}
		databaseURL = strings.TrimSpace(cfg.Database.DatabaseURL)
	}
	switch driver {
	case "", "sqlite":
		return dbrepo.DBOptions{
			Dialect:    dbrepo.DialectSQLite,
			SQLitePath: filepath.Join(dataDir, "tld.db"),
			Migrations: migrations,
		}, nil
	case "postgres", "postgresql":
		if databaseURL == "" {
			return dbrepo.DBOptions{}, fmt.Errorf("database.url or TLD_DATABASE_URL is required when database.driver is postgres")
		}
		return dbrepo.DBOptions{
			Dialect:     dbrepo.DialectPostgres,
			DatabaseURL: databaseURL,
			Migrations:  migrations,
		}, nil
	default:
		return dbrepo.DBOptions{}, fmt.Errorf("unsupported database.driver %q", driver)
	}
}

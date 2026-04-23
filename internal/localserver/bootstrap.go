package localserver

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	assets "github.com/mertcikla/diag/tld"
	"github.com/mertcikla/diag/tld/internal/server"
	"github.com/mertcikla/diag/tld/internal/store"
)

var localWorkspaceID = uuid.MustParse("11111111-1111-1111-1111-111111111111")

type App struct {
	Addr    string
	DBPath  string
	Handler http.Handler
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func AddrFromEnv() string {
	return envOrDefault("TLD_ADDR", "127.0.0.1:"+envOrDefault("PORT", "8081"))
}

func Bootstrap(cwd string) (*App, error) {
	dbPath := filepath.Join(cwd, "data", "tld.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, err
	}

	staticFS, err := assets.StaticFS()
	if err != nil {
		return nil, err
	}

	sqliteStore, err := store.Open(dbPath, assets.FS)
	if err != nil {
		return nil, err
	}

	srv, err := server.New(sqliteStore, staticFS, localWorkspaceID)
	if err != nil {
		return nil, err
	}

	return &App{
		Addr:    AddrFromEnv(),
		DBPath:  dbPath,
		Handler: srv.Routes(),
	}, nil
}

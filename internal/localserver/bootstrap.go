package localserver

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	assets "github.com/mertcikla/tld"
	"github.com/mertcikla/tld/internal/server"
	"github.com/mertcikla/tld/internal/store"
)

var localWorkspaceID = uuid.MustParse("11111111-1111-1111-1111-111111111111")

type App struct {
	Addr            string
	DBPath          string
	InitializedData bool
	Resources       ResourceCounts
	Handler         http.Handler
}

type ResourceCounts struct {
	Views      int
	Elements   int
	Connectors int
}

// ServeOptions overrides the address that Bootstrap listens on.
// An empty field means "use the lower-priority source".
type ServeOptions struct {
	Host string
	Port string
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func AddrFromEnv() string {
	return envOrDefault("TLD_ADDR", "127.0.0.1:"+envOrDefault("PORT", "8060"))
}

func DatabasePath(dataDir string) string {
	return filepath.Join(dataDir, "tld.db")
}

// Bootstrap creates the local server app. opts overrides host/port with the
// highest priority; falls back to AddrFromEnv() when opts is empty.
func Bootstrap(dataDir string, opts ...ServeOptions) (*App, error) {
	var o ServeOptions
	if len(opts) > 0 {
		o = opts[0]
	}
	dbPath := DatabasePath(dataDir)
	initializedData := false
	if _, err := os.Stat(dbPath); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		initializedData = true
	}
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

	apiStore := store.NewAPIAdapter(sqliteStore)
	views, elements, connectors, err := apiStore.GetWorkspaceResourceCounts(context.Background(), localWorkspaceID)
	if err != nil {
		return nil, err
	}

	srv, err := server.New(sqliteStore, staticFS, localWorkspaceID)
	if err != nil {
		return nil, err
	}

	addr := ResolveAddr(o)

	return &App{
		Addr:            addr,
		DBPath:          dbPath,
		InitializedData: initializedData,
		Resources: ResourceCounts{
			Views:      views,
			Elements:   elements,
			Connectors: connectors,
		},
		Handler: srv.Routes(),
	}, nil
}

// ResolveAddr returns the host:port the server will bind to for the given
// options, applying the same priority as Bootstrap (opts > env > default).
func ResolveAddr(o ServeOptions) string {
	if o.Host == "" && o.Port == "" {
		return AddrFromEnv()
	}
	host := "127.0.0.1"
	port := envOrDefault("PORT", "8060")
	if o.Host != "" {
		host = o.Host
	}
	if o.Port != "" {
		port = o.Port
	}
	return host + ":" + port
}

package localserver

import (
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
	Addr    string
	DBPath  string
	Handler http.Handler
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

// Bootstrap creates the local server app. opts overrides host/port with the
// highest priority; falls back to AddrFromEnv() when opts is empty.
func Bootstrap(cwd string, opts ...ServeOptions) (*App, error) {
	var o ServeOptions
	if len(opts) > 0 {
		o = opts[0]
	}
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

	addr := ResolveAddr(o)

	return &App{
		Addr:    addr,
		DBPath:  dbPath,
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

package localserver

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/internal/server"
	"github.com/mertcikla/tld/v2/internal/store"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/mertcikla/tld/v2/pkg/dbrepo"
)

var localWorkspaceID = uuid.Nil

type App struct {
	Addr            string
	DBPath          string
	DBDriver        string
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
	Host           string
	Port           string
	PublicURL      string
	AllowedOrigins []string
	WorkspaceDir   string
	StaticFS       fs.FS
	Config         *workspace.Config
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
	initializedData := localSQLiteWillBeInitialized(o.Config, dbPath)
	if usesLocalSQLite(o.Config) {
		if _, err := os.Stat(dbPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, err
	}

	staticFS := o.StaticFS
	if staticFS == nil {
		embedded, err := assets.StaticFS()
		if err != nil {
			return nil, err
		}
		staticFS = embedded
	}

	var sqliteStore *store.SQLiteStore
	var err error
	if o.Config != nil {
		sqliteStore, err = store.OpenLocal(context.Background(), o.Config, dataDir, assets.FS)
	} else {
		sqliteStore, err = store.Open(dbPath, assets.FS)
	}
	if err != nil {
		return nil, err
	}
	dbDriver := sqliteStore.Dialect()
	if dbDriver != dbrepo.DialectSQLite {
		dbPath = ""
		initializedData = false
	}

	apiStore := store.NewAPIAdapter(sqliteStore, dataDir)
	views, elements, connectors, err := apiStore.GetWorkspaceResourceCounts(context.Background(), localWorkspaceID)
	if err != nil {
		return nil, err
	}

	publicURL := o.PublicURL
	allowedOrigins := o.AllowedOrigins
	if o.Config != nil {
		if publicURL == "" {
			publicURL = o.Config.Serve.PublicURL
		}
		if len(allowedOrigins) == 0 {
			allowedOrigins = o.Config.Serve.AllowedOrigins
		}
	}
	srv, err := server.NewWithOptions(sqliteStore, staticFS, localWorkspaceID, server.Options{
		DataDir:        dataDir,
		WorkspaceDir:   o.WorkspaceDir,
		PublicURL:      publicURL,
		AllowedOrigins: allowedOrigins,
	})
	if err != nil {
		return nil, err
	}

	addr := ResolveAddr(o)

	return &App{
		Addr:            addr,
		DBPath:          dbPath,
		DBDriver:        string(dbDriver),
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

func localSQLiteWillBeInitialized(cfg *workspace.Config, dbPath string) bool {
	if !usesLocalSQLite(cfg) {
		return false
	}
	_, err := os.Stat(dbPath)
	return errors.Is(err, os.ErrNotExist)
}

func usesLocalSQLite(cfg *workspace.Config) bool {
	if cfg == nil {
		return true
	}
	driver := strings.ToLower(strings.TrimSpace(cfg.Database.Driver))
	return driver == "" || driver == string(dbrepo.DialectSQLite)
}

func DisplayURL(o ServeOptions, addr string) string {
	if publicURL := strings.TrimRight(strings.TrimSpace(o.PublicURL), "/"); publicURL != "" {
		return publicURL
	}
	return "http://" + addr
}

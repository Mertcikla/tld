package apply

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"connectrpc.com/connect"
	"github.com/google/uuid"
	assets "github.com/mertcikla/tld/v2"
	"github.com/mertcikla/tld/v2/internal/client"
	"github.com/mertcikla/tld/v2/internal/localserver"
	"github.com/mertcikla/tld/v2/internal/store"
	"github.com/mertcikla/tld/v2/internal/term"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

const (
	TargetAuto   = "auto"
	TargetLocal  = "local"
	TargetRemote = "remote"

	CloudAppURL = "https://tldiagram.com/app"
)

type Runner interface {
	ApplyWorkspacePlan(context.Context, *diagv1.ApplyPlanRequest) (*diagv1.ApplyPlanResponse, error)
	SupportsDryRun() bool
	Name() string
	TargetLabel() string
}

type remoteRunner struct {
	serverURL string
	apiKey    string
	debug     bool
}

func (r remoteRunner) ApplyWorkspacePlan(ctx context.Context, req *diagv1.ApplyPlanRequest) (*diagv1.ApplyPlanResponse, error) {
	c := client.New(r.serverURL, r.apiKey, r.debug)
	resp, err := c.ApplyWorkspacePlan(ctx, connect.NewRequest(req))
	if err != nil {
		return nil, err
	}
	return resp.Msg, nil
}

func (r remoteRunner) UpdateViewName(ctx context.Context, viewID int32, name string) (*diagv1.View, error) {
	c := client.New(r.serverURL, r.apiKey, r.debug)
	resp, err := c.UpdateView(ctx, connect.NewRequest(&diagv1.UpdateViewRequest{
		ViewId: viewID,
		Name:   name,
	}))
	if err != nil {
		return nil, err
	}
	return resp.Msg.GetView(), nil
}

func (r remoteRunner) SupportsDryRun() bool { return true }
func (r remoteRunner) Name() string         { return TargetRemote }
func (r remoteRunner) TargetLabel() string  { return client.NormalizeURL(r.serverURL) }

type localRunner struct {
	dbPath       string
	dataDir      string
	previousMeta *workspace.Meta
}

func (r localRunner) ApplyWorkspacePlan(ctx context.Context, req *diagv1.ApplyPlanRequest) (*diagv1.ApplyPlanResponse, error) {
	if err := os.MkdirAll(filepath.Dir(r.dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	sqliteStore, err := store.Open(r.dbPath, assets.FS)
	if err != nil {
		return nil, err
	}
	defer func() { _ = sqliteStore.Legacy().Close() }()
	adapter := store.NewAPIAdapter(sqliteStore)
	if _, err := sqliteStore.DB().ExecContext(ctx, `BEGIN IMMEDIATE`); err != nil {
		return nil, err
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = sqliteStore.DB().ExecContext(context.Background(), `ROLLBACK`)
		}
	}()
	resp, err := adapter.ApplyPlan(ctx, uuid.Nil, req)
	if err != nil {
		return nil, err
	}
	if err := adapter.PruneMissingCLIResources(ctx, uuid.Nil, r.previousMeta, req); err != nil {
		return nil, err
	}
	if _, err := sqliteStore.DB().ExecContext(ctx, `COMMIT`); err != nil {
		return nil, err
	}
	committed = true
	return resp, nil
}

func (r localRunner) UpdateViewName(ctx context.Context, viewID int32, name string) (*diagv1.View, error) {
	sqliteStore, err := store.Open(r.dbPath, assets.FS)
	if err != nil {
		return nil, err
	}
	defer func() { _ = sqliteStore.Legacy().Close() }()
	adapter := store.NewAPIAdapter(sqliteStore)
	existing, err := adapter.GetView(ctx, viewID, uuid.Nil)
	if err != nil {
		return nil, err
	}
	return adapter.UpdateView(ctx, viewID, uuid.Nil, name, existing.LevelLabel)
}

func (r localRunner) SupportsDryRun() bool { return false }
func (r localRunner) Name() string         { return TargetLocal }
func (r localRunner) TargetLabel() string  { return r.dbPath }
func (r localRunner) DataDir() string      { return r.dataDir }

func NewRunner(cfg workspace.Config, targetOverride, dataDirFlag string, debug bool, previousMeta *workspace.Meta) (Runner, error) {
	target, err := ResolveTarget(cfg, targetOverride)
	if err != nil {
		return nil, err
	}
	switch target {
	case TargetRemote:
		return remoteRunner{serverURL: cfg.ServerURL, apiKey: cfg.APIKey, debug: debug}, nil
	case TargetLocal:
		dataDir, err := workspace.ResolveDataDir(&cfg, dataDirFlag)
		if err != nil {
			return nil, err
		}
		return localRunner{dbPath: localserver.DatabasePath(dataDir), dataDir: dataDir, previousMeta: previousMeta}, nil
	default:
		return nil, fmt.Errorf("unknown apply target %q", target)
	}
}

func ResolveTarget(cfg workspace.Config, targetOverride string) (string, error) {
	target := strings.ToLower(strings.TrimSpace(targetOverride))
	if target == "" {
		target = strings.ToLower(strings.TrimSpace(cfg.Apply.Target))
	}
	if target == "" {
		target = TargetAuto
	}
	switch target {
	case TargetAuto:
		if strings.TrimSpace(cfg.APIKey) != "" && strings.TrimSpace(cfg.WorkspaceID) != "" {
			return TargetRemote, nil
		}
		return TargetLocal, nil
	case "cloud":
		return TargetRemote, nil
	case TargetLocal, TargetRemote:
		return target, nil
	default:
		return "", fmt.Errorf("apply target must be auto, local, cloud, or remote")
	}
}

func TargetDisplayName(target string) string {
	if target == TargetRemote {
		return "cloud"
	}
	return target
}

func RenderTargetInfo(out io.Writer, runner Runner) {
	term.Label(out, 20, "Target", TargetDisplayName(runner.Name()))
	switch runner.Name() {
	case TargetRemote:
		term.Label(out, 20, "Cloud API", term.URL(out, runner.TargetLabel()))
	case TargetLocal:
		term.Label(out, 20, "Local DB", term.Path(out, runner.TargetLabel()))
	default:
		term.Label(out, 20, "Target detail", runner.TargetLabel())
	}
}

func RenderPostApplyLocation(out io.Writer, runner Runner) {
	switch r := runner.(type) {
	case localRunner:
		if proc, ok := runningLocalServer(r.dataDir); ok {
			addr := proc.Addr
			if addr == "" {
				addr = localserver.AddrFromEnv()
			}
			term.Label(out, 20, "View at", term.URL(out, "http://"+addr))
			return
		}
		term.Label(out, 20, "Start app", serveCommand(r.dataDir))
	case remoteRunner:
		term.Label(out, 20, "View at", term.URL(out, CloudAppURL))
	}
}

func runningLocalServer(dataDir string) (localserver.ProcessRecord, bool) {
	reg, err := localserver.PruneProcessRegistry()
	if err != nil {
		return localserver.ProcessRecord{}, false
	}
	for _, proc := range reg.Processes {
		if proc.Kind == localserver.ProcessKindServer && proc.DataDir == dataDir {
			return proc, true
		}
	}
	return localserver.ProcessRecord{}, false
}

func serveCommand(dataDir string) string {
	if defaultDataDir, err := workspace.DataDir(); err == nil && defaultDataDir == dataDir {
		return "tld serve"
	}
	return "tld serve --data-dir " + shellQuote(dataDir)
}

var shellSafePattern = regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]+$`)

func shellQuote(value string) string {
	if shellSafePattern.MatchString(value) {
		return value
	}
	return strconv.Quote(value)
}

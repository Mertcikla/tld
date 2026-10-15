package cmdutil

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/mertcikla/tld/v2/internal/exec"
	"github.com/mertcikla/tld/v2/internal/localserver"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/spf13/cobra"
)

// Mode describes where a command session reads and writes its data.
type Mode int

const (
	// ModeDB works directly against the configured target (local DB or cloud)
	// and never touches workspace YAML files.
	ModeDB Mode = iota
	// ModeWorkspace keeps the historic behavior: YAML is loaded for refs and
	// refreshed as a cache after target writes.
	ModeWorkspace
)

// Session resolves the data source for one command invocation. In DB mode it
// materializes workspace state from the target export; in workspace mode it
// wraps the local YAML files.
type Session struct {
	Wdir   string
	Mode   Mode
	Config workspace.Config

	command *cobra.Command
	target  string
	dataDir string
	ctx     context.Context

	ws       *workspace.Workspace
	runner   exec.Runner
	readLive bool
}

// OpenSession decides between DB mode and workspace mode for a command.
// Workspace mode is used when --yaml is passed or when wdir holds workspace
// marker files; otherwise the target store (local DB, or cloud when configured)
// is the only data source.
func OpenSession(cmd *cobra.Command, wdir, target, dataDir string) (*Session, error) {
	target, dataDir = TargetOverrides(cmd, target, dataDir)
	ctx := context.Background()
	if cmd != nil {
		ctx = cmd.Context()
	}
	s := &Session{
		Wdir:    wdir,
		command: cmd,
		target:  target,
		dataDir: dataDir,
		ctx:     ctx,
	}
	if WorkspaceConfigured(cmd, wdir) {
		ws, err := workspace.Load(wdir)
		if err != nil {
			return nil, WithHint(fmt.Errorf("load workspace: %w", err), "Fix the workspace YAML or omit --yaml/--workspace to work on the local database directly.")
		}
		s.ws = ws
		s.Config = ws.Config
		s.Mode = ModeWorkspace
		source, err := ReadSource(cmd, true)
		if err != nil {
			return nil, err
		}
		s.readLive = source == "live"
		return s, nil
	}
	cfg, err := workspace.LoadGlobalConfig()
	if err != nil {
		return nil, fmt.Errorf("load global config: %w", err)
	}
	s.Config = *cfg
	s.Mode = ModeDB
	if _, err := ReadSource(cmd, false); err != nil {
		return nil, err
	}
	return s, nil
}

// WorkspaceConfigured reports whether the invocation should use YAML files:
// --yaml forces workspace mode, otherwise workspace marker files must exist.
func WorkspaceConfigured(cmd *cobra.Command, wdir string) bool {
	if yamlFlagEnabled(cmd) {
		return true
	}
	return workspace.IsWorkspaceDir(workspace.ResolveDir(wdir))
}

func yamlFlagEnabled(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	flags := cmd.Flags()
	if flags == nil || flags.Lookup("yaml") == nil {
		return false
	}
	enabled, err := flags.GetBool("yaml")
	return err == nil && enabled
}

// HasWorkspace reports whether YAML cache reads and writes are in play.
func (s *Session) HasWorkspace() bool { return s.Mode == ModeWorkspace }

// Context derives a command context with the session's cache policy applied.
// DB-mode contexts disable exec cache writes.
func (s *Session) Context(base context.Context) context.Context {
	if base == nil {
		base = s.ctx
	}
	return exec.WithCache(base, s.HasWorkspace())
}

// Runner lazily opens the target runner. Callers should Close the session.
func (s *Session) Runner() (exec.Runner, error) {
	if s.runner != nil {
		return s.runner, nil
	}
	runner, err := exec.NewRunner(s.Config, s.target, s.dataDir, false)
	if err != nil {
		return nil, err
	}
	if runner.Name() == exec.TargetRemote {
		if err := EnsureAPIKey(s.Config.APIKey); err != nil {
			return nil, err
		}
	}
	s.runner = runner
	return s.runner, nil
}

// Close releases the target runner, if one was opened.
func (s *Session) Close() error {
	if s.runner != nil {
		return s.runner.Close()
	}
	return nil
}

// CacheError pins recovery to the same destination as the committed write.
func (s *Session) CacheError(err error) error {
	detail := CacheError(err).(*CommandError)
	target, targetErr := exec.ResolveTarget(s.Config, s.target)
	if targetErr != nil {
		return detail
	}
	hint := fmt.Sprintf("Run 'tld pull --target %s", target)
	if target == exec.TargetLocal && s.dataDir != "" {
		hint += fmt.Sprintf(" --data-dir %q", s.dataDir)
	}
	if s.Wdir != "" {
		hint += fmt.Sprintf(" --workspace %q", s.Wdir)
	}
	detail.Hint = hint + "' to refresh the workspace before continuing."
	return detail
}

// LoadWorkspace returns the workspace state for this session. In workspace
// mode it is the YAML files; in DB mode it is a snapshot materialized from the
// target export (refs are synthesized from names).
func (s *Session) LoadWorkspace() (*workspace.Workspace, error) {
	if s.readLive {
		runner, err := s.Runner()
		if err != nil {
			return nil, err
		}
		exported, err := runner.ExportWorkspace(s.ctx)
		if err != nil {
			return nil, fmt.Errorf("read target workspace: %w", err)
		}
		return ConvertExportResponse(s.ws, exported), nil
	}
	if s.ws != nil {
		return s.ws, nil
	}
	if s.Mode == ModeWorkspace {
		ws, err := workspace.Load(s.Wdir)
		if err != nil {
			return nil, WithHint(fmt.Errorf("load workspace: %w", err), "Fix the workspace YAML or omit --yaml/--workspace to work on the local database directly.")
		}
		s.ws = ws
		return ws, nil
	}
	runner, err := s.Runner()
	if err != nil {
		return nil, err
	}
	export, err := runner.ExportWorkspace(s.ctx)
	if err != nil {
		return nil, fmt.Errorf("read %s workspace: %w", exec.TargetDisplayName(runner.Name()), err)
	}
	s.ws = ConvertExportResponse(emptyWorkspace(s.Wdir, s.Config), export)
	return s.ws, nil
}

// TargetOverrides supports inherited root flags as well as legacy command flags.
func TargetOverrides(cmd *cobra.Command, target, dataDir string) (string, string) {
	value := func(name, supplied string) string {
		if supplied != "" || cmd == nil {
			return supplied
		}
		if f := cmd.Flags().Lookup(name); f != nil && f.Value.String() != "" {
			return f.Value.String()
		}
		if f := cmd.Root().PersistentFlags().Lookup(name); f != nil {
			return f.Value.String()
		}
		return ""
	}
	return value("target", target), value("data-dir", dataDir)
}

func AddReadSourceFlag(cmd *cobra.Command) {
	cmd.Flags().String("source", "auto", "read auto, cache, or live; auto uses live with an explicit target/data-dir, otherwise workspace cache")
}

func ReadSource(cmd *cobra.Command, hasWorkspace bool) (string, error) {
	if cmd == nil || cmd.Flags().Lookup("source") == nil {
		if hasWorkspace {
			return "cache", nil
		}
		return "live", nil
	}
	source, _ := cmd.Flags().GetString("source")
	switch source {
	case "live":
		return source, nil
	case "cache":
		if !hasWorkspace {
			return "", WithCode(errors.New("no workspace cache to read"), "workspace_required", "Run 'tld init' or use --source live.")
		}
		return source, nil
	case "auto":
		if !hasWorkspace {
			return "live", nil
		}
		for _, name := range []string{"target", "data-dir"} {
			if cmd.Flags().Changed(name) || cmd.Root().PersistentFlags().Changed(name) {
				return "live", nil
			}
		}
		return "cache", nil
	default:
		return "", WithCode(fmt.Errorf("invalid --source %q", source), "invalid_value", "Use --source auto, cache, or live.")
	}
}

// Reload drops any cached materialized workspace so the next LoadWorkspace
// reflects target writes. In workspace mode the YAML files are re-read.
func (s *Session) Reload() (*workspace.Workspace, error) {
	s.ws = nil
	return s.LoadWorkspace()
}

// LocalWorkspace materializes a workspace snapshot from the local DB without
// touching the remote target or creating any files. It is safe for completion
// helpers: a missing database yields an empty workspace.
func LocalWorkspace(wdir string) (*workspace.Workspace, error) {
	cfg, err := workspace.LoadGlobalConfig()
	if err != nil {
		return nil, err
	}
	dataDir, err := workspace.ResolveDataDir(cfg, "")
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(localserver.DatabasePath(dataDir)); err != nil {
		return emptyWorkspace(wdir, *cfg), nil
	}
	runner, err := exec.NewRunner(*cfg, exec.TargetLocal, "", false)
	if err != nil {
		return nil, err
	}
	defer func() { _ = runner.Close() }()
	export, err := runner.ExportWorkspace(context.Background())
	if err != nil {
		return nil, err
	}
	return ConvertExportResponse(emptyWorkspace(wdir, *cfg), export), nil
}

func emptyWorkspace(wdir string, cfg workspace.Config) *workspace.Workspace {
	return &workspace.Workspace{
		Dir:        wdir,
		Config:     cfg,
		Elements:   map[string]*workspace.Element{},
		Connectors: map[string]*workspace.Connector{},
		Meta: &workspace.Meta{
			Elements:   map[string]*workspace.ResourceMetadata{},
			Views:      map[string]*workspace.ResourceMetadata{},
			Connectors: map[string]*workspace.ResourceMetadata{},
		},
	}
}

// ResolveElementArg maps a user-supplied element argument (workspace ref,
// element name, or numeric ID) to the ref used by the materialized workspace.
func ResolveElementArg(ws *workspace.Workspace, arg string) (string, error) {
	arg = strings.TrimSpace(arg)
	if ws == nil {
		return "", errors.New("workspace is required")
	}
	if arg == "" {
		return "", errors.New("element reference is required")
	}
	if el, ok := ws.Elements[arg]; ok && el != nil {
		return arg, nil
	}
	if id, err := strconv.Atoi(arg); err == nil && id > 0 {
		if ref := refForResourceID(ws, id); ref != "" {
			return ref, nil
		}
	}
	var exact, folded []string
	for ref, el := range ws.Elements {
		if el == nil {
			continue
		}
		if el.Name == arg {
			exact = append(exact, ref)
		}
		if strings.EqualFold(strings.TrimSpace(el.Name), arg) {
			folded = append(folded, ref)
		}
	}
	if len(exact) == 1 {
		return exact[0], nil
	}
	if len(exact) > 1 {
		folded = exact
	}
	if len(folded) == 1 {
		return folded[0], nil
	}
	if len(folded) > 1 {
		sort.Strings(folded)
		return "", fmt.Errorf("element %q is ambiguous: %s", arg, strings.Join(folded, ", "))
	}
	return "", fmt.Errorf("element %q not found", arg)
}

// ResolveViewArg maps a user-supplied view argument (view ref, view/owner
// name, numeric view ID, or "root") to the owning element ref, or "root".
func ResolveViewArg(ws *workspace.Workspace, arg string) (string, error) {
	arg = strings.TrimSpace(arg)
	if arg == "" || arg == workspace.RootRef {
		return workspace.RootRef, nil
	}
	if ws != nil && ws.Meta != nil {
		if id, err := strconv.Atoi(arg); err == nil && id > 0 {
			for ref, m := range ws.Meta.Views {
				if m != nil && int(m.ID) == id {
					return ref, nil
				}
			}
		}
	}
	ref, err := ResolveElementArg(ws, arg)
	if err != nil {
		return "", err
	}
	return ref, nil
}

// ResolveConnectorArg maps a user-supplied connector argument (connector key
// or numeric connector ID) to the key used by the materialized workspace.
func ResolveConnectorArg(ws *workspace.Workspace, arg string) (string, error) {
	arg = strings.TrimSpace(arg)
	if ws == nil {
		return "", errors.New("workspace is required")
	}
	if arg == "" {
		return "", errors.New("connector reference is required")
	}
	if _, ok := ws.Connectors[arg]; ok {
		return arg, nil
	}
	if normalized := workspace.NormalizeConnectorKey(arg); normalized != "" {
		if _, ok := ws.Connectors[normalized]; ok {
			return normalized, nil
		}
	}
	if id, err := strconv.Atoi(arg); err == nil && id > 0 {
		if ws.Meta != nil {
			for key, m := range ws.Meta.Connectors {
				if m != nil && int(m.ID) == id {
					return key, nil
				}
			}
		}
	}
	return "", fmt.Errorf("connector %q not found", arg)
}

func refForResourceID(ws *workspace.Workspace, id int) string {
	if ws == nil || ws.Meta == nil {
		return ""
	}
	if ref := refForMetadataID(ws.Meta.Elements, id); ref != "" {
		return ref
	}
	return refForMetadataID(ws.Meta.Views, id)
}

func refForMetadataID(metadata map[string]*workspace.ResourceMetadata, id int) string {
	for ref, m := range metadata {
		if m != nil && int(m.ID) == id {
			return ref
		}
	}
	return ""
}

// RefreshCache includes server-created relationships (for example those copied
// when a shared element is placed) so listed selectors work on the next write.
func (s *Session) RefreshCache(base *workspace.Workspace) error {
	if !s.HasWorkspace() {
		return nil
	}
	runner, err := s.Runner()
	if err != nil {
		return err
	}
	exported, err := runner.ExportWorkspace(s.ctx)
	if err != nil {
		return err
	}
	actual := ConvertExportResponse(base, exported)
	PreservePendingResources(base, actual)
	if err := exec.SaveCache(s.Wdir, actual); err != nil {
		return err
	}
	s.ws = actual
	return nil
}

// RequireAppliedCache protects unapplied YAML edits before a mutation replaces
// the persisted cache with a fresh target snapshot. Sync intentionally bypasses
// this check because it is the operation that applies those edits.
func (s *Session) RequireAppliedCache() error {
	if !s.HasWorkspace() {
		return nil
	}
	return EnsureCacheInSync(s.Wdir)
}

func EnsureCacheInSync(dir string) error {
	lock, err := workspace.LoadLockFile(dir)
	if err != nil {
		return fmt.Errorf("load lock file: %w", err)
	}
	if lock == nil || strings.TrimSpace(lock.WorkspaceHash) == "" {
		return nil
	}
	hash, err := workspace.CalculateWorkspaceHash(dir)
	if err != nil {
		return fmt.Errorf("calculate workspace hash: %w", err)
	}
	if hash != lock.WorkspaceHash {
		return WithCode(errors.New("workspace YAML is out of sync with the last applied state"), "cache_out_of_sync", "Run 'tld sync' to apply local YAML edits, or review 'tld pull --dry-run' before refreshing from the target. Use the same workspace, target, and data directory.")
	}
	return nil
}

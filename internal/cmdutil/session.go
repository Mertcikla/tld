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

	ws     *workspace.Workspace
	runner exec.Runner
}

// OpenSession decides between DB mode and workspace mode for a command.
// Workspace mode is used when --yaml is passed or when wdir holds workspace
// marker files; otherwise the target store (local DB, or cloud when configured)
// is the only data source.
func OpenSession(cmd *cobra.Command, wdir, target, dataDir string) (*Session, error) {
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
		return s, nil
	}
	cfg, err := workspace.LoadGlobalConfig()
	if err != nil {
		return nil, fmt.Errorf("load global config: %w", err)
	}
	s.Config = *cfg
	s.Mode = ModeDB
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

// LoadWorkspace returns the workspace state for this session. In workspace
// mode it is the YAML files; in DB mode it is a snapshot materialized from the
// target export (refs are synthesized from names).
func (s *Session) LoadWorkspace() (*workspace.Workspace, error) {
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
	exact := ""
	var folded []string
	for ref, el := range ws.Elements {
		if el == nil {
			continue
		}
		if el.Name == arg {
			exact = ref
			break
		}
		if strings.EqualFold(strings.TrimSpace(el.Name), arg) {
			folded = append(folded, ref)
		}
	}
	if exact != "" {
		return exact, nil
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

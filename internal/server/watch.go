package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"buf.build/gen/go/tldiagramcom/diagram/connectrpc/go/codeindex/v1/codeindexv1connect"
	codeindexv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"connectrpc.com/connect"
	cstore "github.com/mertcikla/tld/v2/internal/codeindex/store"
	"github.com/mertcikla/tld/v2/internal/store"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

// watchChild is a CLI watcher process supervised by the server.
type watchChild struct {
	repositoryID string
	repoRoot     string
	pid          int
	cancel       context.CancelFunc
	done         chan struct{}
}

func (c *watchChild) running() bool {
	select {
	case <-c.done:
		return false
	default:
		return true
	}
}

// watchManager spawns and tracks `tld index <root> --watch` child processes.
// Status is read from the shared codeindex watch record, so externally started
// CLI watchers are visible to the same control surface.
type watchManager struct {
	mu         sync.Mutex
	children   map[string]*watchChild
	dataDir    string
	idx        *cstore.Store
	executable func() (string, error)
}

func newWatchManager(dataDir string, idx *cstore.Store) *watchManager {
	return &watchManager{
		children:   map[string]*watchChild{},
		dataDir:    dataDir,
		idx:        idx,
		executable: os.Executable,
	}
}

// resolveCLI finds a `tld` executable. The Wails desktop binary cannot run the
// index command itself, so it falls back to an installed CLI on PATH.
func (m *watchManager) resolveCLI() (string, error) {
	if path, err := exec.LookPath(cliBinaryName()); err == nil {
		return path, nil
	}
	if exe, err := m.executable(); err == nil && filepath.Base(exe) == cliBinaryName() {
		return exe, nil
	}
	return "", fmt.Errorf("the tld CLI is not installed; install it to run a watcher")
}

func (m *watchManager) cliAvailable() bool {
	_, err := m.resolveCLI()
	return err == nil
}

// start launches a detached child and waits for its control record to appear.
// It reserves the repository slot under the manager lock so concurrent starts
// cannot both spawn, reaps dead children first, and refuses when a live
// watcher (managed or external) already owns the repository.
func (m *watchManager) start(ctx context.Context, st cstore.WatchState, embed, materialize bool) (cstore.WatchState, error) {
	exe, err := m.resolveCLI()
	if err != nil {
		return cstore.WatchState{}, err
	}
	m.reap()
	m.mu.Lock()
	if existing, ok := m.children[st.RepositoryID]; ok && existing.running() {
		m.mu.Unlock()
		return cstore.WatchState{}, fmt.Errorf("a watcher is already running for this repository (pid %d)", existing.pid)
	}
	// Reserve the slot so a concurrent start cannot slip through before the
	// child publishes its claim.
	reservation := &watchChild{repositoryID: st.RepositoryID, repoRoot: st.RepoRoot, done: make(chan struct{})}
	m.children[st.RepositoryID] = reservation
	m.mu.Unlock()

	release := func() {
		m.mu.Lock()
		if m.children[st.RepositoryID] == reservation {
			delete(m.children, st.RepositoryID)
		}
		m.mu.Unlock()
	}

	// A live watcher not managed by us must not be duplicated.
	if existing, ok, err := m.idx.WatchState(ctx, st.RepositoryID); err == nil && ok && existing.Live(time.Now()) {
		release()
		return cstore.WatchState{}, fmt.Errorf("a watcher is already running for this repository (pid %d)", existing.OwnerPID)
	}

	args := []string{"index", st.RepoRoot, "--watch", "--watch-owner", "server"}
	if m.dataDir != "" {
		args = append(args, "--data-dir", m.dataDir)
	}
	if !embed {
		args = append(args, "--embed=false")
	}
	if materialize {
		args = append(args, "--materialize")
	}
	childCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	cmd := exec.CommandContext(childCtx, exe, args...)
	cmd.Dir = st.RepoRoot
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.SysProcAttr = getSysProcAttr()
	if err := cmd.Start(); err != nil {
		cancel()
		release()
		return cstore.WatchState{}, fmt.Errorf("start watcher: %w", err)
	}
	child := &watchChild{repositoryID: st.RepositoryID, repoRoot: st.RepoRoot, pid: cmd.Process.Pid, cancel: cancel, done: make(chan struct{})}
	m.mu.Lock()
	m.children[st.RepositoryID] = child
	m.mu.Unlock()
	go func() {
		_ = cmd.Wait()
		close(child.done)
	}()

	// Wait for the child to publish its own live control record so the returned
	// status reflects the running watcher rather than our seed.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-child.done:
			release()
			return cstore.WatchState{}, fmt.Errorf("watcher exited immediately; run `tld index %s --watch` to see why", st.RepoRoot)
		case <-time.After(50 * time.Millisecond):
		}
		if current, ok, err := m.idx.WatchState(ctx, st.RepositoryID); err == nil && ok && current.Live(time.Now()) && current.OwnerPID == child.pid {
			return current, nil
		}
	}
	// The child is alive but never claimed a live record (e.g. it lost a race).
	release()
	cancel()
	return cstore.WatchState{}, fmt.Errorf("watcher did not claim the repository; it may be indexing under another owner")
}

// reap drops entries whose process has exited.
func (m *watchManager) reap() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, child := range m.children {
		if !child.running() {
			delete(m.children, id)
		}
	}
}

// stop terminates a managed child and cooperatively stops any external watcher.
// When an external process owns the record, it is signalled directly once it
// has had a chance to honor the cooperative stop, so a non-cooperative or
// legacy watcher can never stay stuck.
func (m *watchManager) stop(ctx context.Context, repositoryID string) {
	m.reap()
	m.mu.Lock()
	child, managed := m.children[repositoryID]
	if managed {
		delete(m.children, repositoryID)
	}
	m.mu.Unlock()

	if managed {
		if child.cancel != nil {
			child.cancel()
		}
		if m.waitChild(child, 5*time.Second) {
			return
		}
	}

	// External or unresponsive watcher: give it a moment to observe the stop
	// flag, then terminate the recorded process and clear the row.
	st, ok, err := m.idx.WatchState(ctx, repositoryID)
	if err != nil || !ok {
		return
	}
	if st.Live(time.Now()) && st.OwnerPID > 0 {
		deadline := time.Now().Add(cstore.WatchStopDeadline)
		for time.Now().Before(deadline) {
			if current, ok, err := m.idx.WatchState(ctx, repositoryID); err != nil || !ok || !current.Live(time.Now()) {
				break
			}
			time.Sleep(200 * time.Millisecond)
		}
		if current, ok, err := m.idx.WatchState(ctx, repositoryID); err == nil && ok && current.Live(time.Now()) && current.OwnerPID > 0 {
			if proc, err := os.FindProcess(current.OwnerPID); err == nil {
				_ = proc.Kill()
			}
		}
	}
	_ = m.idx.ForceClearWatchState(ctx, repositoryID)
}

func (m *watchManager) waitChild(child *watchChild, timeout time.Duration) bool {
	select {
	case <-child.done:
		return true
	case <-time.After(timeout):
	}
	if child.pid > 0 {
		if proc, err := os.FindProcess(child.pid); err == nil {
			_ = proc.Kill()
		}
	}
	select {
	case <-child.done:
		return true
	case <-time.After(time.Second):
		return false
	}
}

func (m *watchManager) managed(repositoryID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	child, ok := m.children[repositoryID]
	return ok && child.running()
}

// Close stops every managed watcher during server shutdown.
func (m *watchManager) Close() error {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	children := make([]*watchChild, 0, len(m.children))
	for _, child := range m.children {
		children = append(children, child)
	}
	m.children = map[string]*watchChild{}
	m.mu.Unlock()
	for _, child := range children {
		if child.cancel != nil {
			child.cancel()
		}
	}
	for _, child := range children {
		m.waitChild(child, 5*time.Second)
	}
	return nil
}

// watchService exposes the manager over ConnectRPC.
type watchService struct {
	codeindexv1connect.UnimplementedWatchServiceHandler
	idx     *cstore.Store
	manager *watchManager
}

func registerWatchHandlers(mux *http.ServeMux, sqliteStore *store.SQLiteStore, dataDir string, configs ...*workspace.Config) *watchManager {
	idx := cstore.NewStore(sqliteStore.DB(), sqliteStore.BunDB(), sqliteStore.Dialect())
	manager := newWatchManager(dataDir, idx)
	svc := &watchService{idx: idx, manager: manager}
	path, handler := codeindexv1connect.NewWatchServiceHandler(svc)
	mux.Handle("/api"+path, http.StripPrefix("/api", handler))
	return manager
}

func (s *watchService) StartWatch(ctx context.Context, req *connect.Request[codeindexv1.StartWatchRequest]) (*connect.Response[codeindexv1.WatchStatus], error) {
	repositoryID := strings.TrimSpace(req.Msg.GetRepositoryId())
	if repositoryID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("repository_id is required"))
	}
	repo, err := s.idx.Repository(ctx, repositoryID)
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	if strings.TrimSpace(repo.Root) == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("repository has no local path"))
	}
	// Reset any stale record so a crashed watcher cannot block a fresh start.
	if existing, ok, err := s.idx.WatchState(ctx, repositoryID); err == nil && ok && !existing.Live(time.Now()) {
		_ = s.idx.ForceClearWatchState(ctx, repositoryID)
	}
	st := cstore.WatchState{RepositoryID: repositoryID, RepoRoot: repo.Root, OwnerKind: "server", State: "starting", StartedUnix: time.Now().Unix()}
	if _, err := s.manager.start(ctx, st, req.Msg.GetEmbed(), req.Msg.GetMaterialize()); err != nil {
		return nil, connect.NewError(connect.CodeAlreadyExists, err)
	}
	return connect.NewResponse(s.status(ctx, repositoryID)), nil
}

func (s *watchService) StopWatch(ctx context.Context, req *connect.Request[codeindexv1.StopWatchRequest]) (*connect.Response[codeindexv1.WatchStatus], error) {
	repositoryID := strings.TrimSpace(req.Msg.GetRepositoryId())
	if repositoryID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("repository_id is required"))
	}
	if err := s.idx.RequestWatchStop(ctx, repositoryID); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	s.manager.stop(ctx, repositoryID)
	return connect.NewResponse(s.status(ctx, repositoryID)), nil
}

func (s *watchService) GetWatchStatus(ctx context.Context, req *connect.Request[codeindexv1.GetWatchStatusRequest]) (*connect.Response[codeindexv1.WatchStatus], error) {
	repositoryID := strings.TrimSpace(req.Msg.GetRepositoryId())
	if repositoryID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("repository_id is required"))
	}
	return connect.NewResponse(s.status(ctx, repositoryID)), nil
}

func (s *watchService) ListWatches(ctx context.Context, _ *connect.Request[codeindexv1.ListWatchesRequest]) (*connect.Response[codeindexv1.ListWatchesResponse], error) {
	s.manager.reap()
	_ = s.idx.ReapStaleWatchStates(ctx)
	states, err := s.idx.ListWatchStates(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	out := &codeindexv1.ListWatchesResponse{}
	for _, st := range states {
		out.Watchers = append(out.Watchers, s.statusFor(st, true))
	}
	return connect.NewResponse(out), nil
}

func (s *watchService) status(ctx context.Context, repositoryID string) *codeindexv1.WatchStatus {
	st, ok, err := s.idx.WatchState(ctx, repositoryID)
	if err != nil {
		ok = false
	}
	if ok && !st.Live(time.Now()) {
		_ = s.idx.ForceClearWatchState(ctx, repositoryID)
		st.State = "stopped"
		st.StopRequested = false
	}
	return s.statusFor(st, ok)
}

func (s *watchService) statusFor(st cstore.WatchState, found bool) *codeindexv1.WatchStatus {
	return buildWatchStatus(st, found, s.manager.managed(st.RepositoryID), s.manager.cliAvailable())
}

func buildWatchStatus(st cstore.WatchState, found, managed, cliAvailable bool) *codeindexv1.WatchStatus {
	live := found && st.Live(time.Now())
	status := &codeindexv1.WatchStatus{
		RepositoryId:       st.RepositoryID,
		Running:            live,
		Managed:            managed,
		State:              watchStateLabel(st, found),
		Stage:              st.Stage,
		OwnerKind:          st.OwnerKind,
		OwnerPid:           int64(st.OwnerPID),
		RepoRoot:           st.RepoRoot,
		GitBranch:          st.GitBranch,
		GitRevision:        st.GitRevision,
		SnapshotId:         st.SnapshotID,
		ContentFingerprint: st.ContentFingerprint,
		ChangedFiles:       uint32(st.ChangedFiles),
		PendingFiles:       uint32(st.PendingFiles),
		StartedUnix:        st.StartedUnix,
		LastScanUnix:       st.LastScanUnix,
		LastScanMs:         st.LastScanMS,
		HeartbeatUnix:      st.HeartbeatUnix,
		StopRequested:      st.StopRequested && live,
		PollIntervalMs:     st.PollIntervalMS,
		DebounceMs:         st.DebounceMS,
		Error:              st.Error,
		CliAvailable:       cliAvailable,
	}
	if !cliAvailable {
		status.InstallHint = "Install the tld CLI to start a watcher from the app."
	}
	return status
}

func watchStateLabel(st cstore.WatchState, found bool) string {
	if !found || !st.Live(time.Now()) {
		return "stopped"
	}
	if st.StopRequested {
		return "stopping"
	}
	if st.State == "" {
		return "running"
	}
	return st.State
}

func cliBinaryName() string {
	if os.PathSeparator == '\\' {
		return "tld.exe"
	}
	return "tld"
}

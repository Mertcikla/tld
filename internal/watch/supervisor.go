package watch

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// WatcherProcess describes a watch child process managed by the Supervisor.
type WatcherProcess struct {
	RepositoryID int64  `json:"repository_id"`
	RepoRoot     string `json:"repo_root"`
	PID          int    `json:"pid"`
	StartedAt    string `json:"started_at"`
	Command      string `json:"command"`
	executable   string
	args         []string
	cancel       context.CancelFunc
	done         chan error
	err          error
}

// Snapshot returns a copy safe for JSON encoding.
func (p WatcherProcess) Snapshot() WatcherProcess {
	return WatcherProcess{
		RepositoryID: p.RepositoryID,
		RepoRoot:     p.RepoRoot,
		PID:          p.PID,
		StartedAt:    p.StartedAt,
		Command:      p.Command,
	}
}

// Supervisor launches and tracks `tld watch` child processes started from the
// local server. It is safe for concurrent use.
type Supervisor struct {
	mu        sync.Mutex
	processes map[int64]*WatcherProcess
	// executable resolves the tld binary used to spawn watchers. Defaults to
	// os.Executable(). Overridable for tests.
	executable func() (string, error)
}

func NewSupervisor() *Supervisor {
	return &Supervisor{
		processes:  map[int64]*WatcherProcess{},
		executable: os.Executable,
	}
}

// StartOptions configures a spawned watcher.
type StartOptions struct {
	RepositoryID int64
	RepoRoot     string
	DataDir      string
	// Args are extra CLI flags appended after "watch <path> --data-dir <dir>".
	Args []string
}

// Start launches a detached `tld watch <repoRoot> --data-dir <dir> --no-serve`
// process. It returns an error if a watcher is already running for the repo.
func (s *Supervisor) Start(ctx context.Context, opts StartOptions) (WatcherProcess, error) {
	if s == nil {
		return WatcherProcess{}, fmt.Errorf("watch supervisor is unavailable")
	}
	if opts.RepositoryID <= 0 {
		return WatcherProcess{}, fmt.Errorf("repository id is required")
	}
	if strings.TrimSpace(opts.RepoRoot) == "" {
		return WatcherProcess{}, fmt.Errorf("repository path is required")
	}
	executable, err := s.executable()
	if err != nil {
		return WatcherProcess{}, fmt.Errorf("resolve tld executable: %w", err)
	}

	s.mu.Lock()
	if existing, ok := s.processes[opts.RepositoryID]; ok && existing.running() {
		s.mu.Unlock()
		return WatcherProcess{}, fmt.Errorf("watch is already running for this repository (pid %d)", existing.PID)
	}
	s.mu.Unlock()

	args := []string{"watch", opts.RepoRoot, "--no-serve"}
	if strings.TrimSpace(opts.DataDir) != "" {
		args = append(args, "--data-dir", opts.DataDir)
	}
	args = append(args, opts.Args...)

	childCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	cmd := exec.CommandContext(childCtx, executable, args...)
	cmd.Dir = opts.RepoRoot
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		cancel()
		return WatcherProcess{}, fmt.Errorf("start watcher: %w", err)
	}

	process := &WatcherProcess{
		RepositoryID: opts.RepositoryID,
		RepoRoot:     opts.RepoRoot,
		PID:          cmd.Process.Pid,
		StartedAt:    time.Now().UTC().Format(time.RFC3339),
		Command:      strings.Join(append([]string{filepath.Base(executable)}, args...), " "),
		executable:   executable,
		args:         args,
		cancel:       cancel,
		done:         make(chan error, 1),
	}

	s.mu.Lock()
	s.processes[opts.RepositoryID] = process
	s.mu.Unlock()

	go func() {
		waitErr := cmd.Wait()
		process.err = waitErr
		close(process.done)
	}()

	return process.Snapshot(), nil
}

// Stop terminates a running watcher for the repository. It returns false when
// no watcher is tracked.
func (s *Supervisor) Stop(repositoryID int64) (bool, error) {
	if s == nil {
		return false, nil
	}
	s.mu.Lock()
	process, ok := s.processes[repositoryID]
	if ok {
		delete(s.processes, repositoryID)
	}
	s.mu.Unlock()
	if !ok {
		return false, nil
	}
	return true, process.terminate()
}

// Status returns the tracked watcher for a repository, if any.
func (s *Supervisor) Status(repositoryID int64) (WatcherProcess, bool) {
	if s == nil {
		return WatcherProcess{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	process, ok := s.processes[repositoryID]
	if !ok {
		return WatcherProcess{}, false
	}
	return process.Snapshot(), true
}

// List returns snapshots of every tracked watcher.
func (s *Supervisor) List() []WatcherProcess {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]WatcherProcess, 0, len(s.processes))
	for _, process := range s.processes {
		out = append(out, process.Snapshot())
	}
	return out
}

// Reap removes watchers whose process has exited.
func (s *Supervisor) Reap() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, process := range s.processes {
		if !process.running() {
			delete(s.processes, id)
		}
	}
}

// Close stops every tracked watcher.
func (s *Supervisor) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	processes := make([]*WatcherProcess, 0, len(s.processes))
	for _, process := range s.processes {
		processes = append(processes, process)
	}
	s.processes = map[int64]*WatcherProcess{}
	s.mu.Unlock()
	var errs []error
	for _, process := range processes {
		if err := process.terminate(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (p *WatcherProcess) running() bool {
	if p == nil {
		return false
	}
	select {
	case <-p.done:
		return false
	default:
		return true
	}
}

func (p *WatcherProcess) terminate() error {
	if p == nil {
		return nil
	}
	if p.cancel != nil {
		p.cancel()
	}
	select {
	case <-p.done:
		return nil
	case <-time.After(5 * time.Second):
	}
	if p.PID > 0 {
		if proc, err := os.FindProcess(p.PID); err == nil {
			_ = proc.Kill()
		}
	}
	select {
	case <-p.done:
	case <-time.After(time.Second):
	}
	return nil
}

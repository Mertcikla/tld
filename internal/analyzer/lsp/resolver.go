package lsp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mertcikla/tld/v2/internal/analyzer"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

const (
	StateRequested     = "requested"
	StateAvailable     = "available"
	StateActive        = "active"
	StateUnavailable   = "unavailable"
	StateFailed        = "failed"
	StateDisabled      = "disabled"
	StateMemoryLimited = "memory_limited"
)

type EventLogger interface {
	InfoContext(ctx context.Context, msg string, args ...any)
	ErrorContext(ctx context.Context, msg string, args ...any)
}

type ResolverConfig struct {
	Enabled           bool
	HealthInterval    time.Duration
	DefinitionTimeout time.Duration
	MemoryLimitBytes  int64
	Commands          map[analyzer.Language]string
	Logger            EventLogger
}

type DefinitionLocation struct {
	FilePath string
	Line     int
}

type CallLocation struct {
	FilePath string
	Line     int
	Name     string
}

type StatusSnapshot struct {
	Enabled               bool           `json:"enabled"`
	HealthIntervalSeconds int            `json:"health_interval_seconds,omitempty"`
	MemoryLimitBytes      int64          `json:"memory_limit_bytes,omitempty"`
	MemoryMonitoring      string         `json:"memory_monitoring,omitempty"`
	Servers               []ServerStatus `json:"servers,omitempty"`
}

type ServerStatus struct {
	Language        string `json:"language"`
	Command         string `json:"command,omitempty"`
	CommandSource   string `json:"command_source,omitempty"`
	Path            string `json:"path,omitempty"`
	State           string `json:"state"`
	PID             int    `json:"pid,omitempty"`
	ServerName      string `json:"server_name,omitempty"`
	ServerVersion   string `json:"server_version,omitempty"`
	Definition      bool   `json:"definition"`
	MemoryBytes     int64  `json:"memory_bytes,omitempty"`
	RestartCount    int    `json:"restart_count,omitempty"`
	LastHealthcheck string `json:"last_healthcheck,omitempty"`
	LastError       string `json:"last_error,omitempty"`
}

type MultiLanguageResolver struct {
	RootDir string

	mu       sync.Mutex
	cfg      ResolverConfig
	sessions map[analyzer.Language]*Session
	statuses map[analyzer.Language]*ServerStatus
	opened   map[string]struct{}
	contents map[string]string
}

func NewMultiLanguageResolver(rootDir string) *MultiLanguageResolver {
	return NewMultiLanguageResolverWithConfig(rootDir, ResolverConfig{
		Enabled:           true,
		HealthInterval:    time.Minute,
		DefinitionTimeout: 10 * time.Second,
		MemoryLimitBytes:  4294967296,
	})
}

func NewMultiLanguageResolverWithConfig(rootDir string, cfg ResolverConfig) *MultiLanguageResolver {
	if cfg.HealthInterval <= 0 {
		cfg.HealthInterval = time.Minute
	}
	if cfg.DefinitionTimeout <= 0 {
		cfg.DefinitionTimeout = 10 * time.Second
	}
	if cfg.MemoryLimitBytes <= 0 {
		cfg.MemoryLimitBytes = 4294967296
	}
	return &MultiLanguageResolver{
		RootDir:  rootDir,
		cfg:      cfg,
		sessions: make(map[analyzer.Language]*Session),
		statuses: make(map[analyzer.Language]*ServerStatus),
		opened:   make(map[string]struct{}),
		contents: make(map[string]string),
	}
}

func SnapshotLanguages(languages []analyzer.Language, cfg ResolverConfig) StatusSnapshot {
	resolver := NewMultiLanguageResolverWithConfig("", cfg)
	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	for _, language := range languages {
		status := resolver.ensureRequestedLocked(language)
		if !cfg.Enabled {
			status.State = StateDisabled
		}
	}
	return resolver.snapshotLocked()
}

func (r *MultiLanguageResolver) ResolveDefinitions(ctx context.Context, ref analyzer.Ref) ([]DefinitionLocation, error) {
	if r == nil || r.RootDir == "" || ref.FilePath == "" || ref.Line <= 0 {
		return nil, nil
	}
	language, ok := analyzer.DetectLanguage(ref.FilePath)
	if !ok {
		return nil, nil
	}
	session, ok, err := r.sessionForLanguage(ctx, language)
	if err != nil || !ok {
		return nil, err
	}
	if err := r.openDocument(ctx, session, ref.FilePath); err != nil {
		r.markFailed(ctx, language, "open_document", err)
		return nil, err
	}
	column := 0
	if ref.Column > 0 {
		column = ref.Column - 1
	}
	definitionCtx := ctx
	cancel := func() {}
	if r.cfg.DefinitionTimeout > 0 {
		definitionCtx, cancel = context.WithTimeout(ctx, r.cfg.DefinitionTimeout)
	}
	defer cancel()
	locations, err := session.Definition(definitionCtx, &protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri.File(ref.FilePath)},
			Position: protocol.Position{
				Line:      uint32(ref.Line - 1),
				Character: uint32(column),
			},
		},
	})
	if err != nil {
		r.markFailed(ctx, language, "definition", err)
		return nil, err
	}
	resolved := make([]DefinitionLocation, 0, len(locations))
	for _, location := range locations {
		filePath := filepath.Clean(location.URI.Filename())
		if filePath == "" {
			continue
		}
		resolved = append(resolved, DefinitionLocation{
			FilePath: filePath,
			Line:     int(location.Range.Start.Line) + 1,
		})
	}
	return resolved, nil
}

func (r *MultiLanguageResolver) Implementations(ctx context.Context, filePath string, line, column int) ([]DefinitionLocation, error) {
	if r == nil || r.RootDir == "" || filePath == "" || line <= 0 {
		return nil, nil
	}
	language, ok := analyzer.DetectLanguage(filePath)
	if !ok {
		return nil, nil
	}
	session, ok, err := r.sessionForLanguage(ctx, language)
	if err != nil || !ok {
		return nil, err
	}
	if !session.SupportsImplementation() {
		return nil, nil
	}
	if err := r.openDocument(ctx, session, filePath); err != nil {
		r.markFailed(ctx, language, "open_document", err)
		return nil, err
	}
	if column <= 0 {
		column = 1
	}
	callCtx := ctx
	cancel := func() {}
	if r.cfg.DefinitionTimeout > 0 {
		callCtx, cancel = context.WithTimeout(ctx, r.cfg.DefinitionTimeout)
	}
	defer cancel()
	locations, err := session.Implementation(callCtx, &protocol.ImplementationParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri.File(filePath)},
			Position: protocol.Position{
				Line:      uint32(line - 1),
				Character: uint32(column - 1),
			},
		},
	})
	if err != nil {
		r.markFailed(ctx, language, "implementation", err)
		return nil, err
	}
	return definitionLocationsFromProtocol(locations), nil
}

func (r *MultiLanguageResolver) References(ctx context.Context, filePath string, line, column int) ([]DefinitionLocation, error) {
	if r == nil || r.RootDir == "" || filePath == "" || line <= 0 {
		return nil, nil
	}
	language, ok := analyzer.DetectLanguage(filePath)
	if !ok {
		return nil, nil
	}
	session, ok, err := r.sessionForLanguage(ctx, language)
	if err != nil || !ok {
		return nil, err
	}
	if !session.SupportsReferences() {
		return nil, nil
	}
	if err := r.openDocument(ctx, session, filePath); err != nil {
		r.markFailed(ctx, language, "open_document", err)
		return nil, err
	}
	if column <= 0 {
		column = 1
	}
	callCtx := ctx
	cancel := func() {}
	if r.cfg.DefinitionTimeout > 0 {
		callCtx, cancel = context.WithTimeout(ctx, r.cfg.DefinitionTimeout)
	}
	defer cancel()
	locations, err := session.References(callCtx, &protocol.ReferenceParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri.File(filePath)},
			Position: protocol.Position{
				Line:      uint32(line - 1),
				Character: uint32(column - 1),
			},
		},
		Context: protocol.ReferenceContext{IncludeDeclaration: false},
	})
	if err != nil {
		r.markFailed(ctx, language, "references", err)
		return nil, err
	}
	return definitionLocationsFromProtocol(locations), nil
}

func (r *MultiLanguageResolver) IncomingCalls(ctx context.Context, filePath string, line, column int) ([]CallLocation, error) {
	if r == nil || r.RootDir == "" || filePath == "" || line <= 0 {
		return nil, nil
	}
	language, ok := analyzer.DetectLanguage(filePath)
	if !ok {
		return nil, nil
	}
	session, ok, err := r.sessionForLanguage(ctx, language)
	if err != nil || !ok {
		return nil, err
	}
	if !session.SupportsCallHierarchy() {
		return nil, nil
	}
	if err := r.openDocument(ctx, session, filePath); err != nil {
		r.markFailed(ctx, language, "open_document", err)
		return nil, err
	}
	if column <= 0 {
		column = 1
	}
	callCtx := ctx
	cancel := func() {}
	if r.cfg.DefinitionTimeout > 0 {
		callCtx, cancel = context.WithTimeout(ctx, r.cfg.DefinitionTimeout)
	}
	defer cancel()
	items, err := session.PrepareCallHierarchy(callCtx, &protocol.CallHierarchyPrepareParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri.File(filePath)},
			Position: protocol.Position{
				Line:      uint32(line - 1),
				Character: uint32(column - 1),
			},
		},
	})
	if err != nil {
		r.markFailed(ctx, language, "call_hierarchy_prepare", err)
		return nil, err
	}
	var calls []CallLocation
	for _, item := range items {
		incoming, err := session.IncomingCalls(callCtx, item)
		if err != nil {
			r.markFailed(ctx, language, "incoming_calls", err)
			return nil, err
		}
		for _, call := range incoming {
			filePath := filepath.Clean(call.From.URI.Filename())
			if filePath == "" {
				continue
			}
			calls = append(calls, CallLocation{
				FilePath: filePath,
				Line:     int(call.From.Range.Start.Line) + 1,
				Name:     call.From.Name,
			})
		}
	}
	return calls, nil
}

func (r *MultiLanguageResolver) OutgoingCalls(ctx context.Context, filePath string, line, column int) ([]CallLocation, error) {
	if r == nil || r.RootDir == "" || filePath == "" || line <= 0 {
		return nil, nil
	}
	language, ok := analyzer.DetectLanguage(filePath)
	if !ok {
		return nil, nil
	}
	session, ok, err := r.sessionForLanguage(ctx, language)
	if err != nil || !ok {
		return nil, err
	}
	if !session.SupportsCallHierarchy() {
		return nil, nil
	}
	if err := r.openDocument(ctx, session, filePath); err != nil {
		r.markFailed(ctx, language, "open_document", err)
		return nil, err
	}
	if column <= 0 {
		column = 1
	}
	callCtx := ctx
	cancel := func() {}
	if r.cfg.DefinitionTimeout > 0 {
		callCtx, cancel = context.WithTimeout(ctx, r.cfg.DefinitionTimeout)
	}
	defer cancel()
	items, err := session.PrepareCallHierarchy(callCtx, &protocol.CallHierarchyPrepareParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: uri.File(filePath)},
			Position: protocol.Position{
				Line:      uint32(line - 1),
				Character: uint32(column - 1),
			},
		},
	})
	if err != nil {
		r.markFailed(ctx, language, "call_hierarchy_prepare", err)
		return nil, err
	}
	var calls []CallLocation
	for _, item := range items {
		outgoing, err := session.OutgoingCalls(callCtx, item)
		if err != nil {
			r.markFailed(ctx, language, "outgoing_calls", err)
			return nil, err
		}
		for _, call := range outgoing {
			filePath := filepath.Clean(call.To.URI.Filename())
			if filePath == "" {
				continue
			}
			calls = append(calls, CallLocation{
				FilePath: filePath,
				Line:     int(call.To.Range.Start.Line) + 1,
				Name:     call.To.Name,
			})
		}
	}
	return calls, nil
}

func (r *MultiLanguageResolver) Snapshot() StatusSnapshot {
	if r == nil {
		return StatusSnapshot{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.snapshotLocked()
}

func (r *MultiLanguageResolver) Close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var errs []error
	for language, session := range r.sessions {
		if session == nil {
			continue
		}
		if err := session.Close(); err != nil {
			errs = append(errs, err)
		}
		delete(r.sessions, language)
	}
	return errors.Join(errs...)
}

func definitionLocationsFromProtocol(locations []protocol.Location) []DefinitionLocation {
	resolved := make([]DefinitionLocation, 0, len(locations))
	for _, location := range locations {
		filePath := filepath.Clean(location.URI.Filename())
		if filePath == "" {
			continue
		}
		resolved = append(resolved, DefinitionLocation{
			FilePath: filePath,
			Line:     int(location.Range.Start.Line) + 1,
		})
	}
	return resolved
}

func (r *MultiLanguageResolver) sessionForLanguage(ctx context.Context, language analyzer.Language) (*Session, bool, error) {
	r.mu.Lock()
	if !r.cfg.Enabled {
		status := r.ensureRequestedLocked(language)
		status.State = StateDisabled
		r.mu.Unlock()
		return nil, false, nil
	}
	status := r.ensureRequestedLocked(language)
	if status.State == StateUnavailable || status.State == StateDisabled || status.State == StateFailed || status.State == StateMemoryLimited {
		r.mu.Unlock()
		return nil, false, nil
	}
	if session, ok := r.sessions[language]; ok {
		if r.healthcheckLocked(ctx, language, session) {
			r.mu.Unlock()
			return session, true, nil
		}
	}
	command, err := ResolveServerCommandWithOverrides(language, r.cfg.Commands, nil)
	if err != nil {
		status.State = StateUnavailable
		status.LastError = err.Error()
		r.logError(ctx, "watch.lsp.command_unavailable", err, language, status)
		r.mu.Unlock()
		return nil, false, nil
	}
	status.Path = command.Path
	status.Command = commandDisplay(command)
	status.CommandSource = command.CommandSource
	status.State = StateAvailable
	r.logInfo(ctx, "watch.lsp.command_resolved", language, status, "path", command.Path)
	r.mu.Unlock()

	session, err := StartSession(ctx, SessionConfig{
		Language: language,
		RootDir:  r.RootDir,
		Command:  command,
	})
	r.mu.Lock()
	defer r.mu.Unlock()
	status = r.ensureRequestedLocked(language)
	if err != nil {
		status.State = StateFailed
		status.LastError = err.Error()
		r.logError(ctx, "watch.lsp.start_failed", err, language, status)
		return nil, false, nil
	}
	if !session.SupportsDefinition() {
		status.State = StateFailed
		status.PID = session.PID()
		status.Definition = false
		status.LastError = "language server does not support definition lookup"
		_ = session.Close()
		r.logError(ctx, "watch.lsp.unsupported_definition", errors.New(status.LastError), language, status)
		return nil, false, nil
	}
	r.sessions[language] = session
	r.fillActiveStatusLocked(ctx, language, status, session)
	r.logInfo(ctx, "watch.lsp.started", language, status)
	return session, true, nil
}

func (r *MultiLanguageResolver) healthcheckLocked(ctx context.Context, language analyzer.Language, session *Session) bool {
	status := r.ensureRequestedLocked(language)
	if r.cfg.HealthInterval > 0 && status.LastHealthcheck != "" {
		checkedAt, err := time.Parse(time.RFC3339, status.LastHealthcheck)
		if err == nil && time.Since(checkedAt) < r.cfg.HealthInterval {
			return true
		}
	}
	status.LastHealthcheck = time.Now().UTC().Format(time.RFC3339)
	if session == nil || session.PID() <= 0 || !processAlive(session.PID()) {
		err := fmt.Errorf("language server process is not running")
		r.restartLocked(ctx, language, "healthcheck_failed", err)
		return false
	}
	if r.cfg.MemoryLimitBytes > 0 {
		if memoryBytes, ok, err := processMemoryBytes(session.PID()); err != nil {
			status.LastError = "memory check failed: " + err.Error()
			r.logError(ctx, "watch.lsp.memory_check_failed", err, language, status)
		} else if ok {
			status.MemoryBytes = memoryBytes
			if memoryBytes > r.cfg.MemoryLimitBytes {
				r.restartLocked(ctx, language, "memory_limit_exceeded", fmt.Errorf("language server memory %d exceeded limit %d", memoryBytes, r.cfg.MemoryLimitBytes))
				return false
			}
		}
	}
	status.State = StateActive
	status.Definition = session.SupportsDefinition()
	return true
}

func (r *MultiLanguageResolver) openDocument(ctx context.Context, session *Session, filePath string) error {
	cleanPath := filepath.Clean(filePath)
	r.mu.Lock()
	if _, ok := r.opened[cleanPath]; ok {
		r.mu.Unlock()
		return nil
	}
	content, ok := r.contents[cleanPath]
	r.mu.Unlock()
	if !ok {
		data, err := os.ReadFile(cleanPath)
		if err != nil {
			return fmt.Errorf("read %s: %w", cleanPath, err)
		}
		content = string(data)
		r.mu.Lock()
		r.contents[cleanPath] = content
		r.mu.Unlock()
	}
	if err := session.OpenDocument(ctx, cleanPath, content); err != nil {
		return fmt.Errorf("open %s in language server: %w", cleanPath, err)
	}
	r.mu.Lock()
	r.opened[cleanPath] = struct{}{}
	r.mu.Unlock()
	return nil
}

// func (r *MultiLanguageResolver) restartAfterFailure(ctx context.Context, language analyzer.Language, reason string, err error) {
// 	r.mu.Lock()
// 	defer r.mu.Unlock()
// 	r.restartLocked(ctx, language, reason, err)
// }

func (r *MultiLanguageResolver) markFailed(ctx context.Context, language analyzer.Language, reason string, err error) {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	status := r.ensureRequestedLocked(language)

	if isNonFatalLSPError(err) {
		r.logInfo(ctx, "watch.lsp."+reason+".non_fatal", language, status, "error", err)
		return
	}

	status.State = StateFailed
	status.LastError = err.Error()
	r.logError(ctx, "watch.lsp."+reason+".failed", err, language, status)
}

func (r *MultiLanguageResolver) restartLocked(ctx context.Context, language analyzer.Language, reason string, err error) {
	status := r.ensureRequestedLocked(language)
	if session := r.sessions[language]; session != nil {
		_ = session.Close()
		delete(r.sessions, language)
	}
	for opened := range r.opened {
		delete(r.opened, opened)
	}
	status.State = StateFailed
	if reason == "memory_limit_exceeded" {
		status.State = StateMemoryLimited
	}
	status.RestartCount++
	status.PID = 0
	status.LastError = err.Error()
	r.logError(ctx, "watch.lsp.restart_scheduled", err, language, status, "reason", reason)
}

func (r *MultiLanguageResolver) fillActiveStatusLocked(ctx context.Context, language analyzer.Language, status *ServerStatus, session *Session) {
	status.State = StateActive
	status.PID = session.PID()
	status.Definition = session.SupportsDefinition()
	status.LastHealthcheck = time.Now().UTC().Format(time.RFC3339)
	if info := session.ServerInfo(); info != nil {
		status.ServerName = info.Name
		status.ServerVersion = info.Version
	}
	if memoryBytes, ok, err := processMemoryBytes(session.PID()); err != nil {
		status.LastError = "memory check failed: " + err.Error()
		r.logError(ctx, "watch.lsp.memory_check_failed", err, language, status)
	} else if ok {
		status.MemoryBytes = memoryBytes
	}
}

func (r *MultiLanguageResolver) ensureRequestedLocked(language analyzer.Language) *ServerStatus {
	if status, ok := r.statuses[language]; ok {
		return status
	}
	status := &ServerStatus{Language: string(language), State: StateRequested}
	overrideCommand := strings.TrimSpace(r.cfg.Commands[language])
	commands := DefaultCommands(language)
	if len(commands) == 0 {
		status.State = StateUnavailable
		status.LastError = ErrServerNotConfigured{Language: language}.Error()
	} else if overrideCommand != "" {
		status.Command = overrideCommand
		status.CommandSource = CommandSourceOverride
		if command, err := ResolveServerCommandWithOverrides(language, r.cfg.Commands, nil); err == nil {
			status.Path = command.Path
			status.Command = commandDisplay(command)
			status.CommandSource = command.CommandSource
			status.State = StateAvailable
		} else {
			status.State = StateUnavailable
			status.LastError = err.Error()
		}
	} else {
		status.Command = commands[0].Display()
		status.CommandSource = CommandSourceDefault
		if command, err := ResolveServerCommandWithOverrides(language, r.cfg.Commands, nil); err == nil {
			status.Path = command.Path
			status.Command = commandDisplay(command)
			status.CommandSource = command.CommandSource
			status.State = StateAvailable
		} else {
			status.State = StateUnavailable
			status.LastError = err.Error()
		}
	}
	r.statuses[language] = status
	return status
}

func (r *MultiLanguageResolver) snapshotLocked() StatusSnapshot {
	servers := make([]ServerStatus, 0, len(r.statuses))
	for _, status := range r.statuses {
		copyStatus := *status
		servers = append(servers, copyStatus)
	}
	sort.Slice(servers, func(i, j int) bool {
		return servers[i].Language < servers[j].Language
	})
	monitoring := "unavailable"
	if processMemorySupported() {
		monitoring = "available"
	}
	return StatusSnapshot{
		Enabled:               r.cfg.Enabled,
		HealthIntervalSeconds: int(r.cfg.HealthInterval.Seconds()),
		MemoryLimitBytes:      r.cfg.MemoryLimitBytes,
		MemoryMonitoring:      monitoring,
		Servers:               servers,
	}
}

func (r *MultiLanguageResolver) logInfo(ctx context.Context, msg string, language analyzer.Language, status *ServerStatus, args ...any) {
	if r.cfg.Logger == nil {
		return
	}
	fields := lspLogFields(language, status, args...)
	r.cfg.Logger.InfoContext(ctx, msg, fields...)
}

func (r *MultiLanguageResolver) logError(ctx context.Context, msg string, err error, language analyzer.Language, status *ServerStatus, args ...any) {
	if r.cfg.Logger == nil {
		return
	}
	fields := lspLogFields(language, status, append([]any{"error", err}, args...)...)
	r.cfg.Logger.ErrorContext(ctx, msg, fields...)
}

func lspLogFields(language analyzer.Language, status *ServerStatus, args ...any) []any {
	fields := []any{"language", string(language)}
	if status != nil {
		fields = append(fields,
			"state", status.State,
			"command", status.Command,
			"command_source", status.CommandSource,
			"path", status.Path,
			"pid", status.PID,
			"memory_bytes", status.MemoryBytes,
			"restart_count", status.RestartCount,
		)
	}
	fields = append(fields, args...)
	return fields
}

func commandDisplay(command ResolvedCommand) string {
	parts := []string{command.Path}
	parts = append(parts, command.Args...)
	return strings.Join(parts, " ")
}

func isNonFatalLSPError(err error) bool {
	if err == nil {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no package metadata") ||
		strings.Contains(msg, "no build state") ||
		strings.Contains(msg, "not found") ||
		strings.Contains(msg, "was not found")
}

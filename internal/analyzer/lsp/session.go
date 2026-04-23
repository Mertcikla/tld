package lsp

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/mertcikla/diag/tld/internal/analyzer"
	jsonrpc2 "go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
	"go.uber.org/zap"
)

const closeTimeout = 3 * time.Second

type SessionConfig struct {
	Language              analyzer.Language
	RootDir               string
	Command               ResolvedCommand
	ProcessEnv            []string
	InitializationOptions any
}

type Session struct {
	language     analyzer.Language
	rootDir      string
	command      ResolvedCommand
	stderr       bytes.Buffer
	process      *exec.Cmd
	transport    *processTransport
	conn         jsonrpc2.Conn
	server       protocol.Server
	serverInfo   *protocol.ServerInfo
	capabilities protocol.ServerCapabilities
	waitDone     chan error

	closeOnce sync.Once
}

func StartSession(ctx context.Context, cfg SessionConfig) (*Session, error) {
	if cfg.RootDir == "" {
		return nil, fmt.Errorf("root directory is required")
	}
	rootDir, err := filepath.Abs(cfg.RootDir)
	if err != nil {
		return nil, fmt.Errorf("resolve root directory: %w", err)
	}
	command := cfg.Command
	if command.Path == "" {
		resolved, err := ResolveServerCommand(cfg.Language)
		if err != nil {
			return nil, err
		}
		command = resolved
	}
	if command.Path == "" {
		return nil, fmt.Errorf("resolved LSP command path is empty")
	}

	process := exec.CommandContext(ctx, command.Path, command.Args...)
	process.Dir = rootDir
	process.Env = append(os.Environ(), cfg.ProcessEnv...)

	stdin, err := process.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("open %s stdin: %w", command.Path, err)
	}
	stdout, err := process.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("open %s stdout: %w", command.Path, err)
	}

	session := &Session{
		language: cfg.Language,
		rootDir:  rootDir,
		command:  command,
		process:  process,
		waitDone: make(chan error, 1),
	}
	process.Stderr = &session.stderr

	if err := process.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", command.Path, err)
	}
	go func() {
		session.waitDone <- process.Wait()
	}()

	session.transport = &processTransport{
		reader: stdout,
		writer: stdin,
		kill: func() error {
			if process.Process == nil {
				return nil
			}
			if err := process.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
				return err
			}
			return nil
		},
	}

	workspaceURI := uri.File(rootDir)
	client := &noopClient{
		workspaceFolders: []protocol.WorkspaceFolder{{
			URI:  string(workspaceURI),
			Name: filepath.Base(rootDir),
		}},
	}

	_, conn, server := protocol.NewClient(context.Background(), client, jsonrpc2.NewStream(session.transport), zap.NewNop())
	session.conn = conn
	session.server = server

	initializeParams := &protocol.InitializeParams{
		ProcessID:             int32(os.Getpid()),
		ClientInfo:            &protocol.ClientInfo{Name: "tld-cli", Version: "dev"},
		RootURI:               workspaceURI,
		InitializationOptions: cfg.InitializationOptions,
		Capabilities:          protocol.ClientCapabilities{},
		WorkspaceFolders: []protocol.WorkspaceFolder{{
			URI:  string(workspaceURI),
			Name: filepath.Base(rootDir),
		}},
	}
	result, err := session.server.Initialize(ctx, initializeParams)
	if err != nil {
		_ = session.Close()
		return nil, fmt.Errorf("initialize %s: %w%s", command.Path, err, formatStderrSuffix(session.stderr.String()))
	}
	if err := session.server.Initialized(ctx, &protocol.InitializedParams{}); err != nil {
		_ = session.Close()
		return nil, fmt.Errorf("initialized %s: %w%s", command.Path, err, formatStderrSuffix(session.stderr.String()))
	}

	session.serverInfo = result.ServerInfo
	session.capabilities = result.Capabilities
	return session, nil
}

func (s *Session) ServerInfo() *protocol.ServerInfo {
	return s.serverInfo
}

func (s *Session) SupportsDefinition() bool {
	return capabilityEnabled(s.capabilities.DefinitionProvider)
}

func (s *Session) SupportsTypeDefinition() bool {
	return capabilityEnabled(s.capabilities.TypeDefinitionProvider)
}

func (s *Session) SupportsCallHierarchy() bool {
	return capabilityEnabled(s.capabilities.CallHierarchyProvider)
}

func (s *Session) OpenDocument(ctx context.Context, filePath, text string) error {
	if s.server == nil {
		return fmt.Errorf("LSP session is not initialized")
	}
	if filePath == "" {
		return fmt.Errorf("file path is required")
	}
	uri := uri.File(filePath)
	return s.server.DidOpen(ctx, &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:        uri,
			LanguageID: languageIDForPath(filePath, s.language),
			Version:    1,
			Text:       text,
		},
	})
}

func (s *Session) CloseDocument(ctx context.Context, filePath string) error {
	if s.server == nil {
		return fmt.Errorf("LSP session is not initialized")
	}
	if filePath == "" {
		return fmt.Errorf("file path is required")
	}
	return s.server.DidClose(ctx, &protocol.DidCloseTextDocumentParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri.File(filePath)},
	})
}

func (s *Session) Definition(ctx context.Context, params *protocol.DefinitionParams) ([]protocol.Location, error) {
	if s.server == nil {
		return nil, fmt.Errorf("LSP session is not initialized")
	}
	return s.server.Definition(ctx, params)
}

func (s *Session) TypeDefinition(ctx context.Context, params *protocol.TypeDefinitionParams) ([]protocol.Location, error) {
	if s.server == nil {
		return nil, fmt.Errorf("LSP session is not initialized")
	}
	return s.server.TypeDefinition(ctx, params)
}

func (s *Session) PrepareCallHierarchy(ctx context.Context, params *protocol.CallHierarchyPrepareParams) ([]protocol.CallHierarchyItem, error) {
	if s.server == nil {
		return nil, fmt.Errorf("LSP session is not initialized")
	}
	return s.server.PrepareCallHierarchy(ctx, params)
}

func (s *Session) IncomingCalls(ctx context.Context, item protocol.CallHierarchyItem) ([]protocol.CallHierarchyIncomingCall, error) {
	if s.server == nil {
		return nil, fmt.Errorf("LSP session is not initialized")
	}
	return s.server.IncomingCalls(ctx, &protocol.CallHierarchyIncomingCallsParams{Item: item})
}

func (s *Session) OutgoingCalls(ctx context.Context, item protocol.CallHierarchyItem) ([]protocol.CallHierarchyOutgoingCall, error) {
	if s.server == nil {
		return nil, fmt.Errorf("LSP session is not initialized")
	}
	return s.server.OutgoingCalls(ctx, &protocol.CallHierarchyOutgoingCallsParams{Item: item})
}

func (s *Session) Close() error {
	var closeErr error
	s.closeOnce.Do(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), closeTimeout)
		defer cancel()

		var errs []error
		if s.server != nil {
			if err := s.server.Shutdown(shutdownCtx); err != nil && !isIgnorableSessionError(err) {
				errs = append(errs, fmt.Errorf("shutdown language server: %w", err))
			}
			if err := s.server.Exit(shutdownCtx); err != nil && !isIgnorableSessionError(err) {
				errs = append(errs, fmt.Errorf("exit language server: %w", err))
			}
		}
		if s.conn != nil {
			if err := s.conn.Close(); err != nil && !isIgnorableSessionError(err) {
				errs = append(errs, fmt.Errorf("close LSP connection: %w", err))
			}
			select {
			case <-s.conn.Done():
				if err := s.conn.Err(); err != nil && !isIgnorableSessionError(err) {
					errs = append(errs, fmt.Errorf("LSP connection error: %w", err))
				}
			case <-shutdownCtx.Done():
				errs = append(errs, fmt.Errorf("wait for LSP connection close: %w", shutdownCtx.Err()))
			}
		}
		if s.transport != nil {
			if err := s.transport.Close(); err != nil && !isIgnorableSessionError(err) {
				errs = append(errs, fmt.Errorf("close LSP transport: %w", err))
			}
		}
		if s.waitDone != nil {
			select {
			case err := <-s.waitDone:
				if err != nil && !isIgnorableSessionError(err) {
					errs = append(errs, fmt.Errorf("wait for LSP process: %w", err))
				}
			case <-shutdownCtx.Done():
				errs = append(errs, fmt.Errorf("wait for LSP process: %w", shutdownCtx.Err()))
			}
		}
		closeErr = errors.Join(errs...)
	})
	return closeErr
}

func formatStderrSuffix(stderr string) string {
	if stderr == "" {
		return ""
	}
	return ": " + stderr
}

func capabilityEnabled(value any) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case bool:
		return typed
	default:
		return true
	}
}

func languageIDForPath(filePath string, language analyzer.Language) protocol.LanguageIdentifier {
	ext := filepath.Ext(filePath)
	switch language {
	case analyzer.LanguageC:
		return protocol.CLanguage
	case analyzer.LanguageCPP:
		return protocol.CppLanguage
	case analyzer.LanguageGo:
		return protocol.GoLanguage
	case analyzer.LanguageJava:
		return protocol.JavaLanguage
	case analyzer.LanguageJavaScript:
		if ext == ".jsx" {
			return protocol.JavaScriptReactLanguage
		}
		return protocol.JavaScriptLanguage
	case analyzer.LanguagePython:
		return protocol.PythonLanguage
	case analyzer.LanguageRust:
		return protocol.RustLanguage
	case analyzer.LanguageTypeScript:
		if ext == ".tsx" {
			return protocol.TypeScriptReactLanguage
		}
		return protocol.TypeScriptLanguage
	default:
		return protocol.LanguageIdentifier(language)
	}
}

func isIgnorableSessionError(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, context.Canceled) || errors.Is(err, os.ErrClosed) || errors.Is(err, os.ErrProcessDone)
}

type processTransport struct {
	reader    io.ReadCloser
	writer    io.WriteCloser
	kill      func() error
	closeOnce sync.Once
	closeErr  error
}

func (p *processTransport) Read(buf []byte) (int, error) {
	return p.reader.Read(buf)
}

func (p *processTransport) Write(buf []byte) (int, error) {
	return p.writer.Write(buf)
}

func (p *processTransport) Close() error {
	p.closeOnce.Do(func() {
		var errs []error
		if p.writer != nil {
			if err := p.writer.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
				errs = append(errs, err)
			}
		}
		if p.reader != nil {
			if err := p.reader.Close(); err != nil && !errors.Is(err, os.ErrClosed) {
				errs = append(errs, err)
			}
		}
		if p.kill != nil {
			if err := p.kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
				errs = append(errs, err)
			}
		}
		p.closeErr = errors.Join(errs...)
	})
	return p.closeErr
}

type noopClient struct {
	workspaceFolders []protocol.WorkspaceFolder
}

func (c *noopClient) Progress(context.Context, *protocol.ProgressParams) error {
	return nil
}

func (c *noopClient) WorkDoneProgressCreate(context.Context, *protocol.WorkDoneProgressCreateParams) error {
	return nil
}

func (c *noopClient) LogMessage(context.Context, *protocol.LogMessageParams) error {
	return nil
}

func (c *noopClient) PublishDiagnostics(context.Context, *protocol.PublishDiagnosticsParams) error {
	return nil
}

func (c *noopClient) ShowMessage(context.Context, *protocol.ShowMessageParams) error {
	return nil
}

func (c *noopClient) ShowMessageRequest(context.Context, *protocol.ShowMessageRequestParams) (*protocol.MessageActionItem, error) {
	return nil, nil
}

func (c *noopClient) Telemetry(context.Context, any) error {
	return nil
}

func (c *noopClient) RegisterCapability(context.Context, *protocol.RegistrationParams) error {
	return nil
}

func (c *noopClient) UnregisterCapability(context.Context, *protocol.UnregistrationParams) error {
	return nil
}

func (c *noopClient) ApplyEdit(context.Context, *protocol.ApplyWorkspaceEditParams) (bool, error) {
	return false, nil
}

func (c *noopClient) Configuration(context.Context, *protocol.ConfigurationParams) ([]any, error) {
	return nil, nil
}

func (c *noopClient) WorkspaceFolders(context.Context) ([]protocol.WorkspaceFolder, error) {
	return append([]protocol.WorkspaceFolder{}, c.workspaceFolders...), nil
}

package lsp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mertcikla/diag/tld/internal/analyzer"
	jsonrpc2 "go.lsp.dev/jsonrpc2"
	"go.lsp.dev/protocol"
	"go.lsp.dev/uri"
)

func TestStartSessionAndRequests(t *testing.T) {
	rootDir := t.TempDir()
	filePath := filepath.Join(rootDir, "main.go")
	content := "package main\nfunc Foo() {}\nfunc Bar() { Foo() }\n"
	if err := os.WriteFile(filePath, []byte(content), 0600); err != nil {
		t.Fatalf("write source: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	session, err := StartSession(ctx, SessionConfig{
		Language: analyzer.LanguageGo,
		RootDir:  rootDir,
		Command: ResolvedCommand{
			Path: os.Args[0],
			Args: []string{"-test.run=TestHelperProcessFakeLSP", "--"},
		},
		ProcessEnv: []string{"TLD_LSP_HELPER=1"},
	})
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	defer func() {
		if closeErr := session.Close(); closeErr != nil {
			t.Fatalf("Close: %v", closeErr)
		}
	}()

	if session.ServerInfo() == nil || session.ServerInfo().Name != "fake-lsp" {
		t.Fatalf("unexpected server info: %+v", session.ServerInfo())
	}
	if !session.SupportsDefinition() {
		t.Fatal("expected definition support")
	}
	if !session.SupportsCallHierarchy() {
		t.Fatal("expected call hierarchy support")
	}

	if err := session.OpenDocument(ctx, filePath, content); err != nil {
		t.Fatalf("OpenDocument: %v", err)
	}

	documentURI := uri.File(filePath)
	definitions, err := session.Definition(ctx, &protocol.DefinitionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: documentURI},
			Position:     protocol.Position{Line: 2, Character: 13},
		},
	})
	if err != nil {
		t.Fatalf("Definition: %v", err)
	}
	if len(definitions) != 1 {
		t.Fatalf("expected 1 definition, got %d", len(definitions))
	}
	if definitions[0].URI != documentURI {
		t.Fatalf("unexpected definition URI: %s", definitions[0].URI)
	}

	items, err := session.PrepareCallHierarchy(ctx, &protocol.CallHierarchyPrepareParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{URI: documentURI},
			Position:     protocol.Position{Line: 2, Character: 5},
		},
	})
	if err != nil {
		t.Fatalf("PrepareCallHierarchy: %v", err)
	}
	if len(items) != 1 || items[0].Name != "Bar" {
		t.Fatalf("unexpected call hierarchy items: %+v", items)
	}

	outgoing, err := session.OutgoingCalls(ctx, items[0])
	if err != nil {
		t.Fatalf("OutgoingCalls: %v", err)
	}
	if len(outgoing) != 1 || outgoing[0].To.Name != "Foo" {
		t.Fatalf("unexpected outgoing calls: %+v", outgoing)
	}

	incoming, err := session.IncomingCalls(ctx, outgoing[0].To)
	if err != nil {
		t.Fatalf("IncomingCalls: %v", err)
	}
	if len(incoming) != 1 || incoming[0].From.Name != "Bar" {
		t.Fatalf("unexpected incoming calls: %+v", incoming)
	}

	if err := session.CloseDocument(ctx, filePath); err != nil {
		t.Fatalf("CloseDocument: %v", err)
	}
}

func TestHelperProcessFakeLSP(t *testing.T) {
	if os.Getenv("TLD_LSP_HELPER") != "1" {
		return
	}

	ctx := context.Background()
	conn := jsonrpc2.NewConn(jsonrpc2.NewStream(helperTransport{Reader: os.Stdin, Writer: os.Stdout}))
	state := &fakeServerState{documents: make(map[string]string)}
	conn.Go(ctx, func(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
		if err := handleFakeLSPRequest(ctx, state, reply, req); err != nil {
			_ = reply(ctx, nil, err)
			return err
		}
		return nil
	})
	<-conn.Done()
	os.Exit(0)
}

type fakeServerState struct {
	documents map[string]string
	shutdown  bool
}

func handleFakeLSPRequest(ctx context.Context, state *fakeServerState, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
	switch req.Method() {
	case protocol.MethodInitialize:
		return reply(ctx, &protocol.InitializeResult{
			Capabilities: protocol.ServerCapabilities{
				DefinitionProvider:     true,
				TypeDefinitionProvider: true,
				CallHierarchyProvider:  true,
			},
			ServerInfo: &protocol.ServerInfo{Name: "fake-lsp", Version: "1.0.0"},
		}, nil)
	case protocol.MethodInitialized:
		return reply(ctx, nil, nil)
	case protocol.MethodShutdown:
		state.shutdown = true
		return reply(ctx, nil, nil)
	case protocol.MethodExit:
		return reply(ctx, nil, nil)
	case protocol.MethodTextDocumentDidOpen:
		var params protocol.DidOpenTextDocumentParams
		if err := json.Unmarshal(req.Params(), &params); err != nil {
			return err
		}
		state.documents[string(params.TextDocument.URI)] = params.TextDocument.Text
		return reply(ctx, nil, nil)
	case protocol.MethodTextDocumentDidClose:
		var params protocol.DidCloseTextDocumentParams
		if err := json.Unmarshal(req.Params(), &params); err != nil {
			return err
		}
		delete(state.documents, string(params.TextDocument.URI))
		return reply(ctx, nil, nil)
	case protocol.MethodTextDocumentDefinition:
		var params protocol.DefinitionParams
		if err := json.Unmarshal(req.Params(), &params); err != nil {
			return err
		}
		return reply(ctx, []protocol.Location{{
			URI: params.TextDocument.URI,
			Range: protocol.Range{
				Start: protocol.Position{Line: 1, Character: 5},
				End:   protocol.Position{Line: 1, Character: 8},
			},
		}}, nil)
	case protocol.MethodTextDocumentPrepareCallHierarchy:
		var params protocol.CallHierarchyPrepareParams
		if err := json.Unmarshal(req.Params(), &params); err != nil {
			return err
		}
		return reply(ctx, []protocol.CallHierarchyItem{{
			Name: "Bar",
			Kind: protocol.SymbolKindFunction,
			URI:  params.TextDocument.URI,
			Range: protocol.Range{
				Start: protocol.Position{Line: 2, Character: 0},
				End:   protocol.Position{Line: 2, Character: 20},
			},
			SelectionRange: protocol.Range{
				Start: protocol.Position{Line: 2, Character: 5},
				End:   protocol.Position{Line: 2, Character: 8},
			},
		}}, nil)
	case protocol.MethodCallHierarchyOutgoingCalls:
		var params protocol.CallHierarchyOutgoingCallsParams
		if err := json.Unmarshal(req.Params(), &params); err != nil {
			return err
		}
		return reply(ctx, []protocol.CallHierarchyOutgoingCall{{
			To: protocol.CallHierarchyItem{
				Name: "Foo",
				Kind: protocol.SymbolKindFunction,
				URI:  params.Item.URI,
				Range: protocol.Range{
					Start: protocol.Position{Line: 1, Character: 0},
					End:   protocol.Position{Line: 1, Character: 12},
				},
				SelectionRange: protocol.Range{
					Start: protocol.Position{Line: 1, Character: 5},
					End:   protocol.Position{Line: 1, Character: 8},
				},
			},
			FromRanges: []protocol.Range{{
				Start: protocol.Position{Line: 2, Character: 13},
				End:   protocol.Position{Line: 2, Character: 16},
			}},
		}}, nil)
	case protocol.MethodCallHierarchyIncomingCalls:
		var params protocol.CallHierarchyIncomingCallsParams
		if err := json.Unmarshal(req.Params(), &params); err != nil {
			return err
		}
		return reply(ctx, []protocol.CallHierarchyIncomingCall{{
			From: protocol.CallHierarchyItem{
				Name: "Bar",
				Kind: protocol.SymbolKindFunction,
				URI:  params.Item.URI,
				Range: protocol.Range{
					Start: protocol.Position{Line: 2, Character: 0},
					End:   protocol.Position{Line: 2, Character: 20},
				},
				SelectionRange: protocol.Range{
					Start: protocol.Position{Line: 2, Character: 5},
					End:   protocol.Position{Line: 2, Character: 8},
				},
			},
			FromRanges: []protocol.Range{{
				Start: protocol.Position{Line: 2, Character: 13},
				End:   protocol.Position{Line: 2, Character: 16},
			}},
		}}, nil)
	default:
		return reply(ctx, nil, fmt.Errorf("unsupported method %q", req.Method()))
	}
}

type helperTransport struct {
	io.Reader
	io.Writer
}

func (h helperTransport) Close() error {
	return nil
}

func TestLanguageIDForPath(t *testing.T) {
	if got := languageIDForPath("widget.tsx", analyzer.LanguageTypeScript); got != protocol.TypeScriptReactLanguage {
		t.Fatalf("tsx language id = %s", got)
	}
	if got := languageIDForPath("widget.jsx", analyzer.LanguageJavaScript); got != protocol.JavaScriptReactLanguage {
		t.Fatalf("jsx language id = %s", got)
	}
	if got := languageIDForPath("widget.go", analyzer.LanguageGo); got != protocol.GoLanguage {
		t.Fatalf("go language id = %s", got)
	}
}

func TestCapabilityEnabled(t *testing.T) {
	if capabilityEnabled(nil) {
		t.Fatal("nil capability should be disabled")
	}
	if !capabilityEnabled(true) {
		t.Fatal("bool true capability should be enabled")
	}
	if capabilityEnabled(false) {
		t.Fatal("bool false capability should be disabled")
	}
	if !capabilityEnabled(struct{}{}) {
		t.Fatal("non-bool capability should be enabled")
	}
}

func TestFormatStderrSuffix(t *testing.T) {
	if got := formatStderrSuffix(""); got != "" {
		t.Fatalf("empty stderr suffix = %q", got)
	}
	if got := formatStderrSuffix("boom"); got != ": boom" {
		t.Fatalf("stderr suffix = %q", got)
	}
}

func TestIsIgnorableSessionError(t *testing.T) {
	if !isIgnorableSessionError(io.EOF) {
		t.Fatal("EOF should be ignorable")
	}
	if !isIgnorableSessionError(context.Canceled) {
		t.Fatal("context.Canceled should be ignorable")
	}
	if isIgnorableSessionError(errors.New("boom")) {
		t.Fatal("generic errors should not be ignorable")
	}
}

func TestStartSessionReportsStderrOnInitFailure(t *testing.T) {
	rootDir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := StartSession(ctx, SessionConfig{
		Language: analyzer.LanguageGo,
		RootDir:  rootDir,
		Command: ResolvedCommand{
			Path: os.Args[0],
			Args: []string{"-test.run=TestHelperProcessBrokenLSP", "--"},
		},
		ProcessEnv: []string{"TLD_LSP_HELPER=broken"},
	})
	if err == nil {
		t.Fatal("expected initialization failure")
	}
	if got := err.Error(); !strings.Contains(got, "broken server stderr") {
		t.Fatalf("expected stderr in error, got %q", got)
	}
}

func TestHelperProcessBrokenLSP(t *testing.T) {
	if os.Getenv("TLD_LSP_HELPER") != "broken" {
		return
	}
	fmt.Fprintln(os.Stderr, "broken server stderr")
	transport := helperTransport{Reader: os.Stdin, Writer: os.Stdout}
	conn := jsonrpc2.NewConn(jsonrpc2.NewStream(transport))
	conn.Go(context.Background(), func(ctx context.Context, reply jsonrpc2.Replier, req jsonrpc2.Request) error {
		if req.Method() == protocol.MethodInitialize {
			return reply(ctx, nil, errors.New("initialize failed"))
		}
		return reply(ctx, nil, nil)
	})
	<-conn.Done()
	os.Exit(0)
}

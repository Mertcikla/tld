package lsp

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/mertcikla/diag/tld/internal/analyzer"
	"go.lsp.dev/protocol"
)

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

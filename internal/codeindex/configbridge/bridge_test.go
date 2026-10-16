package configbridge

import (
	"testing"

	"github.com/mertcikla/tld/v2/internal/workspace"
)

func TestFromGlobalUsesIndexSection(t *testing.T) {
	global := workspace.DefaultConfig()
	global.Index.Tools.SCIPGo = "/opt/scip-go"

	cfg := FromGlobal(global)
	if cfg.Tools.SCIPGo != "/opt/scip-go" {
		t.Fatalf("tool path = %q", cfg.Tools.SCIPGo)
	}
}

func TestFromGlobalNilUsesDefaults(t *testing.T) {
	cfg := FromGlobal(nil)
	if cfg.Tools.SCIPGo == "" {
		t.Fatal("expected default tool path")
	}
}

func TestFromGlobalEnvOverridesGlobal(t *testing.T) {
	global := workspace.DefaultConfig()
	global.Index.Tools.SCIPGo = "/opt/scip-go"
	t.Setenv("CODEINDEX_SCIP_GO", "/env/scip-go")

	cfg := FromGlobal(global)
	if cfg.Tools.SCIPGo != "/env/scip-go" {
		t.Fatalf("tool path = %q, want env override", cfg.Tools.SCIPGo)
	}
}

func TestGoplsConfiguration(t *testing.T) {
	global := workspace.DefaultConfig()
	global.Index.Tools.Gopls = "/opt/gopls"
	if got := FromGlobal(global).Tools.Gopls; got != "/opt/gopls" {
		t.Fatalf("gopls=%q", got)
	}
	t.Setenv("CODEINDEX_GOPLS", "/env/gopls")
	if got := FromGlobal(global).Tools.Gopls; got != "/env/gopls" {
		t.Fatalf("env gopls=%q", got)
	}
}

func TestTypeScriptLanguageServerConfiguration(t *testing.T) {
	global := workspace.DefaultConfig()
	global.Index.Tools.TypeScriptLanguageServer = "/opt/typescript-language-server"
	if got := FromGlobal(global).Tools.TypeScriptLanguageServer; got != "/opt/typescript-language-server" {
		t.Fatalf("server=%q", got)
	}
	t.Setenv("CODEINDEX_TYPESCRIPT_LANGUAGE_SERVER", "/env/typescript-language-server")
	if got := FromGlobal(global).Tools.TypeScriptLanguageServer; got != "/env/typescript-language-server" {
		t.Fatalf("env server=%q", got)
	}
}

func TestPyrightConfiguration(t *testing.T) {
	global := workspace.DefaultConfig()
	global.Index.Tools.Pyright = "/opt/pyright-langserver"
	if got := FromGlobal(global).Tools.Pyright; got != "/opt/pyright-langserver" {
		t.Fatalf("server=%q", got)
	}
	t.Setenv("CODEINDEX_PYRIGHT", "/env/pyright-langserver")
	if got := FromGlobal(global).Tools.Pyright; got != "/env/pyright-langserver" {
		t.Fatalf("env server=%q", got)
	}
}

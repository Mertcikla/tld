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

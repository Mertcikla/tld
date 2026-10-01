package configbridge

import (
	"testing"

	"github.com/mertcikla/tld/v2/internal/workspace"
)

func TestFromGlobalUsesIndexSection(t *testing.T) {
	global := workspace.DefaultConfig()
	global.Index.Embedding.Endpoint = "http://index:9000/v1/"
	global.Index.Embedding.Model = "index-model"
	global.Index.Embedding.Dimensions = 768
	global.Index.Tools.SCIPGo = "/opt/scip-go"

	cfg := FromGlobal(global)
	if cfg.Embedding.Endpoint != "http://index:9000/v1" {
		t.Fatalf("endpoint = %q, want index section", cfg.Embedding.Endpoint)
	}
	if cfg.Embedding.Model != "index-model" || cfg.Embedding.Dimensions != 768 {
		t.Fatalf("embedding config = %+v", cfg.Embedding)
	}
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

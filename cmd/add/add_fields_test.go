package add_test

import (
	"reflect"
	"testing"

	"github.com/mertcikla/tld/v2/cmd"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

func TestAddCmdPersistsExtendedFields(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)

	cmd.MustRunCmd(t, dir, "add", "Gateway", "--ref", "gateway", "--kind", "service",
		"--tags", "edge, ingress",
		"--owner", "platform",
		"--symbol", "ServeHTTP",
		"--logo-url", "https://example.test/logo.svg",
		"--density-level", "1",
		"--bypass-noise-gate=false",
	)

	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	el := ws.Elements["gateway"]
	if el == nil {
		t.Fatal("gateway element missing")
	}
	if !reflect.DeepEqual(el.Tags, []string{"edge", "ingress"}) {
		t.Fatalf("tags = %v, want [edge ingress]", el.Tags)
	}
	if el.Owner != "platform" {
		t.Fatalf("owner = %q", el.Owner)
	}
	if el.Symbol != "ServeHTTP" {
		t.Fatalf("symbol = %q", el.Symbol)
	}
	if el.LogoURL != "https://example.test/logo.svg" {
		t.Fatalf("logo_url = %q", el.LogoURL)
	}
	if el.DensityLevel != 1 {
		t.Fatalf("density_level = %d, want 1", el.DensityLevel)
	}
	if el.BypassNoiseGate == nil || *el.BypassNoiseGate {
		t.Fatalf("bypass_noise_gate = %v, want explicit false", el.BypassNoiseGate)
	}
}

func TestAddCmdDefaultsBypassNoiseGateTrue(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.MustRunCmd(t, dir, "add", "API", "--ref", "api", "--kind", "service", "--tags", "core")

	ws, err := workspace.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if el := ws.Elements["api"]; el == nil || len(el.Tags) != 1 || el.Tags[0] != "core" {
		t.Fatalf("tags not persisted: %+v", ws.Elements["api"])
	}
}

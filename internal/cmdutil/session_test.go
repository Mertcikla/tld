package cmdutil

import (
	"strings"
	"testing"

	"github.com/mertcikla/tld/v2/internal/workspace"
)

func resolveWorkspace() *workspace.Workspace {
	return &workspace.Workspace{
		Elements: map[string]*workspace.Element{
			"api-service": {Name: "API Service"},
			"db":          {Name: "Database"},
			"same-a":      {Name: "Same Name"},
			"same-b":      {Name: "Same Name"},
		},
		Connectors: map[string]*workspace.Connector{
			"root/api-service~db": {View: "root", Source: "api-service", Target: "db"},
		},
		Meta: &workspace.Meta{
			Elements: map[string]*workspace.ResourceMetadata{
				"api-service": {ID: 7},
				"db":          {ID: 8},
			},
			Views: map[string]*workspace.ResourceMetadata{
				"api-service": {ID: 9},
			},
			Connectors: map[string]*workspace.ResourceMetadata{
				"root/api-service~db": {ID: 11},
			},
		},
	}
}

func TestResolveElementArg(t *testing.T) {
	ws := resolveWorkspace()
	cases := []struct {
		name string
		arg  string
		want string
	}{
		{"ref", "api-service", "api-service"},
		{"exact name", "Database", "db"},
		{"case-insensitive name", "api service", "api-service"},
		{"element id", "7", "api-service"},
		{"view id", "9", "api-service"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveElementArg(ws, tc.arg)
			if err != nil {
				t.Fatalf("ResolveElementArg(%q) error: %v", tc.arg, err)
			}
			if got != tc.want {
				t.Fatalf("ResolveElementArg(%q) = %q, want %q", tc.arg, got, tc.want)
			}
		})
	}
}

func TestResolveElementArgErrors(t *testing.T) {
	ws := resolveWorkspace()
	if _, err := ResolveElementArg(ws, "missing"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("missing err = %v", err)
	}
	if _, err := ResolveElementArg(ws, "same name"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ambiguous err = %v", err)
	}
	if _, err := ResolveElementArg(ws, ""); err == nil {
		t.Fatal("empty arg should error")
	}
	if _, err := ResolveElementArg(nil, "api-service"); err == nil {
		t.Fatal("nil workspace should error")
	}
}

func TestResolveViewArg(t *testing.T) {
	ws := resolveWorkspace()
	for arg, want := range map[string]string{"": "root", "root": "root", "9": "api-service", "db": "db"} {
		got, err := ResolveViewArg(ws, arg)
		if err != nil {
			t.Fatalf("ResolveViewArg(%q) error: %v", arg, err)
		}
		if got != want {
			t.Fatalf("ResolveViewArg(%q) = %q, want %q", arg, got, want)
		}
	}
}

func TestResolveConnectorArg(t *testing.T) {
	ws := resolveWorkspace()
	for arg, want := range map[string]string{
		"root/api-service~db": "root/api-service~db",
		"11":                  "root/api-service~db",
	} {
		got, err := ResolveConnectorArg(ws, arg)
		if err != nil {
			t.Fatalf("ResolveConnectorArg(%q) error: %v", arg, err)
		}
		if got != want {
			t.Fatalf("ResolveConnectorArg(%q) = %q, want %q", arg, got, want)
		}
	}
	if _, err := ResolveConnectorArg(ws, "root/nope~db"); err == nil {
		t.Fatal("missing connector should error")
	}
}

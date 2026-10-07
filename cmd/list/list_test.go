package list_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mertcikla/tld/v2/cmd"

	"github.com/mertcikla/tld/v2/internal/workspace"
)

func seed(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	cmd.SeedElementWorkspace(t, dir)
	return dir
}

func TestListElements(t *testing.T) {
	dir := seed(t)
	stdout, _ := cmd.MustRunCmd(t, dir, "list", "elements")

	for _, want := range []string{"api", "db", "platform", "service", "yes"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("list elements output missing %q:\n%s", want, stdout)
		}
	}

	filtered, _ := cmd.MustRunCmd(t, dir, "list", "elements", "--kind", "database")
	if !strings.Contains(filtered, "db") || strings.Contains(filtered, "api") {
		t.Fatalf("--kind filter wrong:\n%s", filtered)
	}

	searched, _ := cmd.MustRunCmd(t, dir, "list", "elements", "--search", "platform")
	if !strings.Contains(searched, "platform") || strings.Contains(searched, "\napi\t") {
		t.Fatalf("--search filter wrong:\n%s", searched)
	}
}

func TestListConnectors(t *testing.T) {
	dir := seed(t)
	stdout, _ := cmd.MustRunCmd(t, dir, "list", "connectors")
	if !strings.Contains(stdout, "reads") || !strings.Contains(stdout, "api") || !strings.Contains(stdout, "db") {
		t.Fatalf("list connectors output wrong:\n%s", stdout)
	}

	filtered, _ := cmd.MustRunCmd(t, dir, "list", "connectors", "--view", "platform")
	if !strings.Contains(filtered, "reads") {
		t.Fatalf("--view filter wrong:\n%s", filtered)
	}
}

func TestListViews(t *testing.T) {
	dir := seed(t)
	stdout, _ := cmd.MustRunCmd(t, dir, "list", "views")
	if !strings.Contains(stdout, "platform") || !strings.Contains(stdout, "root") {
		t.Fatalf("list views output wrong:\n%s", stdout)
	}
}

func TestListViewsTree_JSONOutput(t *testing.T) {
	dir := t.TempDir()
	cmd.MustInitWorkspace(t, dir)
	seedViewsWorkspace(t, dir)

	stdout, stderr, err := cmd.RunCmd(t, dir, "list", "views", "--tree", "--format", "json")
	if err != nil {
		t.Fatalf("list views --tree --format json: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	var payload struct {
		Command string         `json:"command"`
		Status  string         `json:"status"`
		Summary map[string]int `json:"summary"`
		Extra   struct {
			Views []map[string]any `json:"views"`
		} `json:"extra"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("unmarshal json output: %v\nstdout=%s", err, stdout)
	}
	if payload.Command != "list views" || payload.Status != "ok" {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if payload.Summary["count"] != 3 || payload.Summary["total"] != 3 {
		t.Fatalf("unexpected summary: %+v", payload.Summary)
	}
	rows := make(map[string]map[string]any, len(payload.Extra.Views))
	for _, row := range payload.Extra.Views {
		ref, _ := row["ref"].(string)
		rows[ref] = row
	}
	if rows["api"]["depth"].(float64) != 2 {
		t.Fatalf("expected api depth 2, got %#v", rows["api"])
	}
	if rows["api"]["path"].(string) != "root/platform/api" {
		t.Fatalf("expected api path, got %#v", rows["api"])
	}
	if rows["platform"]["direct_elements"].(float64) != 2 {
		t.Fatalf("expected platform direct_elements 2, got %#v", rows["platform"])
	}
	if rows["root"]["synthetic"].(bool) != true {
		t.Fatalf("expected root row to be synthetic, got %#v", rows["root"])
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
}

func TestListElementsJSON(t *testing.T) {
	dir := seed(t)
	stdout, _ := cmd.MustRunCmd(t, dir, "list", "elements", "--format", "json")

	var payload struct {
		Command string `json:"command"`
		Status  string `json:"status"`
		Summary struct {
			Count int `json:"count"`
			Total int `json:"total"`
		} `json:"summary"`
		Extra struct {
			Elements []struct {
				Ref  string `json:"ref"`
				Name string `json:"name"`
			} `json:"elements"`
		} `json:"extra"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}
	if payload.Status != "ok" || payload.Summary.Count == 0 {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if len(payload.Extra.Elements) != payload.Summary.Count {
		t.Fatalf("extra.elements = %d, summary.count = %d", len(payload.Extra.Elements), payload.Summary.Count)
	}
}

func seedViewsWorkspace(t *testing.T, dir string) {
	t.Helper()
	connector := &workspace.Connector{View: "platform", Source: "api", Target: "db", Label: "reads"}
	ws := &workspace.Workspace{
		Dir: dir,
		Elements: map[string]*workspace.Element{
			"platform": {
				Name:    "Platform",
				Kind:    "workspace",
				HasView: true,
				Placements: []workspace.ViewPlacement{{
					ParentRef: "root",
				}},
			},
			"api": {
				Name:    "API",
				Kind:    "service",
				HasView: true,
				Placements: []workspace.ViewPlacement{{
					ParentRef: "platform",
				}},
			},
			"worker": {
				Name: "Worker",
				Kind: "service",
				Placements: []workspace.ViewPlacement{{
					ParentRef: "api",
				}},
			},
			"db": {
				Name: "DB",
				Kind: "database",
				Placements: []workspace.ViewPlacement{{
					ParentRef: "platform",
				}},
			},
		},
		Connectors: map[string]*workspace.Connector{
			workspace.ConnectorKey(connector): connector,
		},
	}
	if err := workspace.Save(ws); err != nil {
		t.Fatalf("save workspace: %v", err)
	}
}

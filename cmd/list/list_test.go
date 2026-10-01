package list_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mertcikla/tld/v2/cmd"
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

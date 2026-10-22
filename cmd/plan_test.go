package cmd

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPlanCLIRecordsLiveWorkspaceChanges(t *testing.T) {
	dir := t.TempDir()
	MustRunCmd(t, dir, "add", "Existing service")
	raw, _ := MustRunCmd(t, dir, "plan", "new", "--repository", "test-repo", "--name", "Feature")
	var response struct {
		Plan struct {
			ID string `json:"id"`
		} `json:"plan"`
	}
	if err := json.Unmarshal([]byte(raw), &response); err != nil {
		t.Fatal(err)
	}
	id := response.Plan.ID
	if id == "" {
		t.Fatal(raw)
	}
	MustRunCmd(t, dir, "--plan", id, "add", "Planned worker", "--technology", "go")
	scoped, _ := MustRunCmd(t, dir, "--plan", "Feature", "list", "elements")
	if !strings.Contains(scoped, "Planned worker") || strings.Contains(scoped, "Existing service") {
		t.Fatal("plan read expanded scope", scoped)
	}
	live, _ := MustRunCmd(t, dir, "list", "elements")
	if !strings.Contains(live, "Planned worker") || !strings.Contains(live, "Existing service") {
		t.Fatal("live workspace edit missing", live)
	}
	MustRunCmd(t, dir, "plan", "checklist", id, "--text", "Worker retries failed jobs")
	prompt, _ := MustRunCmd(t, dir, "plan", "prompt", "Feature")
	if !strings.Contains(prompt, "Worker retries failed jobs") || !strings.Contains(prompt, "Technology: go") {
		t.Fatal("prompt lost metadata or checklist", prompt)
	}
	MustRunCmd(t, dir, "plan", "include", "Feature", "Existing service", "--role", "preserve")
	MustRunCmd(t, dir, "plan", "new", "--repository", "test-repo", "--name", "Feature", "--resume=false")
	if _, _, err := RunCmd(t, dir, "plan", "prompt", "Feature"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("duplicate name resolved silently: %v", err)
	}
	assertNoWorkspaceFiles(t, dir)
}

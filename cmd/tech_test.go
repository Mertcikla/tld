package cmd

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTechCommandListsCatalogWithLimitOffset(t *testing.T) {
	dir := t.TempDir()

	stdout, _, err := RunCmd(t, dir, "tech", "--limit", "3", "--offset", "1")
	if err != nil {
		t.Fatalf("tech list: %v\nstdout: %s", err, stdout)
	}

	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) < 6 {
		t.Fatalf("tech list output too short:\n%s", stdout)
	}
	if !strings.HasPrefix(lines[0], "SLUG") {
		t.Fatalf("tech list missing compact table header:\n%s", stdout)
	}
	if !strings.Contains(stdout, "showing 2-4 of ") {
		t.Fatalf("tech list missing pagination footer:\n%s", stdout)
	}
	if !strings.Contains(stdout, "prev: tld tech --limit 3 --offset 0") || !strings.Contains(stdout, "next: tld tech --limit 3 --offset 4") {
		t.Fatalf("tech list missing prev/next commands:\n%s", stdout)
	}
}

func TestTechCommandJSONPagination(t *testing.T) {
	dir := t.TempDir()

	stdout, _, err := RunCmd(t, dir, "--format", "json", "--compact", "tech", "--limit", "2")
	if err != nil {
		t.Fatalf("tech json: %v\nstdout: %s", err, stdout)
	}

	var payload struct {
		Items []struct {
			Slug string `json:"slug"`
			Name string `json:"name"`
		} `json:"items"`
		Limit  int    `json:"limit"`
		Offset int    `json:"offset"`
		Count  int    `json:"count"`
		Total  int    `json:"total"`
		Next   string `json:"next"`
	}
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("unmarshal tech json: %v\n%s", err, stdout)
	}
	if payload.Limit != 2 || payload.Offset != 0 || payload.Count != 2 || payload.Total <= payload.Count || payload.Next == "" {
		t.Fatalf("unexpected tech json pagination: %+v", payload)
	}
	if payload.Items[0].Slug == "" || payload.Items[0].Name == "" {
		t.Fatalf("expected compact catalog items, got %+v", payload.Items)
	}
}

func TestTechSuggestCallsCatalogSuggestions(t *testing.T) {
	dir := t.TempDir()

	stdout, _, err := RunCmd(t, dir, "tech", "suggest", "golan", "--limit", "3")
	if err != nil {
		t.Fatalf("tech suggest: %v\nstdout: %s", err, stdout)
	}
	if !strings.Contains(stdout, "golang") {
		t.Fatalf("tech suggest did not include direct SuggestSimilar result:\n%s", stdout)
	}
}

package dev

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFixtureCommandApprovesSnapshotAndNotes(t *testing.T) {
	repo := initGitRepoNoCommit(t)
	writeFile(t, repo, "main.go", `package main

func Main() {
	helper()
}

func helper() {}
`)
	corpusDir := t.TempDir()

	cmd := NewDevCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"fixture", repo, "--corpus-dir", corpusDir, "--name", "go-main", "--approve", "--note", "covers simple go call graph"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("fixture command: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "Fixture:") || !strings.Contains(out.String(), "fixture approved") {
		t.Fatalf("unexpected fixture output:\n%s", out.String())
	}
	manifestPath := filepath.Join(corpusDir, "go_main", "fixture.json")
	snapshotPath := filepath.Join(corpusDir, "go_main", "golden", "snapshot.json")
	repoCopy := filepath.Join(corpusDir, "go_main", "repo", "main.go")
	for _, path := range []string{manifestPath, snapshotPath, repoCopy} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected fixture artifact %s: %v", path, err)
		}
	}
	var manifest fixtureManifest
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Status != "approved" || len(manifest.Notes) != 1 || manifest.Notes[0] != "covers simple go call graph" {
		t.Fatalf("unexpected fixture manifest: %+v", manifest)
	}
	var snapshot fixtureSnapshot
	data, err = os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Name != "go_main" || len(snapshot.Elements) == 0 || len(snapshot.Decisions) == 0 {
		t.Fatalf("unexpected fixture snapshot: %+v", snapshot)
	}
}

func TestFixtureCommandWritesTaxonomyPathAndMetadata(t *testing.T) {
	repo := initGitRepoNoCommit(t)
	writeFile(t, repo, "main.go", `package main

import "net/http"

func Main() {
	http.HandleFunc("/healthz", health)
}

func health(w http.ResponseWriter, r *http.Request) {}
`)
	corpusDir := t.TempDir()

	cmd := NewDevCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"fixture", repo,
		"--corpus-dir", corpusDir,
		"--name", "go-nethttp-basic-route",
		"--approve",
		"--fixture-language", "go",
		"--fixture-domain", "http",
		"--fixture-framework", "net/http",
		"--fixture-type", "basic route",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("fixture command: %v\n%s", err, out.String())
	}
	manifestPath := filepath.Join(corpusDir, "go", "http", "net_http", "basic_route", "fixture.json")
	var manifest fixtureManifest
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Language != "go" || manifest.Domain != "http" || manifest.Framework != "net_http" || manifest.Type != "basic_route" {
		t.Fatalf("unexpected taxonomy metadata: %+v", manifest)
	}
	if _, err := os.Stat(filepath.Join(corpusDir, "go", "http", "net_http", "basic_route", "golden", "snapshot.json")); err != nil {
		t.Fatalf("expected taxonomy golden snapshot: %v", err)
	}
}

func TestConformanceDiscoversNestedFixtureTaxonomy(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "typescript", "orm", "prisma", "basic_query")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writePrettyJSON(filepath.Join(dir, "fixture.json"), fixtureManifest{
		SchemaVersion: fixtureSchemaVersion,
		Name:          "basic_query",
		Status:        "approved",
		RepoPath:      "repo",
		SnapshotPath:  "golden/snapshot.json",
	}); err != nil {
		t.Fatal(err)
	}

	fixtures, err := discoverConformanceFixtures(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(fixtures) != 1 {
		t.Fatalf("expected one fixture, got %+v", fixtures)
	}
	manifest := fixtures[0].Manifest
	if manifest.Language != "typescript" || manifest.Domain != "orm" || manifest.Framework != "prisma" || manifest.Type != "basic_query" {
		t.Fatalf("expected taxonomy inferred from path, got %+v", manifest)
	}
}

func TestCompareFixtureSnapshotsReportsFactElementAndDecisionDrift(t *testing.T) {
	golden := fixtureSnapshot{
		Elements: []fixtureElement{{OwnerType: "symbol", OwnerKey: "go:a.go:function:Main", Name: "Main"}},
		Facts: []fixtureFact{{
			Type:      "http.route",
			Enricher:  "go.nethttp",
			StableKey: "route:a",
			FilePath:  "a.go",
			Name:      "GET /a",
		}},
		Decisions: []fixtureDecision{{OwnerType: "symbol", OwnerKey: "go:a.go:function:Main", Decision: "visible"}},
	}
	current := fixtureSnapshot{
		Elements: []fixtureElement{{OwnerType: "symbol", OwnerKey: "go:a.go:function:Other", Name: "Other"}},
		Facts: []fixtureFact{{
			Type:      "http.route",
			Enricher:  "go.nethttp",
			StableKey: "route:a",
			FilePath:  "a.go",
			Name:      "POST /a",
		}},
		Decisions: []fixtureDecision{{OwnerType: "symbol", OwnerKey: "go:a.go:function:Main", Decision: "hidden"}},
	}

	diff := compareFixtureSnapshots(golden, current)
	if !diff.Changed || len(diff.ChangedFacts) != 1 || len(diff.MissingElements) != 1 || len(diff.ExtraElements) != 1 || len(diff.ChangedDecisions) != 1 {
		t.Fatalf("unexpected diff: %+v", diff)
	}
}

func TestConformanceCommandWarnModeReportsPassAndDrift(t *testing.T) {
	corpusDir := t.TempDir()
	matchingRepo := initGitRepoNoCommit(t)
	writeFile(t, matchingRepo, "main.go", `package main

func Main() {}
`)
	writeApprovedFixture(t, corpusDir, matchingRepo, "go", "dependency", "stdlib", "import_inventory", "matching_fixture")

	driftRepo := initGitRepoNoCommit(t)
	writeFile(t, driftRepo, "main.go", `package main

func Main() {}
`)
	driftFixtureDir := writeApprovedFixture(t, corpusDir, driftRepo, "go", "http", "nethttp", "basic_route", "drift_fixture")
	var snapshot fixtureSnapshot
	snapshotPath := filepath.Join(driftFixtureDir, "golden", "snapshot.json")
	data, err := os.ReadFile(snapshotPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &snapshot); err != nil {
		t.Fatal(err)
	}
	snapshot.Elements = nil
	if err := writePrettyJSON(snapshotPath, snapshot); err != nil {
		t.Fatal(err)
	}

	cmd := NewDevCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"conformance", "--fixtures", corpusDir, "--mode", "warn"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("conformance command should warn without failing: %v\n%s", err, out.String())
	}
	text := out.String()
	for _, expected := range []string{"Watch fixture conformance", "By category", "go\tdependency\tstdlib\timport_inventory\t1\t0\t0", "go\thttp\tnethttp\tbasic_route\t0\t1\t0", "extra elements"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("conformance output missing %q:\n%s", expected, text)
		}
	}
}

func TestFixtureReviewManifestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	manifestPath := filepath.Join(dir, "fixture.json")
	if err := writePrettyJSON(manifestPath, fixtureManifest{
		SchemaVersion: fixtureSchemaVersion,
		Name:          "basic_route",
		Status:        "approved",
		Language:      "go",
		Domain:        "http",
		Framework:     "nethttp",
		Type:          "basic_route",
		Notes:         []string{"original note"},
		RepoPath:      "repo",
		SnapshotPath:  "golden/snapshot.json",
	}); err != nil {
		t.Fatal(err)
	}
	reviewedAt := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	item := fixtureReviewItem{
		Path: manifestPath,
		Manifest: fixtureManifest{
			ReviewStatus:   "reviewed",
			Accuracy:       "accurate",
			ReviewComments: []string{"route is represented correctly"},
			ReviewedAt:     &reviewedAt,
		},
	}
	if err := writeFixtureReviewManifest(item); err != nil {
		t.Fatal(err)
	}
	var manifest fixtureManifest
	if err := readJSONFile(manifestPath, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Status != "approved" || manifest.Language != "go" || len(manifest.Notes) != 1 {
		t.Fatalf("lost existing manifest fields: %+v", manifest)
	}
	if manifest.ReviewStatus != "reviewed" || manifest.Accuracy != "accurate" || len(manifest.ReviewComments) != 1 || manifest.ReviewedAt == nil || !manifest.ReviewedAt.Equal(reviewedAt) {
		t.Fatalf("review fields not persisted: %+v", manifest)
	}
}

func TestFixtureReviewResumeAndProgress(t *testing.T) {
	items := []fixtureReviewItem{
		{RelPath: "go/http/a", Manifest: fixtureManifest{ReviewStatus: "reviewed"}, Result: conformanceResult{Status: "pass"}},
		{RelPath: "go/http/b", Manifest: fixtureManifest{}, Result: conformanceResult{Status: "drift"}},
		{RelPath: "go/http/c", Manifest: fixtureManifest{ReviewStatus: "skipped"}, Result: conformanceResult{Status: "error"}},
	}
	model := newFixtureReviewModel(context.Background(), "", items, "", "", "")
	if model.selected != 1 {
		t.Fatalf("selected = %d, want first pending fixture", model.selected)
	}
	total, done, skipped := model.reviewTotals()
	if total != 3 || done != 1 || skipped != 1 {
		t.Fatalf("totals = %d/%d/%d, want 3/1/1", total, done, skipped)
	}
	model = newFixtureReviewModel(context.Background(), "", items, "go/http/c", "", "")
	if model.selected != 2 {
		t.Fatalf("selected with start-at = %d, want 2", model.selected)
	}
	model = newFixtureReviewModel(context.Background(), "", items, "", "skipped", "")
	visible := model.visibleIndexes()
	if len(visible) != 1 || visible[0] != 2 {
		t.Fatalf("filtered indexes = %+v, want only skipped fixture", visible)
	}
}

func writeApprovedFixture(t *testing.T, corpusDir, repo, language, domain, framework, fixtureType, name string) string {
	t.Helper()
	cmd := NewDevCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{
		"fixture", repo,
		"--corpus-dir", corpusDir,
		"--name", name,
		"--approve",
		"--fixture-language", language,
		"--fixture-domain", domain,
		"--fixture-framework", framework,
		"--fixture-type", fixtureType,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("fixture command: %v\n%s", err, out.String())
	}
	return filepath.Join(corpusDir, taxonomyValue(language), taxonomyValue(domain), taxonomyValue(framework), taxonomyValue(fixtureType))
}

func initGitRepoNoCommit(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test User")
	return dir
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func writeFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

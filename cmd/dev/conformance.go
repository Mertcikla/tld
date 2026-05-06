package dev

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	assets "github.com/mertcikla/tld"
	tldgit "github.com/mertcikla/tld/internal/git"
	"github.com/mertcikla/tld/internal/localserver"
	storepkg "github.com/mertcikla/tld/internal/store"
	watchpkg "github.com/mertcikla/tld/internal/watch"
	"github.com/spf13/cobra"
)

type conformanceOptions struct {
	FixturesDir string
	Mode        string
}

type conformanceFixture struct {
	Dir      string
	RelPath  string
	Manifest fixtureManifest
}

type conformanceResult struct {
	Fixture      conformanceFixture
	Status       string
	Error        string
	Diff         fixtureDiff
	Current      fixtureSnapshot
	Golden       fixtureSnapshot
	Approved     bool
	Experimental bool
}

type fixtureDiff struct {
	Changed           bool
	FactDelta         int
	ElementDelta      int
	MissingFacts      []string
	ExtraFacts        []string
	ChangedFacts      []string
	MissingElements   []string
	ExtraElements     []string
	ChangedElements   []string
	MissingDecisions  []string
	ExtraDecisions    []string
	ChangedDecisions  []string
	MissingViews      []string
	ExtraViews        []string
	ChangedViews      []string
	MissingConnectors []string
	ExtraConnectors   []string
	ChangedConnectors []string
}

type conformanceGroup struct {
	Language     string
	Domain       string
	Framework    string
	Type         string
	Total        int
	Passed       int
	Drifted      int
	Errored      int
	FactDelta    int
	ElementDelta int
	Experimental int
}

func newConformanceCmd() *cobra.Command {
	opts := conformanceOptions{Mode: "warn"}
	c := &cobra.Command{
		Use:   "conformance",
		Short: "Run watch fixture conformance against a golden corpus",
		Long: `Discovers fixture manifests, runs each fixture repository through the watch pipeline,
compares the generated snapshot to the approved golden snapshot, and prints a categorized report.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConformance(cmd, opts)
		},
	}
	c.Flags().StringVar(&opts.FixturesDir, "fixtures", "", "root directory of the external fixture corpus")
	c.Flags().StringVar(&opts.Mode, "mode", opts.Mode, "conformance mode: warn, strict, or threshold")
	_ = c.MarkFlagRequired("fixtures")
	return c
}

func runConformance(cmd *cobra.Command, opts conformanceOptions) error {
	mode := strings.ToLower(strings.TrimSpace(opts.Mode))
	if mode == "" {
		mode = "warn"
	}
	if mode != "warn" && mode != "strict" && mode != "threshold" {
		return fmt.Errorf("unknown conformance mode %q", opts.Mode)
	}
	fixtures, err := discoverConformanceFixtures(opts.FixturesDir)
	if err != nil {
		return err
	}
	results := make([]conformanceResult, 0, len(fixtures))
	for _, fixture := range fixtures {
		results = append(results, runConformanceFixture(cmd.Context(), fixture))
	}
	printConformanceReport(cmd.OutOrStdout(), results)
	if mode == "warn" {
		return nil
	}
	for _, result := range results {
		if result.Status == "drift" || result.Status == "error" {
			return fmt.Errorf("conformance drift detected")
		}
	}
	return nil
}

func discoverConformanceFixtures(root string) ([]conformanceFixture, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, fmt.Errorf("--fixtures is required")
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	var fixtures []conformanceFixture
	err = filepath.WalkDir(rootAbs, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if d.Name() != "fixture.json" {
			return nil
		}
		var manifest fixtureManifest
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(data, &manifest); err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		dir := filepath.Dir(path)
		inferFixtureTaxonomy(&manifest, rootAbs, dir)
		if manifest.RepoPath == "" {
			manifest.RepoPath = "repo"
		}
		if manifest.SnapshotPath == "" {
			manifest.SnapshotPath = filepath.ToSlash(filepath.Join("golden", "snapshot.json"))
		}
		if manifest.Status == "" {
			manifest.Status = "approved"
		}
		rel, err := filepath.Rel(rootAbs, dir)
		if err != nil {
			return err
		}
		fixtures = append(fixtures, conformanceFixture{Dir: dir, RelPath: filepath.ToSlash(rel), Manifest: manifest})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(fixtures, func(i, j int) bool { return fixtures[i].RelPath < fixtures[j].RelPath })
	return fixtures, nil
}

func runConformanceFixture(ctx context.Context, fixture conformanceFixture) conformanceResult {
	result := conformanceResult{Fixture: fixture}
	status := strings.ToLower(strings.TrimSpace(fixture.Manifest.Status))
	result.Approved = status == "" || status == "approved"
	result.Experimental = status == "experimental"
	if status == "rejected" {
		result.Status = "skipped"
		return result
	}
	goldenPath := filepath.Join(fixture.Dir, filepath.FromSlash(fixture.Manifest.SnapshotPath))
	if err := readJSONFile(goldenPath, &result.Golden); err != nil {
		result.Status = "error"
		result.Error = err.Error()
		return result
	}
	repoPath := filepath.Join(fixture.Dir, filepath.FromSlash(fixture.Manifest.RepoPath))
	scanPath, cleanup, err := fixtureGitRepoPath(repoPath)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		result.Status = "error"
		result.Error = err.Error()
		return result
	}
	current, err := snapshotFixtureRepo(ctx, scanPath, fixture.Manifest.Name)
	if err != nil {
		result.Status = "error"
		result.Error = err.Error()
		return result
	}
	result.Current = current
	result.Diff = compareFixtureSnapshots(result.Golden, current)
	if result.Diff.Changed {
		result.Status = "drift"
	} else {
		result.Status = "pass"
	}
	return result
}

func fixtureGitRepoPath(repoPath string) (string, func(), error) {
	if _, err := tldgit.RepoRoot(repoPath); err == nil {
		return repoPath, nil, nil
	}
	tmpDir, err := os.MkdirTemp("", "tld-watch-conformance-*")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(tmpDir) }
	dst := filepath.Join(tmpDir, "repo")
	if err := copyFixtureRepo(repoPath, dst); err != nil {
		cleanup()
		return "", nil, err
	}
	cmd := exec.Command("git", "init")
	cmd.Dir = dst
	if out, err := cmd.CombinedOutput(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("initialize fixture git repo: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return dst, cleanup, nil
}

func snapshotFixtureRepo(ctx context.Context, repoPath, name string) (fixtureSnapshot, error) {
	tmpDir, err := os.MkdirTemp("", "tld-watch-conformance-db-*")
	if err != nil {
		return fixtureSnapshot{}, err
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()
	sqliteStore, err := storepkg.Open(localserver.DatabasePath(tmpDir), assets.FS)
	if err != nil {
		return fixtureSnapshot{}, err
	}
	defer func() { _ = sqliteStore.DB().Close() }()
	watchStore := watchpkg.NewStore(sqliteStore.DB())
	once, err := watchpkg.NewRunner(watchStore).RunOnce(ctx, watchpkg.OneShotOptions{
		Path:      repoPath,
		Rescan:    true,
		Embedding: watchpkg.EmbeddingConfig{Provider: "none"},
		Settings:  resolveFixtureWatchSettings(nil, 0, 0, 0, 0, 0),
	})
	if err != nil {
		return fixtureSnapshot{}, err
	}
	return buildFixtureSnapshot(ctx, sqliteStore.DB(), watchStore, name, once)
}

func compareFixtureSnapshots(golden, current fixtureSnapshot) fixtureDiff {
	diff := fixtureDiff{
		FactDelta:    len(current.Facts) - len(golden.Facts),
		ElementDelta: len(current.Elements) - len(golden.Elements),
	}
	diff.MissingFacts, diff.ExtraFacts, diff.ChangedFacts = compareFixtureItems(golden.Facts, current.Facts, fixtureFactKey)
	diff.MissingElements, diff.ExtraElements, diff.ChangedElements = compareFixtureItems(golden.Elements, current.Elements, fixtureElementKey)
	diff.MissingDecisions, diff.ExtraDecisions, diff.ChangedDecisions = compareFixtureItems(golden.Decisions, current.Decisions, fixtureDecisionKey)
	diff.MissingViews, diff.ExtraViews, diff.ChangedViews = compareFixtureItems(golden.Views, current.Views, fixtureViewKey)
	diff.MissingConnectors, diff.ExtraConnectors, diff.ChangedConnectors = compareFixtureItems(golden.Connectors, current.Connectors, fixtureConnectorKey)
	diff.Changed = len(diff.MissingFacts)+len(diff.ExtraFacts)+len(diff.ChangedFacts)+
		len(diff.MissingElements)+len(diff.ExtraElements)+len(diff.ChangedElements)+
		len(diff.MissingDecisions)+len(diff.ExtraDecisions)+len(diff.ChangedDecisions)+
		len(diff.MissingViews)+len(diff.ExtraViews)+len(diff.ChangedViews)+
		len(diff.MissingConnectors)+len(diff.ExtraConnectors)+len(diff.ChangedConnectors) > 0
	return diff
}

func compareFixtureItems[T any](golden, current []T, key func(T) string) ([]string, []string, []string) {
	goldenMap := map[string]string{}
	currentMap := map[string]string{}
	for _, item := range golden {
		goldenMap[key(item)] = canonicalFixtureJSON(item)
	}
	for _, item := range current {
		currentMap[key(item)] = canonicalFixtureJSON(item)
	}
	var missing, extra, changed []string
	for key, goldenJSON := range goldenMap {
		currentJSON, ok := currentMap[key]
		if !ok {
			missing = append(missing, key)
			continue
		}
		if currentJSON != goldenJSON {
			changed = append(changed, key)
		}
	}
	for key := range currentMap {
		if _, ok := goldenMap[key]; !ok {
			extra = append(extra, key)
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	sort.Strings(changed)
	return missing, extra, changed
}

func fixtureFactKey(fact fixtureFact) string {
	return strings.Join([]string{fact.Type, fact.Enricher, fact.StableKey}, "|")
}

func fixtureElementKey(element fixtureElement) string {
	return element.OwnerType + "|" + element.OwnerKey
}

func fixtureDecisionKey(decision fixtureDecision) string {
	return decision.OwnerType + "|" + decision.OwnerKey
}

func fixtureViewKey(view fixtureView) string {
	return view.OwnerType + "|" + view.OwnerKey
}

func fixtureConnectorKey(connector fixtureConnector) string {
	return connector.OwnerType + "|" + connector.OwnerKey
}

func canonicalFixtureJSON(value any) string {
	data, _ := json.Marshal(value)
	return string(data)
}

func readJSONFile(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, value); err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	return nil
}

func printConformanceReport(w interface{ Write([]byte) (int, error) }, results []conformanceResult) {
	totals := conformanceTotals(results)
	_, _ = fmt.Fprintln(w, "Watch fixture conformance")
	_, _ = fmt.Fprintf(w, "Fixtures: %d scanned, %d approved, %d experimental, %d skipped\n", totals.Total, totals.Approved, totals.Experimental, totals.Skipped)
	_, _ = fmt.Fprintf(w, "Results:  %d pass, %d drift, %d error\n\n", totals.Passed, totals.Drifted, totals.Errored)
	_, _ = fmt.Fprintln(w, "By category")
	_, _ = fmt.Fprintln(w, "language\tdomain\tframework/library\ttype\tpass\tdrift\terror\tfact_delta\telement_delta")
	for _, group := range conformanceGroups(results) {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%d\t%d\t%d\t%+d\t%+d\n", group.Language, group.Domain, group.Framework, group.Type, group.Passed, group.Drifted, group.Errored, group.FactDelta, group.ElementDelta)
	}
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Fixture details")
	for _, result := range results {
		printConformanceResult(w, result)
	}
}

type conformanceTotal struct {
	Total        int
	Approved     int
	Experimental int
	Skipped      int
	Passed       int
	Drifted      int
	Errored      int
}

func conformanceTotals(results []conformanceResult) conformanceTotal {
	var total conformanceTotal
	for _, result := range results {
		total.Total++
		if result.Approved {
			total.Approved++
		}
		if result.Experimental {
			total.Experimental++
		}
		switch result.Status {
		case "pass":
			total.Passed++
		case "drift":
			total.Drifted++
		case "error":
			total.Errored++
		case "skipped":
			total.Skipped++
		}
	}
	return total
}

func conformanceGroups(results []conformanceResult) []conformanceGroup {
	groups := map[string]*conformanceGroup{}
	for _, result := range results {
		if result.Status == "skipped" {
			continue
		}
		manifest := result.Fixture.Manifest
		key := strings.Join([]string{manifest.Language, manifest.Domain, manifest.Framework, manifest.Type}, "\x00")
		group := groups[key]
		if group == nil {
			group = &conformanceGroup{Language: manifest.Language, Domain: manifest.Domain, Framework: manifest.Framework, Type: manifest.Type}
			groups[key] = group
		}
		group.Total++
		group.FactDelta += result.Diff.FactDelta
		group.ElementDelta += result.Diff.ElementDelta
		if result.Experimental {
			group.Experimental++
		}
		switch result.Status {
		case "pass":
			group.Passed++
		case "drift":
			group.Drifted++
		case "error":
			group.Errored++
		}
	}
	out := make([]conformanceGroup, 0, len(groups))
	for _, group := range groups {
		out = append(out, *group)
	}
	sort.Slice(out, func(i, j int) bool {
		left := strings.Join([]string{out[i].Language, out[i].Domain, out[i].Framework, out[i].Type}, "/")
		right := strings.Join([]string{out[j].Language, out[j].Domain, out[j].Framework, out[j].Type}, "/")
		return left < right
	})
	return out
}

func printConformanceResult(w interface{ Write([]byte) (int, error) }, result conformanceResult) {
	manifest := result.Fixture.Manifest
	_, _ = fmt.Fprintf(w, "- %s [%s/%s/%s/%s]: %s\n", result.Fixture.RelPath, manifest.Language, manifest.Domain, manifest.Framework, manifest.Type, result.Status)
	for _, note := range manifest.Notes {
		_, _ = fmt.Fprintf(w, "  note: %s\n", note)
	}
	if result.Error != "" {
		_, _ = fmt.Fprintf(w, "  error: %s\n", result.Error)
		return
	}
	if result.Status != "drift" {
		return
	}
	_, _ = fmt.Fprintf(w, "  deltas: facts %+d, elements %+d\n", result.Diff.FactDelta, result.Diff.ElementDelta)
	printFixtureDriftLines(w, "missing facts", result.Diff.MissingFacts)
	printFixtureDriftLines(w, "extra facts", result.Diff.ExtraFacts)
	printFixtureDriftLines(w, "changed facts", result.Diff.ChangedFacts)
	printFixtureDriftLines(w, "missing elements", result.Diff.MissingElements)
	printFixtureDriftLines(w, "extra elements", result.Diff.ExtraElements)
	printFixtureDriftLines(w, "changed elements", result.Diff.ChangedElements)
	printFixtureDriftLines(w, "changed decisions", result.Diff.ChangedDecisions)
	printFixtureDriftLines(w, "changed views", result.Diff.ChangedViews)
	printFixtureDriftLines(w, "changed connectors", result.Diff.ChangedConnectors)
}

func printFixtureDriftLines(w interface{ Write([]byte) (int, error) }, label string, values []string) {
	if len(values) == 0 {
		return
	}
	const limit = 5
	shown := values
	if len(shown) > limit {
		shown = shown[:limit]
	}
	_, _ = fmt.Fprintf(w, "  %s: %s", label, strings.Join(shown, ", "))
	if len(values) > limit {
		_, _ = fmt.Fprintf(w, " ... %d more", len(values)-limit)
	}
	_, _ = fmt.Fprintln(w)
}

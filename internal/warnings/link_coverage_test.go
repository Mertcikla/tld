package warnings_test

import (
	"testing"

	"github.com/mertcikla/tld/v2/internal/warnings"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

// arc206Index is a single indexed repository containing two directories.
func arc206Index(paths ...string) []warnings.LinkedRepository {
	return []warnings.LinkedRepository{{ID: "repo", Name: "tld", Root: "/tmp/tld", Paths: paths}}
}

func arc206Warnings(t *testing.T, ws *workspace.Workspace, opts ...warnings.Option) *warnings.WarningGroup {
	t.Helper()
	for _, w := range warnings.Analyze(ws, opts...) {
		if w.RuleCode == "ARC206" {
			return &w
		}
	}
	return nil
}

func TestAnalyze_ARC206FlagsUnlinkedDirectories(t *testing.T) {
	ws := &workspace.Workspace{
		Elements: map[string]*workspace.Element{
			"api": {Name: "API", Kind: "service", FilePath: "A/B/C.go"},
		},
		Config: workspace.Config{Validation: workspace.ValidationConfig{Level: 3}},
	}

	group := arc206Warnings(t, ws, warnings.WithLinkTargets(arc206Index("A/B/C.go", "A/D/E.go")))
	if group == nil {
		t.Fatal("expected ARC206 warning for a half-covered repository")
	}
	if len(group.Violations) != 1 {
		t.Fatalf("violations = %v, want one", group.Violations)
	}
	if want := `Repository "tld" is 50% linked (1/2 directories); unlinked: A/D`; group.Violations[0] != want {
		t.Fatalf("violation = %q, want %q", group.Violations[0], want)
	}
	if group.Score == nil {
		t.Fatal("expected ARC206 to carry a score")
	}
	if group.Score.Grounded != 1 || group.Score.Eligible != 2 {
		t.Fatalf("score = %+v, want 1/2", group.Score)
	}
	if len(group.Score.Reasoning) == 0 {
		t.Fatal("expected reasoning for ARC206")
	}
}

func TestAnalyze_ARC206PassesAtConfiguredThreshold(t *testing.T) {
	ws := &workspace.Workspace{
		Elements: map[string]*workspace.Element{
			"one": {Name: "One", Kind: "service", FilePath: "A/one.go"},
			"two": {Name: "Two", Kind: "service", FilePath: "B/two.go"},
		},
		Config: workspace.Config{Validation: workspace.ValidationConfig{Level: 3}},
	}

	// Two of three directories linked: 66% is below the 75% default.
	if group := arc206Warnings(t, ws, warnings.WithLinkTargets(arc206Index("A/one.go", "B/two.go", "C/three.go"))); group == nil {
		t.Fatal("expected ARC206 warning at 66% coverage")
	}
	// A workspace that lowers the threshold passes with the same links.
	ws.Config.Validation.LinkCoveragePercent = 50
	if group := arc206Warnings(t, ws, warnings.WithLinkTargets(arc206Index("A/one.go", "B/two.go", "C/three.go"))); group != nil {
		t.Fatalf("expected no ARC206 warning at a 50%% threshold, got %+v", group)
	}
}

func TestAnalyze_ARC206FolderLinkCoversSubtree(t *testing.T) {
	ws := &workspace.Workspace{
		Elements: map[string]*workspace.Element{
			"subsystem": {Name: "Subsystem", Kind: "component", FilePath: "A/B/"},
		},
		Config: workspace.Config{Validation: workspace.ValidationConfig{Level: 3}},
	}

	if group := arc206Warnings(t, ws, warnings.WithLinkTargets(arc206Index("A/B/C.go", "A/B/D/E.go", "A/D.go"))); group == nil {
		t.Fatal("expected the uncovered sibling directory to be flagged")
	}
	group := arc206Warnings(t, ws, warnings.WithLinkTargets(arc206Index("A/B/C.go", "A/B/D/E.go")))
	if group != nil {
		t.Fatalf("a folder link covers its subtree, got %+v", group)
	}
}

func TestAnalyze_ARC206IgnoresCodeindexAndExemptElements(t *testing.T) {
	classify := func(element *workspace.Element) bool { return element.Name == "generated" }
	ws := &workspace.Workspace{
		Elements: map[string]*workspace.Element{
			"generated": {Name: "generated", Kind: "file", FilePath: "A/D.go"},
			"external":  {Name: "External", Kind: "service", FilePath: "https://example.com", Tags: []string{"external"}},
			"ignored":   {Name: "Ignored", Kind: "service", FilePath: "A/E.go", Tags: []string{"$ignored"}},
			"repoOnly":  {Name: "RepoOnly", Kind: "service", Repo: "github.com/owner/repo"},
			"authored":  {Name: "Authored", Kind: "service", FilePath: "A/B/C.go#L18"},
		},
		Config: workspace.Config{Validation: workspace.ValidationConfig{Level: 3}},
	}

	targets := arc206Index("A/B/C.go", "A/D/E.go")
	group := arc206Warnings(t, ws, warnings.WithLinkTargets(targets), warnings.WithCodeindexElementClassifier(classify))
	if group == nil {
		t.Fatal("expected mapped and exempt links to be excluded from coverage")
	}
	if want := `Repository "tld" is 50% linked (1/2 directories); unlinked: A/D`; group.Violations[0] != want {
		t.Fatalf("violation = %q, want %q", group.Violations[0], want)
	}
}

func TestAnalyze_ARC206ScoresRepositoriesSeparately(t *testing.T) {
	ws := &workspace.Workspace{
		Elements: map[string]*workspace.Element{
			"x": {Name: "X", Kind: "service", RepositoryID: "repo-x", FilePath: "one/a/x.go"},
			"y": {Name: "Y", Kind: "service", RepositoryID: "repo-y", FilePath: "two/a.go"},
		},
		Config: workspace.Config{Validation: workspace.ValidationConfig{Level: 3}},
	}
	targets := []warnings.LinkedRepository{
		{ID: "repo-x", Name: "x", Root: "/tmp/x", Paths: []string{"one/a/x.go", "one/b/y.go"}},
		{ID: "repo-y", Name: "y", Root: "/tmp/y", Paths: []string{"two/a.go"}},
	}

	group := arc206Warnings(t, ws, warnings.WithLinkTargets(targets))
	if group == nil {
		t.Fatal("expected ARC206 warning for the half-covered repository")
	}
	if len(group.Violations) != 1 {
		t.Fatalf("violations = %v, want one", group.Violations)
	}
	if want := `Repository "x" is 50% linked (1/2 directories); unlinked: one/b`; group.Violations[0] != want {
		t.Fatalf("violation = %q, want %q", group.Violations[0], want)
	}
	if group.Score.Grounded != 2 || group.Score.Eligible != 3 {
		t.Fatalf("score = %+v, want 2/3 across repositories", group.Score)
	}
}

func TestAnalyze_ARC206ResolvesAbsoluteAndSingleRepositoryLinks(t *testing.T) {
	ws := &workspace.Workspace{
		Elements: map[string]*workspace.Element{
			"absolute": {Name: "Absolute", Kind: "service", FilePath: "/tmp/tld/A/B/C.go"},
			"legacy":   {Name: "Legacy", Kind: "service", Repo: "/tmp/tld", FilePath: "A/D/E.go"},
		},
		Config: workspace.Config{Validation: workspace.ValidationConfig{Level: 3}},
	}

	if group := arc206Warnings(t, ws, warnings.WithLinkTargets(arc206Index("A/B/C.go", "A/D/E.go"))); group != nil {
		t.Fatalf("expected absolute and legacy-scoped links to resolve, got %+v", group)
	}
}

func TestAnalyze_ARC206ReportsTopMostUnlinkedDirectories(t *testing.T) {
	ws := &workspace.Workspace{
		Elements: map[string]*workspace.Element{
			"root": {Name: "Root", Kind: "service", FilePath: "root.go"},
		},
		Config: workspace.Config{Validation: workspace.ValidationConfig{Level: 3}},
	}
	targets := arc206Index("a/b/c.go", "a/b/d/e.go", "a/x/y.go")

	report := warnings.LinkCoverage(ws, warnings.WithLinkTargets(targets))
	if len(report.Repos) != 1 {
		t.Fatalf("repos = %+v, want one", report.Repos)
	}
	if want := []string{"a/b", "a/x"}; len(report.Repos[0].Unlinked) != len(want) || report.Repos[0].Unlinked[0] != want[0] || report.Repos[0].Unlinked[1] != want[1] {
		t.Fatalf("unlinked = %v, want %v (nested directories pruned)", report.Repos[0].Unlinked, want)
	}
}

func TestAnalyze_ARC206HiddenBelowStrict(t *testing.T) {
	ws := &workspace.Workspace{
		Elements: map[string]*workspace.Element{
			"api": {Name: "API", Kind: "service", FilePath: "A/B/C.go"},
		},
		Config: workspace.Config{Validation: workspace.ValidationConfig{Level: 2}},
	}

	if group := arc206Warnings(t, ws, warnings.WithLinkTargets(arc206Index("A/B/C.go", "A/D.go"))); group != nil {
		t.Fatalf("ARC206 runs at strict level only, got %+v", group)
	}
}

func TestAnalyze_ARC206InertWithoutCodeindex(t *testing.T) {
	ws := &workspace.Workspace{
		Elements: map[string]*workspace.Element{
			"api": {Name: "API", Kind: "service", FilePath: "A/B/C.go"},
		},
		Config: workspace.Config{Validation: workspace.ValidationConfig{Level: 3}},
	}

	if group := arc206Warnings(t, ws); group != nil {
		t.Fatalf("expected no ARC206 without an indexed repository, got %+v", group)
	}
	report := warnings.LinkCoverage(ws)
	if report.Eligible != 0 || len(report.Reasoning) == 0 {
		t.Fatalf("report = %+v, want no repositories scored", report)
	}
}

func TestLinkCoverageThreshold(t *testing.T) {
	if got := warnings.LinkCoverageThreshold(nil); got != workspace.DefaultLinkCoveragePercent {
		t.Fatalf("default threshold = %d, want %d", got, workspace.DefaultLinkCoveragePercent)
	}
	if got := warnings.LinkCoverageThreshold(&workspace.Workspace{}); got != workspace.DefaultLinkCoveragePercent {
		t.Fatalf("unset threshold = %d, want %d", got, workspace.DefaultLinkCoveragePercent)
	}
	ws := &workspace.Workspace{Config: workspace.Config{Validation: workspace.ValidationConfig{LinkCoveragePercent: 90}}}
	if got := warnings.LinkCoverageThreshold(ws); got != 90 {
		t.Fatalf("configured threshold = %d, want 90", got)
	}
}

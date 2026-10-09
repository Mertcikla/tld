package warnings_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/mertcikla/tld/v2/internal/warnings"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

func TestAnalyze_TechnologyValidation(t *testing.T) {
	tests := []struct {
		name             string
		level            int
		technology       string
		includeRules     []string
		excludeRules     []string
		wantWarningCount int
		wantRuleCode     string
		wantRuleName     string
	}{
		{
			name:             "valid technology",
			level:            2,
			technology:       "Go, React",
			wantWarningCount: 0,
		},
		{
			name:             "invalid technology",
			level:            2,
			technology:       "UnknownTech",
			wantWarningCount: 1,
			wantRuleCode:     "ARC103",
			wantRuleName:     "Unknown Technology",
		},
		{
			name:             "mixed valid and invalid",
			level:            2,
			technology:       "Go, NonExistentTech",
			wantWarningCount: 1,
			wantRuleCode:     "ARC103",
			wantRuleName:     "Unknown Technology",
		},
		{
			name:             "multiple invalid",
			level:            2,
			technology:       "TechA / TechB",
			wantWarningCount: 1,
			wantRuleCode:     "ARC103",
			wantRuleName:     "Unknown Technology",
		},
		{
			name:             "empty technology level 2",
			level:            2,
			technology:       "",
			wantWarningCount: 1,
			wantRuleCode:     "ARC102",
			wantRuleName:     "Missing Tech",
		},
		{
			name:             "invalid technology level 1",
			level:            1,
			technology:       "UnknownTech",
			wantWarningCount: 0, // level 2 only
		},
		{
			name:             "include rule enables code outside level",
			level:            1,
			technology:       "",
			includeRules:     []string{"arc102"},
			wantWarningCount: 1,
			wantRuleCode:     "ARC102",
			wantRuleName:     "Missing Tech",
		},
		{
			name:             "exclude rule disables default code",
			level:            2,
			technology:       "",
			excludeRules:     []string{"ARC102"},
			wantWarningCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ws := &workspace.Workspace{
				Elements: map[string]*workspace.Element{
					"svc1": {
						Name:       "Service 1",
						Kind:       "service",
						Technology: tt.technology,
					},
				},
				Config: workspace.Config{
					Validation: workspace.ValidationConfig{
						Level:        tt.level,
						IncludeRules: tt.includeRules,
						ExcludeRules: tt.excludeRules,
					},
				},
			}

			archWarnings := warnings.Analyze(ws)
			count := 0
			found := false
			for _, g := range archWarnings {
				count += len(g.Violations)
				if g.RuleCode == tt.wantRuleCode && g.RuleName == tt.wantRuleName {
					found = true
				}
			}

			if count != tt.wantWarningCount {
				t.Errorf("got %d warnings, want %d", count, tt.wantWarningCount)
			}
			if tt.wantWarningCount > 0 && !found {
				t.Errorf("did not find warning rule %q (%s)", tt.wantRuleName, tt.wantRuleCode)
			}
		})
	}
}

func TestAnalyze_ARC204DuplicateNames(t *testing.T) {
	ws := &workspace.Workspace{
		Elements: map[string]*workspace.Element{
			"api":     {Name: "API", Kind: "service"},
			"api-dup": {Name: "API", Kind: "service"},
			"db":      {Name: "DB", Kind: "database"},
		},
		Config: workspace.Config{
			Validation: workspace.ValidationConfig{Level: 3},
		},
	}

	var found *warnings.WarningGroup
	archWarnings := warnings.Analyze(ws)
	for i := range archWarnings {
		if archWarnings[i].RuleCode == "ARC204" {
			found = &archWarnings[i]
		}
	}
	if found == nil {
		t.Fatalf("expected ARC204 warning, got %+v", archWarnings)
	}
	if len(found.Violations) != 1 || !strings.Contains(found.Violations[0], `"api-dup"`) {
		t.Fatalf("unexpected ARC204 violations: %+v", found.Violations)
	}
}

func TestAnalyze_ARC204HiddenBelowStrict(t *testing.T) {
	ws := &workspace.Workspace{
		Elements: map[string]*workspace.Element{
			"api":     {Name: "API", Kind: "service"},
			"api-dup": {Name: "API", Kind: "service"},
		},
		Config: workspace.Config{
			Validation: workspace.ValidationConfig{Level: 2},
		},
	}

	for _, warning := range warnings.Analyze(ws) {
		if warning.RuleCode == "ARC204" {
			t.Fatalf("expected ARC204 to be disabled below strict level, got %+v", warning)
		}
	}
}

func arc204Violations(t *testing.T, ws *workspace.Workspace) []string {
	t.Helper()
	for _, warning := range warnings.Analyze(ws) {
		if warning.RuleCode == "ARC204" {
			return warning.Violations
		}
	}
	return nil
}

func TestAnalyze_ARC204IgnoresDistinctCodePaths(t *testing.T) {
	ws := &workspace.Workspace{
		Elements: map[string]*workspace.Element{
			"types-a": {Name: "types.ts", Kind: "file", RepositoryID: "repo", Repo: "github.com/acme/app", FilePath: "frontend/src/components/ViewExplorer/types.ts"},
			"types-b": {Name: "types.ts", Kind: "file", RepositoryID: "repo", Repo: "github.com/acme/app", FilePath: "frontend/src/platform/types.ts"},
		},
		Config: workspace.Config{
			Validation: workspace.ValidationConfig{Level: 3},
		},
	}

	if violations := arc204Violations(t, ws); len(violations) != 0 {
		t.Fatalf("expected distinct code paths to be exempt, got %+v", violations)
	}
}

func TestAnalyze_ARC204FlagsSameCodePath(t *testing.T) {
	ws := &workspace.Workspace{
		Elements: map[string]*workspace.Element{
			"file":     {Name: "types.ts", Kind: "file", RepositoryID: "repo", FilePath: "frontend/src/types.ts"},
			"file-dup": {Name: "types.ts", Kind: "file", RepositoryID: "repo", FilePath: "frontend/src/types.ts"},
		},
		Config: workspace.Config{
			Validation: workspace.ValidationConfig{Level: 3},
		},
	}

	violations := arc204Violations(t, ws)
	if len(violations) != 1 || !strings.Contains(violations[0], `"file-dup"`) {
		t.Fatalf("expected duplicate source location to be flagged, got %+v", violations)
	}
}

func TestAnalyze_ARC204FlagsCodeBackedAgainstHandAuthored(t *testing.T) {
	ws := &workspace.Workspace{
		Elements: map[string]*workspace.Element{
			"types": {Name: "types.ts", Kind: "file", RepositoryID: "repo", FilePath: "frontend/src/types.ts"},
			"doc":   {Name: "types.ts", Kind: "document"},
		},
		Config: workspace.Config{
			Validation: workspace.ValidationConfig{Level: 3},
		},
	}

	violations := arc204Violations(t, ws)
	if len(violations) != 1 || !strings.Contains(violations[0], `"types"`) || !strings.Contains(violations[0], `"doc"`) {
		t.Fatalf("expected hand-authored name to collide with code-backed element, got %+v", violations)
	}
}

func TestAnalyze_ARC204SeparatesRepositoriesWithSamePath(t *testing.T) {
	ws := &workspace.Workspace{
		Elements: map[string]*workspace.Element{
			"repo-a": {Name: "main.go", Kind: "file", RepositoryID: "repo-a", FilePath: "cmd/main.go"},
			"repo-b": {Name: "main.go", Kind: "file", RepositoryID: "repo-b", FilePath: "cmd/main.go"},
		},
		Config: workspace.Config{
			Validation: workspace.ValidationConfig{Level: 3},
		},
	}

	if violations := arc204Violations(t, ws); len(violations) != 0 {
		t.Fatalf("expected elements in different repositories to be distinct, got %+v", violations)
	}
}

func TestAnalyze_ARC205HiddenBelowStrict(t *testing.T) {
	ws := &workspace.Workspace{
		Elements: map[string]*workspace.Element{
			"a": {Name: "A", Kind: "struct"},
			"b": {Name: "B", Kind: "struct"},
		},
		Config: workspace.Config{
			Validation: workspace.ValidationConfig{Level: 2},
		},
	}

	for _, warning := range warnings.Analyze(ws) {
		if warning.RuleCode == "ARC205" {
			t.Fatalf("expected ARC205 to be disabled below strict level, got %+v", warning)
		}
	}
}

func TestGrounding_CountsAllNonExternalElements(t *testing.T) {
	ws := &workspace.Workspace{
		Elements: map[string]*workspace.Element{
			"repo":   {Name: "Repo", Kind: "repository"},
			"file":   {Name: "main.go", Kind: "file", FilePath: "cmd/main.go", Symbol: "main"},
			"config": {Name: "Config", Kind: "struct"},
			"api":    {Name: "API", Kind: "service", FilePath: "api.go"},
		},
		Config: workspace.Config{
			Validation: workspace.ValidationConfig{Level: 3},
		},
	}

	// Every element counts; only external links and codeindex-owned elements
	// are ignored.
	report := warnings.Grounding(ws)
	if report.Eligible != 4 {
		t.Fatalf("eligible = %d, want 4", report.Eligible)
	}
	if report.Grounded != 2 {
		t.Fatalf("grounded = %d, want 2", report.Grounded)
	}
	if report.Value != 5 {
		t.Fatalf("score = %d, want 5", report.Value)
	}
}

func TestGrounding_ExcludesCodeindexElements(t *testing.T) {
	ws := &workspace.Workspace{
		Elements: map[string]*workspace.Element{
			// Materialized from the codeindex: linked, but excluded anyway.
			"main.go": {Name: "main.go", Kind: "file", RepositoryID: "repo-1", Repo: "github.com/acme/app", FilePath: "cmd/main.go"},
			// User-authored and ungrounded.
			"model": {Name: "Model", Kind: "struct"},
		},
		Config: workspace.Config{
			Validation: workspace.ValidationConfig{Level: 3},
		},
	}

	report := warnings.Grounding(ws, warnings.WithCodeindexElementClassifier(func(el *workspace.Element) bool {
		return el != nil && el.RepositoryID == "repo-1"
	}))
	if report.Eligible != 1 {
		t.Fatalf("eligible = %d, want 1", report.Eligible)
	}
	if report.Grounded != 0 || report.Value != 0 {
		t.Fatalf("grounded/value = %d/%d, want 0/0", report.Grounded, report.Value)
	}
}

func TestGrounding_PerViewScoresAndThreshold(t *testing.T) {
	ws := &workspace.Workspace{
		Elements: map[string]*workspace.Element{
			"platform": {Name: "Platform", Kind: "workspace", HasView: true, Placements: []workspace.ViewPlacement{{ParentRef: "root"}}},
			"handler":  {Name: "Handler", Kind: "function", FilePath: "handler.go", Symbol: "Handle", Placements: []workspace.ViewPlacement{{ParentRef: "platform"}}},
			"model":    {Name: "Model", Kind: "struct", Placements: []workspace.ViewPlacement{{ParentRef: "platform"}}},
		},
		Config: workspace.Config{
			Validation: workspace.ValidationConfig{Level: 3},
		},
	}

	report := warnings.Grounding(ws)
	var platform *warnings.ViewScore
	for i := range report.Views {
		if report.Views[i].ViewRef == "platform" {
			platform = &report.Views[i]
		}
	}
	if platform == nil {
		t.Fatalf("expected a score for view \"platform\", got %+v", report.Views)
	}
	if platform.Eligible != 2 || platform.Grounded != 1 || platform.Value != 5 {
		t.Fatalf("platform view score = %+v, want eligible=2 grounded=1 value=5", platform)
	}
	if len(platform.Ungrounded) != 1 || platform.Ungrounded[0] != "model" {
		t.Fatalf("ungrounded = %+v, want [model]", platform.Ungrounded)
	}
}

func TestAnalyze_ARC205FlagsLowGrounding(t *testing.T) {
	ws := &workspace.Workspace{
		Elements: map[string]*workspace.Element{
			"handler": {Name: "Handler", Kind: "function"},
			"model":   {Name: "Model", Kind: "struct"},
		},
		Config: workspace.Config{
			Validation: workspace.ValidationConfig{Level: 3},
		},
	}

	var found *warnings.WarningGroup
	archWarnings := warnings.Analyze(ws)
	for i := range archWarnings {
		if archWarnings[i].RuleCode == "ARC205" {
			found = &archWarnings[i]
		}
	}
	if found == nil {
		t.Fatal("expected ARC205 warning for ungrounded code-like elements")
	}
	if found.Score == nil {
		t.Fatalf("expected ARC205 to carry a score, got %+v", found)
	}
	if found.Score.Value != 0 {
		t.Fatalf("score = %d, want 0", found.Score.Value)
	}
	if len(found.Score.Reasoning) == 0 {
		t.Fatalf("expected reasoning for ARC205, got %+v", found.Score)
	}
}

func TestGrounding_ExternalTagExempt(t *testing.T) {
	ws := &workspace.Workspace{
		Elements: map[string]*workspace.Element{
			"vendored": {Name: "Vendored", Kind: "file", Repo: "github.com/acme/other", Tags: []string{"external"}},
			"local":    {Name: "Local", Kind: "struct"},
		},
		Config: workspace.Config{
			Validation: workspace.ValidationConfig{Level: 3},
		},
	}

	report := warnings.Grounding(ws)
	if report.External != 1 {
		t.Fatalf("external = %d, want 1", report.External)
	}
	if report.Eligible != 1 {
		t.Fatalf("eligible = %d, want 1", report.Eligible)
	}
	if report.Value != 0 {
		t.Fatalf("value = %d, want 0", report.Value)
	}
}

func TestGroundingDetails_OrderedByViewDepth(t *testing.T) {
	ws := &workspace.Workspace{
		Elements: map[string]*workspace.Element{
			"platform": {Name: "Platform", Kind: "workspace", HasView: true, Placements: []workspace.ViewPlacement{{ParentRef: "root"}}},
			"core":     {Name: "Core", Kind: "workspace", HasView: true, Placements: []workspace.ViewPlacement{{ParentRef: "platform"}}},
			"a":        {Name: "A", Kind: "struct", Placements: []workspace.ViewPlacement{{ParentRef: "root"}}},
			"b":        {Name: "B", Kind: "struct", Placements: []workspace.ViewPlacement{{ParentRef: "platform"}}},
			"c":        {Name: "C", Kind: "struct", Placements: []workspace.ViewPlacement{{ParentRef: "core"}}},
			"d":        {Name: "D", Kind: "struct", FilePath: "d.go", Placements: []workspace.ViewPlacement{{ParentRef: "core"}}},
		},
		Config: workspace.Config{
			Validation: workspace.ValidationConfig{Level: 3},
		},
	}

	_, details := warnings.GroundingDetails(ws)
	got := make([]string, 0, len(details))
	byRef := make(map[string]warnings.GroundingElement, len(details))
	for _, detail := range details {
		got = append(got, detail.Ref)
		byRef[detail.Ref] = detail
	}
	if want := []string{"a", "platform", "b", "core", "c", "d"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
	if byRef["a"].Depth != 0 || byRef["b"].Depth != 1 || byRef["c"].Depth != 2 {
		t.Fatalf("depths = a:%d b:%d c:%d", byRef["a"].Depth, byRef["b"].Depth, byRef["c"].Depth)
	}
	if !byRef["d"].Grounded {
		t.Fatalf("expected d to be grounded")
	}
}

func TestAnalyze_DeadEndDrilldownUsesOwnedViews(t *testing.T) {
	ws := &workspace.Workspace{
		Elements: map[string]*workspace.Element{
			"platform": {
				Name:       "Platform",
				Kind:       "workspace",
				HasView:    true,
				Placements: []workspace.ViewPlacement{{ParentRef: "root"}},
			},
		},
		Config: workspace.Config{
			Validation: workspace.ValidationConfig{Level: 1},
		},
	}

	archWarnings := warnings.Analyze(ws)
	found := false
	for _, warning := range archWarnings {
		if warning.RuleCode == "ARC006" {
			found = true
			break
		}
	}

	if !found {
		t.Fatalf("expected ARC006 warning for owned view with no content, archWarnings=%+v", archWarnings)
	}
}

func TestAnalyze_ARC002ExemptsRootSingleSystemContext(t *testing.T) {
	ws := &workspace.Workspace{
		Elements: map[string]*workspace.Element{
			"catch2": {
				Name:       "Catch2",
				Kind:       "system",
				Placements: []workspace.ViewPlacement{{ParentRef: "root"}},
			},
		},
		Connectors: map[string]*workspace.Connector{},
		Config: workspace.Config{
			Validation: workspace.ValidationConfig{Level: 1},
		},
	}

	archWarnings := warnings.Analyze(ws)
	for _, warning := range archWarnings {
		if warning.RuleCode == "ARC002" || warning.RuleCode == "ARC005" {
			t.Fatalf("expected %s to be exempt for root single-system context, got %+v", warning.RuleCode, warning)
		}
	}
}

func TestAnalyze_ARC002StillFlagsNonRootIsolatedElement(t *testing.T) {
	ws := &workspace.Workspace{
		Elements: map[string]*workspace.Element{
			"platform": {
				Name:       "Platform",
				Kind:       "workspace",
				HasView:    true,
				Placements: []workspace.ViewPlacement{{ParentRef: "root"}},
			},
			"other": {
				Name:       "Other",
				Kind:       "workspace",
				HasView:    true,
				Placements: []workspace.ViewPlacement{{ParentRef: "root"}},
			},
			"api": {
				Name:       "API",
				Kind:       "service",
				Placements: []workspace.ViewPlacement{{ParentRef: "platform"}},
			},
		},
		Connectors: map[string]*workspace.Connector{},
		Config: workspace.Config{
			Validation: workspace.ValidationConfig{Level: 1},
		},
	}

	archWarnings := warnings.Analyze(ws)
	found := false
	for _, warning := range archWarnings {
		if warning.RuleCode != "ARC002" {
			continue
		}
		for _, violation := range warning.Violations {
			if strings.Contains(violation, "\"api\"") {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("expected ARC002 violation for isolated non-root element, archWarnings=%+v", archWarnings)
	}
}

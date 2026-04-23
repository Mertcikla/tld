package planner_test

import (
	"strings"
	"testing"

	"github.com/mertcikla/tld/internal/planner"
	"github.com/mertcikla/tld/internal/workspace"
)

func reportWorkspace() *workspace.Workspace {
	return &workspace.Workspace{
		Elements: map[string]*workspace.Element{
			"platform": {Name: "Platform", Kind: "workspace", HasView: true, ViewLabel: "System"},
			"api": {
				Name:       "API",
				Kind:       "service",
				HasView:    true,
				Placements: []workspace.ViewPlacement{{ParentRef: "platform"}},
			},
			"db": {
				Name:       "DB",
				Kind:       "database",
				Placements: []workspace.ViewPlacement{{ParentRef: "platform"}},
			},
		},
		Connectors: map[string]*workspace.Connector{
			"platform:api:db:reads": {View: "platform", Source: "api", Target: "db", Label: "reads", Direction: "forward"},
		},
	}
}

func buildPlan(t *testing.T, w *workspace.Workspace) *planner.Plan {
	t.Helper()
	plan, err := planner.Build(w, false)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return plan
}

func TestRenderPlanMarkdown_Header(t *testing.T) {
	w := reportWorkspace()
	plan := buildPlan(t, w)
	var buf strings.Builder
	planner.RenderPlanMarkdown(&buf, plan, w, false)
	out := buf.String()
	if !strings.Contains(out, "Plan: 6 to create (+), 0 to update (~), 0 to delete (-)") {
		t.Fatalf("missing plan summary: %q", out)
	}
	if !strings.Contains(out, "# Element Plan") {
		t.Fatalf("missing header: %q", out)
	}
	if strings.Contains(out, "## View Structure") {
		t.Fatalf("view structure should be hidden when not verbose: %q", out)
	}
	if !strings.Contains(out, "## Summary") {
		t.Fatalf("missing summary section: %q", out)
	}
}

func TestRenderPlanMarkdown_SummaryTableCounts(t *testing.T) {
	w := reportWorkspace()
	plan := buildPlan(t, w)
	var buf strings.Builder
	planner.RenderPlanMarkdown(&buf, plan, w, false)
	out := buf.String()
	if !strings.Contains(out, "| Elements   |     3 |") {
		t.Fatalf("wrong element count: %q", out)
	}
	if !strings.Contains(out, "| Views      |     2 |") {
		t.Fatalf("wrong view count: %q", out)
	}
	if !strings.Contains(out, "| Connectors |     1 |") {
		t.Fatalf("wrong connector count: %q", out)
	}
}

func TestRenderPlanMarkdown_ViewStructure(t *testing.T) {
	w := reportWorkspace()
	plan := buildPlan(t, w)

	t.Run("hidden when not verbose", func(t *testing.T) {
		var buf strings.Builder
		planner.RenderPlanMarkdown(&buf, plan, w, false)
		out := buf.String()
		if strings.Contains(out, "## View Structure") {
			t.Fatalf("View Structure should be hidden: %q", out)
		}
	})

	t.Run("visible when verbose", func(t *testing.T) {
		var buf strings.Builder
		planner.RenderPlanMarkdown(&buf, plan, w, true)
		out := buf.String()
		if !strings.Contains(out, "## View Structure") {
			t.Fatalf("View Structure should be visible: %q", out)
		}
		if !strings.Contains(out, "platform") || !strings.Contains(out, "api") || !strings.Contains(out, "db") {
			t.Fatalf("missing element tree entries: %q", out)
		}
		if !strings.Contains(out, "[view]") {
			t.Fatalf("missing view marker: %q", out)
		}
	})
}

func TestRenderPlanMarkdown_VerboseSections(t *testing.T) {
	w := reportWorkspace()
	plan := buildPlan(t, w)
	var buf strings.Builder
	planner.RenderPlanMarkdown(&buf, plan, w, true)
	out := buf.String()
	if !strings.Contains(out, "## Actions") {
		t.Fatalf("missing actions section: %q", out)
	}
	if !strings.Contains(out, "+ element api") || !strings.Contains(out, "+ connector platform:api:db:reads") {
		t.Fatalf("missing action lines: %q", out)
	}
}

func TestRenderPlanMarkdown_VerboseHint(t *testing.T) {
	w := reportWorkspace()
	plan := buildPlan(t, w)
	var buf strings.Builder
	planner.RenderPlanMarkdown(&buf, plan, w, false)
	out := buf.String()
	if strings.Contains(out, "## Elements") {
		t.Fatalf("verbose elements section should be hidden: %q", out)
	}
}

func TestRenderPlanMarkdown_EmptyWorkspace(t *testing.T) {
	w := &workspace.Workspace{Elements: map[string]*workspace.Element{}, Connectors: map[string]*workspace.Connector{}}
	plan := buildPlan(t, w)
	var buf strings.Builder
	planner.RenderPlanMarkdown(&buf, plan, w, false)
	out := buf.String()
	if !strings.Contains(out, "# Element Plan") {
		t.Fatalf("expected header for empty workspace: %q", out)
	}
}

func TestRenderPlanMarkdown_UsesUpdatePrefixesWhenMetadataExists(t *testing.T) {
	w := reportWorkspace()
	w.Meta = &workspace.Meta{
		Elements:   map[string]*workspace.ResourceMetadata{"platform": {}, "api": {}, "db": {}},
		Views:      map[string]*workspace.ResourceMetadata{"platform": {}, "api": {}},
		Connectors: map[string]*workspace.ResourceMetadata{"platform:api:db:reads": {}},
	}
	plan := buildPlan(t, w)
	var buf strings.Builder
	planner.RenderPlanMarkdown(&buf, plan, w, true)
	out := buf.String()
	if !strings.Contains(out, "Plan: 0 to create (+), 6 to update (~), 0 to delete (-)") {
		t.Fatalf("wrong update summary: %q", out)
	}
	if !strings.Contains(out, "~ element api") || !strings.Contains(out, "~ connector platform:api:db:reads") {
		t.Fatalf("missing update prefixes: %q", out)
	}
}

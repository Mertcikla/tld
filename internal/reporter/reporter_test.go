package reporter_test

import (
	"strings"
	"testing"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	"github.com/mertcikla/tld/internal/planner"
	"github.com/mertcikla/tld/internal/reporter"
	"github.com/mertcikla/tld/internal/workspace"
)

func emptyPlan(t *testing.T) *planner.Plan {
	t.Helper()
	plan, err := planner.Build(&workspace.Workspace{
		Elements:   map[string]*workspace.Element{},
		Connectors: map[string]*workspace.Connector{},
	}, false)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return plan
}

func TestRenderExecutionMarkdown_SuccessHeader(t *testing.T) {
	var buf strings.Builder
	reporter.RenderExecutionMarkdown(&buf, emptyPlan(t), nil, true, false)
	if !strings.Contains(buf.String(), "## Status: SUCCESS") {
		t.Errorf("missing SUCCESS header: %q", buf.String())
	}
}

func TestRenderExecutionMarkdown_FailureHeader(t *testing.T) {
	var buf strings.Builder
	reporter.RenderExecutionMarkdown(&buf, emptyPlan(t), nil, false, false)
	if !strings.Contains(buf.String(), "## Status: ROLLED BACK") {
		t.Errorf("missing ROLLED BACK header: %q", buf.String())
	}
}

func TestRenderExecutionMarkdown_NilResponseNoCrash(t *testing.T) {
	var buf strings.Builder
	// Should not panic
	reporter.RenderExecutionMarkdown(&buf, emptyPlan(t), nil, false, false)
	out := buf.String()
	if !strings.Contains(out, "## Status:") {
		t.Errorf("expected status line: %q", out)
	}
}

func TestRenderExecutionMarkdown_SummaryTable(t *testing.T) {
	resp := &diagv1.ApplyPlanResponse{
		Summary: &diagv1.PlanSummary{
			ElementsPlanned:   3,
			ElementsCreated:   3,
			ViewsPlanned:      2,
			ViewsCreated:      2,
			ConnectorsPlanned: 1,
			ConnectorsCreated: 1,
		},
	}
	var buf strings.Builder
	reporter.RenderExecutionMarkdown(&buf, emptyPlan(t), resp, true, false)
	out := buf.String()

	if !strings.Contains(out, "## Planned vs Created") {
		t.Errorf("missing summary section: %q", out)
	}
	if !strings.Contains(out, "| Elements | 3 | 3 |") {
		t.Errorf("wrong element count: %q", out)
	}
	if !strings.Contains(out, "| Views | 2 | 2 |") {
		t.Errorf("wrong view count: %q", out)
	}
}

func TestRenderExecutionMarkdown_CreatedViewsSection(t *testing.T) {
	resp := &diagv1.ApplyPlanResponse{
		CreatedViews: []*diagv1.ViewSummary{
			{Id: 1, Name: "System"},
			{Id: 2, Name: "Container"},
		},
	}
	var buf strings.Builder
	reporter.RenderExecutionMarkdown(&buf, emptyPlan(t), resp, true, true)
	out := buf.String()

	if !strings.Contains(out, "### Views") {
		t.Errorf("missing Views section: %q", out)
	}
	if !strings.Contains(out, "1") || !strings.Contains(out, "System") {
		t.Errorf("view 1 not in output: %q", out)
	}
	if !strings.Contains(out, "2") || !strings.Contains(out, "Container") {
		t.Errorf("view 2 not in output: %q", out)
	}
}

func TestRenderExecutionMarkdown_CreatedSectionsAbsentOnFailure(t *testing.T) {
	resp := &diagv1.ApplyPlanResponse{
		CreatedViews: []*diagv1.ViewSummary{{Id: 1, Name: "System"}},
	}
	var buf strings.Builder
	reporter.RenderExecutionMarkdown(&buf, emptyPlan(t), resp, false, true)
	out := buf.String()

	if strings.Contains(out, "## Created Resources") {
		t.Errorf("Created Resources should be absent on failure: %q", out)
	}
}

func TestRenderExecutionMarkdown_CreatedElementsSection(t *testing.T) {
	resp := &diagv1.ApplyPlanResponse{
		CreatedElements: []*diagv1.Element{
			{Id: 10, Name: "API Gateway", Kind: new("service")},
		},
	}
	var buf strings.Builder
	reporter.RenderExecutionMarkdown(&buf, emptyPlan(t), resp, true, true)
	out := buf.String()

	if !strings.Contains(out, "### Elements") {
		t.Errorf("missing Elements section: %q", out)
	}
	if !strings.Contains(out, "10") || !strings.Contains(out, "API Gateway") {
		t.Errorf("object not in output: %q", out)
	}
}

func TestRenderExecutionMarkdown_DriftSection(t *testing.T) {
	resp := &diagv1.ApplyPlanResponse{
		Drift: []*diagv1.PlanDriftItem{
			{ResourceType: "diagram", Ref: "old-diag", Reason: "name changed"},
		},
	}
	var buf strings.Builder
	reporter.RenderExecutionMarkdown(&buf, emptyPlan(t), resp, true, false)
	out := buf.String()

	if !strings.Contains(out, "## Drift") {
		t.Errorf("missing Drift section: %q", out)
	}
	if !strings.Contains(out, "diagram") || !strings.Contains(out, "old-diag") || !strings.Contains(out, "name changed") {
		t.Errorf("drift item not in output: %q", out)
	}
}

func TestRenderExecutionMarkdown_DriftAbsent(t *testing.T) {
	resp := &diagv1.ApplyPlanResponse{
		Drift: []*diagv1.PlanDriftItem{},
	}
	var buf strings.Builder
	reporter.RenderExecutionMarkdown(&buf, emptyPlan(t), resp, true, false)
	out := buf.String()

	if strings.Contains(out, "## Drift") {
		t.Errorf("Drift section should be absent when empty: %q", out)
	}
}

func TestRenderExecutionMarkdown_CreatedConnectorsSection(t *testing.T) {
	resp := &diagv1.ApplyPlanResponse{
		CreatedConnectors: []*diagv1.Connector{
			{Id: 100, SourceElementId: 10, TargetElementId: 20},
		},
	}
	var buf strings.Builder
	reporter.RenderExecutionMarkdown(&buf, emptyPlan(t), resp, true, true)
	out := buf.String()

	if !strings.Contains(out, "### Connectors") {
		t.Errorf("missing Connectors section: %q", out)
	}
	if !strings.Contains(out, "100") {
		t.Errorf("edge ID not in output: %q", out)
	}
}

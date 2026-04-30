package planner_test

import (
	"testing"

	"github.com/mertcikla/tld/internal/planner"
	"github.com/mertcikla/tld/internal/workspace"
)

func TestAnalyzePlan_TechnologyValidation(t *testing.T) {
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

			warnings := planner.AnalyzePlan(ws)
			count := 0
			found := false
			for _, g := range warnings {
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

func TestAnalyzePlan_DeadEndDrilldownUsesOwnedViews(t *testing.T) {
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

	warnings := planner.AnalyzePlan(ws)
	found := false
	for _, warning := range warnings {
		if warning.RuleCode == "ARC006" {
			found = true
			break
		}
	}

	if !found {
		t.Fatalf("expected ARC006 warning for owned view with no content, warnings=%+v", warnings)
	}
}

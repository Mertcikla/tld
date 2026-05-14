package planner

import (
	"fmt"
	"strings"

	"github.com/mertcikla/tld/v2/internal/tech"
	"github.com/mertcikla/tld/v2/internal/workspace"
)

// WarningGroup represents a collection of similar architectural warnings.
type WarningGroup struct {
	RuleCode    string
	RuleName    string
	Description string
	Mediation   string
	Violations  []string
}

type warningRule struct {
	Code        string
	Name        string
	Description string
	Mediation   string
	Level       int
	Check       func(ctx *warningContext, rule warningRule)
}

var warningRules = []warningRule{
	{
		Code:        "ARC001",
		Name:        "High Density",
		Description: "View exceeds the element density limit",
		Mediation:   "Split the view into nested views to reduce cognitive load.",
		Level:       1,
		Check: func(ctx *warningContext, rule warningRule) {
			for viewRef, elements := range ctx.viewElements {
				densityLimit := 20
				if ctx.level >= 2 {
					densityLimit = 15
				}
				if len(elements) > densityLimit {
					ctx.addWarning(rule.Code, fmt.Sprintf("View %q has %d elements", viewRef, len(elements)))
				}
			}
		},
	},
	{
		Code:        "ARC002",
		Name:        "Isolated Element",
		Description: "Element has 0 connectors in a view",
		Mediation:   "Explore its relationships further and add connectors in the view where it appears.",
		Level:       1,
		Check: func(ctx *warningContext, rule warningRule) {
			for elementRef, element := range ctx.ws.Elements {
				if element == nil || len(element.Placements) != 1 {
					continue
				}
				for _, placement := range element.Placements {
					viewRef := normalizeWarningViewRef(placement.ParentRef)
					if ctx.elementViews[elementRef][viewRef] == 0 {
						ctx.addWarning(rule.Code, fmt.Sprintf("Element %q in View %q", elementRef, viewRef))
					}
				}
			}
		},
	},
	{
		Code:        "ARC003",
		Name:        "Shared Context",
		Description: "Shared element has no connectors in a specific view",
		Mediation:   "Add connectors to the shared element in this view or remove the placement from this view.",
		Level:       1,
		Check: func(ctx *warningContext, rule warningRule) {
			for elementRef, element := range ctx.ws.Elements {
				if element == nil || len(element.Placements) <= 1 {
					continue
				}
				for _, placement := range element.Placements {
					viewRef := normalizeWarningViewRef(placement.ParentRef)
					if ctx.elementViews[elementRef][viewRef] == 0 {
						ctx.addWarning(rule.Code, fmt.Sprintf("Element %q in View %q", elementRef, viewRef))
					}
				}
			}
		},
	},
	{
		Code:        "ARC004",
		Name:        "Depth Mismatch",
		Description: "View hierarchy is flat",
		Mediation:   "Create nested views to establish a zoomable hierarchy.",
		Level:       1,
		Check: func(ctx *warningContext, rule warningRule) {
			viewCount := 0
			for _, element := range ctx.ws.Elements {
				if element != nil && element.HasView {
					viewCount++
				}
			}
			if ctx.maxDepth < 1 && viewCount > 1 {
				ctx.addWarning(rule.Code, "Workspace views")
			}
		},
	},
	{
		Code:        "ARC005",
		Name:        "Low Insight Ratio",
		Description: "Connectors < Elements",
		Mediation:   "Add more connectors to illustrate how elements interact, rather than just listing them.",
		Level:       1,
		Check: func(ctx *warningContext, rule warningRule) {
			if ctx.allowLowInsight {
				return
			}
			for viewRef, elements := range ctx.viewElements {
				if len(elements) == 0 {
					continue
				}

				// Information-heavy views (types, structs, interfaces) often have low connectivity.
				// If more than 80% of elements are structural, we exempt the view.
				structuralCount := 0
				for _, ref := range elements {
					if el := ctx.ws.Elements[ref]; el != nil {
						kind := strings.ToLower(el.Kind)
						if kind == "struct" || kind == "interface" || kind == "type" || kind == "file" || kind == "folder" {
							structuralCount++
						}
					}
				}
				if structuralCount > len(elements)*8/10 {
					continue
				}

				connectorCount := ctx.viewConnectors[viewRef]
				if connectorCount*2 < len(elements) {
					ctx.addWarning(rule.Code, fmt.Sprintf("View %q (Elements: %d, Connectors: %d)", viewRef, len(elements), connectorCount))
				}
			}
		},
	},
	{
		Code:        "ARC006",
		Name:        "Dead-End Drilldown",
		Description: "Element owns a view but it has no content",
		Mediation:   "Add nested elements or connectors to the element's view.",
		Level:       1,
		Check: func(ctx *warningContext, rule warningRule) {
			for ref, element := range ctx.ws.Elements {
				if element == nil || !element.HasView {
					continue
				}
				if len(ctx.viewElements[ref]) == 0 && ctx.viewConnectors[ref] == 0 {
					ctx.addWarning(rule.Code, fmt.Sprintf("Element %q owns an empty view", ref))
				}
			}
		},
	},
	{
		Code:        "ARC007",
		Name:        "Abstraction Leak",
		Description: "Implementation types at Root level",
		Mediation:   "Move functions and classes into sub-diagrams. Keep root views at the Service/Subsystem level.",
		Level:       1,
		Check: func(ctx *warningContext, rule warningRule) {
			for elementRef, element := range ctx.ws.Elements {
				if element == nil {
					continue
				}
				isRootLevel := len(element.Placements) == 0
				for _, placement := range element.Placements {
					if normalizeWarningViewRef(placement.ParentRef) == syntheticRootViewRef {
						isRootLevel = true
						break
					}
				}
				if isRootLevel {
					lowerType := strings.ToLower(element.Kind)
					if lowerType == "function" || lowerType == "class" {
						ctx.addWarning(rule.Code, fmt.Sprintf("Element %q (Kind: %s)", elementRef, element.Kind))
					}
				}
			}
		},
	},
	{
		Code:        "ARC101",
		Name:        "Generic Labels",
		Description: "Connector label is overly generic",
		Mediation:   "Replace generic labels with domain-specific verbs like 'validates JWT' or 'SQL Query'.",
		Level:       3,
		Check: func(ctx *warningContext, rule warningRule) {
			for connectorRef, connector := range ctx.ws.Connectors {
				if connector != nil && isGenericLabel(connector.Label) {
					ctx.addWarning(rule.Code, fmt.Sprintf("Connector %q in View %q (Label: %q)", connectorRef, normalizeWarningViewRef(connector.View), connector.Label))
				}
			}
		},
	},
	{
		Code:        "ARC102",
		Name:        "Missing Tech",
		Description: "No `technology` field",
		Mediation:   "Add a 'technology' field to the following elements. (e.g. Go, React)",
		Level:       2,
		Check: func(ctx *warningContext, rule warningRule) {
			for elementRef, element := range ctx.ws.Elements {
				if element != nil && element.Technology == "" {
					// Repositories are structural and don't require a specific technology tag.
					if strings.ToLower(element.Kind) == "repository" {
						continue
					}
					ctx.addWarning(rule.Code, fmt.Sprintf("%q", elementRef))
				}
			}
		},
	},
	{
		Code:        "ARC103",
		Name:        "Unknown Technology",
		Description: "Catalog mismatch",
		Mediation:   "Use recognized technology names (e.g. Go, React) or double check spelling.",
		Level:       2,
		Check: func(ctx *warningContext, rule warningRule) {
			for elementRef, element := range ctx.ws.Elements {
				if element == nil || element.Technology == "" {
					continue
				}
				missing := tech.Validate(element.Technology)
				if len(missing) > 0 {
					ctx.addWarning(rule.Code, fmt.Sprintf("Element %q has unknown: %s", elementRef, strings.Join(missing, ", ")))
				}
			}
		},
	},
	{
		Code:        "ARC201",
		Name:        "Missing Desc",
		Description: "`description` field is empty",
		Mediation:   "Add a one-sentence summary to help readers understand the responsibility.",
		Level:       3,
		Check: func(ctx *warningContext, rule warningRule) {
			for elementRef, element := range ctx.ws.Elements {
				if element != nil && element.Description == "" {
					ctx.addWarning(rule.Code, fmt.Sprintf("Element %q", elementRef))
				}
			}
		},
	},
	{
		Code:        "ARC202",
		Name:        "Generic Naming",
		Description: "Vague names make the map harder to understand",
		Mediation:   "Rename the element with a domain-specific, descriptive name.",
		Level:       3,
		Check: func(ctx *warningContext, rule warningRule) {
			for elementRef, element := range ctx.ws.Elements {
				if element != nil && isGenericName(element.Name) {
					ctx.addWarning(rule.Code, fmt.Sprintf("Element %q (Name: %q)", elementRef, element.Name))
				}
			}
		},
	},
	{
		Code:        "ARC203",
		Name:        "Missing Label",
		Description: "Connector has no `label`",
		Mediation:   "A connector without a label is just a line. Add a 'label' field to tell what it does.",
		Level:       3,
		Check: func(ctx *warningContext, rule warningRule) {
			for connectorRef, connector := range ctx.ws.Connectors {
				if connector != nil && connector.Label == "" {
					ctx.addWarning(rule.Code, fmt.Sprintf("Connector %q in View %q", connectorRef, normalizeWarningViewRef(connector.View)))
				}
			}
		},
	},
}

type warningContext struct {
	ws              *workspace.Workspace
	level           int
	allowLowInsight bool
	activeRules     []warningRule
	violations      map[string][]string
	viewElements    map[string][]string
	elementViews    map[string]map[string]int
	viewConnectors  map[string]int
	maxDepth        int
}

// AnalyzePlan evaluates the workspace against architectural best practices and
// returns grouped warnings based on the configured strictness level.
func AnalyzePlan(ws *workspace.Workspace) []WarningGroup {
	if ws == nil {
		return nil
	}

	ctx := newWarningContext(ws)
	ctx.prepareData()
	ctx.checkAll()

	return ctx.toSlice()
}

func newWarningContext(ws *workspace.Workspace) *warningContext {
	level := ws.Config.Validation.Level
	allowLowInsight := ws.Config.Validation.AllowLowInsight
	includeRules := ws.Config.Validation.IncludeRules
	excludeRules := ws.Config.Validation.ExcludeRules

	return &warningContext{
		ws:              ws,
		level:           level,
		allowLowInsight: allowLowInsight,
		activeRules:     resolveConfiguredWarningRules(level, includeRules, excludeRules),
		violations:      make(map[string][]string),
		viewElements:    make(map[string][]string),
		elementViews:    make(map[string]map[string]int),
		viewConnectors:  make(map[string]int),
	}
}

func resolveConfiguredWarningRules(level int, includeRules, excludeRules []string) []warningRule {
	includeMap := make(map[string]bool)
	for _, code := range includeRules {
		includeMap[normalizeWarningRuleCode(code)] = true
	}
	excludeMap := make(map[string]bool)
	for _, code := range excludeRules {
		excludeMap[normalizeWarningRuleCode(code)] = true
	}

	var resolved []warningRule
	for _, rule := range warningRules {
		isEnabled := rule.Level <= level
		if includeMap[rule.Code] {
			isEnabled = true
		}
		if excludeMap[rule.Code] {
			isEnabled = false
		}
		if isEnabled {
			resolved = append(resolved, rule)
		}
	}
	return resolved
}

func normalizeWarningRuleCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

func (ctx *warningContext) addWarning(code, violation string) {
	ctx.violations[code] = append(ctx.violations[code], violation)
}

func (ctx *warningContext) prepareData() {
	for elementRef, element := range ctx.ws.Elements {
		ctx.elementViews[elementRef] = make(map[string]int)
		for _, placement := range element.Placements {
			viewRef := normalizeWarningViewRef(placement.ParentRef)
			ctx.viewElements[viewRef] = append(ctx.viewElements[viewRef], elementRef)
		}
	}

	for _, connector := range ctx.ws.Connectors {
		viewRef := normalizeWarningViewRef(connector.View)
		ctx.viewConnectors[viewRef]++
		if elementViews, ok := ctx.elementViews[connector.Source]; ok {
			elementViews[viewRef]++
		}
		if elementViews, ok := ctx.elementViews[connector.Target]; ok {
			elementViews[viewRef]++
		}
	}

	ctx.calculateMaxDepth()
}

func (ctx *warningContext) calculateMaxDepth() {
	memo := make(map[string]int)
	visiting := make(map[string]bool)
	var viewDepth func(string) int
	viewDepth = func(ref string) int {
		if depth, ok := memo[ref]; ok {
			return depth
		}
		if visiting[ref] {
			return 0
		}
		visiting[ref] = true
		maxDepth := 0
		element := ctx.ws.Elements[ref]
		if element != nil {
			for _, placement := range element.Placements {
				parentRef := normalizeWarningViewRef(placement.ParentRef)
				if parentRef == syntheticRootViewRef {
					continue
				}
				depth := 1
				if parentElement, ok := ctx.ws.Elements[parentRef]; ok && parentElement.HasView {
					depth = viewDepth(parentRef) + 1
				}
				if depth > maxDepth {
					maxDepth = depth
				}
			}
		}
		visiting[ref] = false
		memo[ref] = maxDepth
		return maxDepth
	}

	for ref, element := range ctx.ws.Elements {
		if element == nil || !element.HasView {
			continue
		}
		depth := viewDepth(ref)
		if depth > ctx.maxDepth {
			ctx.maxDepth = depth
		}
	}
}

func (ctx *warningContext) checkAll() {
	for _, rule := range ctx.activeRules {
		ctx.runWarningRule(rule)
	}
}

func (ctx *warningContext) runWarningRule(rule warningRule) {
	if rule.Check != nil {
		rule.Check(ctx, rule)
	}
}

func normalizeWarningViewRef(ref string) string {
	if ref == "" || ref == "root" {
		return syntheticRootViewRef
	}
	return ref
}

func (ctx *warningContext) toSlice() []WarningGroup {
	var result []WarningGroup
	for _, rule := range warningRules {
		if violations, ok := ctx.violations[rule.Code]; ok {
			result = append(result, WarningGroup{
				RuleCode:    rule.Code,
				RuleName:    rule.Name,
				Description: rule.Description,
				Mediation:   rule.Mediation,
				Violations:  violations,
			})
		}
	}
	return result
}

func isGenericName(name string) bool {
	lower := strings.ToLower(name)
	return strings.Contains(lower, "module") || strings.Contains(lower, "stuff") || strings.Contains(lower, "thing")
}

func isGenericLabel(label string) bool {
	lower := strings.ToLower(label)
	return lower == "calls" || lower == "uses" || lower == "connects" || lower == "links" || lower == ""
}

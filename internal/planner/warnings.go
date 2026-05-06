package planner

import (
	"fmt"
	"strings"

	"github.com/mertcikla/tld/internal/tech"
	"github.com/mertcikla/tld/internal/workspace"
)

// WarningGroup represents a collection of similar architectural warnings.
type WarningGroup struct {
	RuleCode    string
	RuleName    string
	Description string
	Mediation   string
	Violations  []string
}

const (
	warningCodeHighDensity      = "ARC001"
	warningCodeIsolatedObject   = "ARC002"
	warningCodeSharedContext    = "ARC003"
	warningCodeDepthMismatch    = "ARC004"
	warningCodeLowInsightRatio  = "ARC005"
	warningCodeDeadEndDrilldown = "ARC006"
	warningCodeAbstractionLeak  = "ARC007"

	warningCodeGenericLabels     = "ARC101"
	warningCodeMissingTech       = "ARC102"
	warningCodeUnknownTechnology = "ARC103"

	warningCodeMissingDesc   = "ARC201"
	warningCodeGenericNaming = "ARC202"
	warningCodeMissingLabel  = "ARC203"
)

type warningRule struct {
	Code        string
	Name        string
	Description string
	Mediation   string
}

var warningRules = []warningRule{
	{
		Code:        warningCodeHighDensity,
		Name:        "High Density",
		Description: "View exceeds the element density limit",
		Mediation:   "Split the view into nested views to reduce cognitive load.",
	},
	{
		Code:        warningCodeIsolatedObject,
		Name:        "Isolated Element",
		Description: "Element has 0 connectors in a view",
		Mediation:   "Explore its relationships further and add connectors in the view where it appears.",
	},
	{
		Code:        warningCodeSharedContext,
		Name:        "Shared Context",
		Description: "Shared element has no connectors in a specific view",
		Mediation:   "Add connectors to the shared element in this view or remove the placement from this view.",
	},
	{
		Code:        warningCodeDepthMismatch,
		Name:        "Depth Mismatch",
		Description: "View hierarchy is flat",
		Mediation:   "Create nested views to establish a zoomable hierarchy.",
	},
	{
		Code:        warningCodeLowInsightRatio,
		Name:        "Low Insight Ratio",
		Description: "Connectors < Elements",
		Mediation:   "Add more connectors to illustrate how elements interact, rather than just listing them.",
	},
	{
		Code:        warningCodeDeadEndDrilldown,
		Name:        "Dead-End Drilldown",
		Description: "Element owns a view but it has no content",
		Mediation:   "Add nested elements or connectors to the element's view, or remove the owned view.",
	},
	{
		Code:        warningCodeAbstractionLeak,
		Name:        "Abstraction Leak",
		Description: "Implementation types at Root level",
		Mediation:   "Move functions and classes into sub-diagrams. Keep root views at the Service/Subsystem level.",
	},
	{
		Code:        warningCodeGenericLabels,
		Name:        "Generic Labels",
		Description: "Connector label is overly generic",
		Mediation:   "Replace generic labels with domain-specific verbs like 'validates JWT' or 'SQL Query'.",
	},
	{
		Code:        warningCodeMissingTech,
		Name:        "Missing Tech",
		Description: "No `technology` field",
		Mediation:   "Add a 'technology' field to the following elements. (e.g. Go, React)",
	},
	{
		Code:        warningCodeUnknownTechnology,
		Name:        "Unknown Technology",
		Description: "Catalog mismatch",
		Mediation:   "Use recognized technology names (e.g. Go, React) or double check spelling.",
	},
	{
		Code:        warningCodeMissingDesc,
		Name:        "Missing Desc",
		Description: "`description` field is empty",
		Mediation:   "Add a one-sentence summary to help readers understand the responsibility.",
	},
	{
		Code:        warningCodeGenericNaming,
		Name:        "Generic Naming",
		Description: "Vague names make the map harder to understand",
		Mediation:   "Rename the element with a domain-specific, descriptive name.",
	},
	{
		Code:        warningCodeMissingLabel,
		Name:        "Missing Label",
		Description: "Connector has no `label`",
		Mediation:   "A connector without a label is just a line. Add a 'label' field to tell what it does.",
	},
}

var warningRuleCodesByLevel = map[int][]string{
	1: {
		warningCodeHighDensity,
		warningCodeIsolatedObject,
		warningCodeSharedContext,
		warningCodeDepthMismatch,
		warningCodeLowInsightRatio,
		warningCodeDeadEndDrilldown,
		warningCodeAbstractionLeak,
	},
	2: {
		warningCodeMissingTech,
		warningCodeUnknownTechnology,
	},
	3: {
		warningCodeGenericLabels,
		warningCodeMissingDesc,
		warningCodeGenericNaming,
		warningCodeMissingLabel,
	},
}

type warningContext struct {
	ws              *workspace.Workspace
	level           int
	allowLowInsight bool
	activeRules     []warningRule
	warnings        map[string]*WarningGroup
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
		warnings:        make(map[string]*WarningGroup),
		viewElements:    make(map[string][]string),
		elementViews:    make(map[string]map[string]int),
		viewConnectors:  make(map[string]int),
	}
}

func resolveConfiguredWarningRules(level int, includeRules, excludeRules []string) []warningRule {
	enabled := make(map[string]struct{})
	for currentLevel := 1; currentLevel <= level; currentLevel++ {
		for _, code := range warningRuleCodesByLevel[currentLevel] {
			enabled[code] = struct{}{}
		}
	}
	for _, code := range includeRules {
		normalized := normalizeWarningRuleCode(code)
		if normalized != "" {
			enabled[normalized] = struct{}{}
		}
	}
	for _, code := range excludeRules {
		delete(enabled, normalizeWarningRuleCode(code))
	}

	resolved := make([]warningRule, 0, len(enabled))
	for _, rule := range warningRules {
		if _, ok := enabled[rule.Code]; ok {
			resolved = append(resolved, rule)
		}
	}
	return resolved
}

func normalizeWarningRuleCode(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

func warningRuleByCode(code string) (warningRule, bool) {
	for _, rule := range warningRules {
		if rule.Code == code {
			return rule, true
		}
	}
	return warningRule{}, false
}

func (ctx *warningContext) addWarning(code, violation string) {
	rule, ok := warningRuleByCode(code)
	if !ok {
		return
	}
	if _, exists := ctx.warnings[code]; !exists {
		ctx.warnings[code] = &WarningGroup{
			RuleCode:    rule.Code,
			RuleName:    rule.Name,
			Description: rule.Description,
			Mediation:   rule.Mediation,
			Violations:  []string{},
		}
	}
	ctx.warnings[code].Violations = append(ctx.warnings[code].Violations, violation)
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
		ctx.runWarningRule(rule.Code)
	}
}

func (ctx *warningContext) runWarningRule(code string) {
	switch code {
	case warningCodeHighDensity:
		ctx.checkHighDensityRule()
	case warningCodeIsolatedObject:
		ctx.checkIsolatedObjectRule()
	case warningCodeSharedContext:
		ctx.checkSharedContextRule()
	case warningCodeDepthMismatch:
		ctx.checkDepthMismatchRule()
	case warningCodeLowInsightRatio:
		ctx.checkLowInsightRatioRule()
	case warningCodeDeadEndDrilldown:
		ctx.checkDeadEndDrilldownRule()
	case warningCodeAbstractionLeak:
		ctx.checkAbstractionLeakRule()
	case warningCodeGenericLabels:
		ctx.checkGenericLabelsRule()
	case warningCodeMissingTech:
		ctx.checkMissingTechRule()
	case warningCodeUnknownTechnology:
		ctx.checkUnknownTechnologyRule()
	case warningCodeMissingDesc:
		ctx.checkMissingDescriptionRule()
	case warningCodeGenericNaming:
		ctx.checkGenericNamingRule()
	case warningCodeMissingLabel:
		ctx.checkMissingLabelRule()
	}
}

func (ctx *warningContext) checkDepthMismatchRule() {
	viewCount := 0
	for _, element := range ctx.ws.Elements {
		if element != nil && element.HasView {
			viewCount++
		}
	}
	if ctx.maxDepth < 1 && viewCount > 1 {
		ctx.addWarning(warningCodeDepthMismatch, "Workspace views")
	}
}

func (ctx *warningContext) checkHighDensityRule() {
	for viewRef, elements := range ctx.viewElements {
		densityLimit := 15
		if ctx.level >= 2 {
			densityLimit = 12
		}
		if len(elements) > densityLimit {
			ctx.addWarning(warningCodeHighDensity, fmt.Sprintf("View %q has %d elements", viewRef, len(elements)))
		}
	}
}

func (ctx *warningContext) checkLowInsightRatioRule() {
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
			ctx.addWarning(warningCodeLowInsightRatio, fmt.Sprintf("View %q (Elements: %d, Connectors: %d)", viewRef, len(elements), connectorCount))
		}
	}
}

func (ctx *warningContext) checkDeadEndDrilldownRule() {
	for ref, element := range ctx.ws.Elements {
		if element == nil || !element.HasView {
			continue
		}
		if len(ctx.viewElements[ref]) == 0 && ctx.viewConnectors[ref] == 0 {
			ctx.addWarning(warningCodeDeadEndDrilldown, fmt.Sprintf("Element %q owns an empty view", ref))
		}
	}
}

func (ctx *warningContext) checkIsolatedObjectRule() {
	for elementRef, element := range ctx.ws.Elements {
		if element == nil || len(element.Placements) != 1 {
			continue
		}
		for _, placement := range element.Placements {
			viewRef := normalizeWarningViewRef(placement.ParentRef)
			if ctx.elementViews[elementRef][viewRef] == 0 {
				ctx.addWarning(warningCodeIsolatedObject, fmt.Sprintf("Element %q in View %q", elementRef, viewRef))
			}
		}
	}
}

func (ctx *warningContext) checkSharedContextRule() {
	for elementRef, element := range ctx.ws.Elements {
		if element == nil || len(element.Placements) <= 1 {
			continue
		}
		for _, placement := range element.Placements {
			viewRef := normalizeWarningViewRef(placement.ParentRef)
			if ctx.elementViews[elementRef][viewRef] == 0 {
				ctx.addWarning(warningCodeSharedContext, fmt.Sprintf("Element %q in View %q", elementRef, viewRef))
			}
		}
	}
}

func (ctx *warningContext) checkAbstractionLeakRule() {
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
				ctx.addWarning(warningCodeAbstractionLeak, fmt.Sprintf("Element %q (Kind: %s)", elementRef, element.Kind))
			}
		}
	}
}

func (ctx *warningContext) checkMissingTechRule() {
	for elementRef, element := range ctx.ws.Elements {
		if element != nil && element.Technology == "" {
			// Repositories are structural and don't require a specific technology tag.
			if strings.ToLower(element.Kind) == "repository" {
				continue
			}
			ctx.addWarning(warningCodeMissingTech, fmt.Sprintf("%q", elementRef))
		}
	}
}

func (ctx *warningContext) checkUnknownTechnologyRule() {
	for elementRef, element := range ctx.ws.Elements {
		if element == nil || element.Technology == "" {
			continue
		}
		missing := tech.Validate(element.Technology)
		if len(missing) > 0 {
			ctx.addWarning(warningCodeUnknownTechnology, fmt.Sprintf("Element %q has unknown: %s", elementRef, strings.Join(missing, ", ")))
		}
	}
}

func (ctx *warningContext) checkGenericLabelsRule() {
	for connectorRef, connector := range ctx.ws.Connectors {
		if connector != nil && isGenericLabel(connector.Label) {
			ctx.addWarning(warningCodeGenericLabels, fmt.Sprintf("Connector %q in View %q (Label: %q)", connectorRef, normalizeWarningViewRef(connector.View), connector.Label))
		}
	}
}

func (ctx *warningContext) checkMissingDescriptionRule() {
	for elementRef, element := range ctx.ws.Elements {
		if element != nil && element.Description == "" {
			ctx.addWarning(warningCodeMissingDesc, fmt.Sprintf("Element %q", elementRef))
		}
	}
}

func (ctx *warningContext) checkGenericNamingRule() {
	for elementRef, element := range ctx.ws.Elements {
		if element != nil && isGenericName(element.Name) {
			ctx.addWarning(warningCodeGenericNaming, fmt.Sprintf("Element %q (Name: %q)", elementRef, element.Name))
		}
	}
}

func (ctx *warningContext) checkMissingLabelRule() {
	for connectorRef, connector := range ctx.ws.Connectors {
		if connector != nil && connector.Label == "" {
			ctx.addWarning(warningCodeMissingLabel, fmt.Sprintf("Connector %q in View %q", connectorRef, normalizeWarningViewRef(connector.View)))
		}
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
		if wg, ok := ctx.warnings[rule.Code]; ok {
			result = append(result, *wg)
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

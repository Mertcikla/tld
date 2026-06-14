package mermaid

import (
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"

	diagv1 "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/diag/v1"
	mcheck "github.com/sammcj/mermaid-check"
	mast "github.com/sammcj/mermaid-check/ast"
)

var (
	tagPattern                     = regexp.MustCompile(`<[^>]*>`)
	breakPattern                   = regexp.MustCompile(`(?i)<br\s*/?>`)
	architectureNodePattern        = regexp.MustCompile(`(?i)^(group|service)\s+([A-Za-z_][\w.-]*)(?:\(([^)]*)\))?\s*\[([^\]]*)\](?:\s+in\s+([A-Za-z_][\w.-]*))?$`)
	architectureJunctionPattern    = regexp.MustCompile(`(?i)^junction\s+([A-Za-z_][\w.-]*)(?:\s+in\s+([A-Za-z_][\w.-]*))?$`)
	architectureEdgePattern        = regexp.MustCompile(`(?i)^([A-Za-z_][\w.-]*(?:\{group\})?):([TBLR])\s*(<)?--(>)?\s*([TBLR]):([A-Za-z_][\w.-]*(?:\{group\})?)(?:\s*:(.*))?$`)
	requirementNodePattern         = regexp.MustCompile(`(?i)^(requirement|functionalRequirement|interfaceRequirement|performanceRequirement|physicalRequirement|designConstraint)\s+([A-Za-z_][\w.-]*)(?:\s*\{(.*))?$`)
	requirementElementPattern      = regexp.MustCompile(`(?i)^element\s+([A-Za-z_][\w.-]*)(?:\s*\{(.*))?$`)
	requirementRelationshipPattern = regexp.MustCompile(`(?i)^([A-Za-z_][\w.-]*)\s+-\s*([A-Za-z_][\w.-]*)\s+->\s+([A-Za-z_][\w.-]*)$`)
	exportedQuotedConnectorPattern = regexp.MustCompile(`^(\s*[A-Za-z_][A-Za-z0-9_]*)\s+--\s+"((?:\\.|[^"\\])*)"\s+-->\s+([A-Za-z_][A-Za-z0-9_]*\s*)$`)
)

func Parse(sourceText string) (*ParsedDiagram, error) {
	source, ok := ExtractMermaidCode(sourceText)
	if !ok {
		source = strings.TrimSpace(sourceText)
	}
	if source == "" {
		return nil, fmt.Errorf("empty Mermaid source")
	}
	if len([]byte(source)) > MaxSourceBytes {
		return nil, fmt.Errorf("mermaid source is too large (%d KiB). limit is %d KiB", ceilKiB(len([]byte(source))), ceilKiB(MaxSourceBytes))
	}

	result := &ParsedDiagram{
		Direction: DirectionLR,
		Source:    source,
	}
	metadata := ParseTldMetadata(source)
	parseSource := normalizeParserSource(StripCommentLines(source))

	var err error
	switch {
	case isArchitectureBetaDiagram(parseSource):
		err = convertArchitectureBeta(result, parseSource)
	case isRequirementDiagram(parseSource):
		err = convertRequirementDiagram(result, parseSource)
	default:
		var diagram mast.Diagram
		diagram, err = mcheck.Parse(parseSource)
		if err == nil {
			err = convertDiagram(result, diagram)
		}
	}
	if err != nil {
		result.Warnings = append(result.Warnings, err.Error())
	}
	if len(result.Elements) == 0 && isERDiagram(parseSource) {
		convertERSource(result, parseSource)
	}
	if isStateDiagram(parseSource) {
		applyStateTransitionLabels(result, parseSource)
	}

	ApplyTldMetadata(result, metadata)
	if len(result.Elements) > MaxElements {
		return nil, fmt.Errorf("mermaid import has %d elements. limit is %d", len(result.Elements), MaxElements)
	}
	if len(result.Connectors) > MaxConnectors {
		return nil, fmt.Errorf("mermaid import has %d connectors. limit is %d", len(result.Connectors), MaxConnectors)
	}
	if len(result.Warnings) == 0 && len(result.Elements) == 0 && len(result.Connectors) == 0 {
		result.Warnings = append(result.Warnings, "No compatible diagram content found.")
	}
	return result, nil
}

func convertDiagram(result *ParsedDiagram, diagram mast.Diagram) error {
	switch d := diagram.(type) {
	case *mast.Flowchart:
		convertFlowchart(result, d)
	case *mast.C4Diagram:
		convertC4(result, d)
	case *mast.SequenceDiagram:
		convertSequence(result, d)
	case *mast.ClassDiagram:
		convertClass(result, d)
	case *mast.ERDiagram:
		convertER(result, d)
	case *mast.StateDiagram:
		convertState(result, d)
	case *mast.SankeyDiagram:
		convertSankey(result, d)
	case *mast.PieDiagram:
		convertPie(result, d)
	case *mast.GitGraphDiagram:
		convertGitGraph(result, d)
	case *mast.QuadrantDiagram:
		convertQuadrant(result, d)
	case *mast.MindmapDiagram:
		convertMindmap(result, d)
	case *mast.JourneyDiagram:
		convertJourney(result, d)
	case *mast.GanttDiagram:
		convertGantt(result, d)
	case *mast.TimelineDiagram:
		convertTimeline(result, d)
	case *mast.XYChartDiagram:
		convertXYChart(result, d)
	default:
		return fmt.Errorf("unsupported diagram type: %s", diagram.GetType())
	}
	return nil
}

func addElement(result *ParsedDiagram, ref, name, kind, description, technology string) {
	if ref == "" {
		return
	}
	cleanName := sanitizeLabel(name)
	for _, element := range result.Elements {
		if element.GetRef() == ref {
			if cleanName != "" && element.GetName() == ref && cleanName != ref {
				element.Name = cleanName
			}
			return
		}
	}
	if cleanName == "" {
		cleanName = ref
	}
	if kind == "" {
		kind = "system"
	}
	element := &diagv1.PlanElement{
		Ref:        ref,
		Name:       cleanName,
		Kind:       &kind,
		Placements: []*diagv1.PlanViewPlacement{{ParentRef: "root"}},
	}
	if description != "" {
		clean := sanitizeLabel(description)
		element.Description = &clean
	}
	if technology != "" {
		element.Technology = &technology
	}
	result.Elements = append(result.Elements, element)
}

func addConnector(result *ParsedDiagram, source, target, label string, options ...func(*diagv1.PlanConnector)) {
	if source == "" || target == "" {
		return
	}
	index := len(result.Connectors)
	cleanLabel := sanitizeLabel(label)
	connector := &diagv1.PlanConnector{
		Ref:              fmt.Sprintf("%s:%s:%d", source, target, index),
		ViewRef:          "root",
		SourceElementRef: source,
		TargetElementRef: target,
		Label:            &cleanLabel,
	}
	for _, option := range options {
		option(connector)
	}
	result.Connectors = append(result.Connectors, connector)
}

func convertFlowchart(result *ParsedDiagram, diagram *mast.Flowchart) {
	result.Direction = DirectionFromString(diagram.Direction)
	visitFlowchartStatements(result, diagram.Statements)
}

func visitFlowchartStatements(result *ParsedDiagram, statements []mast.Statement) {
	for _, statement := range statements {
		switch item := statement.(type) {
		case *mast.NodeDef:
			addElement(result, item.ID, unwrapFlowchartNodeLabel(item.Label), "system", "", "")
		case *mast.Link:
			addElement(result, item.From, item.From, "system", "", "")
			addElement(result, item.To, item.To, "system", "", "")
			addConnector(result, item.From, item.To, item.Label, func(connector *diagv1.PlanConnector) {
				if item.BiDir {
					direction := "both"
					connector.Direction = &direction
				}
			})
		case *mast.Subgraph:
			visitFlowchartStatements(result, item.Statements)
		}
	}
}

func convertC4(result *ParsedDiagram, diagram *mast.C4Diagram) {
	for _, element := range diagram.Elements {
		addC4Element(result, element)
	}
	var visitBoundary func(mast.C4Boundary)
	visitBoundary = func(boundary mast.C4Boundary) {
		addElement(result, boundary.ID, boundary.Label, "container", boundary.Type, "")
		for _, element := range boundary.Elements {
			addC4Element(result, element)
		}
		for _, nested := range boundary.Boundaries {
			visitBoundary(nested)
		}
	}
	for _, boundary := range diagram.Boundaries {
		visitBoundary(boundary)
	}
	for _, rel := range diagram.Relationships {
		label := rel.Label
		if label == "" {
			label = rel.Technology
		}
		addConnector(result, rel.From, rel.To, label)
	}
}

func addC4Element(result *ParsedDiagram, element mast.C4Element) {
	addElement(result, element.ID, firstNonEmpty(element.Label, element.ID), c4Kind(element), element.Description, element.Technology)
}

func c4Kind(element mast.C4Element) string {
	value := strings.ToLower(element.ElementType)
	switch {
	case strings.Contains(value, "person"):
		return "person"
	case element.Database || strings.Contains(value, "db"):
		return "database"
	case strings.Contains(value, "container"):
		return "container"
	case strings.Contains(value, "component"):
		return "component"
	case element.External || strings.Contains(value, "external"):
		return "external"
	default:
		return "system"
	}
}

func convertSequence(result *ParsedDiagram, diagram *mast.SequenceDiagram) {
	visitSequenceStatements(result, diagram.Statements)
}

func visitSequenceStatements(result *ParsedDiagram, statements []mast.SeqStmt) {
	for _, statement := range statements {
		switch item := statement.(type) {
		case *mast.Participant:
			kind := "system"
			if item.Type == "actor" {
				kind = "person"
			}
			addElement(result, item.ID, firstNonEmpty(item.Alias, item.ID), kind, "", "")
		case *mast.Message:
			addElement(result, item.From, item.From, "system", "", "")
			addElement(result, item.To, item.To, "system", "", "")
			addConnector(result, item.From, item.To, item.Text)
		case *mast.Loop:
			visitSequenceStatements(result, item.Statements)
		case *mast.Alt:
			for _, condition := range item.Conditions {
				visitSequenceStatements(result, condition.Statements)
			}
		case *mast.Opt:
			visitSequenceStatements(result, item.Statements)
		case *mast.Par:
			for _, branch := range item.Branches {
				visitSequenceStatements(result, branch.Statements)
			}
		case *mast.Critical:
			visitSequenceStatements(result, item.Statements)
			for _, option := range item.Options {
				visitSequenceStatements(result, option.Statements)
			}
		case *mast.Break:
			visitSequenceStatements(result, item.Statements)
		case *mast.Box:
			for _, participant := range item.Participants {
				addElement(result, participant.ID, firstNonEmpty(participant.Alias, participant.ID), "system", "", "")
			}
		}
	}
}

func convertClass(result *ParsedDiagram, diagram *mast.ClassDiagram) {
	for _, statement := range diagram.Statements {
		switch item := statement.(type) {
		case *mast.Class:
			members := make([]string, 0, len(item.Members))
			for _, member := range item.Members {
				members = append(members, strings.TrimSpace(member.Visibility+member.Name))
			}
			addElement(result, item.Name, item.Name, "component", strings.Join(members, "\n"), "")
		case *mast.Relationship:
			label := firstNonEmpty(item.Label, item.Type)
			addConnector(result, item.From, item.To, label)
		}
	}
}

func convertER(result *ParsedDiagram, diagram *mast.ERDiagram) {
	result.Direction = DirectionFromString(diagram.Direction)
	startElements := len(result.Elements)
	startConnectors := len(result.Connectors)
	for _, entity := range diagram.Entities {
		attributes := make([]string, 0, len(entity.Attributes))
		for _, attr := range entity.Attributes {
			attributes = append(attributes, strings.TrimSpace(strings.Join([]string{attr.Type, attr.Name, strings.Join(attr.Keys, " "), attr.Comment}, " ")))
		}
		addElement(result, entity.Name, firstNonEmpty(entity.Alias, entity.Name), "database", strings.Join(attributes, "\n"), "")
	}
	for _, relationship := range diagram.Relationships {
		addConnector(result, relationship.From, relationship.To, relationship.Label)
	}
	if len(result.Elements) == startElements && len(result.Connectors) == startConnectors {
		convertERSource(result, diagram.Source)
	}
}

func convertState(result *ParsedDiagram, diagram *mast.StateDiagram) {
	visitStateStatements(result, diagram.Statements)
	applyStateTransitionLabels(result, diagram.Source)
}

func applyStateTransitionLabels(result *ParsedDiagram, source string) {
	labels := stateTransitionLabels(source)
	for _, connector := range result.Connectors {
		if connector.GetLabel() != "" {
			continue
		}
		label := labels[connector.GetSourceElementRef()+"\x00"+connector.GetTargetElementRef()]
		if label != "" {
			connector.Label = &label
		}
	}
}

func visitStateStatements(result *ParsedDiagram, statements []mast.StateStmt) {
	for _, statement := range statements {
		switch item := statement.(type) {
		case *mast.State:
			addElement(result, item.ID, firstNonEmpty(item.Description, item.ID), "system", "", "")
			visitStateStatements(result, item.Nested)
		case *mast.Transition:
			addElement(result, item.From, item.From, "system", "", "")
			addElement(result, item.To, item.To, "system", "", "")
			addConnector(result, item.From, item.To, item.Label)
		case *mast.StartState:
			addElement(result, "state_start", "Start", "system", "", "")
			addElement(result, item.To, item.To, "system", "", "")
			addConnector(result, "state_start", item.To, "")
		case *mast.EndState:
			addElement(result, item.From, item.From, "system", "", "")
			addElement(result, "state_end", "End", "system", "", "")
			addConnector(result, item.From, "state_end", "")
		case *mast.Fork:
			addElement(result, item.ID, item.ID, "component", "", "")
		case *mast.Join:
			addElement(result, item.ID, item.ID, "component", "", "")
		case *mast.Choice:
			addElement(result, item.ID, item.ID, "component", "", "")
		}
	}
}

func convertERSource(result *ParsedDiagram, source string) {
	relationshipRe := regexp.MustCompile(`^\s*([A-Za-z_][\w.-]*)\s+\S+\s+([A-Za-z_][\w.-]*)\s*:\s*(.*)$`)
	for _, line := range strings.Split(source, "\n") {
		match := relationshipRe.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		from := match[1]
		to := match[2]
		label := strings.TrimSpace(match[3])
		addElement(result, from, from, "database", "", "")
		addElement(result, to, to, "database", "", "")
		addConnector(result, from, to, label)
	}
}

func stateTransitionLabels(source string) map[string]string {
	transitionRe := regexp.MustCompile(`^\s*([A-Za-z_][\w.-]*|\[\*\])\s*-->\s*([A-Za-z_][\w.-]*|\[\*\])(?:\s*:\s*(.*))?$`)
	labels := map[string]string{}
	for _, line := range strings.Split(source, "\n") {
		match := transitionRe.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		from := normalizeStateEndpoint(match[1])
		to := normalizeStateEndpoint(match[2])
		label := strings.TrimSpace(match[3])
		if label != "" {
			labels[from+"\x00"+to] = label
		}
	}
	return labels
}

func normalizeStateEndpoint(value string) string {
	if value == "[*]" {
		return "state_start"
	}
	return value
}

func isERDiagram(source string) bool {
	return regexp.MustCompile(`(?i)^\s*erDiagram\b`).MatchString(source)
}

func isStateDiagram(source string) bool {
	return regexp.MustCompile(`(?i)^\s*stateDiagram(?:-v2)?\b`).MatchString(source)
}

func convertSankey(result *ParsedDiagram, diagram *mast.SankeyDiagram) {
	for _, link := range diagram.Links {
		addElement(result, link.Source, link.Source, "system", "", "")
		addElement(result, link.Target, link.Target, "system", "", "")
		addConnector(result, link.Source, link.Target, formatFloat(link.Value))
	}
}

func convertPie(result *ParsedDiagram, diagram *mast.PieDiagram) {
	title := firstNonEmpty(diagram.Title, "Pie chart")
	addElement(result, "pie_chart", title, "system", "", "")
	for index, section := range diagram.DataEntries {
		ref := fmt.Sprintf("pie_%d", index+1)
		addElement(result, ref, firstNonEmpty(section.Label, ref), "system", formatFloat(section.Value), "")
		addConnector(result, "pie_chart", ref, formatFloat(section.Value))
	}
}

func convertGitGraph(result *ParsedDiagram, diagram *mast.GitGraphDiagram) {
	mainBranch := firstNonEmpty(diagram.MainBranchName, "main")
	branchHeads := map[string]string{}
	currentBranch := mainBranch
	previousCommit := ""
	addElement(result, currentBranch, currentBranch, "system", "", "")

	for index, op := range diagram.Operations {
		switch op.Type {
		case "branch":
			branch := firstNonEmpty(op.BranchName, op.ID, fmt.Sprintf("branch_%d", index))
			addElement(result, branch, branch, "system", "", "")
			if previousCommit != "" {
				addConnector(result, previousCommit, branch, "branch")
			}
			branchHeads[branch] = firstNonEmpty(previousCommit, branch)
		case "checkout":
			currentBranch = firstNonEmpty(op.BranchName, op.ID, currentBranch)
			addElement(result, currentBranch, currentBranch, "system", "", "")
			previousCommit = branchHeads[currentBranch]
		case "commit":
			ref := firstNonEmpty(op.ID, fmt.Sprintf("commit_%d", index+1))
			addElement(result, ref, ref, "component", "", "")
			addConnector(result, firstNonEmpty(previousCommit, currentBranch), ref, currentBranch)
			previousCommit = ref
			branchHeads[currentBranch] = ref
		case "merge":
			branch := firstNonEmpty(op.BranchName, op.ID)
			branchHead := firstNonEmpty(branchHeads[branch], branch)
			ref := firstNonEmpty(op.ID, fmt.Sprintf("merge_%s_%d", branch, index))
			addElement(result, ref, firstNonEmpty(op.Tag, ref), "component", "", "")
			if previousCommit != "" {
				addConnector(result, previousCommit, ref, currentBranch)
			}
			addConnector(result, branchHead, ref, "merge")
			previousCommit = ref
			branchHeads[currentBranch] = ref
		}
	}
}

func convertQuadrant(result *ParsedDiagram, diagram *mast.QuadrantDiagram) {
	addElement(result, "quadrant_chart", firstNonEmpty(diagram.Title, "Quadrant chart"), "system", "", "")
	for index, point := range diagram.Points {
		ref := fmt.Sprintf("quadrant_%d", index+1)
		addElement(result, ref, firstNonEmpty(point.Name, ref), "system", fmt.Sprintf("[%s, %s]", formatFloat(point.X), formatFloat(point.Y)), "")
		addConnector(result, "quadrant_chart", ref, "")
	}
}

func convertMindmap(result *ParsedDiagram, diagram *mast.MindmapDiagram) {
	var visit func(*mast.MindmapNode, string)
	visit = func(node *mast.MindmapNode, parentRef string) {
		if node == nil {
			return
		}
		ref := sanitizeRef(firstNonEmpty(node.Text, fmt.Sprintf("mindmap_%d", len(result.Elements)+1)))
		addElement(result, ref, firstNonEmpty(node.Text, ref), "system", "", "")
		if parentRef != "" {
			addConnector(result, parentRef, ref, "")
		}
		for _, child := range node.Children {
			visit(child, ref)
		}
	}
	visit(diagram.Root, "")
}

func convertJourney(result *ParsedDiagram, diagram *mast.JourneyDiagram) {
	addElement(result, "journey", firstNonEmpty(diagram.Title, "Journey"), "system", "", "")
	for sectionIndex, section := range diagram.Sections {
		sectionRef := fmt.Sprintf("journey_section_%d", sectionIndex+1)
		addElement(result, sectionRef, firstNonEmpty(section.Name, sectionRef), "system", "", "")
		addConnector(result, "journey", sectionRef, "")
		for taskIndex, task := range section.Tasks {
			taskRef := fmt.Sprintf("%s_task_%d", sectionRef, taskIndex+1)
			addElement(result, taskRef, firstNonEmpty(task.Name, taskRef), "component", fmt.Sprintf("Score: %d", task.Score), "")
			addConnector(result, sectionRef, taskRef, "")
		}
	}
}

func convertGantt(result *ParsedDiagram, diagram *mast.GanttDiagram) {
	addElement(result, "gantt", firstNonEmpty(diagram.Title, "Gantt"), "system", "", "")
	for sectionIndex, section := range diagram.Sections {
		sectionRef := fmt.Sprintf("gantt_section_%d", sectionIndex+1)
		addElement(result, sectionRef, firstNonEmpty(section.Name, sectionRef), "system", "", "")
		addConnector(result, "gantt", sectionRef, "")
		for taskIndex, task := range section.Tasks {
			taskRef := firstNonEmpty(task.ID, fmt.Sprintf("%s_task_%d", sectionRef, taskIndex+1))
			addElement(result, taskRef, firstNonEmpty(task.Name, taskRef), "component", strings.TrimSpace(task.StartDate+" "+task.EndDate), "")
			addConnector(result, sectionRef, taskRef, "")
		}
	}
}

func convertTimeline(result *ParsedDiagram, diagram *mast.TimelineDiagram) {
	addElement(result, "timeline", firstNonEmpty(diagram.Title, "Timeline"), "system", "", "")
	for sectionIndex, section := range diagram.Sections {
		sectionRef := fmt.Sprintf("timeline_section_%d", sectionIndex+1)
		addElement(result, sectionRef, firstNonEmpty(section.Name, sectionRef), "system", "", "")
		addConnector(result, "timeline", sectionRef, "")
		for periodIndex, period := range section.Periods {
			periodRef := fmt.Sprintf("%s_period_%d", sectionRef, periodIndex+1)
			addElement(result, periodRef, firstNonEmpty(period.TimePeriod, periodRef), "component", "", "")
			addConnector(result, sectionRef, periodRef, "")
			for eventIndex, event := range period.Events {
				eventRef := fmt.Sprintf("%s_event_%d", periodRef, eventIndex+1)
				addElement(result, eventRef, firstNonEmpty(event, eventRef), "component", "", "")
				addConnector(result, periodRef, eventRef, "")
			}
		}
	}
}

func convertXYChart(result *ParsedDiagram, diagram *mast.XYChartDiagram) {
	addElement(result, "xychart", firstNonEmpty(diagram.Title, "XY chart"), "system", "", "")
	for seriesIndex, series := range diagram.Series {
		seriesRef := fmt.Sprintf("xy_series_%d", seriesIndex+1)
		addElement(result, seriesRef, fmt.Sprintf("%s %d", firstNonEmpty(series.Type, "series"), seriesIndex+1), "system", "", "")
		addConnector(result, "xychart", seriesRef, "")
		for valueIndex, value := range series.Values {
			pointRef := fmt.Sprintf("%s_point_%d", seriesRef, valueIndex+1)
			name := pointRef
			if valueIndex < len(diagram.XAxis.Categories) {
				name = diagram.XAxis.Categories[valueIndex]
			}
			addElement(result, pointRef, name, "component", formatFloat(value), "")
			addConnector(result, seriesRef, pointRef, formatFloat(value))
		}
	}
}

func convertArchitectureBeta(result *ParsedDiagram, source string) error {
	result.Direction = DirectionLR
	lines := strings.Split(source, "\n")
	start := firstMermaidBodyLineIndex(lines)
	if start < 0 {
		return fmt.Errorf("unable to find architecture-beta diagram body")
	}
	for index, rawLine := range lines[start+1:] {
		line := strings.TrimSuffix(strings.TrimSpace(rawLine), ";")
		if line == "" || strings.HasPrefix(line, "%%") {
			continue
		}
		if match := architectureNodePattern.FindStringSubmatch(line); match != nil {
			nodeType, ref, icon, label, parent := strings.ToLower(match[1]), match[2], match[3], match[4], match[5]
			addElement(result, ref, firstNonEmpty(strings.TrimSpace(label), ref), architectureKind(icon, nodeType), architectureDescription(nodeType, icon, parent), icon)
			continue
		}
		if match := architectureJunctionPattern.FindStringSubmatch(line); match != nil {
			ref, parent := match[1], match[2]
			addElement(result, ref, ref, architectureKind("", "junction"), architectureDescription("junction", "", parent), "")
			continue
		}
		if match := architectureEdgePattern.FindStringSubmatch(line); match != nil {
			sourceRef := stripArchitectureGroupModifier(match[1])
			targetRef := stripArchitectureGroupModifier(match[6])
			addElement(result, sourceRef, sourceRef, "system", "", "")
			addElement(result, targetRef, targetRef, "system", "", "")
			leftArrow, rightArrow := match[3], match[4]
			sourceHandle, targetHandle := architectureHandle(match[2]), architectureHandle(match[5])
			direction := architectureDirection(leftArrow, rightArrow)
			addConnector(result, sourceRef, targetRef, strings.TrimSpace(match[7]), func(connector *diagv1.PlanConnector) {
				connector.Direction = &direction
				connector.SourceHandle = ptrString(sourceHandle)
				connector.TargetHandle = ptrString(targetHandle)
			})
			continue
		}
		result.Warnings = append(result.Warnings, fmt.Sprintf("Unsupported architecture-beta line %d: %s", index+2, line))
	}
	return nil
}

func convertRequirementDiagram(result *ParsedDiagram, source string) error {
	lines := strings.Split(source, "\n")
	for index := 1; index < len(lines); index++ {
		line := strings.TrimSpace(lines[index])
		if line == "" || strings.HasPrefix(line, "%%") || line == "}" {
			continue
		}
		if match := requirementNodePattern.FindStringSubmatch(line); match != nil {
			ref := match[2]
			addElement(result, ref, ref, "system", collectInlineBlockText(lines, &index, match[3]), "")
			continue
		}
		if match := requirementElementPattern.FindStringSubmatch(line); match != nil {
			ref := match[1]
			addElement(result, ref, ref, "component", collectInlineBlockText(lines, &index, match[2]), "")
			continue
		}
		if match := requirementRelationshipPattern.FindStringSubmatch(line); match != nil {
			addConnector(result, match[1], match[3], match[2])
			continue
		}
	}
	return nil
}

func collectInlineBlockText(lines []string, index *int, first string) string {
	parts := []string{}
	if strings.TrimSpace(first) != "" {
		parts = append(parts, strings.TrimSpace(strings.TrimSuffix(first, "}")))
	}
	if first != "" && strings.Contains(first, "}") {
		return strings.Join(parts, "\n")
	}
	for *index+1 < len(lines) {
		*index = *index + 1
		line := strings.TrimSpace(lines[*index])
		if line == "}" {
			break
		}
		parts = append(parts, strings.TrimSuffix(line, "}"))
		if strings.Contains(line, "}") {
			break
		}
	}
	return strings.Join(parts, "\n")
}

func normalizeParserSource(source string) string {
	source = stripFrontmatter(source)
	lines := strings.Split(source, "\n")
	for index, line := range lines {
		if match := exportedQuotedConnectorPattern.FindStringSubmatch(line); match != nil {
			lines[index] = match[1] + " -->|" + strings.ReplaceAll(match[2], "|", "\\|") + "| " + match[3]
		}
	}
	return strings.Join(lines, "\n")
}

func stripFrontmatter(source string) string {
	lines := strings.Split(source, "\n")
	start := firstMermaidBodyLineIndex(lines)
	if start <= 0 {
		return source
	}
	return strings.Join(lines[start:], "\n")
}

func isArchitectureBetaDiagram(source string) bool {
	lines := strings.Split(source, "\n")
	start := firstMermaidBodyLineIndex(lines)
	return start >= 0 && regexp.MustCompile(`(?i)^architecture-beta\b`).MatchString(strings.TrimSpace(lines[start]))
}

func isRequirementDiagram(source string) bool {
	lines := strings.Split(source, "\n")
	start := firstMermaidBodyLineIndex(lines)
	return start >= 0 && regexp.MustCompile(`(?i)^requirementDiagram\b`).MatchString(strings.TrimSpace(lines[start]))
}

func architectureKind(icon, nodeType string) string {
	if nodeType == "group" {
		return "container"
	}
	if nodeType == "junction" {
		return "component"
	}
	value := strings.ToLower(icon)
	switch {
	case strings.Contains(value, "database"), strings.Contains(value, "disk"), strings.Contains(value, "storage"):
		return "database"
	case strings.Contains(value, "internet"), strings.Contains(value, "cloud"):
		return "external"
	case strings.Contains(value, "server"):
		return "service"
	default:
		return "system"
	}
}

func architectureHandle(side string) string {
	switch strings.ToUpper(side) {
	case "T":
		return "top"
	case "B":
		return "bottom"
	case "L":
		return "left"
	case "R":
		return "right"
	default:
		return ""
	}
}

func architectureDirection(leftArrow, rightArrow string) string {
	switch {
	case leftArrow != "" && rightArrow != "":
		return "both"
	case leftArrow != "":
		return "backward"
	case rightArrow != "":
		return "forward"
	default:
		return "none"
	}
}

func architectureDescription(nodeType, icon, parent string) string {
	parts := []string{"Architecture " + nodeType}
	if icon != "" {
		parts = append(parts, "icon: "+icon)
	}
	if parent != "" {
		parts = append(parts, "in: "+parent)
	}
	return strings.Join(parts, "\n")
}

func stripArchitectureGroupModifier(value string) string {
	return regexp.MustCompile(`(?i)\{group\}$`).ReplaceAllString(value, "")
}

func sanitizeLabel(value string) string {
	value = html.UnescapeString(value)
	value = breakPattern.ReplaceAllString(value, " ")
	value = tagPattern.ReplaceAllString(value, " ")
	return strings.TrimSpace(regexp.MustCompile(`\s+`).ReplaceAllString(value, " "))
}

func unwrapFlowchartNodeLabel(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		value = value[1 : len(value)-1]
	}
	return DecodeMermaidLabel(value)
}

func sanitizeRef(value string) string {
	replacer := regexp.MustCompile(`[^A-Za-z0-9_]`)
	sanitized := replacer.ReplaceAllString(value, "_")
	if sanitized == "" {
		return "node"
	}
	if first := sanitized[0]; (first >= 'A' && first <= 'Z') || (first >= 'a' && first <= 'z') || first == '_' {
		return sanitized
	}
	return "node_" + sanitized
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func ceilKiB(bytes int) int {
	return (bytes + 1023) / 1024
}

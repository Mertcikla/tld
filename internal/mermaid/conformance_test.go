package mermaid

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

const defaultMermaidFlowchartConformancePath = "/Users/mertcikla/apps/temp/mermaid/packages/mermaid/src/diagrams/flowchart/parser"

type mermaidConformanceSuite struct {
	name        string
	envPath     string
	defaultPath string
	filePattern string
}

var mermaidConformanceSuites = []mermaidConformanceSuite{
	{
		name:        "flowchart",
		envPath:     "TLD_MERMAID_CONFORMANCE_PATH",
		defaultPath: defaultMermaidFlowchartConformancePath,
		filePattern: "*.spec.js",
	},
}

func TestMermaidFlowchartConformanceReport(t *testing.T) {
	t.Parallel()

	report := runMermaidConformanceSuite(t, mermaidConformanceSuites[0])
	t.Log("\n" + report.String())
}

type mermaidConformanceCase struct {
	file         string
	line         int
	name         string
	source       string
	expectReject bool
	expectations []mermaidSemanticExpectation
}

type mermaidSemanticExpectation struct {
	kind       string
	index      int
	ref        string
	wantString string
	wantInt    int
}

type mermaidConformanceReport struct {
	suite         string
	root          string
	files         []mermaidConformanceFileReport
	cases         int
	passed        int
	failed        int
	skipped       int
	unextractable int
	failures      []mermaidConformanceFailure
}

type mermaidConformanceFileReport struct {
	name          string
	cases         int
	passed        int
	failed        int
	skipped       int
	unextractable int
}

type mermaidConformanceFailure struct {
	file   string
	line   int
	name   string
	reason string
	source string
}

func runMermaidConformanceSuite(t *testing.T, suite mermaidConformanceSuite) mermaidConformanceReport {
	t.Helper()

	root := os.Getenv(suite.envPath)
	if root == "" {
		root = suite.defaultPath
	}
	info, err := os.Stat(root)
	if os.IsNotExist(err) {
		t.Skipf("Mermaid %s conformance path does not exist: %s", suite.name, root)
	}
	if err != nil {
		t.Fatalf("stat Mermaid %s conformance path: %v", suite.name, err)
	}
	if !info.IsDir() {
		t.Fatalf("Mermaid %s conformance path is not a directory: %s", suite.name, root)
	}

	matches, err := filepath.Glob(filepath.Join(root, suite.filePattern))
	if err != nil {
		t.Fatalf("glob Mermaid %s conformance files: %v", suite.name, err)
	}
	sort.Strings(matches)

	report := mermaidConformanceReport{suite: suite.name, root: root}
	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read Mermaid %s conformance file %s: %v", suite.name, path, err)
		}
		rel := filepath.Base(path)
		cases, skipped, unextractable := extractMermaidConformanceCases(rel, string(data))
		fileReport := mermaidConformanceFileReport{
			name:          rel,
			skipped:       skipped,
			unextractable: unextractable,
		}
		for _, testCase := range cases {
			result := evaluateMermaidConformanceCase(testCase)
			fileReport.cases++
			report.cases++
			if result.passed {
				fileReport.passed++
				report.passed++
				continue
			}
			fileReport.failed++
			report.failed++
			report.failures = append(report.failures, mermaidConformanceFailure{
				file:   testCase.file,
				line:   testCase.line,
				name:   testCase.name,
				reason: result.reason,
				source: testCase.source,
			})
		}
		report.skipped += skipped
		report.unextractable += unextractable
		report.files = append(report.files, fileReport)
	}

	return report
}

type mermaidCaseResult struct {
	passed bool
	reason string
}

func evaluateMermaidConformanceCase(testCase mermaidConformanceCase) mermaidCaseResult {
	parsed, err := Parse(testCase.source)
	rejected := err != nil || (parsed != nil && len(parsed.Warnings) > 0)
	if testCase.expectReject {
		if rejected {
			return mermaidCaseResult{passed: true}
		}
		return mermaidCaseResult{reason: "expected parser rejection, got accepted source"}
	}
	if err != nil {
		return mermaidCaseResult{reason: "expected parser acceptance, got error: " + err.Error()}
	}
	if len(parsed.Warnings) > 0 {
		return mermaidCaseResult{reason: "expected parser acceptance, got warning: " + parsed.Warnings[0]}
	}
	for _, expectation := range testCase.expectations {
		if reason := checkMermaidSemanticExpectation(parsed, expectation); reason != "" {
			return mermaidCaseResult{reason: reason}
		}
	}
	return mermaidCaseResult{passed: true}
}

func checkMermaidSemanticExpectation(parsed *ParsedDiagram, expectation mermaidSemanticExpectation) string {
	switch expectation.kind {
	case "direction":
		if string(parsed.Direction) != expectation.wantString {
			return fmt.Sprintf("direction = %q, want %q", parsed.Direction, expectation.wantString)
		}
	case "element_count":
		if len(parsed.Elements) != expectation.wantInt {
			return fmt.Sprintf("elements = %d, want %d", len(parsed.Elements), expectation.wantInt)
		}
	case "element_ref":
		if findMermaidElement(parsed, expectation.wantString) == nil {
			return fmt.Sprintf("missing element ref %q", expectation.wantString)
		}
	case "element_name":
		element := findMermaidElement(parsed, expectation.ref)
		if element == nil {
			return fmt.Sprintf("missing element ref %q for name expectation", expectation.ref)
		}
		if element.GetName() != expectation.wantString {
			return fmt.Sprintf("element %q name = %q, want %q", expectation.ref, element.GetName(), expectation.wantString)
		}
	case "connector_count":
		if len(parsed.Connectors) != expectation.wantInt {
			return fmt.Sprintf("connectors = %d, want %d", len(parsed.Connectors), expectation.wantInt)
		}
	case "connector_source":
		if reason := checkMermaidConnectorIndex(parsed, expectation.index); reason != "" {
			return reason
		}
		if got := parsed.Connectors[expectation.index].GetSourceElementRef(); got != expectation.wantString {
			return fmt.Sprintf("connector[%d].source = %q, want %q", expectation.index, got, expectation.wantString)
		}
	case "connector_target":
		if reason := checkMermaidConnectorIndex(parsed, expectation.index); reason != "" {
			return reason
		}
		if got := parsed.Connectors[expectation.index].GetTargetElementRef(); got != expectation.wantString {
			return fmt.Sprintf("connector[%d].target = %q, want %q", expectation.index, got, expectation.wantString)
		}
	case "connector_label":
		if reason := checkMermaidConnectorIndex(parsed, expectation.index); reason != "" {
			return reason
		}
		connector := parsed.Connectors[expectation.index]
		if connector.GetLabel() != expectation.wantString && connector.GetRelationship() != expectation.wantString {
			return fmt.Sprintf("connector[%d].label = %q/%q, want %q", expectation.index, connector.GetLabel(), connector.GetRelationship(), expectation.wantString)
		}
	case "connector_bidirectional":
		if reason := checkMermaidConnectorIndex(parsed, expectation.index); reason != "" {
			return reason
		}
		wantBoth := expectation.wantString == "both"
		gotBoth := parsed.Connectors[expectation.index].GetDirection() == "both"
		if gotBoth != wantBoth {
			return fmt.Sprintf("connector[%d].bidirectional = %t, want %t", expectation.index, gotBoth, wantBoth)
		}
	}
	return ""
}

func findMermaidElement(parsed *ParsedDiagram, ref string) interface{ GetName() string } {
	for _, element := range parsed.Elements {
		if element.GetRef() == ref {
			return element
		}
	}
	return nil
}

func checkMermaidConnectorIndex(parsed *ParsedDiagram, index int) string {
	if index < 0 || index >= len(parsed.Connectors) {
		return fmt.Sprintf("missing connector[%d], connectors = %d", index, len(parsed.Connectors))
	}
	return ""
}

func (r mermaidConformanceReport) String() string {
	var b strings.Builder
	_, _ = fmt.Fprintf(&b, "Mermaid %s conformance report\n", r.suite)
	_, _ = fmt.Fprintf(&b, "source: %s\n", r.root)
	_, _ = fmt.Fprintf(
		&b,
		"totals: cases=%d pass=%d fail=%d skipped=%d unextractable=%d\n",
		r.cases,
		r.passed,
		r.failed,
		r.skipped,
		r.unextractable,
	)
	for _, file := range r.files {
		_, _ = fmt.Fprintf(
			&b,
			"  %s: cases=%d pass=%d fail=%d skipped=%d unextractable=%d\n",
			file.name,
			file.cases,
			file.passed,
			file.failed,
			file.skipped,
			file.unextractable,
		)
	}
	if len(r.failures) == 0 {
		return b.String()
	}

	const maxFailures = 25
	limit := min(len(r.failures), maxFailures)
	_, _ = fmt.Fprintf(&b, "failures: showing %d of %d\n", limit, len(r.failures))
	for i := 0; i < limit; i++ {
		failure := r.failures[i]
		name := failure.name
		if name == "" {
			name = "parse call"
		}
		_, _ = fmt.Fprintf(
			&b,
			"  %s:%d %s: %s | %s\n",
			failure.file,
			failure.line,
			name,
			failure.reason,
			previewMermaidSource(failure.source),
		)
	}
	return b.String()
}

func previewMermaidSource(source string) string {
	preview := strings.ReplaceAll(source, "\n", `\n`)
	if len(preview) > 160 {
		return preview[:157] + "..."
	}
	return preview
}

type jsTestBlock struct {
	bodyStart  int
	bodyEnd    int
	name       string
	skipped    bool
	parseCount int
}

type jsParseCall struct {
	start int
	open  int
	close int
}

func extractMermaidConformanceCases(fileName, source string) ([]mermaidConformanceCase, int, int) {
	masked := maskJSCode(source)
	blocks := findJSTestBlocks(source, masked)
	calls := findJSParseCalls(masked)
	for callIndex := range calls {
		if block := innermostJSTestBlock(blocks, calls[callIndex].start); block != nil {
			block.parseCount++
		}
	}

	var cases []mermaidConformanceCase
	skipped := 0
	unextractable := 0
	for _, call := range calls {
		block := innermostJSTestBlock(blocks, call.start)
		if block != nil && block.skipped {
			skipped++
			continue
		}

		expr, ok := firstJSCallArgument(source, masked, call.open, call.close)
		if !ok {
			unextractable++
			continue
		}

		env := map[string]string{}
		name := ""
		expectReject := false
		expectations := []mermaidSemanticExpectation(nil)
		if block != nil {
			body := source[block.bodyStart:block.bodyEnd]
			bodyMask := masked[block.bodyStart:block.bodyEnd]
			env = buildJSStringEnvironment(body, bodyMask)
			name = block.name
			expectReject = jsBlockExpectsParseReject(body)
			if block.parseCount == 1 && !expectReject {
				expectations = extractMermaidSemanticExpectations(body, bodyMask, env)
			}
		}

		diagramSource, ok := evalJSStringExpression(expr, env)
		if !ok {
			unextractable++
			continue
		}
		cases = append(cases, mermaidConformanceCase{
			file:         fileName,
			line:         lineNumber(source, call.start),
			name:         name,
			source:       diagramSource,
			expectReject: expectReject,
			expectations: expectations,
		})
	}

	return cases, skipped, unextractable
}

func findJSTestBlocks(source, masked string) []*jsTestBlock {
	var blocks []*jsTestBlock
	for index := 0; index < len(masked); index++ {
		if !hasJSTokenAt(masked, index, "it") {
			continue
		}

		cursor := skipJSSpace(masked, index+len("it"))
		skipped := false
		each := false
		switch {
		case strings.HasPrefix(masked[cursor:], ".skip"):
			cursor = skipJSSpace(masked, cursor+len(".skip"))
			skipped = true
		case strings.HasPrefix(masked[cursor:], ".each"):
			cursor = skipJSSpace(masked, cursor+len(".each"))
			each = true
		}

		if cursor >= len(masked) || masked[cursor] != '(' {
			continue
		}
		open := cursor
		close := findMatchingJS(masked, open, '(', ')')
		if close == -1 {
			continue
		}

		if each {
			cursor = skipJSSpace(masked, close+1)
			if cursor >= len(masked) || masked[cursor] != '(' {
				continue
			}
			open = cursor
			close = findMatchingJS(masked, open, '(', ')')
			if close == -1 {
				continue
			}
		}

		bodyStart, bodyEnd := firstJSBraceRange(masked, open+1, close)
		if bodyStart == -1 {
			continue
		}
		blocks = append(blocks, &jsTestBlock{
			bodyStart: bodyStart + 1,
			bodyEnd:   bodyEnd,
			name:      firstJSStringArgument(source[open+1 : close]),
			skipped:   skipped,
		})
	}
	return blocks
}

func findJSParseCalls(masked string) []jsParseCall {
	var calls []jsParseCall
	for offset := 0; offset < len(masked); {
		index := strings.Index(masked[offset:], "parse")
		if index == -1 {
			break
		}
		index += offset
		offset = index + len("parse")
		if !hasJSTokenAt(masked, index, "parse") {
			continue
		}
		previous := previousNonSpace(masked, index)
		if previous == -1 || masked[previous] != '.' {
			continue
		}
		cursor := skipJSSpace(masked, index+len("parse"))
		if cursor >= len(masked) || masked[cursor] != '(' {
			continue
		}
		close := findMatchingJS(masked, cursor, '(', ')')
		if close == -1 {
			continue
		}
		calls = append(calls, jsParseCall{start: index, open: cursor, close: close})
	}
	return calls
}

func firstJSCallArgument(source, masked string, open, close int) (string, bool) {
	if open < 0 || close <= open || close > len(source) {
		return "", false
	}
	depth := 0
	for index := open + 1; index < close; index++ {
		switch masked[index] {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				return strings.TrimSpace(source[open+1 : index]), true
			}
		}
	}
	return strings.TrimSpace(source[open+1 : close]), true
}

func buildJSStringEnvironment(source, masked string) map[string]string {
	env := map[string]string{}
	for _, statement := range splitJSStatements(source, masked) {
		name, expr, ok := jsStringAssignment(statement)
		if !ok {
			continue
		}
		value, ok := evalJSStringExpression(expr, env)
		if !ok {
			continue
		}
		env[name] = value
	}
	return env
}

func splitJSStatements(source, masked string) []string {
	var statements []string
	start := 0
	depth := 0
	for index := 0; index < len(masked); index++ {
		switch masked[index] {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			if depth > 0 {
				depth--
			}
		case ';':
			if depth == 0 {
				statements = append(statements, strings.TrimSpace(source[start:index]))
				start = index + 1
			}
		}
	}
	return statements
}

func jsStringAssignment(statement string) (string, string, bool) {
	statement = strings.TrimSpace(statement)
	for _, prefix := range []string{"const ", "let "} {
		if strings.HasPrefix(statement, prefix) {
			statement = strings.TrimSpace(statement[len(prefix):])
			break
		}
	}

	equal := strings.Index(statement, "=")
	if equal == -1 || strings.Contains(statement[:equal], ".") {
		return "", "", false
	}
	name := strings.TrimSpace(statement[:equal])
	if !isJSIdentifier(name) {
		return "", "", false
	}
	if strings.HasPrefix(strings.TrimSpace(statement[equal:]), "==") {
		return "", "", false
	}
	return name, strings.TrimSpace(statement[equal+1:]), true
}

func evalJSStringExpression(expr string, env map[string]string) (string, bool) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return "", false
	}
	if inner, ok := unwrapJSCall(expr, "cleanupComments"); ok {
		return evalJSStringExpression(inner, env)
	}
	if inner, ok := unwrapJSParens(expr); ok {
		return evalJSStringExpression(inner, env)
	}

	parts := splitJSTopLevelPlus(expr)
	if len(parts) > 1 {
		var b strings.Builder
		for _, part := range parts {
			value, ok := evalJSStringExpression(part, env)
			if !ok {
				return "", false
			}
			b.WriteString(value)
		}
		return b.String(), true
	}

	if value, ok := env[expr]; ok {
		return value, true
	}
	if isJSStringLiteral(expr) {
		return decodeJSStringLiteral(expr)
	}
	return "", false
}

func unwrapJSCall(expr, name string) (string, bool) {
	if !strings.HasPrefix(expr, name) {
		return "", false
	}
	cursor := skipJSSpace(expr, len(name))
	if cursor >= len(expr) || expr[cursor] != '(' {
		return "", false
	}
	masked := maskJSCode(expr)
	close := findMatchingJS(masked, cursor, '(', ')')
	if close == -1 || strings.TrimSpace(expr[close+1:]) != "" {
		return "", false
	}
	arg, ok := firstJSCallArgument(expr, masked, cursor, close)
	return arg, ok
}

func unwrapJSParens(expr string) (string, bool) {
	if !strings.HasPrefix(expr, "(") {
		return "", false
	}
	masked := maskJSCode(expr)
	close := findMatchingJS(masked, 0, '(', ')')
	if close != len(expr)-1 {
		return "", false
	}
	return strings.TrimSpace(expr[1:close]), true
}

func splitJSTopLevelPlus(expr string) []string {
	masked := maskJSCode(expr)
	var parts []string
	start := 0
	depth := 0
	for index := 0; index < len(masked); index++ {
		switch masked[index] {
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			if depth > 0 {
				depth--
			}
		case '+':
			if depth == 0 {
				parts = append(parts, strings.TrimSpace(expr[start:index]))
				start = index + 1
			}
		}
	}
	if len(parts) == 0 {
		return nil
	}
	parts = append(parts, strings.TrimSpace(expr[start:]))
	return parts
}

func isJSStringLiteral(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) < 2 {
		return false
	}
	quote := value[0]
	return (quote == '\'' || quote == '"' || quote == '`') && value[len(value)-1] == quote
}

func decodeJSStringLiteral(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if !isJSStringLiteral(value) {
		return "", false
	}
	quote := value[0]
	body := value[1 : len(value)-1]
	if quote == '`' && strings.Contains(body, "${") {
		return "", false
	}

	var b strings.Builder
	for index := 0; index < len(body); index++ {
		if body[index] != '\\' {
			b.WriteByte(body[index])
			continue
		}
		index++
		if index >= len(body) {
			return "", false
		}
		switch body[index] {
		case '\n':
			continue
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case 'b':
			b.WriteByte('\b')
		case 'f':
			b.WriteByte('\f')
		case 'v':
			b.WriteByte('\v')
		case '0':
			b.WriteByte(0)
		case 'u':
			if index+4 >= len(body) {
				return "", false
			}
			code, err := strconv.ParseInt(body[index+1:index+5], 16, 32)
			if err != nil {
				return "", false
			}
			b.WriteRune(rune(code))
			index += 4
		default:
			b.WriteByte(body[index])
		}
	}
	return b.String(), true
}

func jsBlockExpectsParseReject(body string) bool {
	normalized := strings.ReplaceAll(body, " ", "")
	normalized = strings.ReplaceAll(normalized, "\n", "")
	if strings.Contains(normalized, ".not.toThrow") {
		return false
	}
	return strings.Contains(body, ".toThrow") || strings.Contains(body, "toThrowError")
}

func extractMermaidSemanticExpectations(source, masked string, env map[string]string) []mermaidSemanticExpectation {
	var expectations []mermaidSemanticExpectation
	for offset := 0; offset < len(masked); {
		index := strings.Index(masked[offset:], "expect")
		if index == -1 {
			break
		}
		index += offset
		offset = index + len("expect")
		if !hasJSTokenAt(masked, index, "expect") {
			continue
		}
		open := skipJSSpace(masked, index+len("expect"))
		if open >= len(masked) || masked[open] != '(' {
			continue
		}
		close := findMatchingJS(masked, open, '(', ')')
		if close == -1 {
			continue
		}
		wantOpen, wantClose, ok := jsExpectationValueRange(masked, close+1)
		if !ok {
			continue
		}
		expr := strings.TrimSpace(source[open+1 : close])
		wantExpr := strings.TrimSpace(source[wantOpen+1 : wantClose])
		expectations = append(expectations, mermaidExpectationFromJS(expr, wantExpr, env)...)
		offset = wantClose + 1
	}
	return expectations
}

func jsExpectationValueRange(masked string, start int) (int, int, bool) {
	cursor := skipJSSpace(masked, start)
	for _, method := range []string{".toBe", ".toEqual"} {
		if !strings.HasPrefix(masked[cursor:], method) {
			continue
		}
		open := skipJSSpace(masked, cursor+len(method))
		if open >= len(masked) || masked[open] != '(' {
			return 0, 0, false
		}
		close := findMatchingJS(masked, open, '(', ')')
		return open, close, close != -1
	}
	return 0, 0, false
}

func mermaidExpectationFromJS(expr, wantExpr string, env map[string]string) []mermaidSemanticExpectation {
	if expectation, ok := mermaidStringExpectationFromJS(expr, wantExpr, env); ok {
		return []mermaidSemanticExpectation{expectation}
	}
	if expectation, ok := mermaidIntExpectationFromJS(expr, wantExpr); ok {
		return []mermaidSemanticExpectation{expectation}
	}
	return nil
}

func mermaidStringExpectationFromJS(expr, wantExpr string, env map[string]string) (mermaidSemanticExpectation, bool) {
	want, ok := evalJSStringExpression(wantExpr, env)
	if !ok {
		return mermaidSemanticExpectation{}, false
	}

	switch {
	case expr == "direction" || expr == "flow.parser.yy.getDirection()":
		return mermaidSemanticExpectation{kind: "direction", wantString: want}, true
	case strings.HasPrefix(expr, "edges["):
		index, property, ok := parseJSEdgeProperty(expr)
		if !ok {
			return mermaidSemanticExpectation{}, false
		}
		switch property {
		case "start":
			return mermaidSemanticExpectation{kind: "connector_source", index: index, wantString: want}, true
		case "end":
			return mermaidSemanticExpectation{kind: "connector_target", index: index, wantString: want}, true
		case "text":
			return mermaidSemanticExpectation{kind: "connector_label", index: index, wantString: want}, true
		case "type":
			if strings.HasPrefix(want, "double_arrow") {
				return mermaidSemanticExpectation{kind: "connector_bidirectional", index: index, wantString: "both"}, true
			}
			if strings.HasPrefix(want, "arrow") {
				return mermaidSemanticExpectation{kind: "connector_bidirectional", index: index, wantString: "forward"}, true
			}
		}
	default:
		ref, property, ok := parseJSVertexProperty(expr, env)
		if !ok {
			return mermaidSemanticExpectation{}, false
		}
		switch property {
		case "id":
			return mermaidSemanticExpectation{kind: "element_ref", wantString: want}, true
		case "text":
			return mermaidSemanticExpectation{kind: "element_name", ref: ref, wantString: want}, true
		}
	}
	return mermaidSemanticExpectation{}, false
}

func mermaidIntExpectationFromJS(expr, wantExpr string) (mermaidSemanticExpectation, bool) {
	want, err := strconv.Atoi(strings.TrimSpace(wantExpr))
	if err != nil {
		return mermaidSemanticExpectation{}, false
	}
	switch strings.TrimSpace(expr) {
	case "edges.length":
		return mermaidSemanticExpectation{kind: "connector_count", wantInt: want}, true
	case "vert.size", "vertices.size":
		return mermaidSemanticExpectation{kind: "element_count", wantInt: want}, true
	default:
		return mermaidSemanticExpectation{}, false
	}
}

func parseJSEdgeProperty(expr string) (int, string, bool) {
	closeBracket := strings.Index(expr, "]")
	if closeBracket == -1 || !strings.HasPrefix(expr, "edges[") {
		return 0, "", false
	}
	index, err := strconv.Atoi(strings.TrimSpace(expr[len("edges["):closeBracket]))
	if err != nil {
		return 0, "", false
	}
	property := strings.TrimPrefix(strings.TrimSpace(expr[closeBracket+1:]), ".")
	if property == "" || strings.ContainsAny(property, ".[]()") {
		return 0, "", false
	}
	return index, property, true
}

func parseJSVertexProperty(expr string, env map[string]string) (string, string, bool) {
	getIndex := strings.Index(expr, ".get(")
	if getIndex == -1 {
		return "", "", false
	}
	objectName := strings.TrimSpace(expr[:getIndex])
	if objectName != "vert" && objectName != "vertices" {
		return "", "", false
	}
	open := getIndex + len(".get")
	masked := maskJSCode(expr)
	close := findMatchingJS(masked, open, '(', ')')
	if close == -1 {
		return "", "", false
	}
	ref, ok := evalJSStringExpression(expr[open+1:close], env)
	if !ok {
		return "", "", false
	}
	property := strings.TrimPrefix(strings.TrimSpace(expr[close+1:]), ".")
	if property == "" || strings.ContainsAny(property, ".[]()") {
		return "", "", false
	}
	return ref, property, true
}

func innermostJSTestBlock(blocks []*jsTestBlock, position int) *jsTestBlock {
	var current *jsTestBlock
	for _, block := range blocks {
		if position < block.bodyStart || position > block.bodyEnd {
			continue
		}
		if current == nil || block.bodyEnd-block.bodyStart < current.bodyEnd-current.bodyStart {
			current = block
		}
	}
	return current
}

func firstJSStringArgument(source string) string {
	source = strings.TrimSpace(source)
	if !isJSStringLiteralPrefix(source) {
		return ""
	}
	literal, ok := readJSStringLiteralPrefix(source)
	if !ok {
		return ""
	}
	value, ok := decodeJSStringLiteral(literal)
	if !ok {
		return ""
	}
	return value
}

func firstJSBraceRange(masked string, start, limit int) (int, int) {
	for index := start; index < limit; index++ {
		if masked[index] != '{' {
			continue
		}
		close := findMatchingJS(masked, index, '{', '}')
		if close != -1 {
			return index, close
		}
	}
	return -1, -1
}

func findMatchingJS(masked string, open int, left, right byte) int {
	if open < 0 || open >= len(masked) || masked[open] != left {
		return -1
	}
	depth := 0
	for index := open; index < len(masked); index++ {
		switch masked[index] {
		case left:
			depth++
		case right:
			depth--
			if depth == 0 {
				return index
			}
		}
	}
	return -1
}

func maskJSCode(source string) string {
	out := []byte(source)
	for index := 0; index < len(out); index++ {
		switch out[index] {
		case '/', '\'', '"', '`':
		default:
			continue
		}

		switch {
		case out[index] == '/' && index+1 < len(out) && out[index+1] == '/':
			index = maskJSLineComment(out, index)
		case out[index] == '/' && index+1 < len(out) && out[index+1] == '*':
			index = maskJSBlockComment(out, index)
		case out[index] == '\'', out[index] == '"', out[index] == '`':
			index = maskJSString(out, index, out[index])
		}
	}
	return string(out)
}

func maskJSLineComment(out []byte, start int) int {
	index := start
	for ; index < len(out) && out[index] != '\n'; index++ {
		out[index] = ' '
	}
	return index
}

func maskJSBlockComment(out []byte, start int) int {
	index := start
	for ; index < len(out); index++ {
		if index > start && out[index-1] == '*' && out[index] == '/' {
			out[index-1] = ' '
			out[index] = ' '
			return index
		}
		if out[index] != '\n' {
			out[index] = ' '
		}
	}
	return len(out) - 1
}

func maskJSString(out []byte, start int, quote byte) int {
	out[start] = ' '
	for index := start + 1; index < len(out); index++ {
		if out[index] == '\n' {
			if quote != '`' {
				return index
			}
			continue
		}
		if out[index] == '\\' {
			out[index] = ' '
			if index+1 < len(out) && out[index+1] != '\n' {
				index++
				out[index] = ' '
			}
			continue
		}
		if out[index] == quote {
			out[index] = ' '
			return index
		}
		out[index] = ' '
	}
	return len(out) - 1
}

func skipJSSpace(source string, index int) int {
	for index < len(source) && unicode.IsSpace(rune(source[index])) {
		index++
	}
	return index
}

func previousNonSpace(source string, index int) int {
	for index--; index >= 0; index-- {
		if !unicode.IsSpace(rune(source[index])) {
			return index
		}
	}
	return -1
}

func hasJSTokenAt(source string, index int, token string) bool {
	if index < 0 || index+len(token) > len(source) || source[index:index+len(token)] != token {
		return false
	}
	if index > 0 && isJSIdentifierRune(rune(source[index-1])) {
		return false
	}
	end := index + len(token)
	return end >= len(source) || !isJSIdentifierRune(rune(source[end]))
}

func isJSIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for index, char := range value {
		if index == 0 {
			if char != '_' && char != '$' && !unicode.IsLetter(char) {
				return false
			}
			continue
		}
		if !isJSIdentifierRune(char) {
			return false
		}
	}
	return true
}

func isJSIdentifierRune(char rune) bool {
	return char == '_' || char == '$' || unicode.IsLetter(char) || unicode.IsDigit(char)
}

func isJSStringLiteralPrefix(source string) bool {
	source = strings.TrimSpace(source)
	return len(source) > 0 && (source[0] == '\'' || source[0] == '"' || source[0] == '`')
}

func readJSStringLiteralPrefix(source string) (string, bool) {
	source = strings.TrimSpace(source)
	if !isJSStringLiteralPrefix(source) {
		return "", false
	}
	quote := source[0]
	for index := 1; index < len(source); index++ {
		if source[index] == '\n' && quote != '`' {
			return "", false
		}
		if source[index] == '\\' {
			index++
			continue
		}
		if source[index] == quote {
			return source[:index+1], true
		}
	}
	return "", false
}

func lineNumber(source string, position int) int {
	if position < 0 {
		return 1
	}
	if position > len(source) {
		position = len(source)
	}
	return strings.Count(source[:position], "\n") + 1
}

package parser

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	pb "buf.build/gen/go/tldiagramcom/diagram/protocolbuffers/go/codeindex/v1"
	"github.com/mertcikla/tld/v2/internal/codeindex/graph"
	"gopkg.in/yaml.v3"
)

// Fact is an infrastructure or configuration fact discovered by a deterministic
// scanner. It carries a subject/object pair rather than a captured code body and
// is published as a code_fact so it can be joined, filtered, and retrieved like
// any other Fact.
type Fact struct {
	Kind      pb.FactKind
	Subject   string
	Object    string
	Path      string
	Line      int
	Column    int
	EndLine   int
	EndColumn int
	Text      string
	Extractor string
}

// SkipDirs lists directory names that are never descended into while scanning.
var SkipDirs = map[string]bool{".git": true, "node_modules": true, "vendor": true, "dist": true, "build": true, "coverage": true, ".next": true, ".turbo": true, "target": true, ".gradle": true, "out": true, "test": true, "tests": true, "testdata": true, "__tests__": true, "fixtures": true}

// Scan reads the repository's configuration and deployment sources from the
// discovered source set and returns deterministic infra facts. Taking the
// discovered sources as input keeps the scan aligned with discovery (the same
// excludes, overrides, and ignore rules apply) and avoids a second walk.
func Scan(ctx context.Context, root string, sources map[string]*graph.Source) ([]Fact, error) {
	rels := make([]string, 0, len(sources))
	for rel := range sources {
		rels = append(rels, rel)
	}
	sort.Strings(rels)
	var out []Fact
	var charts []string
	for _, rel := range rels {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		name := filepath.Base(rel)
		path := filepath.Join(root, filepath.FromSlash(rel))
		if name == "Chart.yaml" {
			charts = append(charts, filepath.Dir(path))
		}
		ext := filepath.Ext(rel)
		if IsInfraSource(name, rel) {
			out = append(out, scanInfraFile(path, rel, region(rel))...)
		}
		if isCodeExt(ext) {
			out = append(out, codePatternFacts(path, rel, region(rel))...)
		}
	}
	for _, chart := range charts {
		out = append(out, helmFacts(root, chart)...)
	}
	out = dedupeInfra(out)
	out = append(out, bridgeFacts(out)...)
	return dedupeInfra(out), nil
}

var deployFileNames = map[string]bool{"Dockerfile": true, "docker-compose.yml": true, "compose.yml": true, "compose.yaml": true, "serverless.yml": true}
var manifestFileNames = map[string]bool{"package.json": true, "go.mod": true, "pyproject.toml": true, "Cargo.toml": true}

// IsInfraSource reports whether a file is a configuration or deployment source
// the scanner reads as a manifest.
func IsInfraSource(name, rel string) bool {
	ext := filepath.Ext(name)
	switch ext {
	case ".yaml", ".yml", ".json", ".toml", ".tf", ".proto":
		return true
	}
	if deployFileNames[name] || manifestFileNames[name] || strings.HasPrefix(name, "Dockerfile.") || name == "go.work" {
		return true
	}
	return false
}

func isCodeExt(ext string) bool {
	return map[string]bool{".go": true, ".js": true, ".jsx": true, ".ts": true, ".tsx": true, ".py": true, ".java": true, ".rs": true, ".rb": true, ".php": true, ".cs": true}[ext]
}

func scanInfraFile(path, rel, subject string) []Fact {
	name := filepath.Base(path)
	ext := filepath.Ext(path)
	var out []Fact
	switch {
	case name == "Dockerfile" || strings.HasPrefix(name, "Dockerfile."):
		out = dockerFacts(path, rel, subject)
	case ext == ".yaml" || ext == ".yml":
		out = yamlFacts(path, rel, subject)
	case ext == ".tf":
		out = terraformFacts(path, rel, subject)
	case ext == ".json":
		out = openAPIJSONFacts(path, rel, subject)
	case ext == ".proto":
		out = protoFacts(path, rel, subject)
	}
	out = append(out, workspaceFacts(path, rel)...)
	b, e := os.ReadFile(path)
	if e != nil {
		return out
	}
	lines := bytes.Split(b, []byte("\n"))
	for i := range out {
		n := out[i].Line
		if n > 0 && n <= len(lines) {
			out[i].Text = strings.TrimSpace(string(lines[n-1]))
			if out[i].Column == 0 {
				out[i].Column = 1
				out[i].EndLine = n
				out[i].EndColumn = len(lines[n-1]) + 1
			}
		}
	}
	return out
}

func dockerFacts(path, rel, subject string) []Fact {
	b, e := os.ReadFile(path)
	if e != nil {
		return nil
	}
	out := []Fact{{Kind: pb.FactKind_FACT_KIND_DEPLOYABLE, Subject: subject, Object: "Dockerfile", Path: rel, Line: 1, Text: "Dockerfile", Extractor: "local"}}
	scanner := bufio.NewScanner(bytes.NewReader(b))
	line := 0
	for scanner.Scan() {
		line++
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		fields := strings.Fields(raw)
		if len(fields) < 2 {
			continue
		}
		switch strings.ToUpper(fields[0]) {
		case "FROM":
			out = append(out, mkInfra(pb.FactKind_FACT_KIND_DEPENDENCY, subject, fields[1], rel, line, raw, "local"))
		case "CMD", "ENTRYPOINT":
			out = append(out, mkInfra(pb.FactKind_FACT_KIND_ENTRYPOINT, subject, raw, rel, line, raw, "local"))
		case "ENV":
			if len(fields) > 1 {
				key := strings.SplitN(fields[1], "=", 2)[0]
				out = append(out, mkInfra(pb.FactKind_FACT_KIND_ENV, subject, key, rel, line, raw, "local"))
			}
		}
	}
	return out
}

func yamlFacts(path, rel, subject string) []Fact {
	b, e := os.ReadFile(path)
	if e != nil {
		return nil
	}
	var out []Fact
	// Decode documents independently so a malformed document does not discard
	// the ones that follow it (yaml.Decoder cannot resynchronize after an error).
	for _, root := range yamlDocuments(b) {
		if field(root, "openapi") != nil || field(root, "swagger") != nil {
			out = append(out, openAPIYAMLFacts(root, rel, subject)...)
			continue
		}
		if field(root, "AWSTemplateFormatVersion") != nil || field(root, "Resources") != nil {
			out = append(out, cloudFormationFacts(root, rel)...)
			continue
		}
		if isComposeFile(rel, root) {
			out = append(out, composeFacts(root, rel)...)
			continue
		}
		if kind := scalar(field(root, "kind")); kind != "" {
			if kind == "Component" || kind == "API" || kind == "System" {
				out = append(out, backstageFacts(root, rel, kind)...)
				continue
			}
			out = append(out, kubernetesFacts(root, rel, subject, kind)...)
			continue
		}
		if field(root, "functions") != nil {
			out = append(out, serverlessFacts(root, rel, subject)...)
			continue
		}
	}
	return out
}

// yamlDocuments splits a YAML stream into documents on "---" separator lines and
// decodes each independently, shifting node lines back to their position in the
// original file so provenance survives the split.
func yamlDocuments(b []byte) []*yaml.Node {
	lines := bytes.SplitAfter(b, []byte("\n"))
	var docs []*yaml.Node
	start := 0
	flush := func(end int) {
		if end <= start {
			return
		}
		var doc yaml.Node
		if yaml.Unmarshal(bytes.Join(lines[start:end], nil), &doc) == nil && len(doc.Content) > 0 {
			shiftYAML(&doc, start)
			docs = append(docs, doc.Content[0])
		}
	}
	for i, line := range lines {
		if isYAMLDocSeparator(line) {
			flush(i)
			start = i + 1
		}
	}
	flush(len(lines))
	return docs
}

func isYAMLDocSeparator(line []byte) bool {
	s := strings.TrimSpace(string(line))
	return s == "---" || strings.HasPrefix(s, "--- #")
}

func shiftYAML(n *yaml.Node, offset int) {
	if n == nil || offset == 0 {
		return
	}
	n.Line += offset
	for _, c := range n.Content {
		shiftYAML(c, offset)
	}
}

func mkInfra(kind pb.FactKind, subject, object, rel string, line int, text, extractor string) Fact {
	return Fact{Kind: kind, Subject: subject, Object: object, Path: rel, Line: line, Text: text, Extractor: extractor}
}

func isComposeFile(rel string, root *yaml.Node) bool {
	base := filepath.Base(rel)
	return (strings.Contains(base, "compose") || field(root, "services") != nil) && field(root, "services") != nil
}

func composeFacts(root *yaml.Node, rel string) []Fact {
	var out []Fact
	services := field(root, "services")
	if services == nil || services.Kind != yaml.MappingNode {
		return out
	}
	for i := 0; i+1 < len(services.Content); i += 2 {
		key, val := services.Content[i], services.Content[i+1]
		name := key.Value
		out = append(out, nodeInfra(pb.FactKind_FACT_KIND_DEPLOYABLE, name, "compose service", rel, key))
		if n := field(val, "image"); n != nil {
			out = append(out, nodeInfra(pb.FactKind_FACT_KIND_DEPENDENCY, name, n.Value, rel, n))
			out = append(out, imageCategory(name, n.Value, rel, n)...)
		}
		if n := field(val, "build"); n != nil {
			v := scalar(n)
			if v == "" {
				v = scalar(field(n, "context"))
			}
			if v != "" {
				out = append(out, nodeInfra(pb.FactKind_FACT_KIND_DEPENDENCY, name, v, rel, n))
			}
		}
		if n := field(val, "depends_on"); n != nil {
			for _, target := range values(n) {
				out = append(out, nodeInfra(pb.FactKind_FACT_KIND_DEPENDENCY, name, target, rel, n))
			}
		}
		if n := field(val, "environment"); n != nil {
			for _, env := range keys(n) {
				out = append(out, nodeInfra(pb.FactKind_FACT_KIND_ENV, name, env, rel, n))
			}
		}
	}
	return out
}

func kubernetesFacts(root *yaml.Node, rel, subject, kind string) []Fact {
	name := scalar(field(field(root, "metadata"), "name"))
	if name == "" {
		name = subject
	}
	node := field(root, "kind")
	var out []Fact
	switch kind {
	case "Deployment", "StatefulSet", "DaemonSet", "Job", "CronJob", "Pod":
		out = append(out, nodeInfra(pb.FactKind_FACT_KIND_DEPLOYABLE, name, kind, rel, node))
	default:
		return nil
	}
	walkYaml(root, func(key string, n *yaml.Node) {
		switch key {
		case "image":
			if n.Kind == yaml.ScalarNode {
				out = append(out, nodeInfra(pb.FactKind_FACT_KIND_DEPENDENCY, name, n.Value, rel, n))
				out = append(out, imageCategory(name, n.Value, rel, n)...)
			}
		case "secretKeyRef", "configMapKeyRef":
			out = append(out, nodeInfra(pb.FactKind_FACT_KIND_CONFIG_REF, name, key, rel, n))
		}
	})
	return out
}

func serverlessFacts(root *yaml.Node, rel, subject string) []Fact {
	fns := field(root, "functions")
	if fns == nil || fns.Kind != yaml.MappingNode {
		return nil
	}
	var out []Fact
	for i := 0; i+1 < len(fns.Content); i += 2 {
		key, val := fns.Content[i], fns.Content[i+1]
		name := key.Value
		out = append(out, nodeInfra(pb.FactKind_FACT_KIND_DEPLOYABLE, name, "serverless function", rel, key))
		if n := field(val, "handler"); n != nil {
			out = append(out, nodeInfra(pb.FactKind_FACT_KIND_ENTRYPOINT, name, n.Value, rel, n))
		}
	}
	return out
}

var tfResource = regexp.MustCompile(`^resource\s+"([^"]+)"\s+"([^"]+)"`)

func terraformFacts(path, rel, subject string) []Fact {
	b, e := os.ReadFile(path)
	if e != nil {
		return nil
	}
	var out []Fact
	scanner := bufio.NewScanner(bytes.NewReader(b))
	line := 0
	for scanner.Scan() {
		line++
		raw := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(raw, "#") || strings.HasPrefix(raw, "//") {
			continue
		}
		m := tfResource.FindStringSubmatch(raw)
		if len(m) == 0 {
			continue
		}
		kind, name := m[1], m[2]
		if deployableResource(kind) {
			out = append(out, mkInfra(pb.FactKind_FACT_KIND_DEPLOYABLE, name, kind, rel, line, raw, "hcl"))
		}
		if category := resourceCategory(kind); category != pb.FactKind_FACT_KIND_UNSPECIFIED {
			out = append(out, mkInfra(category, name, kind, rel, line, raw, "hcl"))
		}
	}
	return out
}

func deployableResource(kind string) bool {
	for _, token := range []string{"lambda_function", "cloud_run_service", "ecs_service", "kubernetes_deployment", "container_app", "app_service", "function_app"} {
		if strings.Contains(strings.ToLower(kind), token) {
			return true
		}
	}
	return false
}

func cloudFormationFacts(root *yaml.Node, rel string) []Fact {
	resources := field(root, "Resources")
	if resources == nil || resources.Kind != yaml.MappingNode {
		return nil
	}
	var out []Fact
	for i := 0; i+1 < len(resources.Content); i += 2 {
		key, value := resources.Content[i], resources.Content[i+1]
		kind := scalar(field(value, "Type"))
		if kind == "" {
			continue
		}
		name := key.Value
		if deployableResource(kind) || strings.Contains(kind, "AWS::Lambda::Function") || strings.Contains(kind, "AWS::ECS::Service") {
			out = append(out, nodeInfra(pb.FactKind_FACT_KIND_DEPLOYABLE, name, kind, rel, key))
		}
		if category := resourceCategory(kind); category != pb.FactKind_FACT_KIND_UNSPECIFIED {
			out = append(out, nodeInfra(category, name, kind, rel, key))
		}
	}
	return out
}

func backstageFacts(root *yaml.Node, rel, kind string) []Fact {
	metadata := field(root, "metadata")
	nameNode := field(metadata, "name")
	name := scalar(nameNode)
	if name == "" {
		return nil
	}
	out := []Fact{nodeInfra(pb.FactKind_FACT_KIND_CATALOG_COMPONENT, name, kind, rel, nameNode)}
	spec := field(root, "spec")
	if n := field(spec, "dependsOn"); n != nil {
		for _, dep := range values(n) {
			part := dep
			if at := strings.LastIndexAny(part, "/:"); at >= 0 {
				part = part[at+1:]
			}
			out = append(out, nodeInfra(pb.FactKind_FACT_KIND_DEPENDENCY, name, part, rel, n))
		}
	}
	return out
}

func resourceCategory(kind string) pb.FactKind {
	v := strings.ToLower(kind)
	for _, needle := range []string{"rds", "dynamodb", "redis", "sql_database", "mongodb"} {
		if strings.Contains(v, needle) {
			return pb.FactKind_FACT_KIND_DATASTORE
		}
	}
	for _, needle := range []string{"sqs", "kafka", "pubsub", "rabbitmq"} {
		if strings.Contains(v, needle) {
			return pb.FactKind_FACT_KIND_QUEUE
		}
	}
	return pb.FactKind_FACT_KIND_UNSPECIFIED
}

func imageCategory(subject, image, rel string, n *yaml.Node) []Fact {
	v := strings.ToLower(image)
	for _, needle := range []string{"postgres", "mysql", "mongo", "redis", "sqlite", "surrealdb", "dynamodb"} {
		if strings.Contains(v, needle) {
			return []Fact{nodeInfra(pb.FactKind_FACT_KIND_DATASTORE, subject, needle, rel, n)}
		}
	}
	for _, needle := range []string{"kafka", "rabbitmq", "nats"} {
		if strings.Contains(v, needle) {
			return []Fact{nodeInfra(pb.FactKind_FACT_KIND_QUEUE, subject, needle, rel, n)}
		}
	}
	return nil
}

func nodeInfra(kind pb.FactKind, subject, obj, rel string, n *yaml.Node) Fact {
	line := 1
	text := ""
	column, endLine, endColumn := 0, 0, 0
	if n != nil {
		line = n.Line
		text = n.Value
		column = n.Column
		endLine = n.Line
		endColumn = n.Column + len(n.Value)
	}
	return Fact{Kind: kind, Subject: subject, Object: obj, Path: rel, Line: line, Column: column, EndLine: endLine, EndColumn: endColumn, Text: text, Extractor: "structured"}
}

func field(n *yaml.Node, key string) *yaml.Node {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}
func scalar(n *yaml.Node) string {
	if n != nil && n.Kind == yaml.ScalarNode {
		return n.Value
	}
	return ""
}
func values(n *yaml.Node) []string {
	if n == nil {
		return nil
	}
	var out []string
	switch n.Kind {
	case yaml.SequenceNode:
		for _, v := range n.Content {
			if v.Kind == yaml.ScalarNode {
				out = append(out, v.Value)
			}
		}
	case yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			out = append(out, n.Content[i].Value)
		}
	}
	return out
}
func keys(n *yaml.Node) []string {
	if n == nil {
		return nil
	}
	if n.Kind == yaml.MappingNode {
		return values(n)
	}
	var out []string
	for _, v := range values(n) {
		out = append(out, strings.SplitN(v, "=", 2)[0])
	}
	return out
}
func walkYaml(n *yaml.Node, visit func(string, *yaml.Node)) {
	if n == nil {
		return
	}
	if n.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(n.Content); i += 2 {
			key, val := n.Content[i], n.Content[i+1]
			visit(key.Value, val)
			walkYaml(val, visit)
		}
	} else {
		for _, v := range n.Content {
			walkYaml(v, visit)
		}
	}
}

func region(rel string) string {
	p := strings.Split(rel, "/")
	if len(p) > 1 {
		return p[0]
	}
	return "root"
}

// codePatternFacts records source observations that do not assert an
// architectural role. Calls and their arguments are available through the
// SCIP/tree-sitter graph; a decision model can judge whether they are routes,
// outbound requests, or other runtime interactions. Token matches such as
// GET("size") cannot safely make that judgment here.
var patternRules = []struct {
	kind  pb.FactKind
	re    *regexp.Regexp
	group int
}{
	{pb.FactKind_FACT_KIND_ENV, regexp.MustCompile(`(?:os\.Getenv|process\.env\.|getenv\()\s*\(?["']?([A-Z][A-Z0-9_]+)`), 1},
}

func codePatternFacts(path, rel, subject string) []Fact {
	b, e := os.ReadFile(path)
	if e != nil {
		return nil
	}
	var out []Fact
	scanner := bufio.NewScanner(bytes.NewReader(b))
	scanner.Buffer(make([]byte, 4096), 2<<20)
	line := 0
	for scanner.Scan() {
		line++
		raw := scanner.Text()
		if len(raw) > 2000 {
			continue
		}
		code := stripInlineComment(raw)
		if strings.TrimSpace(code) == "" {
			continue
		}
		for _, rule := range patternRules {
			for _, m := range rule.re.FindAllStringSubmatchIndex(code, -1) {
				start, end := m[2*rule.group], m[2*rule.group+1]
				if start < 0 || end < start {
					continue
				}
				value := strings.TrimSpace(code[start:end])
				if value == "" {
					continue
				}
				f := mkInfra(rule.kind, subject, value, rel, line, strings.TrimSpace(raw), "local")
				f.Column, f.EndLine, f.EndColumn = start+1, line, end+1
				out = append(out, f)
			}
		}
		if strings.Contains(code, "func main(") || strings.Contains(code, "if __name__ ==") || strings.Contains(code, "app.listen(") {
			out = append(out, mkInfra(pb.FactKind_FACT_KIND_ENTRYPOINT, subject, "process", rel, line, strings.TrimSpace(raw), "local"))
		}
		if strings.Contains(raw, "kind: Deployment") || strings.Contains(raw, "kind: Service") {
			out = append(out, mkInfra(pb.FactKind_FACT_KIND_DEPLOYABLE, subject, strings.TrimSpace(raw), rel, line, strings.TrimSpace(raw), "local"))
		}
	}
	return out
}

func stripInlineComment(raw string) string {
	quote := byte(0)
	escaped := false
	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == quote {
				quote = 0
			}
			continue
		}
		if ch == '\'' || ch == '"' || ch == '`' {
			quote = ch
			continue
		}
		if ch == '#' || ch == '/' && i+1 < len(raw) && raw[i+1] == '/' {
			return raw[:i]
		}
	}
	return raw
}

func bridgeFacts(in []Fact) []Fact {
	routes := map[string][]Fact{}
	for _, f := range in {
		if f.Kind == pb.FactKind_FACT_KIND_ROUTE {
			routes[f.Object] = append(routes[f.Object], f)
		}
	}
	var out []Fact
	for _, f := range in {
		if f.Kind != pb.FactKind_FACT_KIND_OUTBOUND {
			continue
		}
		for _, route := range routes[f.Object] {
			if route.Subject == f.Subject {
				continue
			}
			object := f.Subject + "->" + route.Subject + ":" + f.Object
			out = append(out, Fact{Kind: pb.FactKind_FACT_KIND_BRIDGE_HTTP, Subject: f.Subject, Object: object, Path: f.Path, Line: f.Line, Text: f.Text, Extractor: "reconciled"})
		}
	}
	return out
}

func dedupeInfra(in []Fact) []Fact {
	seen := map[string]bool{}
	out := make([]Fact, 0, len(in))
	for _, f := range in {
		key := graph.ID(f.Kind.String(), f.Subject, f.Object, f.Path, fmt.Sprintf("%d:%d", f.Line, f.Column))
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, f)
	}
	return out
}

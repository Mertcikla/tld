package enrich

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/mertcikla/tld/internal/analyzer"
)

type enricherFunc struct {
	meta  Metadata
	match func(FileInput) bool
	run   func(context.Context, FileInput, FactEmitter) error
}

func (e enricherFunc) Metadata() Metadata { return e.meta }
func (e enricherFunc) MatchFile(input FileInput) bool {
	if e.match == nil {
		return true
	}
	return e.match(input)
}
func (e enricherFunc) EnrichFile(ctx context.Context, input FileInput, emit FactEmitter) error {
	if e.run == nil {
		return nil
	}
	return e.run(ctx, input, emit)
}

func DependencyInventory() Enricher {
	return enricherFunc{
		meta: Metadata{ID: "dependency.inventory", Name: "Dependency and import inventory", Mode: ActivationAlways},
		match: func(input FileInput) bool {
			base := path.Base(input.RelPath)
			return base == "go.mod" || base == "package.json" || input.Parsed != nil
		},
		run: dependencyInventoryRun,
	}
}

func GoNetHTTP() Enricher {
	return routeRegexEnricher("go.nethttp", "Go net/http routes", "go", []ActivationSignal{
		{Kind: SignalImport, Value: "net/http"},
		{Kind: SignalDependency, Value: "net/http"},
	}, []*routePattern{
		{re: regexp.MustCompile(`\bhttp\.HandleFunc\(\s*"([^"]+)"`), method: "", framework: "nethttp"},
		{re: regexp.MustCompile(`\b[A-Za-z_][A-Za-z0-9_]*\.HandleFunc\(\s*"([^"]+)"`), method: "", framework: "nethttp"},
	})
}

func GoChi() Enricher {
	return routeRegexEnricher("go.chi", "Go chi routes", "go", []ActivationSignal{
		{Kind: SignalImport, Value: "github.com/go-chi/chi"},
		{Kind: SignalDependency, Value: "github.com/go-chi/chi"},
	}, []*routePattern{
		{re: regexp.MustCompile(`\b[A-Za-z_][A-Za-z0-9_]*\.(Get|Post|Put|Delete|Patch)\(\s*"([^"]+)"`), framework: "chi", methodGroup: 1, pathGroup: 2},
		{re: regexp.MustCompile(`\b[A-Za-z_][A-Za-z0-9_]*\.Route\(\s*"([^"]+)"`), framework: "chi"},
	})
}

func GoGin() Enricher {
	return routeRegexEnricher("go.gin", "Go gin routes", "go", []ActivationSignal{
		{Kind: SignalImport, Value: "github.com/gin-gonic/gin"},
		{Kind: SignalDependency, Value: "github.com/gin-gonic/gin"},
	}, []*routePattern{
		{re: regexp.MustCompile(`\b[A-Za-z_][A-Za-z0-9_]*\.(GET|POST|PUT|DELETE|PATCH)\(\s*"([^"]+)"`), framework: "gin", methodGroup: 1, pathGroup: 2},
		{re: regexp.MustCompile(`\b[A-Za-z_][A-Za-z0-9_]*\.Group\(\s*"([^"]+)"`), framework: "gin"},
	})
}

func TSExpress() Enricher {
	return routeRegexEnricher("ts.express", "Express routes", "typescript,javascript", []ActivationSignal{
		{Kind: SignalImport, Value: "express"},
		{Kind: SignalDependency, Value: "express"},
	}, []*routePattern{
		{re: regexp.MustCompile(`\b(?:app|router)\.(get|post|put|delete|patch)\(\s*["'\x60]([^"'\x60]+)["'\x60]`), framework: "express", methodGroup: 1, pathGroup: 2},
	})
}

func TSNextJS() Enricher {
	return enricherFunc{
		meta: Metadata{
			ID:   "ts.nextjs",
			Name: "Next.js routes",
			Mode: ActivationImportOrDependency,
			Triggers: []ActivationSignal{
				{Kind: SignalDependency, Value: "next"},
				{Kind: SignalImport, Value: "next"},
			},
		},
		match: func(input FileInput) bool {
			route := nextRoutePath(input.RelPath)
			return route != ""
		},
		run: func(ctx context.Context, input FileInput, emit FactEmitter) error {
			route := nextRoutePath(input.RelPath)
			if route == "" {
				return nil
			}
			return emit.EmitFact(Fact{
				Type:       "frontend.route",
				StableKey:  fmt.Sprintf("frontend.route:nextjs:%s:%s", input.RelPath, route),
				Subject:    fileSubject(input.RelPath),
				Source:     SourceSpan{FilePath: input.RelPath, StartLine: 1, EndLine: 1},
				Confidence: 0.95,
				Name:       route,
				Tags:       []string{"frontend:route", "framework:nextjs"},
				Attributes: map[string]string{"framework": "nextjs", "path": route},
			})
		},
	}
}

func TSReactRouter() Enricher {
	return routeRegexEnricher("ts.react_router", "React Router routes", "typescript,javascript", []ActivationSignal{
		{Kind: SignalImport, Value: "react-router"},
		{Kind: SignalImport, Value: "react-router-dom"},
		{Kind: SignalDependency, Value: "react-router"},
		{Kind: SignalDependency, Value: "react-router-dom"},
	}, []*routePattern{
		{re: regexp.MustCompile(`<Route\b[^>]*\bpath\s*=\s*["'{\x60]([^"'}\x60]+)["'}\x60]`), factType: "frontend.route", framework: "react-router", tags: []string{"frontend:route", "framework:react-router"}},
		{re: regexp.MustCompile(`\bpath\s*:\s*["'\x60]([^"'\x60]+)["'\x60]`), factType: "frontend.route", framework: "react-router", tags: []string{"frontend:route", "framework:react-router"}},
	})
}

func TSPrisma() Enricher {
	return enricherFunc{
		meta: Metadata{
			ID:   "ts.prisma",
			Name: "Prisma ORM queries",
			Mode: ActivationImportOrDependency,
			Triggers: []ActivationSignal{
				{Kind: SignalImport, Value: "@prisma/client"},
				{Kind: SignalDependency, Value: "@prisma/client"},
				{Kind: SignalDependency, Value: "prisma"},
			},
		},
		match: matchLanguages("typescript", "javascript"),
		run: func(ctx context.Context, input FileInput, emit FactEmitter) error {
			return emitMatches(input, emit, []*routePattern{{
				re:        regexp.MustCompile(`\bprisma\.([A-Za-z_][A-Za-z0-9_]*)\.(findMany|findUnique|findFirst|create|createMany|update|updateMany|delete|deleteMany|upsert|aggregate|count)\b`),
				factType:  "orm.query",
				framework: "prisma",
				tags:      []string{"orm:prisma"},
				custom: func(match []string) (name string, attrs map[string]string, tags []string) {
					return match[1] + "." + match[2], map[string]string{"orm": "prisma", "model": match[1], "operation": match[2]}, []string{"orm:prisma"}
				},
			}})
		},
	}
}

type routePattern struct {
	re          *regexp.Regexp
	factType    string
	method      string
	framework   string
	methodGroup int
	pathGroup   int
	tags        []string
	custom      func([]string) (string, map[string]string, []string)
}

func routeRegexEnricher(id, name, languages string, triggers []ActivationSignal, patterns []*routePattern) Enricher {
	return enricherFunc{
		meta: Metadata{ID: id, Name: name, Mode: ActivationImportOrDependency, Triggers: triggers},
		match: func(input FileInput) bool {
			allowed := strings.Split(languages, ",")
			return matchLanguages(allowed...)(input)
		},
		run: func(ctx context.Context, input FileInput, emit FactEmitter) error {
			return emitMatches(input, emit, patterns)
		},
	}
}

func emitMatches(input FileInput, emit FactEmitter, patterns []*routePattern) error {
	source := string(input.Source)
	for _, pattern := range patterns {
		matches := pattern.re.FindAllStringSubmatchIndex(source, -1)
		for _, indexes := range matches {
			match := submatches(source, indexes)
			line := lineForOffset(source, indexes[0])
			factType := pattern.factType
			if factType == "" {
				factType = "http.route"
			}
			name, attrs, tags := routeFactValues(pattern, match)
			if name == "" {
				continue
			}
			subject := subjectForLine(input, line)
			key := fmt.Sprintf("%s:%s:%s:%s:%d", factType, pattern.framework, input.RelPath, name, line)
			if err := emit.EmitFact(Fact{
				Type:       factType,
				StableKey:  key,
				Subject:    subject,
				Source:     SourceSpan{FilePath: input.RelPath, StartLine: line, EndLine: line},
				Confidence: 0.90,
				Name:       name,
				Tags:       tags,
				Attributes: attrs,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func routeFactValues(pattern *routePattern, match []string) (string, map[string]string, []string) {
	if pattern.custom != nil {
		return pattern.custom(match)
	}
	method := strings.ToUpper(pattern.method)
	routePath := ""
	if pattern.pathGroup > 0 && pattern.pathGroup < len(match) {
		routePath = match[pattern.pathGroup]
	} else if len(match) > 1 {
		routePath = match[1]
	}
	if pattern.methodGroup > 0 && pattern.methodGroup < len(match) {
		method = strings.ToUpper(match[pattern.methodGroup])
	}
	attrs := map[string]string{"framework": pattern.framework, "path": routePath}
	name := routePath
	if method != "" {
		attrs["method"] = method
		name = method + " " + routePath
	}
	tags := append([]string{}, pattern.tags...)
	if len(tags) == 0 {
		tags = []string{"http:route"}
	}
	if pattern.framework != "" {
		tags = append(tags, "framework:"+pattern.framework)
	}
	return name, attrs, tags
}

func dependencyInventoryRun(ctx context.Context, input FileInput, emit FactEmitter) error {
	base := path.Base(input.RelPath)
	switch base {
	case "go.mod":
		return emitGoModFacts(input, emit)
	case "package.json":
		return emitPackageJSONFacts(input, emit)
	}
	if input.Parsed == nil {
		return nil
	}
	for _, ref := range input.Parsed.Refs {
		if ref.Kind != "import" || strings.TrimSpace(ref.TargetPath) == "" {
			continue
		}
		line := ref.Line
		if line <= 0 {
			line = 1
		}
		if err := emit.EmitFact(Fact{
			Type:       "dependency.import",
			StableKey:  fmt.Sprintf("dependency.import:%s:%s:%d", input.RelPath, ref.TargetPath, line),
			Subject:    fileSubject(input.RelPath),
			Source:     SourceSpan{FilePath: input.RelPath, StartLine: line, StartColumn: ref.Column},
			Confidence: 1,
			Name:       ref.TargetPath,
			Tags:       []string{"dependency:import"},
			Attributes: map[string]string{"module": ref.TargetPath, "name": ref.Name},
		}); err != nil {
			return err
		}
	}
	return nil
}

func emitGoModFacts(input FileInput, emit FactEmitter) error {
	scanner := bufio.NewScanner(strings.NewReader(string(input.Source)))
	line := 0
	for scanner.Scan() {
		line++
		match := goRequireLineRE.FindStringSubmatch(scanner.Text())
		if len(match) != 2 {
			continue
		}
		if err := emit.EmitFact(dependencyFact(input.RelPath, line, match[1], "go")); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func emitPackageJSONFacts(input FileInput, emit FactEmitter) error {
	var pkg struct {
		Dependencies         map[string]string `json:"dependencies"`
		DevDependencies      map[string]string `json:"devDependencies"`
		PeerDependencies     map[string]string `json:"peerDependencies"`
		OptionalDependencies map[string]string `json:"optionalDependencies"`
	}
	if err := json.Unmarshal(input.Source, &pkg); err != nil {
		return nil
	}
	names := map[string]string{}
	add := func(section string, values map[string]string) {
		for name := range values {
			names[name] = section
		}
	}
	add("dependencies", pkg.Dependencies)
	add("devDependencies", pkg.DevDependencies)
	add("peerDependencies", pkg.PeerDependencies)
	add("optionalDependencies", pkg.OptionalDependencies)
	var sorted []string
	for name := range names {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)
	for _, name := range sorted {
		fact := dependencyFact(input.RelPath, 1, name, "npm")
		fact.Attributes["section"] = names[name]
		if err := emit.EmitFact(fact); err != nil {
			return err
		}
	}
	return nil
}

func dependencyFact(relPath string, line int, name, ecosystem string) Fact {
	return Fact{
		Type:       "dependency.module",
		StableKey:  fmt.Sprintf("dependency.module:%s:%s", relPath, name),
		Subject:    fileSubject(relPath),
		Source:     SourceSpan{FilePath: relPath, StartLine: line, EndLine: line},
		Confidence: 1,
		Name:       name,
		Tags:       []string{"dependency:module"},
		Attributes: map[string]string{"module": name, "ecosystem": ecosystem},
	}
}

func matchLanguages(languages ...string) func(FileInput) bool {
	allowed := map[string]struct{}{}
	for _, language := range languages {
		language = strings.TrimSpace(strings.ToLower(language))
		if language != "" {
			allowed[language] = struct{}{}
		}
	}
	return func(input FileInput) bool {
		_, ok := allowed[strings.ToLower(input.Language)]
		return ok
	}
}

func subjectForLine(input FileInput, line int) SubjectRef {
	if input.Parsed != nil {
		for _, sym := range input.Parsed.Symbols {
			end := sym.EndLine
			if end <= 0 {
				end = sym.Line
			}
			if sym.Line <= line && end >= line {
				return SubjectRef{
					Kind:      "symbol",
					StableKey: symbolStableKey(input.Language, input.RelPath, sym),
					FilePath:  input.RelPath,
					Name:      symbolQualifiedName(sym),
				}
			}
		}
	}
	return fileSubject(input.RelPath)
}

func fileSubject(relPath string) SubjectRef {
	return SubjectRef{Kind: "file", StableKey: "file:" + relPath, FilePath: relPath, Name: path.Base(relPath)}
}

func symbolStableKey(language, relPath string, sym analyzer.Symbol) string {
	return fmt.Sprintf("%s:%s:%s:%s", language, relPath, sym.Kind, symbolQualifiedName(sym))
}

func symbolQualifiedName(sym analyzer.Symbol) string {
	if sym.Parent == "" {
		return sym.Name
	}
	return sym.Parent + "." + sym.Name
}

func submatches(source string, indexes []int) []string {
	out := make([]string, 0, len(indexes)/2)
	for i := 0; i < len(indexes); i += 2 {
		if indexes[i] < 0 || indexes[i+1] < 0 {
			out = append(out, "")
			continue
		}
		out = append(out, source[indexes[i]:indexes[i+1]])
	}
	return out
}

func lineForOffset(source string, offset int) int {
	if offset < 0 {
		return 1
	}
	return strings.Count(source[:offset], "\n") + 1
}

func nextRoutePath(relPath string) string {
	rel := filepath.ToSlash(relPath)
	ext := path.Ext(rel)
	if ext == "" {
		return ""
	}
	trimmed := strings.TrimSuffix(rel, ext)
	for _, prefix := range []string{"src/app/", "app/"} {
		if strings.HasPrefix(trimmed, prefix) {
			route := strings.TrimPrefix(trimmed, prefix)
			if !strings.HasSuffix(route, "/page") && !strings.HasSuffix(route, "/route") {
				return ""
			}
			route = strings.TrimSuffix(strings.TrimSuffix(route, "/page"), "/route")
			return normalizeNextRoute(route)
		}
	}
	for _, prefix := range []string{"src/pages/", "pages/"} {
		if strings.HasPrefix(trimmed, prefix) {
			route := strings.TrimPrefix(trimmed, prefix)
			return normalizeNextRoute(route)
		}
	}
	return ""
}

func normalizeNextRoute(route string) string {
	route = strings.Trim(route, "/")
	if route == "" || route == "index" {
		return "/"
	}
	parts := strings.Split(route, "/")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "index" {
			continue
		}
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "[") && strings.HasSuffix(part, "]") {
			part = ":" + strings.Trim(part, "[]")
		}
		if part != "" {
			out = append(out, part)
		}
	}
	return "/" + strings.Join(out, "/")
}

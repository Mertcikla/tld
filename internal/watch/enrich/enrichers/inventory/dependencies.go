package inventory

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/mertcikla/tld/internal/watch/enrich"
)

var goRequireLineRE = regexp.MustCompile(`^\s*([A-Za-z0-9_./~-]+)\s+v[0-9]`)

type Enricher = enrich.Enricher
type Fact = enrich.Fact
type FactEmitter = enrich.FactEmitter
type FileInput = enrich.FileInput
type Metadata = enrich.Metadata
type SourceSpan = enrich.SourceSpan
type SubjectRef = enrich.SubjectRef

const ActivationAlways = enrich.ActivationAlways

var fileSubject = enrich.FileSubject

func DependencyInventory() Enricher {
	return enrich.NewEnricher(
		Metadata{ID: "dependency.inventory", Name: "Dependency and import inventory", Mode: ActivationAlways},
		func(input FileInput) bool {
			base := path.Base(input.RelPath)
			return dependencyManifest(base) || input.Parsed != nil
		},
		dependencyInventoryRun,
	)
}

func dependencyInventoryRun(ctx context.Context, input FileInput, emit FactEmitter) error {
	base := path.Base(input.RelPath)
	switch base {
	case "go.mod":
		return emitGoModFacts(input, emit)
	case "package.json":
		return emitPackageJSONFacts(input, emit)
	case "requirements.txt":
		return emitLineDependencyFacts(input, emit, "python", requirementName)
	case "pyproject.toml", "poetry.lock":
		return emitLineDependencyFacts(input, emit, "python", tomlDependencyName)
	case "Cargo.toml":
		return emitLineDependencyFacts(input, emit, "cargo", cargoDependencyName)
	case "pom.xml":
		return emitPomFacts(input, emit)
	case "build.gradle", "build.gradle.kts":
		return emitLineDependencyFacts(input, emit, "gradle", gradleDependencyName)
	case "CMakeLists.txt", "conanfile.txt", "conanfile.py", "vcpkg.json":
		return emitLineDependencyFacts(input, emit, "cpp", cppDependencyName)
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
			Type:         "dependency.import",
			StableKey:    fmt.Sprintf("dependency.import:%s:%s:%d", input.RelPath, ref.TargetPath, line),
			Subject:      fileSubject(input.RelPath),
			Object:       SubjectRef{Kind: "dependency.module", StableKey: "dependency.module:" + ref.TargetPath, Name: ref.TargetPath},
			Relationship: "imports",
			Source:       SourceSpan{FilePath: input.RelPath, StartLine: line, StartColumn: ref.Column},
			Confidence:   1,
			Name:         ref.TargetPath,
			Tags:         []string{"dependency:import"},
			Attributes:   map[string]string{"module": ref.TargetPath, "name": ref.Name},
			VisibilityHints: map[string]float64{
				"dependency": 1,
			},
		}); err != nil {
			return err
		}
	}
	return nil
}

func dependencyManifest(base string) bool {
	switch base {
	case "go.mod", "package.json", "requirements.txt", "pyproject.toml", "poetry.lock", "Cargo.toml", "pom.xml", "build.gradle", "build.gradle.kts", "CMakeLists.txt", "conanfile.txt", "conanfile.py", "vcpkg.json":
		return true
	default:
		return false
	}
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

func emitLineDependencyFacts(input FileInput, emit FactEmitter, ecosystem string, parse func(string) string) error {
	scanner := bufio.NewScanner(strings.NewReader(string(input.Source)))
	line := 0
	seen := map[string]struct{}{}
	for scanner.Scan() {
		line++
		name := parse(scanner.Text())
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		if err := emit.EmitFact(dependencyFact(input.RelPath, line, name, ecosystem)); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func emitPomFacts(input FileInput, emit FactEmitter) error {
	source := string(input.Source)
	re := regexp.MustCompile(`(?s)<dependency>.*?<groupId>\s*([^<\s]+)\s*</groupId>.*?<artifactId>\s*([^<\s]+)\s*</artifactId>.*?</dependency>`)
	for _, indexes := range re.FindAllStringSubmatchIndex(source, -1) {
		match := enrich.Submatches(source, indexes)
		if len(match) != 3 {
			continue
		}
		name := match[1] + ":" + match[2]
		line := enrich.LineForOffset(source, indexes[0])
		if err := emit.EmitFact(dependencyFact(input.RelPath, line, name, "maven")); err != nil {
			return err
		}
	}
	return nil
}

func requirementName(line string) string {
	line = strings.TrimSpace(strings.Split(line, "#")[0])
	if line == "" || strings.HasPrefix(line, "-") {
		return ""
	}
	return dependencyPrefix(line, "=", "<", ">", "~", "!", "[", ";")
}

func tomlDependencyName(line string) string {
	line = strings.TrimSpace(strings.Split(line, "#")[0])
	if line == "" || strings.HasPrefix(line, "[") {
		return ""
	}
	if strings.HasPrefix(line, "\"") || strings.HasPrefix(line, "'") {
		trimmed := strings.Trim(line, " ,")
		trimmed = strings.Trim(trimmed, `"'`)
		if trimmed != "" && !strings.Contains(trimmed, "=") {
			return requirementName(trimmed)
		}
	}
	if idx := strings.Index(line, "="); idx > 0 {
		name := strings.TrimSpace(line[:idx])
		return strings.Trim(name, `"'`)
	}
	return ""
}

func cargoDependencyName(line string) string {
	name := tomlDependencyName(line)
	switch name {
	case "package", "dependencies", "dev-dependencies", "build-dependencies", "workspace":
		return ""
	default:
		return name
	}
}

func gradleDependencyName(line string) string {
	line = strings.TrimSpace(line)
	for _, quote := range []string{"\"", "'"} {
		start := strings.Index(line, quote)
		if start < 0 {
			continue
		}
		rest := line[start+1:]
		end := strings.Index(rest, quote)
		if end < 0 {
			continue
		}
		value := rest[:end]
		if strings.Count(value, ":") >= 1 {
			return value
		}
	}
	return ""
}

func cppDependencyName(line string) string {
	line = strings.TrimSpace(strings.Split(line, "#")[0])
	if line == "" {
		return ""
	}
	for _, prefix := range []string{"find_package(", "target_link_libraries(", "requires =", "self.requires(", "\"name\":"} {
		if idx := strings.Index(line, prefix); idx >= 0 {
			value := strings.TrimSpace(line[idx+len(prefix):])
			value = strings.Trim(value, ` "'),[]`)
			return dependencyPrefix(value, " ", "/", ")", ",", "\"")
		}
	}
	return ""
}

func dependencyPrefix(value string, stops ...string) string {
	value = strings.TrimSpace(value)
	end := len(value)
	for _, stop := range stops {
		if idx := strings.Index(value, stop); idx >= 0 && idx < end {
			end = idx
		}
	}
	return strings.TrimSpace(value[:end])
}

func dependencyFact(relPath string, line int, name, ecosystem string) Fact {
	return Fact{
		Type:         "dependency.module",
		StableKey:    fmt.Sprintf("dependency.module:%s:%s", relPath, name),
		Subject:      fileSubject(relPath),
		Object:       SubjectRef{Kind: "dependency.module", StableKey: "dependency.module:" + name, Name: name},
		Relationship: "declares_dependency",
		Source:       SourceSpan{FilePath: relPath, StartLine: line, EndLine: line},
		Confidence:   1,
		Name:         name,
		Tags:         []string{"dependency:module"},
		Attributes:   map[string]string{"module": name, "ecosystem": ecosystem},
		VisibilityHints: map[string]float64{
			"dependency": 1,
		},
	}
}

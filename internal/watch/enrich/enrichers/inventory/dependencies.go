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
			return base == "go.mod" || base == "package.json" || input.Parsed != nil
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

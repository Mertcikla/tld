package grpc

import (
	"context"
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/mertcikla/tld/internal/watch/enrich"
)

type ActivationSignal = enrich.ActivationSignal
type Enricher = enrich.Enricher
type Fact = enrich.Fact
type FactEmitter = enrich.FactEmitter
type FileInput = enrich.FileInput
type Metadata = enrich.Metadata
type SourceSpan = enrich.SourceSpan
type SubjectRef = enrich.SubjectRef

const (
	ActivationAlways             = enrich.ActivationAlways
	ActivationImportOrDependency = enrich.ActivationImportOrDependency
	SignalDependency             = enrich.SignalDependency
	SignalImport                 = enrich.SignalImport
)

var (
	fileSubject    = enrich.FileSubject
	lineForOffset  = enrich.LineForOffset
	matchLanguages = enrich.MatchLanguages
	subjectForLine = enrich.SubjectForLine
	submatches     = enrich.Submatches
)

func ProtobufContracts() Enricher {
	return enrich.NewEnricher(
		Metadata{ID: "protobuf.contracts", Name: "Protocol Buffer service contracts", Mode: ActivationAlways},
		matchLanguages("protobuf"),
		func(ctx context.Context, input FileInput, emit FactEmitter) error {
			if generatedLike(input.RelPath, input.Source) {
				return nil
			}
			return emitServiceMatches(input, emit, "grpc.contract", "protobuf", regexp.MustCompile(`(?m)^\s*service\s+([A-Za-z_][A-Za-z0-9_]*)\s*\{`), []string{"protocol:grpc", "arch:contract"})
		},
	)
}

func GoGRPC() Enricher {
	return enrich.NewEnricher(
		Metadata{
			ID: "go.grpc", Name: "Go gRPC glue", Mode: ActivationImportOrDependency,
			Triggers: []ActivationSignal{{Kind: SignalImport, Value: "google.golang.org/grpc"}, {Kind: SignalDependency, Value: "google.golang.org/grpc"}},
		},
		matchLanguages("go"),
		func(ctx context.Context, input FileInput, emit FactEmitter) error {
			if err := emitServiceMatches(input, emit, "grpc.server", "go", regexp.MustCompile(`\b(?:[A-Za-z_][A-Za-z0-9_]*\.)?Register([A-Za-z_][A-Za-z0-9_]*)Server\s*\(`), []string{"protocol:grpc", "grpc:server", "framework:go-grpc"}); err != nil {
				return err
			}
			if err := emitServiceMatches(input, emit, "grpc.client", "go", regexp.MustCompile(`\b(?:[A-Za-z_][A-Za-z0-9_]*\.)?New([A-Za-z_][A-Za-z0-9_]*)Client\s*\(`), []string{"protocol:grpc", "grpc:client", "framework:go-grpc"}); err != nil {
				return err
			}
			return emitEndpointReads(input, emit, "go")
		},
	)
}

func PythonGRPC() Enricher {
	return enrich.NewEnricher(
		Metadata{
			ID: "python.grpc", Name: "Python grpcio glue", Mode: ActivationImportOrDependency,
			Triggers: []ActivationSignal{{Kind: SignalImport, Value: "grpc"}, {Kind: SignalDependency, Value: "grpcio"}},
		},
		matchLanguages("python"),
		func(ctx context.Context, input FileInput, emit FactEmitter) error {
			if generatedLike(input.RelPath, input.Source) {
				return nil
			}
			if err := emitServiceMatches(input, emit, "grpc.server", "python", regexp.MustCompile(`\badd_([A-Za-z_][A-Za-z0-9_]*)Servicer_to_server\s*\(`), []string{"protocol:grpc", "grpc:server", "framework:python-grpc"}); err != nil {
				return err
			}
			if err := emitServiceMatches(input, emit, "grpc.client", "python", regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*)Stub\s*\(`), []string{"protocol:grpc", "grpc:client", "framework:python-grpc"}); err != nil {
				return err
			}
			return emitEndpointReads(input, emit, "python")
		},
	)
}

func NodeGRPC() Enricher {
	return enrich.NewEnricher(
		Metadata{
			ID: "node.grpc", Name: "Node gRPC glue", Mode: ActivationImportOrDependency,
			Triggers: []ActivationSignal{{Kind: SignalImport, Value: "@grpc/grpc-js"}, {Kind: SignalDependency, Value: "@grpc/grpc-js"}, {Kind: SignalDependency, Value: "grpc"}},
		},
		matchLanguages("javascript", "typescript"),
		func(ctx context.Context, input FileInput, emit FactEmitter) error {
			if err := emitServiceMatches(input, emit, "grpc.server", "node", regexp.MustCompile(`\.addService\(\s*[^,\n]*\.([A-Za-z_][A-Za-z0-9_]*)\.service`), []string{"protocol:grpc", "grpc:server", "framework:node-grpc"}); err != nil {
				return err
			}
			return emitEndpointReads(input, emit, "node")
		},
	)
}

func JavaGRPC() Enricher {
	return enrich.NewEnricher(
		Metadata{
			ID: "java.grpc", Name: "Java gRPC glue", Mode: ActivationImportOrDependency,
			Triggers: []ActivationSignal{{Kind: SignalImport, Value: "io.grpc"}, {Kind: SignalDependency, Value: "io.grpc"}},
		},
		func(input FileInput) bool { return matchLanguages("java", "gradle")(input) },
		func(ctx context.Context, input FileInput, emit FactEmitter) error {
			if input.Language == "gradle" {
				return emitBuildDependencyFact(input, emit, "io.grpc", "grpc", "java-grpc")
			}
			if err := emitServiceMatches(input, emit, "grpc.server", "java", regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*)Grpc\.([A-Za-z_][A-Za-z0-9_]*)ImplBase\b`), []string{"protocol:grpc", "grpc:server", "framework:java-grpc"}); err != nil {
				return err
			}
			return emitServiceMatches(input, emit, "grpc.server", "java", regexp.MustCompile(`\bServerBuilder\.forPort\s*\(`), []string{"protocol:grpc", "grpc:server", "framework:java-grpc"})
		},
	)
}

func DotNetGRPC() Enricher {
	return enrich.NewEnricher(
		Metadata{
			ID: "dotnet.grpc", Name: ".NET gRPC glue", Mode: ActivationImportOrDependency,
			Triggers: []ActivationSignal{{Kind: SignalDependency, Value: "Grpc.AspNetCore"}, {Kind: SignalImport, Value: "Grpc.Core"}},
		},
		matchLanguages("c-sharp", "xml"),
		func(ctx context.Context, input FileInput, emit FactEmitter) error {
			if err := emitServiceMatches(input, emit, "grpc.server", "dotnet", regexp.MustCompile(`\bMapGrpcService<([A-Za-z_][A-Za-z0-9_.]*)>\s*\(`), []string{"protocol:grpc", "grpc:server", "framework:dotnet-grpc"}); err != nil {
				return err
			}
			if err := emitServiceMatches(input, emit, "grpc.contract", "dotnet", regexp.MustCompile(`<Protobuf\s+Include=["']([^"']+)["'][^>]*GrpcServices=["']([^"']+)["']`), []string{"protocol:grpc", "arch:contract", "framework:dotnet-grpc"}); err != nil {
				return err
			}
			return emitEndpointReads(input, emit, "dotnet")
		},
	)
}

func emitServiceMatches(input FileInput, emit FactEmitter, factType, framework string, re *regexp.Regexp, tags []string) error {
	source := string(input.Source)
	for _, indexes := range re.FindAllStringSubmatchIndex(source, -1) {
		match := submatches(source, indexes)
		line := lineForOffset(source, indexes[0])
		name := ""
		if len(match) > 1 {
			name = normalizeServiceName(match[1])
		}
		if name == "" {
			name = inferredServiceNameFromPath(input.RelPath)
		}
		if name == "" {
			continue
		}
		relationship := "declares"
		if strings.HasSuffix(factType, ".client") {
			relationship = "calls"
		}
		if err := emit.EmitFact(Fact{
			Type:            factType,
			StableKey:       fmt.Sprintf("%s:%s:%s:%s:%d", factType, framework, input.RelPath, name, line),
			Subject:         subjectForLine(input, line),
			Object:          SubjectRef{Kind: factType, StableKey: factType + ":" + framework + ":" + name, FilePath: input.RelPath, Name: name},
			Relationship:    relationship,
			Source:          SourceSpan{FilePath: input.RelPath, StartLine: line, EndLine: line},
			Confidence:      0.86,
			Name:            name,
			Tags:            tags,
			Attributes:      map[string]string{"framework": framework, "service": name},
			VisibilityHints: map[string]float64{"high_signal": 1},
		}); err != nil {
			return err
		}
	}
	return nil
}

func emitEndpointReads(input FileInput, emit FactEmitter, framework string) error {
	source := string(input.Source)
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`\b(?:os\.Getenv|os\.LookupEnv|os\.environ\.get|process\.env(?:\[[^\]]+\]|\.[A-Za-z_][A-Za-z0-9_]*)|Configuration\[[^\]]+\])\s*\(?\s*["']?([A-Z0-9_]*(?:ADDR|HOST|URL|PORT|REDIS|SPANNER|ALLOYDB|COLLECTOR)[A-Z0-9_]*)["']?`),
		regexp.MustCompile(`\bmustMapEnv\([^,\n]+,\s*"([A-Z0-9_]+)"\s*\)`),
	}
	for _, re := range patterns {
		for _, indexes := range re.FindAllStringSubmatchIndex(source, -1) {
			match := submatches(source, indexes)
			if len(match) < 2 {
				continue
			}
			env := strings.Trim(match[1], `"'[]`)
			target := ""
			line := lineForOffset(source, indexes[0])
			if err := emit.EmitFact(Fact{
				Type:            "runtime.endpoint_ref",
				StableKey:       fmt.Sprintf("runtime.endpoint_ref:%s:%s:%d", input.RelPath, env, line),
				Subject:         subjectForLine(input, line),
				Object:          SubjectRef{Kind: "runtime.endpoint", StableKey: "runtime.endpoint:" + target, Name: target},
				Relationship:    "uses",
				Source:          SourceSpan{FilePath: input.RelPath, StartLine: line, EndLine: line},
				Confidence:      0.62,
				Name:            env,
				Tags:            []string{"arch:endpoint-ref", "framework:" + framework},
				Attributes:      map[string]string{"env": env, "target": target, "framework": framework},
				VisibilityHints: map[string]float64{"high_signal": 0.5},
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func emitBuildDependencyFact(input FileInput, emit FactEmitter, needle, name, framework string) error {
	if !strings.Contains(string(input.Source), needle) {
		return nil
	}
	return emit.EmitFact(Fact{
		Type:            "dependency.module",
		StableKey:       fmt.Sprintf("dependency.module:%s:%s", input.RelPath, name),
		Subject:         fileSubject(input.RelPath),
		Object:          SubjectRef{Kind: "dependency.module", StableKey: "dependency.module:" + name, Name: name},
		Relationship:    "declares_dependency",
		Source:          SourceSpan{FilePath: input.RelPath, StartLine: 1, EndLine: 1},
		Confidence:      1,
		Name:            name,
		Tags:            []string{"dependency:module", "framework:" + framework},
		Attributes:      map[string]string{"module": name, "ecosystem": framework},
		VisibilityHints: map[string]float64{"dependency": 1},
	})
}

func normalizeServiceName(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimSuffix(value, "Servicer")
	value = strings.TrimSuffix(value, "Service")
	if value == "Health" || value == "grpc" || value == "" {
		return ""
	}
	return lowerCamelToService(value)
}

func lowerCamelToService(value string) string {
	var b strings.Builder
	for i, r := range value {
		if r >= 'A' && r <= 'Z' {
			if i > 0 {
				prev := rune(value[i-1])
				if prev >= 'a' && prev <= 'z' {
					b.WriteByte('-')
				}
			}
			r += 'a' - 'A'
		}
		if r == '_' || r == '.' {
			b.WriteByte('-')
			continue
		}
		b.WriteRune(r)
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return ""
	}
	if !strings.HasSuffix(out, "service") && strings.Contains(strings.ToLower(value), "Service") {
		out += "service"
	}
	return out
}

func inferredServiceNameFromPath(rel string) string {
	parts := strings.Split(path.Clean(filepathSlash(rel)), "/")
	for i, part := range parts {
		if part == "src" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	if len(parts) > 1 {
		return parts[len(parts)-2]
	}
	return ""
}

func filepathSlash(value string) string {
	return strings.ReplaceAll(value, "\\", "/")
}

func generatedLike(rel string, data []byte) bool {
	lowerPath := strings.ToLower(rel)
	head := strings.ToLower(string(data[:min(len(data), 4096)]))
	return strings.Contains(lowerPath, "genproto/") || strings.Contains(lowerPath, "_pb2") || strings.Contains(head, "code generated") || strings.Contains(head, "generated by")
}

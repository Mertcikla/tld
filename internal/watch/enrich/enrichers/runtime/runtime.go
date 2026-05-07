package runtimeenrich

import (
	"context"
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/mertcikla/tld/internal/watch/enrich"
)

type Enricher = enrich.Enricher
type Fact = enrich.Fact
type FactEmitter = enrich.FactEmitter
type FileInput = enrich.FileInput
type Metadata = enrich.Metadata
type SourceSpan = enrich.SourceSpan
type SubjectRef = enrich.SubjectRef

const ActivationAlways = enrich.ActivationAlways

var (
	fileSubject    = enrich.FileSubject
	lineForOffset  = enrich.LineForOffset
	matchLanguages = enrich.MatchLanguages
	submatches     = enrich.Submatches
)

func RuntimeManifests() Enricher {
	return enrich.NewEnricher(
		Metadata{ID: "runtime.manifests", Name: "Runtime manifests", Mode: ActivationAlways},
		matchLanguages("yaml", "terraform", "json"),
		func(ctx context.Context, input FileInput, emit FactEmitter) error {
			switch input.Language {
			case "yaml":
				return emitRuntimeYAMLFacts(input, emit)
			case "terraform":
				return emitTerraformFacts(input, emit)
			case "json":
				if path.Base(input.RelPath) == "package.json" || path.Base(input.RelPath) == "package-lock.json" {
					return nil
				}
				return emitOpenAPIFact(input, emit)
			default:
				return nil
			}
		},
	)
}

func emitRuntimeYAMLFacts(input FileInput, emit FactEmitter) error {
	source := string(input.Source)
	if !strings.Contains(strings.ToLower(source), "kind:") && !strings.Contains(strings.ToLower(source), "services:") {
		return nil
	}
	componentRE := regexp.MustCompile(`(?m)^\s*(?:kind:\s*(Deployment|StatefulSet|DaemonSet|Job|CronJob|Pod|Service)|name:\s*([A-Za-z0-9_.-]+)|image:\s*([^#\n]+)|value:\s*["']?([^"'\n]+)["']?)`)
	lines := strings.Split(source, "\n")
	var lastKind, lastName, lastEnvVar string
	for i, line := range lines {
		match := componentRE.FindStringSubmatch(line)
		if len(match) == 0 {
			continue
		}
		if match[1] != "" {
			if runtimeWorkloadKind(match[1]) {
				lastKind = match[1]
				lastName = ""
			}
			continue
		}
		if match[2] != "" {
			if lastKind != "" && lastName == "" {
				lastName = match[2]
				if err := emitComponentFact(input, emit, lastName, "service", "Kubernetes", i+1, []string{"runtime:kubernetes", "arch:deployable"}, map[string]string{"runtime": "kubernetes", "kind": lastKind}); err != nil {
					return err
				}
			} else {
				lastEnvVar = match[2]
			}
			continue
		}
		if match[4] != "" && lastName != "" {
			target := endpointName(match[4], lastEnvVar)
			if target != "" && target != lastName {
				if err := emitConnectorFact(input, emit, lastName, target, protocolFromValue(match[4]), "runtime-dependency", i+1, "runtime manifest env endpoint", 0.78); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func emitTerraformFacts(input FileInput, emit FactEmitter) error {
	re := regexp.MustCompile(`(?m)^\s*resource\s+"([^"]+)"\s+"([^"]+)"`)
	for _, indexes := range re.FindAllStringSubmatchIndex(string(input.Source), -1) {
		match := submatches(string(input.Source), indexes)
		if len(match) < 3 {
			continue
		}
		line := lineForOffset(string(input.Source), indexes[0])
		kind, tech := infrastructureKind(match[1])
		if kind == "" {
			continue
		}
		if err := emitComponentFact(input, emit, match[2], kind, tech, line, []string{"arch:infrastructure"}, map[string]string{"resource_type": match[1]}); err != nil {
			return err
		}
	}
	return nil
}

func emitOpenAPIFact(input FileInput, emit FactEmitter) error {
	if !strings.Contains(strings.ToLower(string(input.Source)), `"openapi"`) {
		return nil
	}
	name := strings.TrimSuffix(path.Base(input.RelPath), path.Ext(input.RelPath))
	return emitComponentFact(input, emit, name, "interface", "OpenAPI", 1, []string{"arch:contract", "protocol:http"}, map[string]string{"protocol": "http"})
}

func emitComponentFact(input FileInput, emit FactEmitter, name, kind, technology string, line int, tags []string, attrs map[string]string) error {
	if attrs == nil {
		attrs = map[string]string{}
	}
	attrs["name"] = name
	attrs["kind"] = kind
	attrs["technology"] = technology
	return emit.EmitFact(Fact{
		Type:            "runtime.component",
		StableKey:       fmt.Sprintf("runtime.component:%s:%s:%d", input.RelPath, name, line),
		Subject:         fileSubject(input.RelPath),
		Object:          SubjectRef{Kind: "runtime.component", StableKey: "runtime.component:" + name, FilePath: input.RelPath, Name: name},
		Relationship:    "declares",
		Source:          SourceSpan{FilePath: input.RelPath, StartLine: line, EndLine: line},
		Confidence:      0.8,
		Name:            name,
		Tags:            append(tags, "arch:component"),
		Attributes:      attrs,
		VisibilityHints: map[string]float64{"high_signal": 0.8},
	})
}

func emitConnectorFact(input FileInput, emit FactEmitter, source, target, label, relationship string, line int, note string, confidence float64) error {
	if label == "" {
		label = "uses"
	}
	return emit.EmitFact(Fact{
		Type:            "runtime.connection",
		StableKey:       fmt.Sprintf("runtime.connection:%s:%s:%s:%s:%d", input.RelPath, source, target, relationship, line),
		Subject:         fileSubject(input.RelPath),
		Object:          SubjectRef{Kind: "runtime.component", StableKey: "runtime.component:" + target, Name: target},
		Relationship:    relationship,
		Source:          SourceSpan{FilePath: input.RelPath, StartLine: line, EndLine: line},
		Confidence:      confidence,
		Name:            source + " -> " + target,
		Tags:            []string{"arch:connection"},
		Attributes:      map[string]string{"source": source, "target": target, "label": label, "note": note},
		VisibilityHints: map[string]float64{"high_signal": 1},
	})
}

func endpointName(value, envName string) string {
	value = strings.Trim(strings.TrimSpace(value), `"'`)
	if value == "" || strings.Contains(value, "{{") {
		return ""
	}
	lower := strings.ToLower(value)
	if lower == "true" || lower == "false" || lower == "null" {
		return ""
	}
	if regexp.MustCompile(`^\d+$`).MatchString(value) {
		return ""
	}

	// Tighten: Require a protocol scheme OR a high-signal environment variable name.
	hasScheme := strings.Contains(value, "://")
	lowerEnv := strings.ToLower(envName)
	highSignalName := strings.HasSuffix(lowerEnv, "_url") || strings.HasSuffix(lowerEnv, "_host") ||
		strings.HasSuffix(lowerEnv, "_uri") || strings.HasSuffix(lowerEnv, "_endpoint") ||
		strings.HasSuffix(lowerEnv, "_address") || strings.HasSuffix(lowerEnv, "_addr")

	if !hasScheme && !highSignalName {
		return ""
	}

	if hasScheme {
		parts := strings.SplitN(value, "://", 2)
		value = parts[1]
	}
	if strings.Contains(value, ":") {
		value = strings.Split(value, ":")[0]
	}
	value = strings.Trim(value, "/")
	if strings.Contains(value, ".") {
		value = strings.Split(value, ".")[0]
	}
	value = strings.ToLower(value)
	if strings.HasPrefix(value, "$") || strings.ContainsAny(value, " /\\_=") {
		return ""
	}
	if strings.HasSuffix(value, "-addr") || strings.HasSuffix(value, "-url") || strings.HasSuffix(value, "-host") || strings.HasSuffix(value, "-port") {
		return ""
	}
	if !regexp.MustCompile(`[a-z]`).MatchString(value) {
		return ""
	}
	return strings.Trim(value, "-_")
}

func runtimeWorkloadKind(kind string) bool {
	switch strings.ToLower(kind) {
	case "deployment", "statefulset", "daemonset", "job", "cronjob", "pod", "service":
		return true
	default:
		return false
	}
}

func protocolFromValue(value string) string {
	lower := strings.ToLower(value)
	switch {
	case strings.Contains(lower, "redis"):
		return "redis"
	case strings.HasPrefix(lower, "http://"), strings.HasPrefix(lower, "https://"):
		return "http"
	case strings.Contains(lower, ":"):
		return "grpc"
	default:
		return "uses"
	}
}

func infrastructureKind(resourceType string) (string, string) {
	lower := strings.ToLower(resourceType)
	switch {
	case strings.Contains(lower, "redis"), strings.Contains(lower, "memcache"), strings.Contains(lower, "cache"):
		return "datastore", "Cache"
	case strings.Contains(lower, "sql"), strings.Contains(lower, "database"), strings.Contains(lower, "spanner"), strings.Contains(lower, "alloydb"), strings.Contains(lower, "postgres"), strings.Contains(lower, "mysql"):
		return "datastore", "Database"
	case strings.Contains(lower, "queue"), strings.Contains(lower, "pubsub"), strings.Contains(lower, "topic"), strings.Contains(lower, "subscription"):
		return "queue", "Messaging"
	case strings.Contains(lower, "bucket"), strings.Contains(lower, "storage"):
		return "datastore", "Object Storage"
	default:
		return "", ""
	}
}

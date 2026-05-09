package compose

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/mertcikla/tld/internal/watch/enrich"
)

func All() []enrich.Enricher {
	return []enrich.Enricher{Compose()}
}

func Compose() enrich.Enricher {
	return enrich.NewEnricher(
		enrich.Metadata{
			ID:   "compose.docker_compose",
			Name: "Docker Compose",
			Mode: enrich.ActivationImportOrDependency,
			Triggers: []enrich.ActivationSignal{
				{Kind: enrich.SignalDependency, Value: "docker-compose"},
			},
		},
		func(input enrich.FileInput) bool {
			if input.Language != "yaml" {
				return false
			}
			return composePathTokens(input.RelPath)
		},
		func(ctx context.Context, input enrich.FileInput, emit enrich.FactEmitter) error {
			return emitComposeFacts(input, emit)
		},
	)
}

func composePathTokens(rel string) bool {
	lower := strings.ToLower(rel)
	return strings.Contains(lower, "docker-compose") || strings.Contains(lower, "compose.yaml") || strings.Contains(lower, "compose.yml")
}

func emitComposeFacts(input enrich.FileInput, emit enrich.FactEmitter) error {
	project, err := parseCompose(input)
	if err != nil || len(project.Services) == 0 {
		return nil
	}
	serviceNames := project.ServiceNames()
	serviceSet := make(map[string]struct{}, len(serviceNames))
	for _, name := range serviceNames {
		serviceSet[name] = struct{}{}
	}
	lineOffsets := serviceLineOffsets(string(input.Source), serviceNames)

	for name, svc := range project.Services {
		line := lineOffsets[name]
		tech, kind := imageToTech(svc.Image)
		if labelKind := svc.Labels["tld.kind"]; labelKind != "" {
			kind = labelKind
		}
		buildCtx := ""
		if svc.Build != nil {
			buildCtx = svc.Build.Context
		}

		if err := emitServiceFact(input, emit, name, tech, kind, buildCtx, svc.Image, svc.Labels, line); err != nil {
			return err
		}
		for _, dep := range dedupKeys(svc.DependsOn) {
			if err := emitDependsOnFact(input, emit, name, dep, line); err != nil {
				return err
			}
		}
		for _, ref := range envEndpointRefs(svc.Environment, serviceNames) {
			if ref != name {
				if err := emitEnvConnectionFact(input, emit, name, ref, line); err != nil {
					return err
				}
			}
		}
		for _, port := range svc.Ports {
			if err := emitPortFact(input, emit, name, port, line); err != nil {
				return err
			}
		}
		for _, vol := range svc.Volumes {
			if err := emitVolumeFact(input, emit, name, vol, line); err != nil {
				return err
			}
		}
	}
	return nil
}

func parseCompose(input enrich.FileInput) (*types.Project, error) {
	config := types.ConfigDetails{
		WorkingDir: input.RepoRoot,
		ConfigFiles: []types.ConfigFile{{
			Filename: input.AbsPath,
			Content:  input.Source,
		}},
	}
	opts := func(o *loader.Options) {
		o.SkipValidation = true
		o.SkipInterpolation = true
		o.SkipNormalization = true
		o.SkipConsistencyCheck = true
		o.SkipResolveEnvironment = true
		o.SkipExtends = true
		o.SkipInclude = true
	}
	project, err := loader.LoadWithContext(context.Background(), config, opts)
	if err != nil {
		return nil, err
	}
	return project, nil
}

func emitServiceFact(input enrich.FileInput, emit enrich.FactEmitter, name, technology, kind, buildCtx, image string, labels types.Labels, line int) error {
	attrs := map[string]string{
		"name":       name,
		"kind":       kind,
		"technology": technology,
	}
	if image != "" {
		attrs["image"] = image
	}
	if buildCtx != "" {
		attrs["build_context"] = buildCtx
	}
	for k, v := range labels {
		if strings.HasPrefix(k, "tld.") {
			attrs[strings.TrimPrefix(k, "tld.")] = v
		}
	}
	tags := []string{
		"arch:component",
		"runtime:compose",
		"arch:deployable",
		"technology:" + tagValue(technology),
		"kind:" + kind,
	}
	return emit.EmitFact(enrich.Fact{
		Type:         "runtime.component",
		StableKey:    fmt.Sprintf("runtime.component:%s:%s:%d", input.RelPath, name, line),
		Subject:      enrich.FileSubject(input.RelPath),
		Object:       enrich.SubjectRef{Kind: "runtime.component", StableKey: "runtime.component:" + name, FilePath: input.RelPath, Name: name},
		Relationship: "declares",
		Source:       enrich.SourceSpan{FilePath: input.RelPath, StartLine: line, EndLine: line},
		Confidence:   0.85,
		Name:         name,
		Tags:         tags,
		Attributes:   attrs,
		VisibilityHints: map[string]float64{
			"high_signal": 0.85,
		},
	})
}

func emitDependsOnFact(input enrich.FileInput, emit enrich.FactEmitter, source, target string, line int) error {
	return emit.EmitFact(enrich.Fact{
		Type:         "runtime.connection",
		StableKey:    fmt.Sprintf("runtime.connection:%s:%s:depends_on:%s:%d", input.RelPath, source, target, line),
		Subject:      enrich.FileSubject(input.RelPath),
		Object:       enrich.SubjectRef{Kind: "runtime.component", StableKey: "runtime.component:" + target, Name: target},
		Relationship: "depends_on",
		Source:       enrich.SourceSpan{FilePath: input.RelPath, StartLine: line, EndLine: line},
		Confidence:   0.75,
		Name:         source + " -> " + target,
		Tags:         []string{"arch:connection"},
		Attributes:   map[string]string{"source": source, "target": target, "label": "depends on"},
		VisibilityHints: map[string]float64{
			"high_signal": 0.7,
		},
	})
}

func emitEnvConnectionFact(input enrich.FileInput, emit enrich.FactEmitter, source, target string, line int) error {
	return emit.EmitFact(enrich.Fact{
		Type:         "runtime.connection",
		StableKey:    fmt.Sprintf("runtime.connection:%s:%s:connects_to:%s:%d", input.RelPath, source, target, line),
		Subject:      enrich.FileSubject(input.RelPath),
		Object:       enrich.SubjectRef{Kind: "runtime.component", StableKey: "runtime.component:" + target, Name: target},
		Relationship: "connects_to",
		Source:       enrich.SourceSpan{FilePath: input.RelPath, StartLine: line, EndLine: line},
		Confidence:   0.55,
		Name:         source + " -> " + target,
		Tags:         []string{"arch:connection", "compose:implicit"},
		Attributes:   map[string]string{"source": source, "target": target, "label": "connects via env", "note": "inferred from environment variable"},
		VisibilityHints: map[string]float64{
			"high_signal": 0.4,
		},
	})
}

func emitPortFact(input enrich.FileInput, emit enrich.FactEmitter, serviceName string, port types.ServicePortConfig, line int) error {
	protocol := port.Protocol
	if protocol == "" {
		protocol = "tcp"
	}
	label := fmt.Sprintf("%d/%s", port.Target, protocol)
	published := port.Published
	if port.Published != "" {
		label = port.Published + ":" + label
	}
	attrs := map[string]string{"service": serviceName, "port": fmt.Sprint(port.Target), "protocol": protocol}
	if published != "" {
		attrs["published"] = published
	}
	return emit.EmitFact(enrich.Fact{
		Type:         "runtime.endpoint",
		StableKey:    fmt.Sprintf("runtime.endpoint:%s:%s:%d:%s:%d", input.RelPath, serviceName, port.Target, protocol, line),
		Subject:      enrich.FileSubject(input.RelPath),
		Object:       enrich.SubjectRef{Kind: "runtime.endpoint", StableKey: fmt.Sprintf("runtime.endpoint:%s:%d", serviceName, port.Target), Name: label},
		Relationship: "exposes",
		Source:       enrich.SourceSpan{FilePath: input.RelPath, StartLine: line, EndLine: line},
		Confidence:   0.80,
		Name:         serviceName + ":" + label,
		Tags:         []string{"arch:endpoint"},
		Attributes:   attrs,
		VisibilityHints: map[string]float64{
			"high_signal": 0.7,
		},
	})
}

func emitVolumeFact(input enrich.FileInput, emit enrich.FactEmitter, serviceName string, vol types.ServiceVolumeConfig, line int) error {
	source := vol.Source
	if source == "" {
		source = vol.Target
	}
	displaySource := composeDisplayVolumeSource(input.RepoRoot, source)
	return emit.EmitFact(enrich.Fact{
		Type:         "storage.volume",
		StableKey:    fmt.Sprintf("storage.volume:%s:%s:%s:%d", input.RelPath, serviceName, source, line),
		Subject:      enrich.FileSubject(input.RelPath),
		Object:       enrich.SubjectRef{Kind: "storage.volume", StableKey: "storage.volume:" + source, Name: displaySource},
		Relationship: "uses",
		Source:       enrich.SourceSpan{FilePath: input.RelPath, StartLine: line, EndLine: line},
		Confidence:   0.70,
		Name:         serviceName + " -> " + displaySource,
		Tags:         []string{"storage:volume"},
		Attributes:   map[string]string{"service": serviceName, "source": displaySource, "target": vol.Target},
		VisibilityHints: map[string]float64{
			"high_signal": 0.5,
		},
	})
}

func composeDisplayVolumeSource(repoRoot, source string) string {
	source = strings.TrimSpace(source)
	if source == "" || !filepath.IsAbs(source) || strings.TrimSpace(repoRoot) == "" {
		return filepath.ToSlash(source)
	}
	rel, err := filepath.Rel(repoRoot, source)
	if err == nil && rel == "." {
		return filepath.ToSlash(filepath.Base(repoRoot)) + "/"
	}
	if err != nil || rel == "." {
		return filepath.ToSlash(source)
	}
	return filepath.ToSlash(rel)
}

func envEndpointRefs(env types.MappingWithEquals, serviceNames []string) []string {
	var refs []string
	for _, value := range env {
		if value == nil {
			continue
		}
		target := extractServiceRef(*value, serviceNames)
		if target == "" {
			continue
		}
		refs = append(refs, target)
	}
	return refs
}

func extractServiceRef(value string, serviceNames []string) string {
	value = strings.TrimSpace(value)
	lower := strings.ToLower(value)

	// Filter out non-signal values
	if value == "" || lower == "true" || lower == "false" || lower == "null" || lower == "localhost" || lower == "127.0.0.1" || lower == "0.0.0.0" {
		return ""
	}

	// Extract hostname from URL or host:port
	host := value
	if strings.Contains(host, "://") {
		parts := strings.SplitN(host, "://", 2)
		host = parts[1]
	}
	if strings.Contains(host, ":") {
		host = strings.SplitN(host, ":", 2)[0]
	}
	host = strings.Trim(host, "/")
	if strings.Contains(host, ".") {
		host = strings.SplitN(host, ".", 2)[0]
	}
	host = strings.ToLower(host)
	if host == "" || strings.ContainsAny(host, " /\\") {
		return ""
	}

	for _, name := range serviceNames {
		if strings.EqualFold(name, host) {
			return name
		}
	}
	return ""
}

func serviceLineOffsets(source string, serviceNames []string) map[string]int {
	lines := strings.Split(source, "\n")
	offsets := make(map[string]int, len(serviceNames))
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		for _, name := range serviceNames {
			if _, ok := offsets[name]; ok {
				continue
			}
			if strings.HasPrefix(trimmed, name+":") {
				offsets[name] = i + 1
			}
		}
	}
	for _, name := range serviceNames {
		if _, ok := offsets[name]; !ok {
			offsets[name] = 1
		}
	}
	return offsets
}

func dedupKeys(m map[string]types.ServiceDependency) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

func tagValue(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.NewReplacer(" / ", "-", "/", "-", " ", "-", "&", "and", ".", "", "+", "plus").Replace(value)
	for strings.Contains(value, "--") {
		value = strings.ReplaceAll(value, "--", "-")
	}
	return strings.Trim(value, "-")
}

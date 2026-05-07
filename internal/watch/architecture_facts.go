package watch

import (
	"encoding/json"
	"path"
	"regexp"
	"strings"
)

func mergeArchitectureModels(models ...architectureModel) architectureModel {
	merged := architectureModel{Components: map[string]*architectureComponent{}, Connectors: map[string]*architectureConnector{}}
	for _, model := range models {
		for key, component := range model.Components {
			if component == nil {
				continue
			}
			existing := merged.Components[key]
			if existing == nil {
				copyComponent := *component
				copyComponent.Tags = append([]string{}, component.Tags...)
				copyComponent.Evidence = append([]architectureEvidence{}, component.Evidence...)
				merged.Components[key] = &copyComponent
				continue
			}
			existing.Technology = firstNonEmpty(existing.Technology, component.Technology)
			existing.Description = firstNonEmpty(existing.Description, component.Description)
			existing.FilePath = firstNonEmpty(existing.FilePath, component.FilePath)
			existing.Tags = appendUnique(existing.Tags, component.Tags...)
			existing.Evidence = append(existing.Evidence, component.Evidence...)
		}
		for key, connector := range model.Connectors {
			if connector == nil {
				continue
			}
			existing := merged.Connectors[key]
			if existing == nil {
				copyConnector := *connector
				copyConnector.Evidence = append([]architectureEvidence{}, connector.Evidence...)
				merged.Connectors[key] = &copyConnector
				continue
			}
			if connector.Confidence > existing.Confidence {
				existing.Confidence = connector.Confidence
			}
			existing.Evidence = append(existing.Evidence, connector.Evidence...)
		}
	}
	return merged
}

func architectureFromFacts(facts []Fact) architectureModel {
	model := architectureModel{Components: map[string]*architectureComponent{}, Connectors: map[string]*architectureConnector{}}
	sourceByFile := map[string]string{}
	for _, fact := range facts {
		if fact.Type == enrichmentVersionType {
			continue
		}
		if !architectureFactType(fact.Type) {
			continue
		}
		attrs := factAttributes(fact)
		source := inferredComponentFromFact(fact, attrs)
		if source != "" {
			sourceByFile[fact.FilePath] = source
			key := architectureKey("component", source)
			component := ensureFactComponent(model, key, source, "service", technologyFromLanguage(fact.FilePath), fact.FilePath, factEvidence(fact, "source-service"))
			component.Tags = appendUnique(component.Tags, "arch:component", "arch:source")
		}
		switch fact.Type {
		case "runtime.component":
			name := firstNonEmpty(attrs["name"], fact.Name, fact.ObjectName)
			if name == "" {
				continue
			}
			kind := firstNonEmpty(attrs["kind"], "service")
			technology := firstNonEmpty(attrs["technology"], "Runtime")
			key := architectureKey(kindKey(kind), name)
			component := ensureFactComponent(model, key, name, kind, technology, fact.FilePath, factEvidence(fact, "runtime-component"))
			component.Tags = appendUnique(component.Tags, fact.Tags...)
		case "runtime.connection":
			source := firstNonEmpty(attrs["source"], sourceByFile[fact.FilePath])
			target := firstNonEmpty(attrs["target"], fact.ObjectName)
			addFactConnector(model, source, target, firstNonEmpty(attrs["label"], "uses"), firstNonEmpty(fact.Relationship, "runtime-dependency"), fact.Confidence, factEvidence(fact, "runtime-connection"))
		case "runtime.endpoint_ref":
			source := sourceByFile[fact.FilePath]
			target := firstNonEmpty(attrs["target"], fact.ObjectName)
			if target != "" && source != "" {
				addFactConnector(model, source, target, "uses", "runtime-dependency", fact.Confidence, factEvidence(fact, "endpoint-reference"))
			}
		case "grpc.server", "grpc.contract":
			name := firstNonEmpty(attrs["service"], fact.Name, fact.ObjectName)
			if name == "" {
				continue
			}
			kind := "interface"
			if fact.Type == "grpc.server" {
				kind = "service"
			}
			key := architectureKey(kindKey(kind), name)
			component := ensureFactComponent(model, key, name, kind, "gRPC", fact.FilePath, factEvidence(fact, fact.Type))
			component.Tags = appendUnique(component.Tags, "protocol:grpc", "arch:contract")
		case "grpc.client":
			source := sourceByFile[fact.FilePath]
			target := firstNonEmpty(attrs["service"], fact.Name, fact.ObjectName)
			addFactConnector(model, source, target, "grpc", "runtime-dependency", fact.Confidence, factEvidence(fact, "grpc-client"))
		case "http.client":
			source := sourceByFile[fact.FilePath]
			target := firstNonEmpty(attrs["target"], "external traffic")
			addFactConnector(model, source, target, "http", "runtime-dependency", fact.Confidence, factEvidence(fact, "http-client"))
		case "datastore.dependency", "external.dependency":
			source := sourceByFile[fact.FilePath]
			target := firstNonEmpty(attrs["name"], fact.Name, fact.ObjectName)
			kind := "external"
			if fact.Type == "datastore.dependency" {
				kind = "datastore"
			}
			if target != "" {
				component := ensureFactComponent(model, architectureKey(kind, target), target, kind, firstNonEmpty(attrs["technology"], "External"), fact.FilePath, factEvidence(fact, fact.Type))
				component.Tags = appendUnique(component.Tags, fact.Tags...)
			}
			addFactConnector(model, source, target, labelForDependency(target), "runtime-dependency", fact.Confidence, factEvidence(fact, fact.Type))
		}
	}
	return model
}

func architectureFactType(factType string) bool {
	switch factType {
	case "runtime.component", "runtime.connection", "runtime.endpoint_ref", "grpc.server", "grpc.contract", "grpc.client", "http.client", "datastore.dependency", "external.dependency":
		return true
	default:
		return false
	}
}

func ensureFactComponent(model architectureModel, key, name, kind, technology, filePath string, evidence architectureEvidence) *architectureComponent {
	if key == "" || name == "" {
		return nil
	}
	if existing := model.Components[key]; existing != nil {
		existing.Technology = firstNonEmpty(existing.Technology, technology)
		existing.FilePath = firstNonEmpty(existing.FilePath, filePath)
		existing.Evidence = append(existing.Evidence, evidence)
		return existing
	}
	component := &architectureComponent{
		Key:        key,
		Name:       name,
		Kind:       kind,
		Technology: technology,
		FilePath:   filePath,
		Tags:       []string{"arch:component"},
		Evidence:   []architectureEvidence{evidence},
	}
	model.Components[key] = component
	return component
}

func addFactConnector(model architectureModel, source, target, label, relationship string, confidence float64, evidence architectureEvidence) {
	source = normalizeFactEndpoint(source)
	target = normalizeFactEndpoint(target)
	if source == "" || target == "" || source == target {
		return
	}
	sourceKey := architectureKey("component", source)
	targetKey := architectureKey("component", target)
	if label == "redis" || strings.Contains(target, "redis") {
		targetKey = architectureKey("component", target)
	}
	if model.Components[sourceKey] == nil {
		ensureFactComponent(model, sourceKey, source, "service", "Runtime", evidence.Path, evidence)
	}
	if model.Components[targetKey] == nil {
		ensureFactComponent(model, targetKey, target, "service", "Runtime", evidence.Path, evidence)
	}
	if label == "" {
		label = "uses"
	}
	if relationship == "" {
		relationship = "runtime-dependency"
	}
	key := sourceKey + "->" + targetKey + ":" + relationship + ":" + label
	if existing := model.Connectors[key]; existing != nil {
		existing.Evidence = append(existing.Evidence, evidence)
		if confidence > existing.Confidence {
			existing.Confidence = confidence
		}
		return
	}
	model.Connectors[key] = &architectureConnector{
		Key:          key,
		SourceKey:    sourceKey,
		TargetKey:    targetKey,
		Label:        label,
		Relationship: relationship,
		Direction:    "forward",
		Confidence:   confidence,
		Evidence:     []architectureEvidence{evidence},
	}
}

func factAttributes(fact Fact) map[string]string {
	var attrs map[string]string
	if fact.AttributesJSON != "" {
		_ = json.Unmarshal([]byte(fact.AttributesJSON), &attrs)
	}
	if attrs == nil {
		attrs = map[string]string{}
	}
	return attrs
}

func inferredComponentFromFact(fact Fact, attrs map[string]string) string {
	if value := attrs["source"]; value != "" {
		return value
	}
	return componentFromPath(fact.FilePath)
}

func componentFromPath(rel string) string {
	rel = path.Clean(strings.ReplaceAll(rel, "\\", "/"))
	parts := strings.Split(rel, "/")
	for i := len(parts) - 2; i >= 0; i-- {
		part := strings.TrimSpace(parts[i])
		if part != "." && part != "" && !architecturePathLayoutToken(part) {
			return part
		}
	}
	return ""
}

func architecturePathLayoutToken(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "app", "apps", "cmd", "internal", "lib", "libs", "pkg", "packages", "service", "services", "source", "src":
		return true
	default:
		return false
	}
}

func normalizeFactEndpoint(value string) string {
	value = strings.Trim(strings.TrimSpace(strings.ToLower(value)), `"'`)
	if value == "" || strings.Contains(value, "{{") || strings.HasPrefix(value, "$") {
		return ""
	}
	if strings.Contains(value, "://") {
		value = strings.SplitN(value, "://", 2)[1]
	}
	if strings.Contains(value, ":") {
		value = strings.Split(value, ":")[0]
	}
	value = strings.Split(value, ".")[0]
	value = strings.Trim(value, "/")
	nonName := regexp.MustCompile(`[^a-z0-9-]+`)
	value = nonName.ReplaceAllString(value, "-")
	return strings.Trim(value, "-")
}

func factEvidence(fact Fact, kind string) architectureEvidence {
	note := fact.Name
	if note == "" {
		note = fact.Type
	}
	return architectureEvidence{Kind: kind, Path: fact.FilePath, Note: note}
}

func technologyFromLanguage(filePath string) string {
	switch strings.ToLower(path.Ext(filePath)) {
	case ".go":
		return "Go"
	case ".py":
		return "Python"
	case ".js", ".mjs", ".cjs":
		return "Javascript"
	case ".ts", ".tsx":
		return "Typescript"
	case ".java":
		return "Java"
	case ".cs", ".csproj":
		return ".NET"
	case ".cpp", ".c", ".h":
		return "C/C++"
	case ".rb":
		return "Ruby"
	case ".php":
		return "PHP"
	default:
		return "Runtime"
	}
}

func kindKey(kind string) string {
	switch strings.ToLower(kind) {
	case "datastore", "queue", "external", "contract":
		return strings.ToLower(kind)
	case "interface":
		return "contract"
	default:
		return "component"
	}
}

func labelForDependency(target string) string {
	if strings.Contains(target, "redis") {
		return "redis"
	}
	if strings.Contains(target, "otel") || strings.Contains(target, "opentelemetry") {
		return "observes"
	}
	return "uses"
}

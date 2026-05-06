package watch

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type architectureModel struct {
	Components map[string]*architectureComponent
	Connectors map[string]*architectureConnector
}

type architectureComponent struct {
	Key         string
	Name        string
	Kind        string
	Technology  string
	Description string
	FilePath    string
	Tags        []string
	Evidence    []architectureEvidence
}

type architectureConnector struct {
	Key          string
	SourceKey    string
	TargetKey    string
	Label        string
	Relationship string
	Description  string
	Confidence   float64
	Evidence     []architectureEvidence
}

type architectureEvidence struct {
	Kind string
	Path string
	Note string
}

type architectureEndpoint struct {
	ComponentKey string
	Name         string
	Hosts        []string
	Ports        []int
	Protocol     string
	FilePath     string
}

type architectureEndpointRef struct {
	SourceKey string
	Target    string
	Protocol  string
	FilePath  string
	Note      string
}

func inferArchitecture(repoRoot string) architectureModel {
	return inferArchitectureWithProgress(repoRoot, nil)
}

func inferArchitectureWithProgress(repoRoot string, progress ProgressSink) architectureModel {
	model := architectureModel{
		Components: map[string]*architectureComponent{},
		Connectors: map[string]*architectureConnector{},
	}
	if strings.TrimSpace(repoRoot) == "" {
		return model
	}
	collector := &architectureCollector{
		root:      repoRoot,
		model:     model,
		endpoints: map[string]architectureEndpoint{},
	}
	files := collectArchitectureFiles(repoRoot)
	progressStart(progress, "Inferring architecture artifacts", len(files))
	defer progressFinish(progress)
	for _, file := range files {
		collector.scanFile(filepath.Join(repoRoot, filepath.FromSlash(file)), file)
		progressAdvance(progress, file)
	}
	collector.resolveEndpointRefs()
	return model
}

func collectArchitectureFiles(repoRoot string) []string {
	var files []string
	_ = filepath.WalkDir(repoRoot, func(absPath string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			if shouldSkipArchitectureDir(entry.Name()) && absPath != repoRoot {
				return filepath.SkipDir
			}
			return nil
		}
		rel := filepath.ToSlash(mustRel(repoRoot, absPath))
		if shouldSkipArchitectureFile(rel, entry.Name()) {
			return nil
		}
		if isArchitectureArtifact(rel) {
			files = append(files, rel)
		}
		return nil
	})
	sort.Strings(files)
	return files
}

func isArchitectureArtifact(rel string) bool {
	switch strings.ToLower(filepath.Ext(rel)) {
	case ".yaml", ".yml", ".proto", ".tf", ".json":
		return true
	default:
		return false
	}
}

type architectureCollector struct {
	root         string
	model        architectureModel
	endpoints    map[string]architectureEndpoint
	endpointRefs []architectureEndpointRef
}

func (c *architectureCollector) scanFile(absPath, rel string) {
	ext := strings.ToLower(filepath.Ext(rel))
	switch ext {
	case ".yaml", ".yml":
		c.scanYAML(absPath, rel)
	case ".proto":
		c.scanProto(absPath, rel)
	case ".tf":
		c.scanTerraform(absPath, rel)
	case ".json":
		c.scanJSONSpec(absPath, rel)
	}
}

func (c *architectureCollector) scanYAML(absPath, rel string) {
	data, err := os.ReadFile(absPath)
	if err != nil || !looksLikeRuntimeYAML(data) {
		return
	}
	dec := yaml.NewDecoder(bytes.NewReader(data))
	for {
		var doc map[string]any
		err := dec.Decode(&doc)
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		if len(doc) == 0 {
			continue
		}
		c.consumeYAMLDocument(doc, rel)
	}
}

func (c *architectureCollector) consumeYAMLDocument(doc map[string]any, rel string) {
	kind := stringValue(doc["kind"])
	apiVersion := stringValue(doc["apiVersion"])
	if isDockerComposeDocument(doc) {
		c.consumeDockerCompose(doc, rel)
		return
	}
	switch strings.ToLower(kind) {
	case "deployment", "statefulset", "daemonset", "replicaset", "job", "cronjob", "pod":
		c.consumeKubernetesWorkload(kind, doc, rel)
	case "service":
		c.consumeKubernetesService(doc, rel)
	case "ingress", "gateway", "httproute", "tcproute", "virtualservice":
		c.consumeIngress(kind, doc, rel)
	case "networkpolicy":
		c.consumeNetworkPolicy(doc, rel)
	case "serviceentry":
		c.consumeExternalServiceEntry(doc, rel)
	default:
		if strings.Contains(strings.ToLower(apiVersion), "gateway.networking") {
			c.consumeIngress(kind, doc, rel)
		}
	}
}

func (c *architectureCollector) consumeKubernetesWorkload(kind string, doc map[string]any, rel string) {
	name := metadataName(doc)
	if name == "" {
		return
	}
	key := architectureKey("component", name)
	component := c.ensureComponent(key, name, "service", "Kubernetes", rel, architectureEvidence{Kind: "deployable", Path: rel, Note: kind})
	component.Tags = appendUnique(component.Tags, "arch:deployable", "runtime:kubernetes")
	for _, container := range kubernetesContainers(doc) {
		if image := stringValue(container["image"]); image != "" {
			component.Technology = firstNonEmpty(component.Technology, imageTechnology(image), "Container")
			component.Tags = appendUnique(component.Tags, "arch:container")
		}
		for _, port := range portsFromList(sliceValue(container["ports"])) {
			c.addEndpoint(name, architectureEndpoint{ComponentKey: key, Name: name, Hosts: []string{name}, Ports: []int{port}, FilePath: rel})
		}
		for _, ref := range endpointRefsFromEnv(sliceValue(container["env"])) {
			ref.SourceKey = key
			ref.FilePath = rel
			c.endpointRefs = append(c.endpointRefs, ref)
		}
	}
}

func (c *architectureCollector) consumeKubernetesService(doc map[string]any, rel string) {
	name := metadataName(doc)
	if name == "" {
		return
	}
	key := architectureKey("component", name)
	component := c.ensureComponent(key, name, "service", "Kubernetes", rel, architectureEvidence{Kind: "endpoint", Path: rel, Note: "Kubernetes Service"})
	component.Tags = appendUnique(component.Tags, "arch:endpoint", "runtime:kubernetes")
	spec := mapValue(doc["spec"])
	if serviceType := stringValue(spec["type"]); strings.EqualFold(serviceType, "LoadBalancer") || strings.EqualFold(serviceType, "NodePort") {
		externalKey := architectureKey("external", "external traffic")
		c.ensureComponent(externalKey, "External traffic", "external", "Network", rel, architectureEvidence{Kind: "ingress", Path: rel, Note: serviceType})
		c.addConnector(externalKey, key, "routes", "ingress", 0.82, architectureEvidence{Kind: "ingress", Path: rel, Note: serviceType})
	}
	ports := portsFromList(sliceValue(spec["ports"]))
	hosts := []string{name}
	if clusterIP := stringValue(spec["clusterIP"]); clusterIP != "" && !strings.EqualFold(clusterIP, "none") {
		hosts = append(hosts, clusterIP)
	}
	c.addEndpoint(name, architectureEndpoint{ComponentKey: key, Name: name, Hosts: hosts, Ports: ports, FilePath: rel})
}

func (c *architectureCollector) consumeIngress(kind string, doc map[string]any, rel string) {
	name := metadataName(doc)
	externalKey := architectureKey("external", "external traffic")
	c.ensureComponent(externalKey, "External traffic", "external", "Network", rel, architectureEvidence{Kind: "ingress", Path: rel, Note: kind})
	for _, target := range serviceNamesFromYAML(doc) {
		if targetKey := c.lookupEndpointTarget(target); targetKey != "" {
			c.addConnector(externalKey, targetKey, "routes", "ingress", 0.82, architectureEvidence{Kind: "ingress", Path: rel, Note: firstNonEmpty(name, kind)})
		}
	}
}

func (c *architectureCollector) consumeNetworkPolicy(doc map[string]any, rel string) {
	targetName := metadataName(doc)
	targetKey := c.lookupEndpointTarget(targetName)
	if targetKey == "" {
		return
	}
	for _, peer := range serviceNamesFromYAML(doc) {
		sourceKey := c.lookupEndpointTarget(peer)
		if sourceKey == "" || sourceKey == targetKey {
			continue
		}
		c.addConnector(sourceKey, targetKey, "allows", "network-policy", 0.55, architectureEvidence{Kind: "network-policy", Path: rel, Note: "ingress policy"})
	}
}

func (c *architectureCollector) consumeExternalServiceEntry(doc map[string]any, rel string) {
	for _, host := range stringList(mapValue(doc["spec"])["hosts"]) {
		key := architectureKey("external", host)
		c.ensureComponent(key, host, "external", "Network", rel, architectureEvidence{Kind: "external-dependency", Path: rel, Note: "service entry"})
		c.addEndpoint(host, architectureEndpoint{ComponentKey: key, Name: host, Hosts: []string{host}, FilePath: rel})
	}
}

func (c *architectureCollector) consumeDockerCompose(doc map[string]any, rel string) {
	services := mapValue(doc["services"])
	for name, raw := range services {
		spec := mapValue(raw)
		key := architectureKey("component", name)
		component := c.ensureComponent(key, name, "service", "Docker Compose", rel, architectureEvidence{Kind: "deployable", Path: rel, Note: "compose service"})
		component.Tags = appendUnique(component.Tags, "arch:deployable", "runtime:compose")
		if image := stringValue(spec["image"]); image != "" {
			component.Technology = firstNonEmpty(imageTechnology(image), "Container")
		}
		c.addEndpoint(name, architectureEndpoint{ComponentKey: key, Name: name, Hosts: []string{name}, Ports: composePorts(spec["ports"]), FilePath: rel})
		for _, dep := range composeDependsOn(spec["depends_on"]) {
			c.addConnector(key, architectureKey("component", dep), "depends on", "runtime-dependency", 0.70, architectureEvidence{Kind: "compose", Path: rel, Note: "depends_on"})
		}
		for _, ref := range endpointRefsFromEnv(composeEnv(spec["environment"])) {
			ref.SourceKey = key
			ref.FilePath = rel
			c.endpointRefs = append(c.endpointRefs, ref)
		}
	}
}

func (c *architectureCollector) scanProto(absPath, rel string) {
	data, err := os.ReadFile(absPath)
	if err != nil || isGeneratedSource(rel, data) {
		return
	}
	re := regexp.MustCompile(`(?m)^\s*service\s+([A-Za-z_][A-Za-z0-9_]*)\s*\{`)
	matches := re.FindAllStringSubmatch(string(data), -1)
	for _, match := range matches {
		name := match[1]
		key := architectureKey("contract", name)
		component := c.ensureComponent(key, name, "interface", "gRPC", rel, architectureEvidence{Kind: "service-contract", Path: rel, Note: "protobuf service"})
		component.Tags = appendUnique(component.Tags, "arch:contract", "protocol:grpc")
		c.addEndpoint(name, architectureEndpoint{ComponentKey: key, Name: name, Hosts: []string{name}, Protocol: "grpc", FilePath: rel})
	}
}

func (c *architectureCollector) scanJSONSpec(absPath, rel string) {
	data, err := os.ReadFile(absPath)
	if err != nil || !bytes.Contains(data, []byte(`"openapi"`)) {
		return
	}
	name := strings.TrimSuffix(path.Base(rel), path.Ext(rel))
	key := architectureKey("contract", name)
	component := c.ensureComponent(key, name, "interface", "OpenAPI", rel, architectureEvidence{Kind: "service-contract", Path: rel, Note: "OpenAPI"})
	component.Tags = appendUnique(component.Tags, "arch:contract", "protocol:http")
}

func (c *architectureCollector) scanTerraform(absPath, rel string) {
	file, err := os.Open(absPath)
	if err != nil {
		return
	}
	defer func() { _ = file.Close() }()
	re := regexp.MustCompile(`^\s*resource\s+"([^"]+)"\s+"([^"]+)"`)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		match := re.FindStringSubmatch(scanner.Text())
		if len(match) != 3 {
			continue
		}
		resourceType, resourceName := match[1], match[2]
		kind, tech, ok := infrastructureKind(resourceType)
		if !ok {
			continue
		}
		key := architectureKey(kind, resourceType+":"+resourceName)
		component := c.ensureComponent(key, resourceName, kind, tech, rel, architectureEvidence{Kind: "infrastructure", Path: rel, Note: resourceType})
		component.Tags = appendUnique(component.Tags, "arch:infrastructure")
		c.addEndpoint(resourceName, architectureEndpoint{ComponentKey: key, Name: resourceName, Hosts: []string{resourceName}, FilePath: rel})
	}
}

func (c *architectureCollector) resolveEndpointRefs() {
	for _, ref := range c.endpointRefs {
		targetKey := c.lookupEndpointTarget(ref.Target)
		if targetKey == "" || ref.SourceKey == "" || ref.SourceKey == targetKey {
			continue
		}
		label := "uses"
		if ref.Protocol != "" {
			label = ref.Protocol
		}
		c.addConnector(ref.SourceKey, targetKey, label, "runtime-dependency", 0.78, architectureEvidence{Kind: "consumed-endpoint", Path: ref.FilePath, Note: ref.Note})
	}
}

func (c *architectureCollector) lookupEndpointTarget(value string) string {
	host := normalizeEndpointHost(value)
	if host == "" {
		return ""
	}
	if ep, ok := c.endpoints[host]; ok {
		return ep.ComponentKey
	}
	return ""
}

func (c *architectureCollector) addEndpoint(host string, ep architectureEndpoint) {
	for _, candidate := range append(ep.Hosts, host, ep.Name) {
		normalized := normalizeEndpointHost(candidate)
		if normalized == "" {
			continue
		}
		c.endpoints[normalized] = ep
		short := strings.Split(normalized, ".")[0]
		if short != "" {
			c.endpoints[short] = ep
		}
	}
}

func (c *architectureCollector) ensureComponent(key, name, kind, technology, filePath string, evidence architectureEvidence) *architectureComponent {
	if existing := c.model.Components[key]; existing != nil {
		existing.Evidence = append(existing.Evidence, evidence)
		existing.Technology = firstNonEmpty(existing.Technology, technology)
		existing.FilePath = firstNonEmpty(existing.FilePath, filePath)
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
	c.model.Components[key] = component
	return component
}

func (c *architectureCollector) addConnector(sourceKey, targetKey, label, relationship string, confidence float64, evidence architectureEvidence) {
	if sourceKey == "" || targetKey == "" || sourceKey == targetKey {
		return
	}
	key := sourceKey + "->" + targetKey + ":" + relationship + ":" + label
	if existing := c.model.Connectors[key]; existing != nil {
		existing.Evidence = append(existing.Evidence, evidence)
		if confidence > existing.Confidence {
			existing.Confidence = confidence
		}
		return
	}
	c.model.Connectors[key] = &architectureConnector{
		Key:          key,
		SourceKey:    sourceKey,
		TargetKey:    targetKey,
		Label:        label,
		Relationship: relationship,
		Confidence:   confidence,
		Evidence:     []architectureEvidence{evidence},
	}
}

func shouldSkipArchitectureDir(name string) bool {
	switch strings.ToLower(name) {
	case ".git", "node_modules", "vendor", "dist", "build", "target", ".cache", ".terraform", "coverage":
		return true
	default:
		return false
	}
}

func shouldSkipArchitectureFile(rel, name string) bool {
	lower := strings.ToLower(name)
	switch filepath.Ext(lower) {
	case ".png", ".jpg", ".jpeg", ".gif", ".svg", ".ico", ".css", ".lock", ".sum", ".md":
		return true
	}
	return strings.Contains(lower, ".test.") || strings.HasSuffix(lower, "_test.go") || strings.HasSuffix(lower, ".generated.go")
}

func looksLikeRuntimeYAML(data []byte) bool {
	lower := bytes.ToLower(data)
	return bytes.Contains(lower, []byte("kind:")) || bytes.Contains(lower, []byte("services:")) || bytes.Contains(lower, []byte("openapi:"))
}

func isDockerComposeDocument(doc map[string]any) bool {
	services := mapValue(doc["services"])
	if len(services) == 0 {
		return false
	}
	for _, raw := range services {
		if _, ok := raw.(map[string]any); ok {
			return true
		}
	}
	return false
}

func metadataName(doc map[string]any) string {
	return stringValue(mapValue(doc["metadata"])["name"])
}

func kubernetesContainers(doc map[string]any) []map[string]any {
	var out []map[string]any
	spec := mapValue(doc["spec"])
	templateSpec := mapValue(mapValue(mapValue(spec["template"])["spec"]))
	for _, raw := range append(sliceValue(templateSpec["initContainers"]), sliceValue(templateSpec["containers"])...) {
		if container := mapValue(raw); len(container) > 0 {
			out = append(out, container)
		}
	}
	for _, raw := range sliceValue(spec["containers"]) {
		if container := mapValue(raw); len(container) > 0 {
			out = append(out, container)
		}
	}
	return out
}

func portsFromList(values []any) []int {
	var out []int
	for _, raw := range values {
		switch v := raw.(type) {
		case map[string]any:
			if port := intValue(v["containerPort"]); port > 0 {
				out = append(out, port)
			}
			if port := intValue(v["port"]); port > 0 {
				out = append(out, port)
			}
			if port := intValue(v["targetPort"]); port > 0 {
				out = append(out, port)
			}
		case int:
			out = append(out, v)
		case string:
			if port, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
				out = append(out, port)
			}
		}
	}
	return uniqueInts(out)
}

func endpointRefsFromEnv(values []any) []architectureEndpointRef {
	var refs []architectureEndpointRef
	for _, raw := range values {
		switch v := raw.(type) {
		case map[string]any:
			value := stringValue(v["value"])
			if value == "" {
				continue
			}
			if target := endpointHostCandidate(value); target != "" {
				refs = append(refs, architectureEndpointRef{Target: target, Protocol: protocolFromEndpointValue(value), Note: "environment endpoint value"})
			}
		case string:
			if idx := strings.Index(v, "="); idx >= 0 {
				if target := endpointHostCandidate(v[idx+1:]); target != "" {
					refs = append(refs, architectureEndpointRef{Target: target, Protocol: protocolFromEndpointValue(v), Note: "environment endpoint value"})
				}
			}
		}
	}
	return refs
}

func endpointHostCandidate(value string) string {
	value = strings.Trim(strings.TrimSpace(value), `"'`)
	if value == "" || strings.Contains(value, "{{") || strings.Contains(value, "$(") {
		return ""
	}
	if parsed, err := url.Parse(value); err == nil && parsed.Hostname() != "" {
		return parsed.Hostname()
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		return host
	}
	if strings.Contains(value, ":") {
		host := strings.Split(value, ":")[0]
		if looksLikeHost(host) {
			return host
		}
	}
	if looksLikeHost(value) {
		return value
	}
	return ""
}

func protocolFromEndpointValue(value string) string {
	lower := strings.ToLower(value)
	switch {
	case strings.HasPrefix(lower, "http://"), strings.HasPrefix(lower, "https://"):
		return "http"
	case strings.Contains(lower, ":"):
		return "uses"
	default:
		return ""
	}
}

func looksLikeHost(value string) bool {
	if value == "" || strings.ContainsAny(value, " /\\") {
		return false
	}
	if strings.Contains(value, ".") {
		return true
	}
	for _, r := range value {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '-' {
			return false
		}
	}
	return true
}

func normalizeEndpointHost(value string) string {
	host := endpointHostCandidate(value)
	if host == "" {
		host = strings.TrimSpace(value)
	}
	host = strings.Trim(strings.ToLower(host), ".")
	if host == "" {
		return ""
	}
	return host
}

func composePorts(raw any) []int {
	var out []int
	for _, item := range sliceValue(raw) {
		switch v := item.(type) {
		case string:
			parts := strings.Split(v, ":")
			if port, err := strconv.Atoi(strings.TrimSpace(parts[len(parts)-1])); err == nil {
				out = append(out, port)
			}
		case int:
			out = append(out, v)
		case map[string]any:
			if port := intValue(v["target"]); port > 0 {
				out = append(out, port)
			}
			if port := intValue(v["published"]); port > 0 {
				out = append(out, port)
			}
		}
	}
	return uniqueInts(out)
}

func composeDependsOn(raw any) []string {
	if m := mapValue(raw); len(m) > 0 {
		return sortedKeys(m)
	}
	var out []string
	for _, item := range sliceValue(raw) {
		if value := stringValue(item); value != "" {
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}

func composeEnv(raw any) []any {
	if m := mapValue(raw); len(m) > 0 {
		out := make([]any, 0, len(m))
		for key, value := range m {
			out = append(out, key+"="+stringValue(value))
		}
		sort.Slice(out, func(i, j int) bool { return fmt.Sprint(out[i]) < fmt.Sprint(out[j]) })
		return out
	}
	return sliceValue(raw)
}

func serviceNamesFromYAML(value any) []string {
	seen := map[string]struct{}{}
	var walk func(any)
	walk = func(raw any) {
		switch v := raw.(type) {
		case map[string]any:
			for key, child := range v {
				lower := strings.ToLower(key)
				if lower == "name" || lower == "service" || lower == "servicename" || lower == "host" {
					if name := endpointHostCandidate(stringValue(child)); name != "" {
						seen[name] = struct{}{}
					}
				}
				walk(child)
			}
		case []any:
			for _, child := range v {
				walk(child)
			}
		}
	}
	walk(value)
	return sortedKeys(seen)
}

func infrastructureKind(resourceType string) (string, string, bool) {
	lower := strings.ToLower(resourceType)
	switch {
	case strings.Contains(lower, "redis"), strings.Contains(lower, "memcache"), strings.Contains(lower, "cache"):
		return "datastore", "Cache", true
	case strings.Contains(lower, "sql"), strings.Contains(lower, "database"), strings.Contains(lower, "spanner"), strings.Contains(lower, "alloydb"), strings.Contains(lower, "postgres"), strings.Contains(lower, "mysql"):
		return "datastore", "Database", true
	case strings.Contains(lower, "queue"), strings.Contains(lower, "pubsub"), strings.Contains(lower, "topic"), strings.Contains(lower, "subscription"):
		return "queue", "Messaging", true
	case strings.Contains(lower, "bucket"), strings.Contains(lower, "storage"):
		return "datastore", "Object Storage", true
	default:
		return "", "", false
	}
}

func isGeneratedSource(rel string, data []byte) bool {
	lowerPath := strings.ToLower(rel)
	lowerHead := strings.ToLower(string(data[:minInt(len(data), 4096)]))
	return strings.Contains(lowerPath, "genproto/") ||
		strings.Contains(lowerPath, "_pb2") ||
		strings.Contains(lowerHead, "code generated") ||
		strings.Contains(lowerHead, "generated by") ||
		strings.Contains(lowerHead, "@generated")
}

func imageTechnology(image string) string {
	base := strings.ToLower(path.Base(strings.Split(image, ":")[0]))
	switch {
	case strings.Contains(base, "redis"):
		return "Redis"
	case strings.Contains(base, "postgres"):
		return "PostgreSQL"
	case strings.Contains(base, "mysql"):
		return "MySQL"
	case strings.Contains(base, "nginx"):
		return "Nginx"
	default:
		return "Container"
	}
}

func architectureKey(kind, name string) string {
	return kind + ":" + architectureSlug(name)
}

func architectureSlug(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "unknown"
	}
	return out
}

func architectureEvidenceDescription(evidence []architectureEvidence, confidence float64) string {
	var parts []string
	if confidence > 0 {
		parts = append(parts, fmt.Sprintf("confidence %.2f", confidence))
	}
	for _, ev := range evidence {
		note := ev.Kind
		if ev.Note != "" {
			note += ": " + ev.Note
		}
		if ev.Path != "" {
			note += " (" + ev.Path + ")"
		}
		parts = append(parts, note)
	}
	return strings.Join(uniqueStrings(parts), "; ")
}

func mapValue(raw any) map[string]any {
	if value, ok := raw.(map[string]any); ok {
		return value
	}
	return nil
}

func sliceValue(raw any) []any {
	if value, ok := raw.([]any); ok {
		return value
	}
	return nil
}

func stringValue(raw any) string {
	switch v := raw.(type) {
	case string:
		return strings.TrimSpace(v)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10)
		}
		return fmt.Sprintf("%g", v)
	default:
		return ""
	}
}

func stringList(raw any) []string {
	var out []string
	for _, item := range sliceValue(raw) {
		if value := stringValue(item); value != "" {
			out = append(out, value)
		}
	}
	return out
}

func intValue(raw any) int {
	switch v := raw.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		i, _ := strconv.Atoi(strings.TrimSpace(v))
		return i
	default:
		return 0
	}
}

func uniqueInts(values []int) []int {
	seen := map[int]struct{}{}
	var out []int
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Ints(out)
	return out
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func appendUnique(values []string, next ...string) []string {
	return uniqueStrings(append(values, next...))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func mustRel(root, absPath string) string {
	rel, err := filepath.Rel(root, absPath)
	if err != nil {
		return absPath
	}
	return rel
}

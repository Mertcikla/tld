package watch

import (
	"slices"
	"sort"
	"strings"
)

func canonicalizeArchitecture(model architectureModel) architectureModel {
	if len(model.Components) == 0 {
		return model
	}
	uf := newArchitectureUnion(model.Components)
	unionExactArchitectureAliases(model, uf)
	unionServiceRootArchitectureAliases(model, uf)
	unionInferredServiceRootArchitectureAliases(model, uf)
	unionGenericArchitectureDependencies(model, uf)
	return rewriteCanonicalArchitecture(model, uf)
}

func pruneDisconnectedArchitecture(model architectureModel) architectureModel {
	if len(model.Components) == 0 || len(model.Connectors) == 0 {
		return architectureModel{Components: map[string]*architectureComponent{}, Connectors: map[string]*architectureConnector{}}
	}
	degree := map[string]int{}
	connectors := map[string]*architectureConnector{}
	for _, connector := range model.Connectors {
		if connector == nil || connector.SourceKey == "" || connector.TargetKey == "" || connector.SourceKey == connector.TargetKey {
			continue
		}
		if model.Components[connector.SourceKey] == nil || model.Components[connector.TargetKey] == nil {
			continue
		}
		degree[connector.SourceKey]++
		degree[connector.TargetKey]++
		copyConnector := *connector
		copyConnector.Evidence = append([]architectureEvidence{}, connector.Evidence...)
		connectors[connector.Key] = &copyConnector
	}
	components := map[string]*architectureComponent{}
	for key, component := range model.Components {
		if component == nil || degree[key] == 0 {
			continue
		}
		copyComponent := *component
		copyComponent.Tags = append([]string{}, component.Tags...)
		copyComponent.Evidence = append([]architectureEvidence{}, component.Evidence...)
		components[key] = &copyComponent
	}
	for key, connector := range connectors {
		if components[connector.SourceKey] == nil || components[connector.TargetKey] == nil {
			delete(connectors, key)
		}
	}
	return architectureModel{Components: components, Connectors: connectors}
}

type architectureUnion struct {
	parent map[string]string
}

func newArchitectureUnion(components map[string]*architectureComponent) *architectureUnion {
	parent := map[string]string{}
	for key := range components {
		parent[key] = key
	}
	return &architectureUnion{parent: parent}
}

func (u *architectureUnion) find(key string) string {
	parent, ok := u.parent[key]
	if !ok {
		return key
	}
	if parent == key {
		return key
	}
	root := u.find(parent)
	u.parent[key] = root
	return root
}

func (u *architectureUnion) union(a, b string) {
	ra, rb := u.find(a), u.find(b)
	if ra == rb {
		return
	}
	if architectureCanonicalRankKey(ra) <= architectureCanonicalRankKey(rb) {
		u.parent[rb] = ra
		return
	}
	u.parent[ra] = rb
}

func (u *architectureUnion) unionInto(parent, child string) {
	rp, rc := u.find(parent), u.find(child)
	if rp == rc {
		return
	}
	u.parent[rc] = rp
}

func unionExactArchitectureAliases(model architectureModel, uf *architectureUnion) {
	byName := map[string][]string{}
	for key, component := range model.Components {
		name := normalizedArchitectureName(component)
		if name == "" {
			continue
		}
		byName[name] = append(byName[name], key)
	}
	for _, keys := range byName {
		if len(keys) < 2 {
			continue
		}
		sort.Strings(keys)
		for _, key := range keys[1:] {
			if architectureComponentsAliasCompatible(model.Components[keys[0]], model.Components[key]) {
				uf.union(keys[0], key)
			}
		}
	}
}

func unionGenericArchitectureDependencies(model architectureModel, uf *architectureUnion) {
	bySourceFamily := map[string]map[string]map[string]struct{}{}
	for _, connector := range model.Connectors {
		if connector == nil {
			continue
		}
		source := uf.find(connector.SourceKey)
		target := uf.find(connector.TargetKey)
		component := model.Components[target]
		if component == nil {
			continue
		}
		family := architectureDependencyFamily(component)
		if family == "" {
			continue
		}
		sourceKey := source + "\x00" + family
		if bySourceFamily[sourceKey] == nil {
			bySourceFamily[sourceKey] = map[string]map[string]struct{}{"generic": {}, "concrete": {}}
		}
		if architectureGenericDependency(component, family) {
			bySourceFamily[sourceKey]["generic"][target] = struct{}{}
			continue
		}
		bySourceFamily[sourceKey]["concrete"][target] = struct{}{}
	}
	for _, group := range bySourceFamily {
		if len(group["generic"]) == 0 || len(group["concrete"]) != 1 {
			continue
		}
		var concrete string
		for key := range group["concrete"] {
			concrete = key
		}
		for generic := range group["generic"] {
			uf.union(concrete, generic)
		}
	}
}

func unionServiceRootArchitectureAliases(model architectureModel, uf *architectureUnion) {
	byRoot := map[string][]string{}
	for key, component := range model.Components {
		root := architectureServiceRootIdentity(component)
		if root == "" {
			continue
		}
		byRoot[root] = append(byRoot[root], key)
	}
	for _, keys := range byRoot {
		if len(keys) < 2 {
			continue
		}
		sort.SliceStable(keys, func(i, j int) bool {
			left, right := model.Components[keys[i]], model.Components[keys[j]]
			leftRank, rightRank := architectureServiceAliasRank(left), architectureServiceAliasRank(right)
			if leftRank != rightRank {
				return leftRank < rightRank
			}
			return keys[i] < keys[j]
		})
		canonical := keys[0]
		for _, key := range keys[1:] {
			if architectureServiceRootAliasCompatible(model, canonical, key) {
				uf.unionInto(canonical, key)
			}
		}
	}
}

func unionInferredServiceRootArchitectureAliases(model architectureModel, uf *architectureUnion) {
	knownRoots := knownArchitectureServiceRoots(model)
	byRoot := map[string][]string{}
	for key, component := range model.Components {
		root := architectureInferredServiceRootIdentity(component, knownRoots)
		if root == "" {
			continue
		}
		byRoot[root] = append(byRoot[root], key)
	}
	for _, keys := range byRoot {
		if len(keys) < 2 {
			continue
		}
		sort.SliceStable(keys, func(i, j int) bool {
			left, right := model.Components[keys[i]], model.Components[keys[j]]
			leftRank, rightRank := architectureServiceAliasRank(left), architectureServiceAliasRank(right)
			if leftRank != rightRank {
				return leftRank < rightRank
			}
			return keys[i] < keys[j]
		})
		canonical := keys[0]
		for _, key := range keys[1:] {
			if architectureInferredServiceRootAliasCompatible(model, canonical, key, knownRoots) {
				uf.unionInto(canonical, key)
			}
		}
	}
}

func rewriteCanonicalArchitecture(model architectureModel, uf *architectureUnion) architectureModel {
	out := architectureModel{Components: map[string]*architectureComponent{}, Connectors: map[string]*architectureConnector{}}
	for key, component := range model.Components {
		if component == nil {
			continue
		}
		root := uf.find(key)
		existing := out.Components[root]
		if existing == nil {
			copyComponent := *component
			copyComponent.Key = root
			copyComponent.Tags = append([]string{}, component.Tags...)
			copyComponent.Evidence = append([]architectureEvidence{}, component.Evidence...)
			out.Components[root] = &copyComponent
			continue
		}
		mergeArchitectureComponent(existing, component)
	}
	connectorPairIndex := map[string]string{}
	connectors := make([]*architectureConnector, 0, len(model.Connectors))
	for _, connector := range model.Connectors {
		if connector != nil {
			connectors = append(connectors, connector)
		}
	}
	sort.SliceStable(connectors, func(i, j int) bool {
		return connectors[i].Key < connectors[j].Key
	})
	for _, connector := range connectors {
		if connector == nil {
			continue
		}
		source := uf.find(connector.SourceKey)
		target := uf.find(connector.TargetKey)
		if source == "" || target == "" || source == target {
			continue
		}
		pairKey := architectureConnectorPairKey(source, target)
		direction := normalizedArchitectureConnectorDirection(connector.Direction)
		if existingKey, ok := connectorPairIndex[pairKey]; ok {
			existing := out.Connectors[existingKey]
			if existing == nil {
				continue
			}
			if existing.SourceKey == target && existing.TargetKey == source {
				direction = reverseArchitectureConnectorDirection(direction)
			}
			existing.Label = ""
			if existing.Relationship != connector.Relationship {
				existing.Relationship = ""
			}
			existing.Direction = mergeArchitectureConnectorDirections(existing.Direction, direction)
			if connector.Confidence > existing.Confidence {
				existing.Confidence = connector.Confidence
			}
			if existing.Description == "" {
				existing.Description = connector.Description
			}
			existing.Evidence = append(existing.Evidence, connector.Evidence...)
			continue
		}
		key := source + "->" + target
		connectorPairIndex[pairKey] = key
		copyConnector := *connector
		copyConnector.Key = key
		copyConnector.SourceKey = source
		copyConnector.TargetKey = target
		copyConnector.Direction = direction
		copyConnector.Evidence = append([]architectureEvidence{}, connector.Evidence...)
		out.Connectors[key] = &copyConnector
	}
	return out
}

func architectureConnectorPairKey(source, target string) string {
	if source <= target {
		return source + "\x00" + target
	}
	return target + "\x00" + source
}

func normalizedArchitectureConnectorDirection(direction string) string {
	switch strings.ToLower(strings.TrimSpace(direction)) {
	case "backward":
		return "backward"
	case "both", "bidirectional":
		return "both"
	case "none":
		return "none"
	default:
		return "forward"
	}
}

func reverseArchitectureConnectorDirection(direction string) string {
	switch normalizedArchitectureConnectorDirection(direction) {
	case "forward":
		return "backward"
	case "backward":
		return "forward"
	default:
		return normalizedArchitectureConnectorDirection(direction)
	}
}

func mergeArchitectureConnectorDirections(a, b string) string {
	forward, backward, none := architectureConnectorDirectionBits(a)
	bForward, bBackward, bNone := architectureConnectorDirectionBits(b)
	forward = forward || bForward
	backward = backward || bBackward
	none = none && bNone
	switch {
	case forward && backward:
		return "both"
	case backward:
		return "backward"
	case forward:
		return "forward"
	case none:
		return "none"
	default:
		return "forward"
	}
}

func architectureConnectorDirectionBits(direction string) (forward, backward, none bool) {
	switch normalizedArchitectureConnectorDirection(direction) {
	case "both":
		return true, true, false
	case "backward":
		return false, true, false
	case "none":
		return false, false, true
	default:
		return true, false, false
	}
}

func mergeArchitectureComponent(dst, src *architectureComponent) {
	if architectureComponentRank(src) < architectureComponentRank(dst) {
		dst.Name = src.Name
		dst.Kind = src.Kind
		dst.Technology = src.Technology
		dst.Description = firstNonEmpty(src.Description, dst.Description)
		dst.FilePath = firstNonEmpty(src.FilePath, dst.FilePath)
	} else {
		dst.Technology = firstNonEmpty(dst.Technology, src.Technology)
		dst.Description = firstNonEmpty(dst.Description, src.Description)
		dst.FilePath = firstNonEmpty(dst.FilePath, src.FilePath)
	}
	dst.Tags = appendUnique(dst.Tags, src.Tags...)
	dst.Evidence = append(dst.Evidence, src.Evidence...)
}

func architectureComponentsAliasCompatible(a, b *architectureComponent) bool {
	if a == nil || b == nil {
		return false
	}
	if normalizedArchitectureName(a) == "" || normalizedArchitectureName(a) != normalizedArchitectureName(b) {
		return false
	}
	if architectureComponentClass(a) == "interface" || architectureComponentClass(b) == "interface" {
		return architectureComponentClass(a) == architectureComponentClass(b)
	}
	return true
}

func normalizedArchitectureName(component *architectureComponent) string {
	if component == nil {
		return ""
	}
	return architectureSlug(component.Name)
}

func architectureServiceRootIdentity(component *architectureComponent) string {
	if component == nil || !architectureServiceAliasCandidate(component) {
		return ""
	}
	root, ok := architectureServiceRootFromTokens(architectureNameTokens(component.Name), false)
	if !ok {
		return ""
	}
	return root
}

func architectureServiceRootFromTokens(tokens []string, allowShort bool) (string, bool) {
	if len(tokens) == 0 {
		return "", false
	}
	filtered := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if architectureRoleToken(token) {
			continue
		}
		filtered = append(filtered, token)
	}
	if len(filtered) == 0 {
		return "", false
	}
	root := strings.Join(filtered, "-")
	if len(root) < 3 && !allowShort {
		return "", false
	}
	return root, true
}

func architectureInferredServiceRootIdentity(component *architectureComponent, knownRoots map[string]struct{}) string {
	if component == nil || !architectureServiceAliasCandidate(component) {
		return ""
	}
	tokens := architectureNameTokens(component.Name)
	if root, ok := architectureServiceRootFromTokens(tokens, true); ok {
		compactRoot := strings.ReplaceAll(root, "-", "")
		for known := range knownRoots {
			if strings.Contains(known, "-") && strings.ReplaceAll(known, "-", "") == compactRoot {
				return known
			}
		}
		if _, known := knownRoots[root]; known {
			return root
		}
		if len(root) >= 3 {
			return root
		}
		if architectureHasRoleToken(tokens) {
			return root
		}
	}
	compact := architectureCompactServiceRoot(component.Name)
	if compact == "" {
		return ""
	}
	for root := range knownRoots {
		if strings.ReplaceAll(root, "-", "") == compact {
			return root
		}
	}
	return ""
}

func architectureNameTokens(value string) []string {
	var tokens []string
	var b strings.Builder
	var prevClass int
	flush := func() {
		if b.Len() == 0 {
			return
		}
		tokens = append(tokens, strings.ToLower(b.String()))
		b.Reset()
	}
	for _, r := range strings.TrimSpace(value) {
		class := architectureRuneClass(r)
		if class == 0 {
			flush()
			prevClass = 0
			continue
		}
		if b.Len() > 0 && class != prevClass {
			if class == 2 || prevClass == 3 || class == 3 {
				flush()
			}
		}
		if r >= 'A' && r <= 'Z' {
			r += 'a' - 'A'
		}
		b.WriteRune(r)
		prevClass = class
	}
	flush()
	return splitArchitectureAcronymSuffixes(tokens)
}

func splitArchitectureAcronymSuffixes(tokens []string) []string {
	var out []string
	for _, token := range tokens {
		if len(token) <= 2 {
			out = append(out, token)
			continue
		}
		matched := false
		for _, suffix := range []string{"controller", "database", "service", "gateway", "adapter", "handler", "server", "client", "worker", "store", "api", "svc", "db"} {
			if strings.HasSuffix(token, suffix) && len(token) > len(suffix) {
				out = append(out, token[:len(token)-len(suffix)], suffix)
				matched = true
				break
			}
		}
		if !matched {
			out = append(out, token)
		}
	}
	return out
}

func architectureRuneClass(r rune) int {
	switch {
	case r >= 'a' && r <= 'z':
		return 1
	case r >= 'A' && r <= 'Z':
		return 2
	case r >= '0' && r <= '9':
		return 3
	default:
		return 0
	}
}

func architectureRoleToken(token string) bool {
	switch token {
	case "service", "svc", "api", "db", "database", "store", "client", "server", "worker", "gateway", "adapter", "controller", "handler":
		return true
	default:
		return false
	}
}

func architectureHasRoleToken(tokens []string) bool {
	return slices.ContainsFunc(tokens, architectureRoleToken)
}

func knownArchitectureServiceRoots(model architectureModel) map[string]struct{} {
	known := map[string]struct{}{}
	for _, component := range model.Components {
		if component == nil || !architectureServiceAliasCandidate(component) {
			continue
		}
		root, ok := architectureServiceRootFromTokens(architectureNameTokens(component.Name), true)
		if !ok {
			continue
		}
		if strings.Contains(root, "-") || architectureHasRoleToken(architectureNameTokens(component.Name)) {
			known[root] = struct{}{}
		}
	}
	return known
}

func architectureCompactServiceRoot(name string) string {
	tokens := architectureNameTokens(name)
	root, ok := architectureServiceRootFromTokens(tokens, true)
	if !ok {
		return ""
	}
	return strings.ReplaceAll(root, "-", "")
}

func architectureServiceAliasCandidate(component *architectureComponent) bool {
	if component == nil {
		return false
	}
	if architectureComponentClass(component) == "interface" {
		return true
	}
	switch component.Kind {
	case "service", "interface":
		return true
	default:
		return false
	}
}

func architectureServiceRootAliasCompatible(model architectureModel, aKey, bKey string) bool {
	a, b := model.Components[aKey], model.Components[bKey]
	if !architectureServiceAliasCandidate(a) || !architectureServiceAliasCandidate(b) {
		return false
	}
	root := architectureServiceRootIdentity(a)
	if root == "" || root != architectureServiceRootIdentity(b) {
		return false
	}
	if architectureComponentsConnected(model, aKey, bKey) {
		return true
	}
	return architectureComponentHasStrongServiceEvidence(a) && architectureComponentHasStrongServiceEvidence(b)
}

func architectureInferredServiceRootAliasCompatible(model architectureModel, aKey, bKey string, knownRoots map[string]struct{}) bool {
	a, b := model.Components[aKey], model.Components[bKey]
	if !architectureServiceAliasCandidate(a) || !architectureServiceAliasCandidate(b) {
		return false
	}
	root := architectureInferredServiceRootIdentity(a, knownRoots)
	if root == "" || root != architectureInferredServiceRootIdentity(b, knownRoots) {
		return false
	}
	if len(root) < 3 {
		return architectureShortRootAliasCompatible(a, b)
	}
	if architectureComponentsConnected(model, aKey, bKey) {
		return true
	}
	return architectureComponentHasStrongServiceEvidence(a) && architectureComponentHasStrongServiceEvidence(b)
}

func architectureShortRootAliasCompatible(a, b *architectureComponent) bool {
	if !architectureComponentHasStrongServiceEvidence(a) || !architectureComponentHasStrongServiceEvidence(b) {
		return false
	}
	return architectureHasRoleToken(architectureNameTokens(a.Name)) || architectureHasRoleToken(architectureNameTokens(b.Name))
}

func architectureComponentsConnected(model architectureModel, aKey, bKey string) bool {
	for _, connector := range model.Connectors {
		if connector == nil {
			continue
		}
		if connector.SourceKey == aKey && connector.TargetKey == bKey || connector.SourceKey == bKey && connector.TargetKey == aKey {
			return true
		}
	}
	return false
}

func architectureComponentHasStrongServiceEvidence(component *architectureComponent) bool {
	if component == nil {
		return false
	}
	for _, ev := range component.Evidence {
		switch ev.Kind {
		case "deployable", "endpoint", "runtime-component", "source-service", "grpc.server", "grpc-client", "grpc.contract", "service-contract":
			return true
		}
	}
	return false
}

func architectureServiceAliasRank(component *architectureComponent) int {
	if component == nil {
		return 99
	}
	name := normalizedArchitectureName(component)
	root := architectureServiceRootIdentity(component)
	switch {
	case name == root+"service":
		return 0
	case component.Kind == "service" && architectureComponentHasEvidence(component, "deployable"):
		return 1
	case component.Kind == "service" && architectureComponentHasEvidence(component, "runtime-component"):
		return 2
	case component.Kind == "service" && name == root:
		return 3
	case architectureComponentClass(component) == "interface":
		return 4
	default:
		return 5
	}
}

func architectureComponentHasEvidence(component *architectureComponent, kind string) bool {
	if component == nil {
		return false
	}
	for _, ev := range component.Evidence {
		if ev.Kind == kind {
			return true
		}
	}
	return false
}

func architectureDependencyFamily(component *architectureComponent) string {
	if component == nil {
		return ""
	}
	for _, tag := range component.Tags {
		tag = strings.TrimSpace(strings.ToLower(tag))
		if value, ok := strings.CutPrefix(tag, "datastore:"); ok && value != "" {
			return architectureSlug(value)
		}
		if value, ok := strings.CutPrefix(tag, "external:"); ok && value != "" {
			return architectureSlug(value)
		}
	}
	tech := architectureSlug(component.Technology)
	name := normalizedArchitectureName(component)
	switch component.Kind {
	case "datastore", "queue", "external":
		return firstNonEmpty(tech, name)
	default:
		if tech != "" && tech != "runtime" && tech != "container" && tech != "kubernetes" && tech != "docker-compose" {
			return tech
		}
	}
	return ""
}

func architectureGenericDependency(component *architectureComponent, family string) bool {
	name := normalizedArchitectureName(component)
	if name == "" || family == "" {
		return false
	}
	return name == family
}

func architectureComponentClass(component *architectureComponent) string {
	if component == nil {
		return ""
	}
	switch component.Kind {
	case "datastore", "queue", "external":
		return component.Kind
	case "interface":
		return "interface"
	default:
		return "component"
	}
}

func architectureCanonicalRankKey(key string) string {
	return architectureRankPrefixFromKey(key) + ":" + key
}

func architectureRankPrefixFromKey(key string) string {
	kind := key
	if before, _, ok := strings.Cut(key, ":"); ok {
		kind = before
	}
	switch kind {
	case "component":
		return "0"
	case "datastore", "queue":
		return "1"
	case "external":
		return "2"
	case "contract":
		return "3"
	default:
		return "4"
	}
}

func architectureComponentRank(component *architectureComponent) int {
	if component == nil {
		return 99
	}
	switch component.Kind {
	case "service":
		return 0
	case "datastore", "queue":
		return 1
	case "external":
		return 2
	case "interface":
		return 3
	default:
		return 4
	}
}

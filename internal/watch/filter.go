package watch

import (
	"context"
	"encoding/json"
	"path"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

type filterResult struct {
	RunID             int64
	SettingsHash      string
	RawGraphHash      string
	VisibleSymbols    map[int64]Symbol
	VisibleReferences []Reference
	VisibleFacts      []Fact
	VisibleFiles      map[string]struct{}
	Incoming          map[int64]int
	Outgoing          map[int64]int
	ChangedFiles      map[string]struct{}
	ContextPolicies   contextPolicySet
	ContextExpansions contextExpansionSet
	Visibility        VisibilityConfig
}

type visibilitySignal struct {
	Name   string  `json:"name"`
	Weight float64 `json:"weight"`
	Reason string  `json:"reason"`
}

type visibilityScore struct {
	Score   float64
	Signals []visibilitySignal
	Forced  bool
	Tier    int
}

func (s *visibilityScore) add(name string, weight float64, reason string) {
	if weight == 0 {
		return
	}
	s.Score += weight
	s.Signals = append(s.Signals, visibilitySignal{Name: name, Weight: weight, Reason: reason})
}

func (s visibilityScore) reason(fallback string) string {
	var reasons []string
	for _, signal := range s.Signals {
		if strings.TrimSpace(signal.Reason) != "" {
			reasons = append(reasons, signal.Reason)
		}
	}
	if len(reasons) == 0 {
		return fallback
	}
	return strings.Join(reasons, "; ")
}

func (s visibilityScore) signalsJSON() string {
	data, err := json.Marshal(s.Signals)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func defaultThresholds(thresholds Thresholds) Thresholds {
	if thresholds.MaxElementsPerView <= 0 {
		thresholds.MaxElementsPerView = 50
	}
	if thresholds.MaxConnectorsPerView <= 0 {
		thresholds.MaxConnectorsPerView = 100
	}
	if thresholds.MaxIncomingPerElement <= 0 {
		thresholds.MaxIncomingPerElement = 25
	}
	if thresholds.MaxOutgoingPerElement <= 0 {
		thresholds.MaxOutgoingPerElement = 40
	}
	if thresholds.MaxExpandedConnectorsPerGroup <= 0 {
		thresholds.MaxExpandedConnectorsPerGroup = 24
	}
	return thresholds
}

func settingsHash(req RepresentRequest) string {
	req.Embedding = normalizeEmbeddingConfig(req.Embedding)
	req.Thresholds = defaultThresholds(req.Thresholds)
	req.Visibility = defaultVisibilityConfig(req.Visibility)
	return stableHash(req)
}

func runFilter(ctx context.Context, store *Store, repositoryID int64, thresholds Thresholds, visibilityCfg VisibilityConfig, rawGraphHash, settingsHash string, embeddings map[int64]Vector, forcedVisibleSymbols map[int64]string, contextPolicies contextPolicySet, identityKeys map[string]string) (filterResult, error) {
	visibilityCfg = defaultVisibilityConfig(visibilityCfg)
	symbols, err := store.SymbolsForRepository(ctx, repositoryID)
	if err != nil {
		return filterResult{}, err
	}
	refs, err := store.QueryReferences(ctx, repositoryID, ReferenceQuery{Limit: -1})
	if err != nil {
		return filterResult{}, err
	}
	facts, err := store.FactsForRepository(ctx, repositoryID)
	if err != nil {
		return filterResult{}, err
	}
	expansions, err := store.ActiveContextExpansionSet(ctx, repositoryID)
	if err != nil {
		return filterResult{}, err
	}
	incoming := map[int64]int{}
	outgoing := map[int64]int{}
	for _, ref := range refs {
		outgoing[ref.SourceSymbolID]++
		incoming[ref.TargetSymbolID]++
	}

	visible := map[int64]Symbol{}
	scores := map[int64]*visibilityScore{}
	symbolsByID := map[int64]Symbol{}
	for _, sym := range symbols {
		symbolsByID[sym.ID] = sym
		scores[sym.ID] = &visibilityScore{}
	}
	factsBySubject := map[string][]Fact{}
	for _, fact := range facts {
		key := ownerMapKey(fact.SubjectKind, fact.SubjectStableKey)
		factsBySubject[key] = append(factsBySubject[key], fact)
	}
	for _, sym := range symbols {
		score := scores[sym.ID]
		if isExportedSymbol(sym) {
			score.add("entrypoint.exported", 1.2, "exported/public symbol")
		}
		if outgoing[sym.ID] > 0 {
			score.add("graph.outgoing", 1, "has resolved outgoing reference")
		}
		for _, fact := range factsBySubject[ownerMapKey("symbol", sym.StableKey)] {
			if highSignalFact(fact) {
				score.add("fact.high_signal", visibilityCfg.Weights.HighSignalFact*fact.Confidence, "has high-signal fact "+fact.Type)
			} else if dependencyFact(fact) {
				score.add("fact.dependency", visibilityCfg.Weights.DependencyFact*fact.Confidence, "has dependency fact")
			}
		}
		if reason, ok := forcedVisibleSymbols[sym.ID]; ok {
			if strings.TrimSpace(reason) == "" {
				reason = "changed since latest watch version"
			}
			score.add("change.changed", visibilityCfg.Weights.Changed, reason)
			score.Forced = true
		}
		if reason, ok := contextPolicies.showSymbol(sym, identityKeys); ok {
			if strings.TrimSpace(reason) == "" {
				reason = "user marked as context"
			}
			score.add("policy.show", visibilityCfg.Weights.UserShow, reason)
			score.Forced = true
		}
		if tier := expansions.symbolTier(sym, identityKeys); tier > 0 {
			score.Tier = tier
			score.add("context.expansion", visibilityCfg.Weights.Selected, "selected context expansion tier "+strconv.Itoa(tier))
			score.Forced = true
		}
		if outgoing[sym.ID] > thresholds.MaxOutgoingPerElement || incoming[sym.ID] > thresholds.MaxIncomingPerElement {
			score.add("noise.high_degree", visibilityCfg.Weights.HighDegreeNoise, "high-degree non-entrypoint collapsed")
		}
		if looksLikeTinyUtility(sym) && outgoing[sym.ID]+incoming[sym.ID] > 8 {
			score.add("noise.utility", visibilityCfg.Weights.UtilityNoise, "utility noise collapsed")
		}
	}
	for _, sym := range symbols {
		score := scores[sym.ID]
		if score.Forced || !visibilityCfg.CoreThresholdEnabled || score.Score >= visibilityCfg.CoreThreshold {
			visible[sym.ID] = sym
		}
	}
	changed := true
	for changed {
		changed = false
		for _, ref := range refs {
			if _, ok := visible[ref.SourceSymbolID]; !ok {
				continue
			}
			if _, ok := visible[ref.TargetSymbolID]; ok {
				continue
			}
			if target, ok := symbolsByID[ref.TargetSymbolID]; ok {
				score := scores[target.ID]
				score.add("graph.proximity", visibilityCfg.Weights.RelationshipProximity, "referenced by visible symbol")
				if score.Forced || !visibilityCfg.CoreThresholdEnabled || score.Score >= visibilityCfg.CoreThreshold {
					visible[target.ID] = target
					changed = true
				}
			}
		}
	}
	for _, ref := range refs {
		if _, ok := forcedVisibleSymbols[ref.SourceSymbolID]; ok {
			if target, ok := symbolsByID[ref.TargetSymbolID]; ok {
				score := scores[target.ID]
				score.add("change.endpoint", visibilityCfg.Weights.Changed, "endpoint of changed symbol")
				score.Forced = true
				visible[target.ID] = target
			}
			continue
		}
		if _, ok := forcedVisibleSymbols[ref.TargetSymbolID]; ok {
			if source, ok := symbolsByID[ref.SourceSymbolID]; ok {
				score := scores[source.ID]
				score.add("change.endpoint", visibilityCfg.Weights.Changed, "endpoint of changed symbol")
				score.Forced = true
				visible[source.ID] = source
			}
		}
	}
	if len(embeddings) > 0 {
		rescueRelatedSymbolsScored(symbols, refs, visible, scores, embeddings, visibilityCfg.Weights.RelationshipProximity)
	}
	for _, sym := range symbols {
		if _, ok := contextPolicies.hideSymbol(sym, identityKeys); ok {
			score := scores[sym.ID]
			score.add("policy.hide", visibilityCfg.Weights.UserHide, "user marked as noise")
			if !score.Forced {
				delete(visible, sym.ID)
			}
		}
	}

	runID, err := store.BeginFilterRun(ctx, repositoryID, settingsHash, rawGraphHash)
	if err != nil {
		return filterResult{}, err
	}
	visibleSymbols := 0
	hiddenSymbols := 0
	for _, sym := range symbols {
		score := scores[sym.ID]
		scoreValue := score.Score
		ownerKey := symbolOwnerKey(sym, identityKeys)
		if _, ok := visible[sym.ID]; ok {
			visibleSymbols++
			if err := store.SaveFilterDecision(ctx, runID, "symbol", sym.ID, ownerKey, "visible", score.reason("visible by graph context"), &scoreValue, score.Tier, score.signalsJSON()); err != nil {
				return filterResult{}, err
			}
			continue
		}
		hiddenSymbols++
		if err := store.SaveFilterDecision(ctx, runID, "symbol", sym.ID, ownerKey, "hidden", score.reason("leaf private symbol without useful outgoing references"), &scoreValue, score.Tier, score.signalsJSON()); err != nil {
			return filterResult{}, err
		}
	}

	var visibleRefs []Reference
	hiddenRefs := 0
	for _, ref := range refs {
		_, sourceOK := visible[ref.SourceSymbolID]
		_, targetOK := visible[ref.TargetSymbolID]
		refOwnerKey := referenceOwnerKey(ref, symbolsByID, identityKeys)
		if _, hidden := contextPolicies.Hide[ownerMapKey("reference", refOwnerKey)]; hidden {
			hiddenRefs++
			scoreValue := visibilityCfg.Weights.UserHide
			if err := store.SaveFilterDecision(ctx, runID, "reference", ref.ID, refOwnerKey, "hidden", "user marked as noise", &scoreValue, 0, `[]`); err != nil {
				return filterResult{}, err
			}
		} else if sourceOK && targetOK {
			visibleRefs = append(visibleRefs, ref)
			scoreValue := 1.0
			if err := store.SaveFilterDecision(ctx, runID, "reference", ref.ID, refOwnerKey, "visible", "connects visible symbols", &scoreValue, 0, `[]`); err != nil {
				return filterResult{}, err
			}
		} else {
			hiddenRefs++
			scoreValue := 0.0
			if err := store.SaveFilterDecision(ctx, runID, "reference", ref.ID, refOwnerKey, "hidden", "unresolved or hidden endpoint", &scoreValue, 0, `[]`); err != nil {
				return filterResult{}, err
			}
		}
	}
	visibleFiles := filesForSymbols(visible)
	for file := range forcedVisibleSymbolsByFile(symbols, forcedVisibleSymbols) {
		visibleFiles[file] = struct{}{}
	}
	for file := range expansionsFiles(symbols, expansions, identityKeys) {
		visibleFiles[file] = struct{}{}
	}
	visibleFacts, err := scoreFacts(ctx, store, runID, facts, visible, visibleFiles, visibilityCfg, expansions)
	if err != nil {
		return filterResult{}, err
	}
	for _, fact := range visibleFacts {
		if strings.TrimSpace(fact.FilePath) != "" {
			visibleFiles[fact.FilePath] = struct{}{}
		}
	}
	if err := store.FinishFilterRun(ctx, runID, "completed", visibleSymbols, hiddenSymbols, len(visibleRefs), hiddenRefs); err != nil {
		return filterResult{}, err
	}
	return filterResult{
		RunID:             runID,
		SettingsHash:      settingsHash,
		RawGraphHash:      rawGraphHash,
		VisibleSymbols:    visible,
		VisibleReferences: visibleRefs,
		VisibleFacts:      visibleFacts,
		VisibleFiles:      visibleFiles,
		Incoming:          incoming,
		Outgoing:          outgoing,
		ContextPolicies:   contextPolicies,
		ContextExpansions: expansions,
		Visibility:        visibilityCfg,
	}, nil
}

func (p contextPolicySet) showSymbol(sym Symbol, identityKeys map[string]string) (string, bool) {
	return p.symbolPolicy(p.Show, sym, identityKeys)
}

func (p contextPolicySet) hideSymbol(sym Symbol, identityKeys map[string]string) (string, bool) {
	return p.symbolPolicy(p.Hide, sym, identityKeys)
}

func (p contextPolicySet) symbolPolicy(policies map[string]string, sym Symbol, identityKeys map[string]string) (string, bool) {
	for _, key := range []string{
		ownerMapKey("symbol", symbolOwnerKey(sym, identityKeys)),
		ownerMapKey("symbol", sym.StableKey),
		ownerMapKey("file", "file:"+sym.FilePath),
	} {
		if reason, ok := policies[key]; ok {
			return reason, true
		}
	}
	dir := path.Dir(sym.FilePath)
	for dir != "." && dir != "/" && dir != "" {
		if reason, ok := policies[ownerMapKey("folder", "folder:"+dir)]; ok {
			return reason, true
		}
		next := path.Dir(dir)
		if next == dir {
			break
		}
		dir = next
	}
	return "", false
}

func forcedVisibleSymbolsByFile(symbols []Symbol, forced map[int64]string) map[string]struct{} {
	out := map[string]struct{}{}
	if len(forced) == 0 {
		return out
	}
	for _, sym := range symbols {
		if _, ok := forced[sym.ID]; ok && strings.TrimSpace(sym.FilePath) != "" {
			out[sym.FilePath] = struct{}{}
		}
	}
	return out
}

func expansionsFiles(symbols []Symbol, expansions contextExpansionSet, identityKeys map[string]string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, sym := range symbols {
		if expansions.symbolTier(sym, identityKeys) > 0 && strings.TrimSpace(sym.FilePath) != "" {
			out[sym.FilePath] = struct{}{}
		}
	}
	return out
}

func scoreFacts(ctx context.Context, store *Store, runID int64, facts []Fact, visibleSymbols map[int64]Symbol, visibleFiles map[string]struct{}, cfg VisibilityConfig, expansions contextExpansionSet) ([]Fact, error) {
	var visible []Fact
	visibleSymbolStable := map[string]struct{}{}
	for _, sym := range visibleSymbols {
		visibleSymbolStable[sym.StableKey] = struct{}{}
	}
	for _, fact := range facts {
		if fact.Type == enrichmentVersionType {
			continue
		}
		score := visibilityScore{}
		if highSignalFact(fact) {
			score.add("fact.high_signal", cfg.Weights.HighSignalFact*fact.Confidence, "high-signal fact "+fact.Type)
		} else if dependencyFact(fact) {
			score.add("fact.dependency", cfg.Weights.DependencyFact*fact.Confidence, "dependency fact")
		}
		if fact.SubjectKind == "symbol" {
			if _, ok := visibleSymbolStable[fact.SubjectStableKey]; ok {
				score.add("fact.subject_visible", cfg.Weights.RelationshipProximity, "subject symbol is visible")
			}
		}
		if _, ok := visibleFiles[fact.FilePath]; ok {
			score.add("fact.file_visible", cfg.Weights.RelationshipProximity, "source file is visible")
		}
		if tier := expansions.fileTier(fact.FilePath); tier > 0 {
			score.Tier = tier
			score.add("context.expansion", cfg.Weights.Selected, "selected context expansion tier "+strconv.Itoa(tier))
			score.Forced = true
		}
		if hintWeight := factVisibilityHint(fact, "score"); hintWeight != 0 {
			score.add("fact.hint", hintWeight, "enricher visibility hint")
		}
		decision := "hidden"
		if (highSignalFact(fact) || dependencyFact(fact)) && (score.Forced || !cfg.CoreThresholdEnabled || score.Score >= cfg.CoreThreshold) {
			decision = "visible"
			visible = append(visible, fact)
		}
		scoreValue := score.Score
		if err := store.SaveFilterDecision(ctx, runID, "fact", fact.ID, factOwnerKey(fact), decision, score.reason("fact below visibility threshold"), &scoreValue, score.Tier, score.signalsJSON()); err != nil {
			return nil, err
		}
	}
	return visible, nil
}

func factVisibilityHint(fact Fact, key string) float64 {
	if strings.TrimSpace(fact.VisibilityHintsJSON) == "" {
		return 0
	}
	values := map[string]float64{}
	if err := json.Unmarshal([]byte(fact.VisibilityHintsJSON), &values); err != nil {
		return 0
	}
	return values[key]
}

func highSignalFact(fact Fact) bool {
	if factVisibilityHint(fact, "high_signal") > 0 {
		return true
	}
	switch fact.Type {
	case "http.route", "frontend.route", "orm.query":
		return true
	default:
		return false
	}
}

func dependencyFact(fact Fact) bool {
	return strings.HasPrefix(fact.Type, "dependency.")
}

func factOwnerKey(fact Fact) string {
	return "fact:" + fact.Enricher + ":" + fact.StableKey
}

func rescueRelatedSymbolsScored(symbols []Symbol, refs []Reference, visible map[int64]Symbol, scores map[int64]*visibilityScore, embeddings map[int64]Vector, weight float64) {
	byID := map[int64]Symbol{}
	for _, sym := range symbols {
		byID[sym.ID] = sym
	}
	for _, ref := range refs {
		sourceVisible := visible[ref.SourceSymbolID]
		targetVisible := visible[ref.TargetSymbolID]
		switch {
		case sourceVisible.ID != 0 && targetVisible.ID == 0:
			if target, ok := byID[ref.TargetSymbolID]; ok && embeddingSimilar(sourceVisible.ID, target.ID, embeddings, 0.82) {
				visible[target.ID] = target
				scores[target.ID].add("embedding.neighbor", weight, "embedding-similar graph neighbor")
			}
		case targetVisible.ID != 0 && sourceVisible.ID == 0:
			if source, ok := byID[ref.SourceSymbolID]; ok && embeddingSimilar(targetVisible.ID, source.ID, embeddings, 0.82) {
				visible[source.ID] = source
				scores[source.ID].add("embedding.neighbor", weight, "embedding-similar graph neighbor")
			}
		}
	}
}

func embeddingSimilar(leftID, rightID int64, embeddings map[int64]Vector, threshold float64) bool {
	left, leftOK := embeddings[leftID]
	right, rightOK := embeddings[rightID]
	if !leftOK || !rightOK {
		return false
	}
	return CosineSimilarity(left, right) >= threshold
}

func isExportedSymbol(sym Symbol) bool {
	if sym.Name == "" {
		return false
	}
	first := []rune(sym.Name)[0]
	return unicode.IsUpper(first)
}

func looksLikeTinyUtility(sym Symbol) bool {
	name := strings.ToLower(sym.Name)
	file := strings.ToLower(path.Base(sym.FilePath))
	for _, marker := range []string{"log", "logger", "metric", "trace", "debug", "helper", "util"} {
		if strings.Contains(name, marker) || strings.Contains(file, marker) {
			return true
		}
	}
	return false
}

func stableClusterKey(repositoryID int64, parentScope, settingsHash string, memberKeys []string) string {
	keys := append([]string(nil), memberKeys...)
	sort.Strings(keys)
	return "cluster:" + strconv.FormatInt(repositoryID, 10) + ":" + parentScope + ":" + settingsHash + ":" + stableHash(keys)
}

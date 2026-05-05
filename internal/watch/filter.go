package watch

import (
	"context"
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
	Incoming          map[int64]int
	Outgoing          map[int64]int
	ChangedFiles      map[string]struct{}
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
	return stableHash(req)
}

func runFilter(ctx context.Context, store *Store, repositoryID int64, thresholds Thresholds, rawGraphHash, settingsHash string, embeddings map[int64]Vector, forcedVisibleSymbols map[int64]string) (filterResult, error) {
	symbols, err := store.SymbolsForRepository(ctx, repositoryID)
	if err != nil {
		return filterResult{}, err
	}
	refs, err := store.QueryReferences(ctx, repositoryID, ReferenceQuery{Limit: -1})
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
	reasons := map[int64]string{}
	for _, sym := range symbols {
		switch {
		case isExportedSymbol(sym):
			visible[sym.ID] = sym
			reasons[sym.ID] = "exported Go symbol"
		case outgoing[sym.ID] > 0:
			visible[sym.ID] = sym
			reasons[sym.ID] = "has resolved outgoing reference"
		}
	}
	for _, sym := range symbols {
		if reason, ok := forcedVisibleSymbols[sym.ID]; ok {
			visible[sym.ID] = sym
			if strings.TrimSpace(reason) == "" {
				reason = "changed since latest watch version"
			}
			reasons[sym.ID] = reason
		}
	}
	forcedContextSymbols := map[int64]struct{}{}
	for id := range forcedVisibleSymbols {
		forcedContextSymbols[id] = struct{}{}
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
			if target, ok := symbolByID(symbols, ref.TargetSymbolID); ok {
				visible[target.ID] = target
				reasons[target.ID] = "incoming reference from visible symbol"
				changed = true
			}
		}
	}
	forceChangedSymbolEndpoints(symbols, refs, forcedVisibleSymbols, visible, reasons, forcedContextSymbols)

	for _, sym := range symbols {
		if isExportedSymbol(sym) {
			continue
		}
		if _, forced := forcedContextSymbols[sym.ID]; forced {
			continue
		}
		if outgoing[sym.ID] > thresholds.MaxOutgoingPerElement || incoming[sym.ID] > thresholds.MaxIncomingPerElement {
			delete(visible, sym.ID)
			reasons[sym.ID] = "high-degree non-entrypoint collapsed"
			continue
		}
		if looksLikeTinyUtility(sym) && outgoing[sym.ID]+incoming[sym.ID] > 8 {
			delete(visible, sym.ID)
			reasons[sym.ID] = "utility noise collapsed"
		}
	}
	if len(embeddings) > 0 {
		rescueRelatedSymbols(symbols, refs, visible, reasons, embeddings)
	}

	runID, err := store.BeginFilterRun(ctx, repositoryID, settingsHash, rawGraphHash)
	if err != nil {
		return filterResult{}, err
	}
	visibleSymbols := 0
	hiddenSymbols := 0
	for _, sym := range symbols {
		if _, ok := visible[sym.ID]; ok {
			visibleSymbols++
			reason := reasons[sym.ID]
			if reason == "" {
				reason = "visible by graph context"
			}
			if err := store.SaveFilterDecision(ctx, runID, "symbol", sym.ID, "visible", reason, nil); err != nil {
				return filterResult{}, err
			}
			continue
		}
		hiddenSymbols++
		reason := reasons[sym.ID]
		if reason == "" {
			reason = "leaf private symbol without useful outgoing references"
		}
		if err := store.SaveFilterDecision(ctx, runID, "symbol", sym.ID, "hidden", reason, nil); err != nil {
			return filterResult{}, err
		}
	}

	var visibleRefs []Reference
	hiddenRefs := 0
	for _, ref := range refs {
		_, sourceOK := visible[ref.SourceSymbolID]
		_, targetOK := visible[ref.TargetSymbolID]
		if sourceOK && targetOK {
			visibleRefs = append(visibleRefs, ref)
			if err := store.SaveFilterDecision(ctx, runID, "reference", ref.ID, "visible", "connects visible symbols", nil); err != nil {
				return filterResult{}, err
			}
		} else {
			hiddenRefs++
			if err := store.SaveFilterDecision(ctx, runID, "reference", ref.ID, "hidden", "unresolved or hidden endpoint", nil); err != nil {
				return filterResult{}, err
			}
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
		Incoming:          incoming,
		Outgoing:          outgoing,
	}, nil
}

func rescueRelatedSymbols(symbols []Symbol, refs []Reference, visible map[int64]Symbol, reasons map[int64]string, embeddings map[int64]Vector) {
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
				reasons[target.ID] = "embedding-similar graph neighbor"
			}
		case targetVisible.ID != 0 && sourceVisible.ID == 0:
			if source, ok := byID[ref.SourceSymbolID]; ok && embeddingSimilar(targetVisible.ID, source.ID, embeddings, 0.82) {
				visible[source.ID] = source
				reasons[source.ID] = "embedding-similar graph neighbor"
			}
		}
	}
}

func forceChangedSymbolEndpoints(symbols []Symbol, refs []Reference, changedSymbols map[int64]string, visible map[int64]Symbol, reasons map[int64]string, forced map[int64]struct{}) {
	if len(changedSymbols) == 0 {
		return
	}
	byID := map[int64]Symbol{}
	for _, sym := range symbols {
		byID[sym.ID] = sym
	}
	for _, ref := range refs {
		if _, ok := changedSymbols[ref.SourceSymbolID]; ok {
			if target, ok := byID[ref.TargetSymbolID]; ok {
				visible[target.ID] = target
				forced[target.ID] = struct{}{}
				reasons[target.ID] = "endpoint of changed symbol"
			}
			continue
		}
		if _, ok := changedSymbols[ref.TargetSymbolID]; ok {
			if source, ok := byID[ref.SourceSymbolID]; ok {
				visible[source.ID] = source
				forced[source.ID] = struct{}{}
				reasons[source.ID] = "endpoint of changed symbol"
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

func symbolByID(symbols []Symbol, id int64) (Symbol, bool) {
	for _, sym := range symbols {
		if sym.ID == id {
			return sym, true
		}
	}
	return Symbol{}, false
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

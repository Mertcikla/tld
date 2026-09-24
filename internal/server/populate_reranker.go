package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mertcikla/tld/v2/internal/watch"
)

var populateRerankerEndpoint = ""
var populateRerankerModel = "jina-reranker-v3"
var populateRerankerHTTPClient = &http.Client{Timeout: 8 * time.Second}
var populateRerankerObservedMetrics = newPopulateRerankerMetrics()
var populateGroupMarkerTagPattern = regexp.MustCompile(`^group:[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

const (
	populateRerankerMaxDocumentRunes = 6000
	populateRerankerFileSnippetLines = 40
	populateRerankerTopFileSymbols   = 8
)

type populateRerankRequest struct {
	Model     string   `json:"model"`
	Query     string   `json:"query"`
	Documents []string `json:"documents"`
	TopN      int      `json:"top_n"`
}

type populateRerankResponse struct {
	Results []populateRerankResult `json:"results"`
	Data    []populateRerankResult `json:"data"`
}

type populateRerankResult struct {
	Index          int     `json:"index"`
	RelevanceScore float64 `json:"relevance_score"`
	Score          float64 `json:"score"`
}

type populateRerankerMetrics struct {
	mu                       sync.Mutex
	requestAttemptsTotal     uint64
	requestSuccessTotal      uint64
	requestFailureTotal      uint64
	appliedTotal             uint64
	fallbacksTotal           uint64
	fallbacksByReason        map[string]uint64
	lastRequestDocumentCount int
	lastResultCount          int
	requestLatencyCount      uint64
	requestLatencyTotal      time.Duration
	requestLatencyMin        time.Duration
	requestLatencyMax        time.Duration
	requestLatencyLast       time.Duration
	requestLatencyLE100      uint64
	requestLatencyLE250      uint64
	requestLatencyLE500      uint64
	requestLatencyLE1000     uint64
	requestLatencyGT1000     uint64
}

type populateRerankerMetricsSnapshot struct {
	RequestAttemptsTotal     uint64                                 `json:"request_attempts_total"`
	RequestSuccessTotal      uint64                                 `json:"request_success_total"`
	RequestFailureTotal      uint64                                 `json:"request_failure_total"`
	AppliedTotal             uint64                                 `json:"applied_total"`
	FallbacksTotal           uint64                                 `json:"fallbacks_total"`
	FallbacksByReason        map[string]uint64                      `json:"fallbacks_by_reason"`
	LastRequestDocumentCount int                                    `json:"last_request_document_count"`
	LastResultCount          int                                    `json:"last_result_count"`
	RequestLatencyMS         populateRerankerLatencyMetricsSnapshot `json:"request_latency_ms"`
}

type populateRerankerLatencyMetricsSnapshot struct {
	Count  uint64  `json:"count"`
	Avg    float64 `json:"avg"`
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
	Last   float64 `json:"last"`
	LE100  uint64  `json:"le_100"`
	LE250  uint64  `json:"le_250"`
	LE500  uint64  `json:"le_500"`
	LE1000 uint64  `json:"le_1000"`
	GT1000 uint64  `json:"gt_1000"`
}

func configurePopulateReranker(endpoint string) {
	if endpoint = strings.TrimSpace(endpoint); endpoint != "" {
		populateRerankerEndpoint = endpoint
	}
}

func newPopulateRerankerMetrics() *populateRerankerMetrics {
	return &populateRerankerMetrics{fallbacksByReason: map[string]uint64{}}
}

func snapshotPopulateRerankerMetrics() populateRerankerMetricsSnapshot {
	return populateRerankerObservedMetrics.snapshot()
}

func resetPopulateRerankerMetrics() {
	populateRerankerObservedMetrics.reset()
}

func (m *populateRerankerMetrics) reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requestAttemptsTotal = 0
	m.requestSuccessTotal = 0
	m.requestFailureTotal = 0
	m.appliedTotal = 0
	m.fallbacksTotal = 0
	m.fallbacksByReason = map[string]uint64{}
	m.lastRequestDocumentCount = 0
	m.lastResultCount = 0
	m.requestLatencyCount = 0
	m.requestLatencyTotal = 0
	m.requestLatencyMin = 0
	m.requestLatencyMax = 0
	m.requestLatencyLast = 0
	m.requestLatencyLE100 = 0
	m.requestLatencyLE250 = 0
	m.requestLatencyLE500 = 0
	m.requestLatencyLE1000 = 0
	m.requestLatencyGT1000 = 0
}

func (m *populateRerankerMetrics) recordRequest(duration time.Duration, documentCount, resultCount int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requestAttemptsTotal++
	m.lastRequestDocumentCount = documentCount
	m.lastResultCount = resultCount
	m.requestLatencyCount++
	m.requestLatencyTotal += duration
	m.requestLatencyLast = duration
	if m.requestLatencyCount == 1 || duration < m.requestLatencyMin {
		m.requestLatencyMin = duration
	}
	if duration > m.requestLatencyMax {
		m.requestLatencyMax = duration
	}
	switch {
	case duration <= 100*time.Millisecond:
		m.requestLatencyLE100++
	case duration <= 250*time.Millisecond:
		m.requestLatencyLE250++
	case duration <= 500*time.Millisecond:
		m.requestLatencyLE500++
	case duration <= time.Second:
		m.requestLatencyLE1000++
	default:
		m.requestLatencyGT1000++
	}
	if err != nil {
		m.requestFailureTotal++
		return
	}
	m.requestSuccessTotal++
}

func (m *populateRerankerMetrics) recordApplied() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.appliedTotal++
}

func (m *populateRerankerMetrics) recordFallback(reason string) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.fallbacksTotal++
	m.fallbacksByReason[reason]++
}

func (m *populateRerankerMetrics) snapshot() populateRerankerMetricsSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	fallbacksByReason := make(map[string]uint64, len(m.fallbacksByReason))
	for reason, count := range m.fallbacksByReason {
		fallbacksByReason[reason] = count
	}
	latency := populateRerankerLatencyMetricsSnapshot{
		Count:  m.requestLatencyCount,
		Min:    durationToMilliseconds(m.requestLatencyMin),
		Max:    durationToMilliseconds(m.requestLatencyMax),
		Last:   durationToMilliseconds(m.requestLatencyLast),
		LE100:  m.requestLatencyLE100,
		LE250:  m.requestLatencyLE250,
		LE500:  m.requestLatencyLE500,
		LE1000: m.requestLatencyLE1000,
		GT1000: m.requestLatencyGT1000,
	}
	if m.requestLatencyCount > 0 {
		latency.Avg = durationToMilliseconds(time.Duration(int64(m.requestLatencyTotal) / int64(m.requestLatencyCount)))
	}
	return populateRerankerMetricsSnapshot{
		RequestAttemptsTotal:     m.requestAttemptsTotal,
		RequestSuccessTotal:      m.requestSuccessTotal,
		RequestFailureTotal:      m.requestFailureTotal,
		AppliedTotal:             m.appliedTotal,
		FallbacksTotal:           m.fallbacksTotal,
		FallbacksByReason:        fallbacksByReason,
		LastRequestDocumentCount: m.lastRequestDocumentCount,
		LastResultCount:          m.lastResultCount,
		RequestLatencyMS:         latency,
	}
}

func durationToMilliseconds(duration time.Duration) float64 {
	return float64(duration.Microseconds()) / 1000
}

func bestEffortPopulateRerank(ctx context.Context, db *sql.DB, repoID int64, query populateQuery, candidates []populateCandidate, limit int) {
	if limit <= 0 || len(candidates) == 0 || strings.TrimSpace(populateRerankerEndpoint) == "" {
		return
	}
	shortlistSize := limit * 2
	if shortlistSize > len(candidates) {
		shortlistSize = len(candidates)
	}
	if shortlistSize == 0 {
		return
	}
	documents, err := buildPopulateRerankDocuments(ctx, db, repoID, candidates[:shortlistSize])
	if err != nil {
		populateRerankerObservedMetrics.recordFallback("document_build_error")
		return
	}
	if len(documents) == 0 {
		populateRerankerObservedMetrics.recordFallback("empty_documents")
		return
	}
	requestStarted := time.Now()
	results, err := runPopulateReranker(ctx, populateRerankerQuery(query), documents)
	populateRerankerObservedMetrics.recordRequest(time.Since(requestStarted), len(documents), len(results), err)
	if err != nil {
		populateRerankerObservedMetrics.recordFallback("request_error")
		return
	}
	if len(results) == 0 {
		populateRerankerObservedMetrics.recordFallback("empty_results")
		return
	}
	applied := false
	for _, result := range results {
		if result.Index < 0 || result.Index >= shortlistSize {
			continue
		}
		score := result.RelevanceScore
		if score == 0 {
			score = result.Score
		}
		if score < 0 {
			score = 0
		}
		cand := &candidates[result.Index]
		if cand.hasRerankerScore && cand.rerankerScore >= score {
			continue
		}
		cand.rerankerScore = score
		cand.hasRerankerScore = true
		cand.element.SimilarityScore = score
		cand.element.MatchReason = populateMatchReason(*cand)
		applied = true
	}
	if applied {
		populateRerankerObservedMetrics.recordApplied()
		sortPopulateCandidates(candidates)
		return
	}
	populateRerankerObservedMetrics.recordFallback("no_applicable_results")
}

func sortPopulateCandidates(candidates []populateCandidate) {
	sort.Slice(candidates, func(i, j int) bool {
		left := candidates[i]
		right := candidates[j]
		if left.hasRerankerScore != right.hasRerankerScore {
			return left.hasRerankerScore
		}
		if left.hasRerankerScore && right.hasRerankerScore && left.rerankerScore != right.rerankerScore {
			return left.rerankerScore > right.rerankerScore
		}
		if left.finalScore == right.finalScore {
			return left.element.ID < right.element.ID
		}
		return left.finalScore > right.finalScore
	})
}

func populateRerankerQuery(query populateQuery) string {
	return strings.TrimSpace(query.Base)
}

func runPopulateReranker(ctx context.Context, query string, documents []string) ([]populateRerankResult, error) {
	payload, err := json.Marshal(populateRerankRequest{
		Model:     populateRerankerModel,
		Query:     query,
		Documents: documents,
		TopN:      len(documents),
	})
	if err != nil {
		return nil, fmt.Errorf("marshal populate rerank request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, populateRerankerEndpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("build populate rerank request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := populateRerankerHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("populate rerank request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read populate rerank response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("populate rerank status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var decoded populateRerankResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("decode populate rerank response: %w", err)
	}
	if len(decoded.Results) > 0 {
		return decoded.Results, nil
	}
	if len(decoded.Data) > 0 {
		return decoded.Data, nil
	}
	return nil, fmt.Errorf("populate rerank response missing results")
}

func buildPopulateRerankDocuments(ctx context.Context, db *sql.DB, repoID int64, candidates []populateCandidate) ([]string, error) {
	repoRoot, _ := populateRepositoryRoot(ctx, db, repoID)
	documents := make([]string, 0, len(candidates))
	for _, cand := range candidates {
		document, err := buildPopulateRerankDocument(ctx, db, repoID, repoRoot, cand)
		if err != nil {
			return nil, err
		}
		documents = append(documents, document)
	}
	return documents, nil
}

func buildPopulateRerankDocument(ctx context.Context, db *sql.DB, repoID int64, repoRoot string, cand populateCandidate) (string, error) {
	parts := []string{}
	appendPart := func(label, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		parts = append(parts, label+" "+value)
	}
	appendPart("name", cand.element.Name)
	appendPart("kind", cand.kind())
	appendPart("description", derefString(cand.element.Description))
	appendPart("technology", derefString(cand.element.Technology))
	appendPart("language", derefString(cand.element.Language))
	appendPart("path", derefString(cand.element.FilePath))
	if tags := populateTagSummary(cand.element.Tags); tags != "" {
		appendPart("tags", tags)
	}
	if signals := watch.SemanticSignals(cand.element.Name, cand.kind(), derefString(cand.element.FilePath), populateTagJSONWithoutGroups(cand.element.Tags)); len(signals) > 0 {
		appendPart("responsibilities", strings.Join(signals, ", "))
	}
	childSummary, err := populateChildSummary(ctx, db, cand.element.ID)
	if err != nil {
		return "", err
	}
	appendPart("child architecture", childSummary)
	refSummary, err := populateReferenceSummary(ctx, db, repoID, derefString(cand.element.FilePath))
	if err != nil {
		return "", err
	}
	appendPart("code references", refSummary)
	codeContext, err := populateCodeContext(ctx, db, repoID, repoRoot, cand)
	if err != nil {
		return "", err
	}
	appendPart("code context", codeContext)
	return clipPopulateRerankDocument(strings.Join(parts, "\n")), nil
}

func populateRepositoryRoot(ctx context.Context, db *sql.DB, repoID int64) (string, error) {
	var repoRoot string
	err := db.QueryRowContext(ctx, `SELECT repo_root FROM watch_repositories WHERE id = ?`, repoID).Scan(&repoRoot)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(repoRoot), nil
}

func populateChildSummary(ctx context.Context, db *sql.DB, elementID int64) (string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT child.name, COALESCE(child.kind, ''), COALESCE(child.file_path, '')
		FROM views v
		JOIN placements p ON p.view_id = v.id
		JOIN elements child ON child.id = p.element_id
		WHERE v.owner_element_id = ?
		ORDER BY p.id
		LIMIT 16`, elementID)
	if err != nil {
		return "", err
	}
	defer func() { _ = rows.Close() }()
	children := []string{}
	for rows.Next() {
		var name, kind, filePath string
		if err := rows.Scan(&name, &kind, &filePath); err != nil {
			return "", err
		}
		child := strings.TrimSpace(strings.Join(compactPopulateStrings(name, kind, filePath), " "))
		if child != "" {
			children = append(children, child)
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return strings.Join(children, ", "), nil
}

func populateReferenceSummary(ctx context.Context, db *sql.DB, repoID int64, filePath string) (string, error) {
	filePath = strings.TrimSpace(filePath)
	if filePath == "" {
		return "", nil
	}
	rows, err := db.QueryContext(ctx, `
		SELECT DISTINCT target.name
		FROM watch_symbols source
		JOIN watch_references ref ON ref.source_symbol_id = source.id
		JOIN watch_symbols target ON target.id = ref.target_symbol_id
		WHERE source.repository_id = ? AND source.file_id IN (SELECT id FROM watch_files WHERE repository_id = ? AND path = ?)
		ORDER BY target.name
		LIMIT 12`, repoID, repoID, filePath)
	if err != nil {
		return "", err
	}
	defer func() { _ = rows.Close() }()
	refs := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return "", err
		}
		name = strings.TrimSpace(name)
		if name != "" {
			refs = append(refs, name)
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return strings.Join(refs, ", "), nil
}

func populateCodeContext(ctx context.Context, db *sql.DB, repoID int64, repoRoot string, cand populateCandidate) (string, error) {
	if cand.ownerType == "symbol" {
		return populateSymbolContext(ctx, db, repoID, repoRoot, cand.ownerKey)
	}
	if cand.ownerType == "file" || cand.kind() == "file" {
		return populateFileContext(ctx, db, repoID, repoRoot, derefString(cand.element.FilePath), derefString(cand.element.Language))
	}
	return "", nil
}

func populateSymbolContext(ctx context.Context, db *sql.DB, repoID int64, repoRoot, stableKey string) (string, error) {
	var name, qualifiedName, kind, filePath string
	var startLine int
	var endLine sql.NullInt64
	err := db.QueryRowContext(ctx, `
		SELECT s.name, s.qualified_name, COALESCE(s.kind, ''), f.path, s.start_line, s.end_line
		FROM watch_symbols s
		JOIN watch_files f ON f.id = s.file_id
		WHERE s.repository_id = ? AND s.stable_key = ?`, repoID, stableKey).Scan(&name, &qualifiedName, &kind, &filePath, &startLine, &endLine)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", err
	}
	parts := []string{}
	if strings.TrimSpace(qualifiedName) != "" {
		parts = append(parts, "symbol "+qualifiedName)
	} else if strings.TrimSpace(name) != "" {
		parts = append(parts, "symbol "+name)
	}
	if kind != "" {
		parts = append(parts, "kind "+kind)
	}
	if filePath != "" {
		parts = append(parts, "path "+filePath)
	}
	end := startLine + populateRerankerFileSnippetLines - 1
	if endLine.Valid && int(endLine.Int64) > 0 {
		end = int(endLine.Int64)
	}
	if snippet := readPopulateCodeExcerpt(repoRoot, filePath, startLine, end, populateRerankerFileSnippetLines); snippet != "" {
		parts = append(parts, "snippet\n"+snippet)
	}
	return strings.Join(parts, "\n"), nil
}

func populateFileContext(ctx context.Context, db *sql.DB, repoID int64, repoRoot, filePath, language string) (string, error) {
	filePath = strings.TrimSpace(filePath)
	if filePath == "" {
		return "", nil
	}
	parts := []string{"file path " + filePath}
	if strings.TrimSpace(language) != "" {
		parts = append(parts, "language "+language)
	}
	rows, err := db.QueryContext(ctx, `
		SELECT name, COALESCE(kind, '')
		FROM watch_symbols s
		JOIN watch_files f ON f.id = s.file_id
		WHERE s.repository_id = ? AND f.repository_id = ? AND f.path = ?
		ORDER BY s.start_line
		LIMIT ?`, repoID, repoID, filePath, populateRerankerTopFileSymbols)
	if err != nil {
		return "", err
	}
	defer func() { _ = rows.Close() }()
	symbols := []string{}
	for rows.Next() {
		var name, kind string
		if err := rows.Scan(&name, &kind); err != nil {
			return "", err
		}
		symbol := strings.TrimSpace(strings.Join(compactPopulateStrings(name, kind), " "))
		if symbol != "" {
			symbols = append(symbols, symbol)
		}
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if len(symbols) > 0 {
		parts = append(parts, "symbols "+strings.Join(symbols, ", "))
	}
	if snippet := readPopulateCodeExcerpt(repoRoot, filePath, 1, populateRerankerFileSnippetLines, populateRerankerFileSnippetLines); snippet != "" {
		parts = append(parts, "snippet\n"+snippet)
	}
	return strings.Join(parts, "\n"), nil
}

func readPopulateCodeExcerpt(repoRoot, filePath string, startLine, endLine, maxLines int) string {
	repoRoot = strings.TrimSpace(repoRoot)
	filePath = strings.TrimSpace(filePath)
	if repoRoot == "" || filePath == "" || maxLines <= 0 {
		return ""
	}
	cleanRel := filepath.Clean(filepath.FromSlash(filePath))
	if filepath.IsAbs(cleanRel) || cleanRel == "." || cleanRel == ".." || strings.HasPrefix(cleanRel, ".."+string(filepath.Separator)) {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(repoRoot, cleanRel))
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	if len(lines) == 0 {
		return ""
	}
	if startLine <= 0 {
		startLine = 1
	}
	if endLine < startLine {
		endLine = startLine + maxLines - 1
	}
	if endLine > startLine+maxLines-1 {
		endLine = startLine + maxLines - 1
	}
	if startLine > len(lines) {
		return ""
	}
	if endLine > len(lines) {
		endLine = len(lines)
	}
	return strings.TrimSpace(strings.Join(lines[startLine-1:endLine], "\n"))
}

func populateTagSummary(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var tags []string
	if err := json.Unmarshal(raw, &tags); err == nil {
		visible := tags[:0]
		for _, tag := range tags {
			if !isPopulateGroupMarkerTag(tag) {
				visible = append(visible, tag)
			}
		}
		return strings.Join(compactPopulateStrings(visible...), ", ")
	}
	return strings.TrimSpace(string(raw))
}

func populateTagJSONWithoutGroups(raw json.RawMessage) string {
	var tags []string
	if err := json.Unmarshal(raw, &tags); err != nil {
		return string(raw)
	}
	visible := tags[:0]
	for _, tag := range tags {
		if !isPopulateGroupMarkerTag(tag) {
			visible = append(visible, tag)
		}
	}
	encoded, err := json.Marshal(visible)
	if err != nil {
		return "[]"
	}
	return string(encoded)
}

func isPopulateGroupMarkerTag(tag string) bool {
	return populateGroupMarkerTagPattern.MatchString(tag)
}

func clipPopulateRerankDocument(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return text
	}
	runes := []rune(text)
	if len(runes) <= populateRerankerMaxDocumentRunes {
		return text
	}
	return strings.TrimSpace(string(runes[:populateRerankerMaxDocumentRunes]))
}

func compactPopulateStrings(values ...string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

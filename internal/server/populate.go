package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/mertcikla/tld/v2/internal/store"
)

type populateElementResult struct {
	ID                   int64           `json:"id"`
	Name                 string          `json:"name"`
	Kind                 *string         `json:"kind"`
	Description          *string         `json:"description"`
	Technology           *string         `json:"technology"`
	URL                  *string         `json:"url"`
	LogoURL              *string         `json:"logo_url"`
	TechnologyConnectors json.RawMessage `json:"technology_connectors"`
	Tags                 json.RawMessage `json:"tags"`
	Repo                 *string         `json:"repo,omitempty"`
	Branch               *string         `json:"branch,omitempty"`
	FilePath             *string         `json:"file_path,omitempty"`
	Language             *string         `json:"language,omitempty"`
	CreatedAt            string          `json:"created_at"`
	UpdatedAt            string          `json:"updated_at"`
	SimilarityScore      float64         `json:"similarity_score"`
	MatchKind            string          `json:"match_kind,omitempty"`
	MatchReason          string          `json:"match_reason,omitempty"`
}

// registerPopulateHandlers serves the "populate view" search. Candidates are
// existing workspace elements matched lexically against the query and not yet
// placed in the view. The previous watch-graph embedding/reranker pipeline was
// removed with the legacy engine; similarity search over codeindex embeddings
// can be layered back on through CodeIndexStore.SimilarFacts.
func registerPopulateHandlers(mux *http.ServeMux, sqliteStore *store.SQLiteStore) {
	mux.HandleFunc("GET /api/debug/populate-reranker-metrics", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, map[string]any{"enabled": false, "metrics": map[string]any{}})
	})

	mux.HandleFunc("GET /api/views/{id}/populate-query", func(w http.ResponseWriter, r *http.Request) {
		viewID, ok := parseViewID(w, r)
		if !ok {
			return
		}
		query, err := buildPopulateQuery(r.Context(), sqliteStore.DB(), viewID, "")
		if err != nil {
			if err == sql.ErrNoRows {
				writeJSONError(w, http.StatusNotFound, "view not found")
			} else {
				writeJSONError(w, http.StatusBadRequest, err.Error())
			}
			return
		}
		writeJSON(w, map[string]string{"query": query, "enriched_query": query})
	})

	mux.HandleFunc("GET /api/views/{id}/populate", func(w http.ResponseWriter, r *http.Request) {
		viewID, ok := parseViewID(w, r)
		if !ok {
			return
		}
		query := strings.TrimSpace(r.URL.Query().Get("q"))
		if query == "" {
			writeJSON(w, map[string]any{"results": []any{}})
			return
		}
		limit := 5
		if parsed, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && parsed > 0 {
			limit = parsed
		}
		if limit > 50 {
			limit = 50
		}
		results, err := searchPopulateElements(r.Context(), sqliteStore.DB(), viewID, query, limit)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to search elements: "+err.Error())
			return
		}
		writeJSON(w, map[string]any{"results": results})
	})
}

func buildPopulateQuery(ctx context.Context, db *sql.DB, viewID int64, userQuery string) (string, error) {
	var viewName string
	if err := db.QueryRowContext(ctx, `SELECT name FROM views WHERE id = ?`, viewID).Scan(&viewName); err != nil {
		return "", err
	}
	if base := strings.TrimSpace(userQuery); base != "" {
		return base, nil
	}
	return strings.TrimSpace(viewName), nil
}

func searchPopulateElements(ctx context.Context, db *sql.DB, viewID int64, query string, limit int) ([]populateElementResult, error) {
	tokens := populateTokens(query)
	rows, err := db.QueryContext(ctx, `
		SELECT el.id, el.name, el.kind, el.description, el.technology, el.url, el.logo_url,
		       el.technology_connectors, el.tags, el.repo, el.branch, el.file_path, el.language,
		       el.created_at, el.updated_at
		FROM elements el
		WHERE el.id NOT IN (SELECT element_id FROM placements WHERE view_id = ?)`,
		viewID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	scored := make([]populateElementResult, 0)
	for rows.Next() {
		var (
			el       populateElementResult
			techJSON string
			tagsJSON string
		)
		if err := rows.Scan(&el.ID, &el.Name, &el.Kind, &el.Description, &el.Technology, &el.URL, &el.LogoURL,
			&techJSON, &tagsJSON, &el.Repo, &el.Branch, &el.FilePath, &el.Language, &el.CreatedAt, &el.UpdatedAt); err != nil {
			return nil, err
		}
		el.TechnologyConnectors = rawJSON(techJSON, "[]")
		el.Tags = rawJSON(tagsJSON, "[]")
		score := populateLexicalScore(tokens, el)
		if score <= 0 {
			continue
		}
		el.SimilarityScore = score
		el.MatchReason = "lexical"
		if el.Kind != nil {
			el.MatchKind = *el.Kind
		}
		scored = append(scored, el)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].SimilarityScore == scored[j].SimilarityScore {
			return scored[i].Name < scored[j].Name
		}
		return scored[i].SimilarityScore > scored[j].SimilarityScore
	})
	if len(scored) > limit {
		scored = scored[:limit]
	}
	return scored, nil
}

func populateTokens(query string) []string {
	fields := strings.FieldsFunc(strings.ToLower(query), func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	})
	seen := map[string]bool{}
	out := make([]string, 0, len(fields))
	for _, token := range fields {
		if len(token) < 2 || seen[token] {
			continue
		}
		seen[token] = true
		out = append(out, token)
	}
	return out
}

func populateLexicalScore(tokens []string, el populateElementResult) float64 {
	if len(tokens) == 0 {
		return 0
	}
	haystack := strings.ToLower(strings.Join([]string{
		el.Name,
		derefString(el.Kind),
		derefString(el.FilePath),
		derefString(el.Language),
		derefString(el.Repo),
	}, " "))
	matched := 0
	for _, token := range tokens {
		if strings.Contains(haystack, token) {
			matched++
		}
	}
	if matched == 0 {
		return 0
	}
	return float64(matched) / float64(len(tokens))
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func rawJSON(value, fallback string) json.RawMessage {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || !json.Valid([]byte(trimmed)) {
		return json.RawMessage(fallback)
	}
	return json.RawMessage(trimmed)
}

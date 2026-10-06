package server

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/mertcikla/tld/v2/internal/mermaid"
)

// registerImpactMermaidHandler exposes the reusable repository impact to
// Mermaid exporter over the plain HTTP mux. The corresponding Mermaid service
// RPC would require a proto change, so the endpoint reuses the persisted
// impact diagram for a repository/comparison key instead.
func registerImpactMermaidHandler(mux *http.ServeMux, svc *mapperService) {
	mux.HandleFunc("GET /api/repositories/{id}/impact/mermaid", svc.impactMermaid)
}

func (s *mapperService) impactMermaid(w http.ResponseWriter, r *http.Request) {
	repositoryID := strings.TrimSpace(r.PathValue("id"))
	comparisonKey := strings.TrimSpace(r.URL.Query().Get("comparisonKey"))
	if repositoryID == "" || comparisonKey == "" {
		writeJSONError(w, http.StatusBadRequest, "repository id and comparisonKey are required")
		return
	}
	diagram, err := s.idx.Impact(r.Context(), repositoryID, comparisonKey)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSONError(w, http.StatusNotFound, "impact diagram not found")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "could not load impact diagram")
		return
	}
	code := mermaid.ExportImpactDiagram(diagram, mermaid.ImpactExportOptions{IncludeMetadata: true})
	markdown := ""
	if queryBool(r, "markdown") {
		markdown = mermaid.MermaidBlock(code)
	}
	writeJSON(w, map[string]any{
		"code":     code,
		"markdown": markdown,
		"warnings": []string{},
	})
}

func queryBool(r *http.Request, key string) bool {
	switch strings.ToLower(strings.TrimSpace(r.URL.Query().Get(key))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

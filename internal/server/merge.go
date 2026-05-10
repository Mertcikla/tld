package server

import (
	"encoding/json"
	"net/http"

	"github.com/mertcikla/tld/internal/app"
	"github.com/mertcikla/tld/internal/store"
)

type mergeElementsRequest struct {
	SourceID   int64             `json:"source_id"`
	SurvivorID int64             `json:"survivor_id"`
	Resolved   app.MergeResolved `json:"resolved"`
}

func registerMergeHandlers(mux *http.ServeMux, sqliteStore *store.SQLiteStore) {
	mux.HandleFunc("POST /api/elements/merge", func(w http.ResponseWriter, r *http.Request) {
		var req mergeElementsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if req.SourceID <= 0 || req.SurvivorID <= 0 {
			writeJSONError(w, http.StatusBadRequest, "source_id and survivor_id are required")
			return
		}
		result, err := sqliteStore.Legacy().MergeElements(r.Context(), req.SourceID, req.SurvivorID, req.Resolved)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, map[string]any{
			"survivor":   result.Survivor,
			"deleted_id": result.DeletedID,
		})
	})
}

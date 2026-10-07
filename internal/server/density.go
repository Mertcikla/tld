package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/mertcikla/tld/v2/pkg/app"
)

type densityRequest struct {
	DensityLevel int `json:"density_level"`
}

type visibilityOverrideRequest struct {
	ResourceType string `json:"resource_type"`
	ResourceID   int64  `json:"resource_id"`
	LevelDelta   int    `json:"level_delta"`
}

type noiseGateInitializeRequest struct {
	DensityLevel *int `json:"density_level"`
}

func registerDensityHandlers(mux *http.ServeMux, appStore *app.Store) {
	mux.HandleFunc("GET /api/views/{id}/density", func(w http.ResponseWriter, r *http.Request) {
		viewID, ok := parseViewID(w, r)
		if !ok {
			return
		}
		level, err := appStore.ViewDensityLevel(r.Context(), viewID)
		if err != nil {
			writeDensityError(w, err)
			return
		}
		writeJSON(w, map[string]any{"view_id": viewID, "density_level": level})
	})

	mux.HandleFunc("PUT /api/views/{id}/density", func(w http.ResponseWriter, r *http.Request) {
		viewID, ok := parseViewID(w, r)
		if !ok {
			return
		}
		var req densityRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		if err := appStore.SetViewDensityLevel(r.Context(), viewID, req.DensityLevel); err != nil {
			writeDensityError(w, err)
			return
		}
		writeJSON(w, map[string]any{"view_id": viewID, "density_level": req.DensityLevel})
	})

	mux.HandleFunc("POST /api/views/{id}/noise-gate/initialize", func(w http.ResponseWriter, r *http.Request) {
		viewID, ok := parseViewID(w, r)
		if !ok {
			return
		}
		var req noiseGateInitializeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			writeJSONError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		result, err := appStore.InitializeViewNoiseGate(r.Context(), viewID, req.DensityLevel)
		if err != nil {
			writeDensityError(w, err)
			return
		}
		writeJSON(w, result)
	})

	mux.HandleFunc("GET /api/views/{id}/visibility-overrides", func(w http.ResponseWriter, r *http.Request) {
		viewID, ok := parseViewID(w, r)
		if !ok {
			return
		}
		overrides, err := appStore.VisibilityOverrides(r.Context(), viewID)
		if err != nil {
			writeDensityError(w, err)
			return
		}
		writeJSON(w, map[string]any{"overrides": overrides})
	})

	mux.HandleFunc("PUT /api/views/{id}/visibility-overrides", func(w http.ResponseWriter, r *http.Request) {
		viewID, ok := parseViewID(w, r)
		if !ok {
			return
		}
		var req visibilityOverrideRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
		override, err := appStore.SetVisibilityOverride(r.Context(), viewID, req.ResourceType, req.ResourceID, req.LevelDelta)
		if err != nil {
			writeDensityError(w, err)
			return
		}
		writeJSON(w, map[string]any{"override": override})
	})

	mux.HandleFunc("POST /api/views/{id}/visibility-overrides/{resource_type}/{resource_id}/promote", func(w http.ResponseWriter, r *http.Request) {
		adjustVisibilityOverride(w, r, appStore, 1)
	})
	mux.HandleFunc("POST /api/views/{id}/visibility-overrides/{resource_type}/{resource_id}/demote", func(w http.ResponseWriter, r *http.Request) {
		adjustVisibilityOverride(w, r, appStore, -1)
	})
	mux.HandleFunc("DELETE /api/views/{id}/visibility-overrides/{resource_type}/{resource_id}", func(w http.ResponseWriter, r *http.Request) {
		viewID, resourceType, resourceID, ok := parseOverridePath(w, r)
		if !ok {
			return
		}
		if err := appStore.DeleteVisibilityOverride(r.Context(), viewID, resourceType, resourceID); err != nil {
			writeDensityError(w, err)
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	})
}

func adjustVisibilityOverride(w http.ResponseWriter, r *http.Request, appStore *app.Store, step int) {
	viewID, resourceType, resourceID, ok := parseOverridePath(w, r)
	if !ok {
		return
	}
	override, err := appStore.AdjustVisibilityOverride(r.Context(), viewID, resourceType, resourceID, step)
	if err != nil {
		writeDensityError(w, err)
		return
	}
	writeJSON(w, map[string]any{"override": override})
}

func parseViewID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	viewID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || viewID <= 0 {
		writeJSONError(w, http.StatusBadRequest, "invalid view id")
		return 0, false
	}
	return viewID, true
}

func parseOverridePath(w http.ResponseWriter, r *http.Request) (int64, string, int64, bool) {
	viewID, ok := parseViewID(w, r)
	if !ok {
		return 0, "", 0, false
	}
	resourceType := r.PathValue("resource_type")
	if err := app.ValidateResourceType(resourceType); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return 0, "", 0, false
	}
	resourceID, err := strconv.ParseInt(r.PathValue("resource_id"), 10, 64)
	if err != nil || resourceID <= 0 {
		writeJSONError(w, http.StatusBadRequest, "invalid resource id")
		return 0, "", 0, false
	}
	return viewID, resourceType, resourceID, true
}

func writeDensityError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		writeJSONError(w, http.StatusNotFound, "view not found")
	default:
		writeJSONError(w, http.StatusBadRequest, err.Error())
	}
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}

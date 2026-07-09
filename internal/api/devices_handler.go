package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"routeviewnet/internal/storage"
)

func parsePagination(r *http.Request) (limit int, cursor int64, err error) {
	limit = storage.ClampLimit(atoiDefault(r.URL.Query().Get("limit"), 50))
	cursor, err = storage.DecodeCursor(r.URL.Query().Get("cursor"))
	return
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}

func (s *Server) handleListDevices(w http.ResponseWriter, r *http.Request) {
	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid cursor")
		return
	}
	devices, next, err := s.DB.ListDevices(r.Context(), limit, cursor)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"items":       devices,
		"next_cursor": nullable(next),
		// Honest scope disclosure (§7.2).
		"note": "Devices visible from this Linux machine. IPv6 neighbors are not shown in this release.",
	})
}

// patchDeviceRequest is the strict PATCH body (§10.2).
type patchDeviceRequest struct {
	Nickname *string `json:"nickname"`
	Trusted  *bool   `json:"trusted"`
}

func (s *Server) handlePatchDevice(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id < 1 {
		writeError(w, http.StatusBadRequest, "invalid_request", "Invalid device id")
		return
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	dec.DisallowUnknownFields()
	var req patchDeviceRequest
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Body must be JSON with only nickname and/or trusted")
		return
	}
	if req.Nickname == nil && req.Trusted == nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "Provide nickname and/or trusted")
		return
	}
	if req.Nickname != nil && len(*req.Nickname) > 100 {
		writeError(w, http.StatusBadRequest, "invalid_request", "Nickname too long (max 100)")
		return
	}
	dev, err := s.DB.UpdateDeviceLabel(r.Context(), id, req.Nickname, req.Trusted)
	if errors.Is(err, storage.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "Device not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	s.Bus.Publish("device.updated", dev)
	writeJSON(w, http.StatusOK, dev)
}

func nullable(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

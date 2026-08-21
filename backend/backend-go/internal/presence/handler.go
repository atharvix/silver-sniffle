package presence

import (
	"encoding/json"
	"net/http"

	"github.com/atharvix/kinjo-backend/internal/domain"
	"github.com/atharvix/kinjo-backend/internal/middleware"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) UpdateLocation(w http.ResponseWriter, r *http.Request) {
	email, ok := middleware.GetUserEmail(r.Context())
	if !ok {
		respondJSON(w, http.StatusUnauthorized, map[string]string{"error": "Authorization token required."})
		return
	}

	var req domain.UpdateLocationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	resp, err := h.service.UpdateLocation(r.Context(), email, req.Latitude, req.Longitude)
	if err != nil {
		respondError(w, err)
		return
	}

	respondJSON(w, http.StatusOK, resp)
}

func (h *Handler) Heartbeat(w http.ResponseWriter, r *http.Request) {
	email, ok := middleware.GetUserEmail(r.Context())
	if !ok {
		respondJSON(w, http.StatusUnauthorized, map[string]string{"error": "Authorization token required."})
		return
	}

	resp, err := h.service.RecordHeartbeat(r.Context(), email)
	if err != nil {
		respondError(w, err)
		return
	}

	respondJSON(w, http.StatusOK, resp)
}

func (h *Handler) GoOffline(w http.ResponseWriter, r *http.Request) {
	var req domain.GoOfflineRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	resp, err := h.service.GoOffline(r.Context(), req.Token)
	if err != nil {
		respondError(w, err)
		return
	}

	respondJSON(w, http.StatusOK, resp)
}

func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func respondError(w http.ResponseWriter, err error) {
	status := domain.ErrToStatus(err)
	msg := err.Error()
	var appErr *domain.AppError
	if json.Unmarshal([]byte(msg), &appErr) == nil && appErr.Message != "" {
		msg = appErr.Message
	}

	respondJSON(w, status, map[string]string{"error": msg})
}

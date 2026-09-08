package discovery

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/atharvix/kinjo-backend/internal/domain"
	"github.com/atharvix/kinjo-backend/internal/middleware"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) GetNearbyProfiles(w http.ResponseWriter, r *http.Request) {
	email, ok := middleware.GetUserEmail(r.Context())
	if !ok {
		respondJSON(w, http.StatusUnauthorized, map[string]string{"error": "Authorization token required."})
		return
	}

	latStr := r.URL.Query().Get("lat")
	lonStr := r.URL.Query().Get("lon")
	var lat, lon *float64
	if latStr != "" && lonStr != "" {
		if l, err := strconv.ParseFloat(latStr, 64); err == nil {
			if ln, err := strconv.ParseFloat(lonStr, 64); err == nil {
				lat = &l
				lon = &ln
			}
		}
	}

	resp, err := h.service.GetNearbyProfilesWithLocation(r.Context(), email, lat, lon)
	if err != nil {
		respondError(w, err)
		return
	}

	respondJSON(w, http.StatusOK, resp)
}

func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func respondError(w http.ResponseWriter, err error) {
	status := domain.ErrToStatus(err)
	msg := err.Error()

	var appErr *domain.AppError
	if errors.As(err, &appErr) && appErr.Message != "" {
		msg = appErr.Message
	}

	respondJSON(w, status, map[string]string{"error": msg})
}

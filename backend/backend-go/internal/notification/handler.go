package notification

import (
	"context"
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

func (h *Handler) RegisterToken(w http.ResponseWriter, r *http.Request) {
	email, ok := middleware.GetUserEmail(r.Context())
	if !ok {
		respondJSON(w, http.StatusUnauthorized, map[string]string{"error": "Authorization token required."})
		return
	}

	var req domain.RegisterTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body."})
		return
	}

	if err := h.service.RegisterToken(r.Context(), email, req.Token, req.Platform); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, domain.NotificationResponse{
		Success: true,
		Message: "Device token registered successfully",
	})
}

func (h *Handler) SendCustomNotification(w http.ResponseWriter, r *http.Request) {
	var req domain.SendCustomNotificationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body."})
		return
	}

	if req.TargetEmail == "" || req.Title == "" || req.Body == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "target_email, title, and body are required."})
		return
	}

	if err := s_or_h_send(r.Context(), h.service, req); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, domain.NotificationResponse{
		Success: true,
		Message: "Custom notification queued successfully",
	})
}

func s_or_h_send(ctx context.Context, service *Service, req domain.SendCustomNotificationRequest) error {
	return service.SendCustomNotification(ctx, req)
}

func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

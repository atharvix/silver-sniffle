package notification

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/atharvix/kinjo-backend/internal/domain"
	"github.com/atharvix/kinjo-backend/internal/middleware"
)

type Handler struct {
	service     *Service
	adminEmails map[string]struct{}
}

// NewHandler builds the notification handler. adminEmails is the allowlist of
// accounts permitted to broadcast notifications; an empty list disables the
// broadcast endpoint entirely.
func NewHandler(service *Service, adminEmails []string) *Handler {
	admins := make(map[string]struct{}, len(adminEmails))
	for _, candidate := range adminEmails {
		if trimmed := strings.ToLower(strings.TrimSpace(candidate)); trimmed != "" {
			admins[trimmed] = struct{}{}
		}
	}
	return &Handler{service: service, adminEmails: admins}
}

func (h *Handler) isAdmin(email string) bool {
	if len(h.adminEmails) == 0 {
		return false
	}
	_, ok := h.adminEmails[strings.ToLower(strings.TrimSpace(email))]
	return ok
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
	// Broadcasting to the whole user base is a privileged action: without this
	// check any authenticated account could push arbitrary messages to everyone.
	caller, ok := middleware.GetUserEmail(r.Context())
	if !ok {
		respondJSON(w, http.StatusUnauthorized, map[string]string{"error": "Authorization token required."})
		return
	}
	if !h.isAdmin(caller) {
		respondJSON(w, http.StatusForbidden, map[string]string{"error": "Admin privileges are required to send notifications."})
		return
	}

	var req domain.SendCustomNotificationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body."})
		return
	}

	if req.TargetEmail == "" || req.Title == "" || req.Body == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "target_email, title, and body are required."})
		return
	}

	if err := h.service.SendCustomNotification(r.Context(), req); err != nil {
		respondJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	respondJSON(w, http.StatusOK, domain.NotificationResponse{
		Success: true,
		Message: "Custom notification queued successfully",
	})
}

func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

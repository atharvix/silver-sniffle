package connection

import (
	"encoding/json"
	"net/http"

	"github.com/atharvix/kinjo-backend/internal/domain"
	"github.com/atharvix/kinjo-backend/internal/middleware"
)

type Handler struct { service *Service }

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	email, ok := middleware.GetUserEmail(r.Context())
	if !ok { respondJSON(w, http.StatusUnauthorized, map[string]string{"error": "Authorization token required."}); return }
	var req domain.ConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil { respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"}); return }
	resp, err := h.service.Create(r.Context(), email, req)
	if err != nil { respondError(w, err); return }
	respondJSON(w, http.StatusOK, resp)
}

func (h *Handler) ListIncoming(w http.ResponseWriter, r *http.Request) {
	email, ok := middleware.GetUserEmail(r.Context())
	if !ok { respondJSON(w, http.StatusUnauthorized, map[string]string{"error": "Authorization token required."}); return }
	notifications, err := h.service.ListIncoming(r.Context(), email)
	if err != nil { respondError(w, err); return }
	respondJSON(w, http.StatusOK, map[string]any{"notifications": notifications})
}

func respondJSON(w http.ResponseWriter, status int, data any) { w.Header().Set("Content-Type", "application/json"); w.WriteHeader(status); _ = json.NewEncoder(w).Encode(data) }
func respondError(w http.ResponseWriter, err error) { respondJSON(w, domain.ErrToStatus(err), map[string]string{"error": err.Error()}) }
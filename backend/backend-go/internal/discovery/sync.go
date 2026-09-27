package discovery

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/atharvix/kinjo-backend/internal/domain"
	"github.com/atharvix/kinjo-backend/internal/middleware"
)

// PresenceUpdater records that the caller is online. Declared here (and
// satisfied by presence.Service) so discovery does not import presence.
type PresenceUpdater interface {
	UpdateLocation(ctx context.Context, email string, lat, lon float64) (*domain.UpdateLocationResponse, error)
	RecordHeartbeat(ctx context.Context, email string) (*domain.HeartbeatResponse, error)
}

// SyncHandler serves the single call the card deck needs. Polling used to be
// three requests (location, heartbeat, nearby) every 8 seconds per user; this
// is one.
type SyncHandler struct {
	service  *Service
	presence PresenceUpdater
}

func NewSyncHandler(service *Service, presence PresenceUpdater) *SyncHandler {
	return &SyncHandler{service: service, presence: presence}
}

// Sync records the caller's presence and returns nearby profiles in one round
// trip. Coordinates are optional: without them the caller's stored location is
// used and only the presence heartbeat is refreshed.
func (h *SyncHandler) Sync(w http.ResponseWriter, r *http.Request) {
	email, ok := middleware.GetUserEmail(r.Context())
	if !ok {
		respondJSON(w, http.StatusUnauthorized, map[string]string{"error": "Authorization token required."})
		return
	}

	var req domain.UpdateLocationRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&req); err != nil && err != io.EOF {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	if req.Latitude != nil && req.Longitude != nil {
		// UpdateLocation already refreshes last_seen_at, so no separate heartbeat.
		if _, err := h.presence.UpdateLocation(r.Context(), email, *req.Latitude, *req.Longitude); err != nil {
			respondError(w, err)
			return
		}
	} else if _, err := h.presence.RecordHeartbeat(r.Context(), email); err != nil {
		respondError(w, err)
		return
	}

	resp, err := h.service.GetNearbyProfilesWithLocation(r.Context(), email, req.Latitude, req.Longitude)
	if err != nil {
		respondError(w, err)
		return
	}

	respondJSON(w, http.StatusOK, resp)
}

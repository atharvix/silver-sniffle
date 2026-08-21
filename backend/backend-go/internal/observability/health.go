package observability

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

type Pinger interface {
	Ping(ctx context.Context) error
}

type HealthHandler struct {
	db Pinger
}

func NewHealthHandler(db Pinger) *HealthHandler {
	return &HealthHandler{db: db}
}

// Healthz returns 200 OK if the process is alive (Liveness check)
func (h *HealthHandler) Healthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// Readyz returns 200 OK if all critical dependencies (Postgres) are reachable (Readiness check)
func (h *HealthHandler) Readyz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if h.db != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		if err := h.db.Ping(ctx); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"status": "unavailable",
				"error":  "database ping failed: " + err.Error(),
			})
			return
		}
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
}

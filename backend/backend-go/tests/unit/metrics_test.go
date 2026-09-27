package unit

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/atharvix/kinjo-backend/internal/config"
	kinjohttp "github.com/atharvix/kinjo-backend/internal/http"
	"github.com/atharvix/kinjo-backend/internal/observability"
)

// /metrics describes the whole deployment, so it must never answer an
// unauthenticated caller.
func TestMetricsEndpointIsGated(t *testing.T) {
	t.Run("disabled without a token", func(t *testing.T) {
		router := testRouter(t, "")
		assertStatus(t, router, "", http.StatusForbidden)
		assertStatus(t, router, "Bearer anything", http.StatusForbidden)
	})

	t.Run("requires the configured token", func(t *testing.T) {
		router := testRouter(t, "s3cret")
		assertStatus(t, router, "", http.StatusUnauthorized)
		assertStatus(t, router, "Bearer wrong", http.StatusUnauthorized)
		assertStatus(t, router, "Bearer s3cret", http.StatusOK)
	})
}

func testRouter(t *testing.T, metricsToken string) http.Handler {
	t.Helper()

	cfg := &config.Config{
		Environment:    "development",
		AllowedOrigins: []string{"*"},
		// Not "local", so the router does not mount the uploads file server.
		StorageDriver: "supabase",
		MetricsToken:  metricsToken,
	}

	return kinjohttp.NewRouter(cfg, observability.NewNopLogger(), nil, kinjohttp.Handlers{}, nil)
}

func assertStatus(t *testing.T, router http.Handler, authHeader string, want int) {
	t.Helper()

	req := httptest.NewRequest("GET", "/metrics", nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != want {
		t.Errorf("GET /metrics with %q status = %d, want %d", authHeader, w.Code, want)
	}
}

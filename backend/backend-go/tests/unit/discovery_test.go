package unit

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/atharvix/kinjo-backend/internal/config"
	"github.com/atharvix/kinjo-backend/internal/discovery"
	"github.com/atharvix/kinjo-backend/internal/domain"
)

type stubDiscoveryRepo struct {
	caller *domain.Profile
	nearby []discovery.NearbyRecord
}

func (s *stubDiscoveryRepo) GetCallerProfile(context.Context, string) (*domain.Profile, error) {
	return s.caller, nil
}

func (s *stubDiscoveryRepo) FindNearbyProfiles(context.Context, string, float64, float64, float64, time.Time, int) ([]discovery.NearbyRecord, error) {
	return s.nearby, nil
}

func ptrF(f float64) *float64 { return &f }

func newDiscoveryService(repo discovery.Repository) *discovery.Service {
	return discovery.NewService(repo, &config.Config{NearbyRadiusMeters: 30}, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
}

// A legacy row whose photo is still inline base64 must never reach the deck:
// the whole image would travel inside the JSON, uncacheable, on every poll and
// a single deck measured ~160KB because of exactly this.
func TestNearbyDropsInlineBase64Photos(t *testing.T) {
	verified := time.Now()
	repo := &stubDiscoveryRepo{
		caller: &domain.Profile{
			Email:          "me@example.com",
			Latitude:       ptrF(1.0),
			Longitude:      ptrF(2.0),
			FaceVerifiedAt: &verified,
			FaceScanHash:   "scan-hash",
		},
		nearby: []discovery.NearbyRecord{
			{Email: "legacy@example.com", Name: "Legacy", PhotoURL: "data:image/jpeg;base64,AAAA"},
			{Email: "hosted@example.com", Name: "Hosted", PhotoURL: "/uploads/ok.jpg"},
		},
	}

	resp, err := newDiscoveryService(repo).GetNearbyProfilesWithLocation(context.Background(), "me@example.com", nil, nil)
	if err != nil {
		t.Fatalf("GetNearbyProfilesWithLocation() error = %v", err)
	}
	if len(resp.Profiles) != 2 {
		t.Fatalf("expected 2 cards, got %d", len(resp.Profiles))
	}
	if resp.Profiles[0].Photo != "" {
		t.Errorf("inline base64 photo leaked into the deck: %q", resp.Profiles[0].Photo)
	}
	if resp.Profiles[1].Photo != "/uploads/ok.jpg" {
		t.Errorf("hosted photo URL was dropped: %q", resp.Profiles[1].Photo)
	}
}

// Accounts verified before the liveness flow have no scan hash and must be sent
// back through the live face scan rather than keeping the weaker verification.
func TestNearbyRejectsCallerWithoutScanHash(t *testing.T) {
	verified := time.Now()
	repo := &stubDiscoveryRepo{
		caller: &domain.Profile{
			Email:          "legacy@example.com",
			Latitude:       ptrF(1.0),
			Longitude:      ptrF(2.0),
			FaceVerifiedAt: &verified, // verified, but before migration 000004
		},
	}

	_, err := newDiscoveryService(repo).GetNearbyProfilesWithLocation(context.Background(), "legacy@example.com", nil, nil)
	if err == nil {
		t.Fatal("expected legacy verification without a scan hash to be rejected")
	}
	var appErr *domain.AppError
	if !errors.As(err, &appErr) || appErr.StatusCode != 403 {
		t.Fatalf("expected a 403 AppError, got %#v", err)
	}
}

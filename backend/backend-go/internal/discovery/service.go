package discovery

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/atharvix/kinjo-backend/internal/config"
	"github.com/atharvix/kinjo-backend/internal/domain"
	"github.com/atharvix/kinjo-backend/internal/observability"
)

type Service struct {
	repo    Repository
	cfg     *config.Config
	logger  *slog.Logger
	metrics *observability.Metrics
}

func NewService(
	repo Repository,
	cfg *config.Config,
	logger *slog.Logger,
	metrics *observability.Metrics,
) *Service {
	return &Service{
		repo:    repo,
		cfg:     cfg,
		logger:  logger,
		metrics: metrics,
	}
}

const (
	// defaultRadiusMeters is the fallback geofence. Indoor GPS drifts 15-40m, so
	// this is tunable via NEARBY_RADIUS_METERS — people in one room otherwise
	// drop out of each other's decks.
	defaultRadiusMeters = 30.0
	MaxNearbyLimit      = 1000
	// defaultPresenceTTL is used only when no configuration is supplied.
	defaultPresenceTTL = 30 * 24 * time.Hour
)

// radiusMeters resolves the configured discovery radius, falling back when the
// service was built without configuration.
func (s *Service) radiusMeters() float64 {
	if s.cfg != nil && s.cfg.NearbyRadiusMeters > 0 {
		return s.cfg.NearbyRadiusMeters
	}
	return defaultRadiusMeters
}

func (s *Service) GetNearbyProfilesWithLocation(ctx context.Context, email string, lat, lon *float64) (*domain.NearbyProfilesResponse, error) {
	caller, err := s.repo.GetCallerProfile(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrProfileNotFound) {
			return nil, domain.NewAppError(404, "Profile not found. Please create a profile first.", domain.ErrProfileNotFound)
		}
		s.logger.ErrorContext(ctx, "failed to get caller profile", slog.String("email", email), slog.String("error", err.Error()))
		return nil, domain.NewAppError(500, "Failed to fetch nearby profiles. Please try again.", domain.ErrInternal)
	}

	// Verifications recorded before the liveness flow (migration 000004) carry
	// no scan hash. They are treated as unverified so those accounts re-scan
	// under the hardened flow instead of keeping the weaker verification.
	if caller.FaceVerifiedAt == nil || caller.FaceScanHash == "" {
		return nil, domain.NewAppError(403, "Face verification required. Please complete the live face scan.", domain.ErrForbidden)
	}

	var targetLat, targetLon float64
	if lat != nil && lon != nil {
		targetLat = *lat
		targetLon = *lon
	} else {
		if caller.Latitude == nil || caller.Longitude == nil {
			return nil, domain.NewAppError(400, "No location stored for your profile. Please update your location first.", domain.ErrNoLocation)
		}
		targetLat = *caller.Latitude
		targetLon = *caller.Longitude
	}

	// Profiles whose last activity predates the presence TTL (or that went
	// offline explicitly) must not be discoverable.
	presenceTTL := defaultPresenceTTL
	if s.cfg != nil && s.cfg.PresenceTTL > 0 {
		presenceTTL = s.cfg.PresenceTTL
	}
	presenceCutoff := time.Now().Add(-presenceTTL)

	records, err := s.repo.FindNearbyProfiles(ctx, email, targetLat, targetLon, s.radiusMeters(), presenceCutoff, MaxNearbyLimit)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to find nearby profiles", slog.String("email", email), slog.String("error", err.Error()))
		return nil, domain.NewAppError(500, "Failed to fetch nearby profiles. Please try again.", domain.ErrInternal)
	}

	cards := make([]domain.NearbyProfileCard, 0, len(records))
	for _, r := range records {
		// A legacy row can still hold its photo inline as a base64 data URL.
		// Shipping that would put the whole image inside every deck response,
		// where it can never be cached and is re-sent on every poll — a single
		// deck measured ~160KB. Drop it here; `kinjo-admin migrate-photos` moves
		// those rows onto real storage so the URL comes back.
		photo := r.PhotoURL
		if strings.HasPrefix(photo, "data:") {
			photo = ""
		}

		// There is no dedicated headline/AI-summary storage yet, so the bio is
		// used for both card fields.
		bio := strings.TrimSpace(r.Bio)
		cards = append(cards, domain.NearbyProfileCard{
			Email:               r.Email,
			Name:                r.Name,
			Photo:               photo,
			DistanceMeters:      r.DistanceMeters,
			Headline:            bio,
			ConversationStarter: bio,
		})
	}

	if s.metrics != nil {
		s.metrics.ActiveNearbyUsers.Set(float64(len(cards)))
	}

	return &domain.NearbyProfilesResponse{Profiles: cards}, nil
}

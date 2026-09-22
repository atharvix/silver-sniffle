package discovery

import (
	"context"
	"errors"
	"log/slog"
	"strings"

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
	NearbyRadiusMeters = 30.0
	MaxNearbyLimit     = 1000
)

func (s *Service) GetNearbyProfiles(ctx context.Context, email string) (*domain.NearbyProfilesResponse, error) {
	return s.GetNearbyProfilesWithLocation(ctx, email, nil, nil)
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

	if caller.FaceVerifiedAt == nil {
		return nil, domain.NewAppError(403, "Face verification required. Please verify your face first.", domain.ErrForbidden)
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

	records, err := s.repo.FindNearbyProfiles(ctx, email, targetLat, targetLon, NearbyRadiusMeters, MaxNearbyLimit)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to find nearby profiles", slog.String("email", email), slog.String("error", err.Error()))
		return nil, domain.NewAppError(500, "Failed to fetch nearby profiles. Please try again.", domain.ErrInternal)
	}

	cards := make([]domain.NearbyProfileCard, 0, len(records))
	for _, r := range records {
		headline := strings.TrimSpace(r.Bio)
		if r.Headline != nil && *r.Headline != "" {
			headline = *r.Headline
		}

		starter := strings.TrimSpace(r.Bio)
		if r.AISummary != nil && *r.AISummary != "" {
			starter = *r.AISummary
		}

		cards = append(cards, domain.NearbyProfileCard{
			Email:               r.Email,
			Name:                r.Name,
			Photo:               r.PhotoURL,
			DistanceMeters:      r.DistanceMeters,
			Headline:            headline,
			ConversationStarter: starter,
		})
	}

	if s.metrics != nil {
		s.metrics.ActiveNearbyUsers.Set(float64(len(cards)))
	}

	return &domain.NearbyProfilesResponse{Profiles: cards}, nil
}

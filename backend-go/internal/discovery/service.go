package discovery

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/atharvix/kinjo-backend/internal/ai"
	"github.com/atharvix/kinjo-backend/internal/config"
	"github.com/atharvix/kinjo-backend/internal/domain"
	"github.com/atharvix/kinjo-backend/internal/observability"
)

type Service struct {
	repo      Repository
	aiService ai.Service
	cfg       *config.Config
	logger    *slog.Logger
	metrics   *observability.Metrics
}

func NewService(
	repo Repository,
	aiService ai.Service,
	cfg *config.Config,
	logger *slog.Logger,
	metrics *observability.Metrics,
) *Service {
	return &Service{
		repo:      repo,
		aiService: aiService,
		cfg:       cfg,
		logger:    logger,
		metrics:   metrics,
	}
}

const (
	NearbyRadiusMeters = 30.0
	MaxNearbyLimit     = 50
)

func (s *Service) GetNearbyProfiles(ctx context.Context, email string) (*domain.NearbyProfilesResponse, error) {
	caller, err := s.repo.GetCallerProfile(ctx, email)
	if err != nil {
		if err == domain.ErrProfileNotFound {
			return nil, domain.NewAppError(404, "Profile not found. Please create a profile first.", domain.ErrProfileNotFound)
		}
		s.logger.ErrorContext(ctx, "failed to get caller profile", slog.String("email", email), slog.String("error", err.Error()))
		return nil, domain.NewAppError(500, "Failed to fetch nearby profiles. Please try again.", domain.ErrInternal)
	}

	if caller.Latitude == nil || caller.Longitude == nil {
		return nil, domain.NewAppError(400, "No location stored for your profile. Please update your location first.", domain.ErrNoLocation)
	}

	now := time.Now()
	presenceCutoff := now.Add(-s.cfg.PresenceTTL)

	if caller.LastSeenAt == nil || caller.LastSeenAt.Before(presenceCutoff) {
		return nil, domain.NewAppError(400, "Your location sharing has expired. Please share your location again.", domain.ErrLocationExpired)
	}

	records, err := s.repo.FindNearbyProfiles(ctx, email, *caller.Latitude, *caller.Longitude, NearbyRadiusMeters, presenceCutoff, MaxNearbyLimit)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to find nearby profiles", slog.String("email", email), slog.String("error", err.Error()))
		return nil, domain.NewAppError(500, "Failed to fetch nearby profiles. Please try again.", domain.ErrInternal)
	}

	cards := make([]domain.NearbyProfileCard, 0, len(records))
	for _, r := range records {
		headline := strings.TrimSpace(r.About)
		if r.Headline != nil && *r.Headline != "" {
			headline = *r.Headline
		}

		starter := strings.TrimSpace(r.About)
		if r.AISummary != nil && *r.AISummary != "" {
			starter = *r.AISummary
		}

		cards = append(cards, domain.NearbyProfileCard{
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

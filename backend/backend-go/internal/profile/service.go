package profile

import (
	"context"
	"html"
	"log/slog"
	"strings"

	"github.com/atharvix/kinjo-backend/internal/config"
	"github.com/atharvix/kinjo-backend/internal/domain"
	"github.com/atharvix/kinjo-backend/internal/storage"
)

type Service struct {
	repo    Repository
	storage storage.Storage
	cfg     *config.Config
	logger  *slog.Logger
}

func NewService(repo Repository, storage storage.Storage, cfg *config.Config, logger *slog.Logger) *Service {
	return &Service{
		repo:    repo,
		storage: storage,
		cfg:     cfg,
		logger:  logger,
	}
}

func (s *Service) UpsertProfile(ctx context.Context, email string, req *domain.UpsertProfileRequest) (*domain.ProfileResponse, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, domain.NewAppError(400, "Name is required.", domain.ErrBadRequest)
	}
	if len(name) > 100 {
		name = name[:100]
	}
	name = html.EscapeString(name)

	bio := ""
	if req.Bio != nil {
		bio = strings.TrimSpace(*req.Bio)
		if len(bio) > 1000 {
			bio = bio[:1000]
		}
		bio = html.EscapeString(bio)
	}

	photoURL := ""
	if req.Photo != nil && *req.Photo != "" {
		// Store photo string directly in DB (local/cloud file upload disabled for now)
		photoURL = *req.Photo
	} else if existing, err := s.repo.GetByEmail(ctx, email); err == nil && existing != nil {
		photoURL = existing.PhotoURL
	}
	p := &domain.Profile{
		Email:     email,
		Name:      name,
		Bio:       bio,
		PhotoURL:  photoURL,
		Latitude:  req.Latitude,
		Longitude: req.Longitude,
	}

	if err := s.repo.Upsert(ctx, p); err != nil {
		s.logger.ErrorContext(ctx, "failed to upsert profile", slog.String("email", email), slog.String("error", err.Error()))
		return nil, domain.NewAppError(500, "Failed to save profile. Please try again.", domain.ErrInternal)
	}

	s.logger.InfoContext(ctx, "profile upserted successfully", slog.String("email", email))

	return &domain.ProfileResponse{
		Success:  true,
		Message:  "Profile saved.",
		PhotoURL: photoURL,
	}, nil
}

func (s *Service) GetMyProfile(ctx context.Context, email string) (*domain.MyProfileResponse, error) {
	p, err := s.repo.GetByEmail(ctx, email)
	if err != nil {
		if err == domain.ErrProfileNotFound {
			return nil, domain.NewAppError(404, "Profile not found. Please create a profile first.", domain.ErrProfileNotFound)
		}
		s.logger.ErrorContext(ctx, "failed to fetch profile", slog.String("email", email), slog.String("error", err.Error()))
		return nil, domain.NewAppError(500, "Failed to fetch profile. Please try again.", domain.ErrInternal)
	}

	return &domain.MyProfileResponse{
		Email: p.Email,
		Name:  p.Name,
		Bio:   p.Bio,
		Photo: p.PhotoURL,
	}, nil
}

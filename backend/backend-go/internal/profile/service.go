package profile

import (
	"context"
	"log/slog"
	"net/url"
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

	about := ""
	if req.About != nil {
		about = strings.TrimSpace(*req.About)
	}

	photoURL := ""
	if req.Photo != nil && *req.Photo != "" {
		processedURL, err := storage.ProcessImage(ctx, s.storage, *req.Photo, s.cfg.MaxPhotoBytes)
		if err != nil {
			s.logger.WarnContext(ctx, "failed to process profile photo", slog.String("email", email), slog.String("error", err.Error()))
			return nil, domain.NewAppError(400, "Invalid profile photo format or image too large.", domain.ErrBadRequest)
		}
		photoURL = processedURL
	}
	for network, link := range req.SocialLinks {
		parsed, err := url.Parse(strings.TrimSpace(link))
		if err != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || parsed.Host == "" {
			return nil, domain.NewAppError(400, "Social links must use a valid http or https URL.", domain.ErrBadRequest)
		}
		req.SocialLinks[network] = parsed.String()
	}

	p := &domain.Profile{
		Email:    email,
		Name:     name,
		About:    about,
		PhotoURL: photoURL,
		SocialLinks: req.SocialLinks,
	}

	if err := s.repo.Upsert(ctx, p); err != nil {
		s.logger.ErrorContext(ctx, "failed to upsert profile", slog.String("email", email), slog.String("error", err.Error()))
		return nil, domain.NewAppError(500, "Failed to save profile. Please try again.", domain.ErrInternal)
	}

	s.logger.InfoContext(ctx, "profile upserted successfully", slog.String("email", email))

	return &domain.ProfileResponse{
		Success: true,
		Message: "Profile saved.",
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
		About: p.About,
		Photo: p.PhotoURL,
	}, nil
}

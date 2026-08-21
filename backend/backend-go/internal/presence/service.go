package presence

import (
	"context"
	"log/slog"

	"github.com/atharvix/kinjo-backend/internal/domain"
	"github.com/atharvix/kinjo-backend/internal/middleware"
)

type Service struct {
	repo           Repository
	tokenValidator middleware.TokenValidator
	logger         *slog.Logger
}

func NewService(repo Repository, tokenValidator middleware.TokenValidator, logger *slog.Logger) *Service {
	return &Service{
		repo:           repo,
		tokenValidator: tokenValidator,
		logger:         logger,
	}
}

func (s *Service) UpdateLocation(ctx context.Context, email string, lat, lon float64) (*domain.UpdateLocationResponse, error) {
	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return nil, domain.NewAppError(400, "Invalid latitude or longitude coordinates.", domain.ErrBadRequest)
	}

	if err := s.repo.UpdateLocation(ctx, email, lat, lon); err != nil {
		if err == domain.ErrProfileNotFound {
			return nil, domain.NewAppError(404, "Profile not found. Please create a profile first.", domain.ErrProfileNotFound)
		}
		s.logger.ErrorContext(ctx, "failed to update location", slog.String("email", email), slog.String("error", err.Error()))
		return nil, domain.NewAppError(500, "Failed to update location. Please try again.", domain.ErrInternal)
	}

	s.logger.InfoContext(ctx, "location updated", slog.String("email", email), slog.Float64("lat", lat), slog.Float64("lon", lon))

	return &domain.UpdateLocationResponse{
		Success: true,
		Message: "Location updated.",
	}, nil
}

func (s *Service) RecordHeartbeat(ctx context.Context, email string) (*domain.HeartbeatResponse, error) {
	if err := s.repo.RecordHeartbeat(ctx, email); err != nil {
		if err == domain.ErrProfileNotFound {
			return nil, domain.NewAppError(404, "Profile not found. Please create a profile first.", domain.ErrProfileNotFound)
		}
		s.logger.ErrorContext(ctx, "failed to record heartbeat", slog.String("email", email), slog.String("error", err.Error()))
		return nil, domain.NewAppError(500, "Failed to record heartbeat. Please try again.", domain.ErrInternal)
	}

	return &domain.HeartbeatResponse{Success: true}, nil
}

func (s *Service) GoOffline(ctx context.Context, token string) (*domain.HeartbeatResponse, error) {
	email, err := s.tokenValidator.GetEmailFromToken(ctx, token)
	if err != nil {
		return nil, domain.NewAppError(401, "Verification token is invalid or has expired.", domain.ErrUnauthorized)
	}

	if err := s.repo.MarkOffline(ctx, email); err != nil {
		s.logger.ErrorContext(ctx, "failed to mark profile offline", slog.String("email", email), slog.String("error", err.Error()))
		return nil, domain.NewAppError(500, "Failed to go offline.", domain.ErrInternal)
	}

	s.logger.InfoContext(ctx, "profile marked offline", slog.String("email", email))

	return &domain.HeartbeatResponse{Success: true}, nil
}

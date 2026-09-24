package presence

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/atharvix/kinjo-backend/internal/domain"
	"github.com/atharvix/kinjo-backend/internal/middleware"
)

type NearbyNotifier interface {
	NotifyNearbyNewUser(ctx context.Context, email, name string, lat, lon float64) error
}

type Service struct {
	repo           Repository
	tokenValidator middleware.TokenValidator
	notifier       NearbyNotifier
	logger         *slog.Logger
}

func NewService(repo Repository, tokenValidator middleware.TokenValidator, logger *slog.Logger) *Service {
	return &Service{
		repo:           repo,
		tokenValidator: tokenValidator,
		logger:         logger,
	}
}

func (s *Service) SetNotifier(n NearbyNotifier) {
	s.notifier = n
}

func (s *Service) UpdateLocation(ctx context.Context, email string, lat, lon float64) (*domain.UpdateLocationResponse, error) {
	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return nil, domain.NewAppError(400, "Invalid latitude or longitude coordinates.", domain.ErrBadRequest)
	}

	res, err := s.repo.UpdateLocationWithResult(ctx, email, lat, lon)
	if err != nil {
		if errors.Is(err, domain.ErrForbidden) {
			return nil, domain.NewAppError(403, "Face verification required.", domain.ErrForbidden)
		}
		if errors.Is(err, domain.ErrProfileNotFound) {
			return nil, domain.NewAppError(404, "Profile not found. Please create a profile first.", domain.ErrProfileNotFound)
		}
		s.logger.ErrorContext(ctx, "failed to update location", slog.String("email", email), slog.String("error", err.Error()))
		return nil, domain.NewAppError(500, "Failed to update location. Please try again.", domain.ErrInternal)
	}

	s.logger.InfoContext(ctx, "location updated", slog.String("email", email))

	if res != nil && res.IsFirstLocation && s.notifier != nil {
		go func(e, n string, la, lo float64) {
			bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = s.notifier.NotifyNearbyNewUser(bgCtx, e, n, la, lo)
		}(email, res.Name, lat, lon)
	}

	return &domain.UpdateLocationResponse{
		Success: true,
		Message: "Location updated.",
	}, nil
}

func (s *Service) RecordHeartbeat(ctx context.Context, email string) (*domain.HeartbeatResponse, error) {
	if err := s.repo.RecordHeartbeat(ctx, email); err != nil {
		if errors.Is(err, domain.ErrForbidden) {
			return nil, domain.NewAppError(403, "Face verification required.", domain.ErrForbidden)
		}
		if errors.Is(err, domain.ErrProfileNotFound) {
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

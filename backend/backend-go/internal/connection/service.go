package connection

import (
	"context"
	"strings"

	"github.com/atharvix/kinjo-backend/internal/domain"
)

type Service struct { repo Repository }

func NewService(repo Repository) *Service { return &Service{repo: repo} }

func (s *Service) Create(ctx context.Context, requesterEmail string, req domain.ConnectionRequest) (*domain.ConnectionResponse, error) {
	recipient := strings.TrimSpace(req.RecipientEmail)
	if recipient == "" || strings.EqualFold(recipient, requesterEmail) {
		return nil, domain.NewAppError(400, "A valid recipient is required.", domain.ErrBadRequest)
	}
	if err := s.repo.Create(ctx, requesterEmail, recipient); err != nil {
		if err == domain.ErrProfileNotFound {
			return nil, domain.NewAppError(404, "That profile is no longer available.", domain.ErrProfileNotFound)
		}
		return nil, domain.NewAppError(500, "Unable to save this connection.", domain.ErrInternal)
	}
	return &domain.ConnectionResponse{Success: true, Message: "Connection request sent."}, nil
}

func (s *Service) ListIncoming(ctx context.Context, email string) ([]domain.ConnectionNotification, error) {
	return s.repo.ListIncoming(ctx, email)
}
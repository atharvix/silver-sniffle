package notification

import (
	"context"
	"fmt"
	"log"

	"github.com/atharvix/kinjo-backend/internal/domain"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) RegisterToken(ctx context.Context, email, token, platform string) error {
	if email == "" || token == "" {
		return fmt.Errorf("email and token are required")
	}
	if platform == "" {
		platform = "android"
	}
	return s.repo.SaveToken(ctx, email, token, platform)
}

func (s *Service) SendCustomNotification(ctx context.Context, req domain.SendCustomNotificationRequest) error {
	tokens, err := s.repo.GetTokensByEmail(ctx, req.TargetEmail)
	if err != nil {
		return fmt.Errorf("failed to fetch target tokens: %w", err)
	}

	if len(tokens) == 0 {
		log.Printf("[Notification] No registered tokens found for user %s (notification payload stored/logged)", req.TargetEmail)
		return nil
	}

	for _, t := range tokens {
		log.Printf("[Notification Push] Sending to %s (%s, platform: %s): Title='%s', Body='%s'", t.Email, t.Token, t.Platform, req.Title, req.Body)
		// FCM or APNs dispatch can be integrated here with credentials when FCM key is provided.
	}
	return nil
}

func (s *Service) NotifyNearbyUsers(ctx context.Context, senderEmail string, lat, lon float64) error {
	// Radius 30 meters
	tokens, err := s.repo.GetTokensForNearbyUsers(ctx, lat, lon, 30.0, senderEmail)
	if err != nil {
		return fmt.Errorf("failed to fetch nearby device tokens: %w", err)
	}

	for _, t := range tokens {
		log.Printf("[Notification Push] Nearby Alert to %s (%s): Someone new is within 30m of you!", t.Email, t.Token)
	}
	return nil
}

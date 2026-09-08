package notification

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

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
	var tokens []domain.DeviceToken
	var err error

	targetLower := strings.TrimSpace(strings.ToLower(req.TargetEmail))

	switch targetLower {
	case "*", "all", "":
		log.Printf("[Notification Engine] Target: ALL USERS (Database Broadcast). Fetching tokens...")
		tokens, err = s.repo.GetAllTokens(ctx)
	case "new", "recent", "new_users":
		log.Printf("[Notification Engine] Target: NEW USERS (Registered in last 24h). Fetching tokens...")
		tokens, err = s.repo.GetRecentTokens(ctx, 24*time.Hour)
	default:
		if strings.Contains(targetLower, ",") {
			rawParts := strings.Split(targetLower, ",")
			var cleanEmails []string
			for _, part := range rawParts {
				trimmed := strings.TrimSpace(part)
				if trimmed != "" {
					cleanEmails = append(cleanEmails, trimmed)
				}
			}
			log.Printf("[Notification Engine] Target: %d BATCH USERS (%v). Fetching tokens...", len(cleanEmails), cleanEmails)
			tokens, err = s.repo.GetTokensByEmails(ctx, cleanEmails)
		} else {
			log.Printf("[Notification Engine] Target: SINGLE USER (%s). Fetching token...", targetLower)
			tokens, err = s.repo.GetTokensByEmail(ctx, targetLower)
		}
	}

	if err != nil {
		return fmt.Errorf("failed to fetch device tokens for target '%s': %w", req.TargetEmail, err)
	}

	if len(tokens) == 0 {
		log.Printf("[Notification Engine] No active device tokens found for target '%s' (payload logged: Title='%s')", req.TargetEmail, req.Title)
		return nil
	}

	log.Printf("[Notification Engine] Successfully targeted %d device token(s) for payload Title='%s'", len(tokens), req.Title)
	for _, t := range tokens {
		log.Printf("[Notification Push] Delivering to %s (%s, platform: %s): Title='%s', Body='%s'", t.Email, t.Token, t.Platform, req.Title, req.Body)
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

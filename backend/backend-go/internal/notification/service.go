package notification

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/atharvix/kinjo-backend/internal/discovery"
	"github.com/atharvix/kinjo-backend/internal/domain"
)

type Service struct {
	repo          Repository
	fcm           FCMClient
	discoveryRepo discovery.Repository
}

func NewService(repo Repository, fcm FCMClient, discoveryRepo discovery.Repository) *Service {
	return &Service{
		repo:          repo,
		fcm:           fcm,
		discoveryRepo: discoveryRepo,
	}
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
		if s.fcm != nil {
			if err := s.fcm.Send(ctx, t.Token, req.Title, req.Body, nil); err != nil {
				log.Printf("[Notification Push Error] Failed to deliver to %s (%s): %v", t.Email, t.Token, err)
			}
		}
	}
	return nil
}

// NotifyNearbyNewUser dispatches push notifications to active verified users within 30 meters
// when a new user completes onboarding and establishes their location.
func (s *Service) NotifyNearbyNewUser(ctx context.Context, email, name string, lat, lon float64) error {
	if s.discoveryRepo == nil || s.fcm == nil || !s.fcm.IsConfigured() {
		return nil
	}

	records, err := s.discoveryRepo.FindNearbyProfiles(ctx, email, lat, lon, 30.0, time.Now().Add(-24*time.Hour), 50)
	if err != nil || len(records) == 0 {
		return err
	}

	emails := make([]string, 0, len(records))
	for _, r := range records {
		emails = append(emails, r.Email)
	}

	tokens, err := s.repo.GetTokensByEmails(ctx, emails)
	if err != nil || len(tokens) == 0 {
		return err
	}

	displayName := strings.TrimSpace(name)
	if displayName == "" {
		displayName = "A new user"
	}

	title := "New person nearby! 👋"
	body := fmt.Sprintf("%s just joined Kinjo within 30m of you.", displayName)
	data := map[string]string{
		"type":  "new_user_nearby",
		"email": email,
	}

	for _, t := range tokens {
		if err := s.fcm.Send(ctx, t.Token, title, body, data); err != nil {
			log.Printf("[Notification Engine] Failed sending new user alert to %s: %v", t.Email, err)
		}
	}
	log.Printf("[Notification Engine] Sent nearby new user push to %d device(s) within 30m for %s", len(tokens), email)
	return nil
}

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/atharvix/kinjo-backend/internal/ai"
	"github.com/atharvix/kinjo-backend/internal/auth"
	"github.com/atharvix/kinjo-backend/internal/config"
	"github.com/atharvix/kinjo-backend/internal/discovery"
	"github.com/atharvix/kinjo-backend/internal/domain"
	"github.com/atharvix/kinjo-backend/internal/email"
	kinjohttp "github.com/atharvix/kinjo-backend/internal/http"
	"github.com/atharvix/kinjo-backend/internal/observability"
	"github.com/atharvix/kinjo-backend/internal/presence"
	"github.com/atharvix/kinjo-backend/internal/profile"
	"github.com/atharvix/kinjo-backend/internal/storage"
)

// In-Memory Test Repositories for Integration Testing without PostgreSQL
type MockFullRepo struct {
	SavedOTPs      map[string]string
	SavedTokens    map[string]string
	VerifiedEmails map[string]bool
	Profiles       map[string]*domain.Profile
}

func NewMockFullRepo() *MockFullRepo {
	return &MockFullRepo{
		SavedOTPs:      make(map[string]string),
		SavedTokens:    make(map[string]string),
		VerifiedEmails: make(map[string]bool),
		Profiles:       make(map[string]*domain.Profile),
	}
}

// Auth Repository Methods
func (m *MockFullRepo) SaveOTP(ctx context.Context, emailStr, otpHash string, expiresAt time.Time) error {
	m.SavedOTPs[emailStr] = otpHash
	return nil
}

func (m *MockFullRepo) VerifyAndIssueToken(ctx context.Context, emailStr, plainOTP string, tokenHash string, tokenExpiresAt time.Time, maxAttempts int) error {
	expectedHash := auth.HashString(plainOTP)
	if m.SavedOTPs[emailStr] != expectedHash {
		return domain.NewAppError(400, "Incorrect OTP", nil)
	}
	delete(m.SavedOTPs, emailStr)
	m.SavedTokens[tokenHash] = emailStr
	m.VerifiedEmails[emailStr] = true
	return nil
}

func (m *MockFullRepo) GetEmailFromToken(ctx context.Context, tokenHash string) (string, error) {
	if emailStr, ok := m.SavedTokens[tokenHash]; ok {
		return emailStr, nil
	}
	return "", domain.ErrUnauthorized
}

func (m *MockFullRepo) IsEmailVerified(ctx context.Context, emailStr string) (bool, error) {
	return m.VerifiedEmails[emailStr], nil
}

func (m *MockFullRepo) ConsumeVerifiedEmail(ctx context.Context, emailStr string) error {
	delete(m.VerifiedEmails, emailStr)
	return nil
}

func (m *MockFullRepo) CleanupExpired(ctx context.Context) error {
	return nil
}

// Profile Repository Methods
func (m *MockFullRepo) Upsert(ctx context.Context, p *domain.Profile) error {
	existing, ok := m.Profiles[p.Email]
	now := time.Now()
	if ok {
		existing.Name = p.Name
		existing.About = p.About
		if p.PhotoURL != "" {
			existing.PhotoURL = p.PhotoURL
		}
		existing.UpdatedAt = now
	} else {
		p.CreatedAt = now
		p.UpdatedAt = now
		m.Profiles[p.Email] = p
	}
	return nil
}

func (m *MockFullRepo) GetByEmail(ctx context.Context, emailStr string) (*domain.Profile, error) {
	p, ok := m.Profiles[emailStr]
	if !ok {
		return nil, domain.ErrProfileNotFound
	}
	return p, nil
}

// Presence Repository Methods
func (m *MockFullRepo) UpdateLocation(ctx context.Context, emailStr string, lat, lon float64) error {
	p, ok := m.Profiles[emailStr]
	if !ok {
		return domain.ErrProfileNotFound
	}
	now := time.Now()
	p.Latitude = &lat
	p.Longitude = &lon
	p.LastSeenAt = &now
	p.UpdatedAt = now
	return nil
}

func (m *MockFullRepo) RecordHeartbeat(ctx context.Context, emailStr string) error {
	p, ok := m.Profiles[emailStr]
	if !ok {
		return domain.ErrProfileNotFound
	}
	now := time.Now()
	p.LastSeenAt = &now
	return nil
}

func (m *MockFullRepo) MarkOffline(ctx context.Context, emailStr string) error {
	p, ok := m.Profiles[emailStr]
	if !ok {
		return domain.ErrProfileNotFound
	}
	p.LastSeenAt = nil
	return nil
}

// Discovery Repository Methods
func (m *MockFullRepo) GetCallerProfile(ctx context.Context, emailStr string) (*domain.Profile, error) {
	return m.GetByEmail(ctx, emailStr)
}

func (m *MockFullRepo) FindNearbyProfiles(ctx context.Context, emailStr string, lat, lon, radiusMeters float64, presenceCutoff time.Time, limit int) ([]discovery.NearbyRecord, error) {
	var records []discovery.NearbyRecord
	for e, p := range m.Profiles {
		if e == emailStr || p.Latitude == nil || p.Longitude == nil || p.LastSeenAt == nil {
			continue
		}
		if p.LastSeenAt.Before(presenceCutoff) {
			continue
		}
		rec := discovery.NearbyRecord{
			Name:           p.Name,
			PhotoURL:       p.PhotoURL,
			About:          p.About,
			DistanceMeters: 10.0, // mock close distance
		}
		records = append(records, rec)
	}
	return records, nil
}

func TestE2E_FullFlow(t *testing.T) {
	cfg := &config.Config{
		Port:           8080,
		Environment:    "development",
		AllowedOrigins: []string{"*"},
		TokenTTL:       24 * time.Hour,
		OtpTTL:         10 * time.Minute,
		MaxOtpAttempts: 5,
		PresenceTTL:    20 * time.Second,
		StorageDriver:  "local",
		StorageDir:     "./test_uploads",
		BaseURL:        "http://localhost:8080",
	}

	logger := observability.NewNopLogger()
	mockRepo := NewMockFullRepo()
	mockEmail := email.NewMockService(logger)
	mockStorage, _ := storage.NewLocalStorage("./test_uploads", "http://localhost:8080")
	aiService := ai.NewOpenAIService("", "", logger, nil)

	authService := auth.NewService(mockRepo, mockEmail, cfg, logger, nil)
	profileService := profile.NewService(mockRepo, mockStorage, cfg, logger)
	presenceService := presence.NewService(mockRepo, authService, logger)
	discoveryService := discovery.NewService(mockRepo, aiService, cfg, logger, nil)

	handlers := kinjohttp.Handlers{
		Health:    observability.NewHealthHandler(nil),
		Auth:      auth.NewHandler(authService, nil),
		Profile:   profile.NewHandler(profileService),
		Presence:  presence.NewHandler(presenceService),
		Discovery: discovery.NewHandler(discoveryService),
	}

	router := kinjohttp.NewRouter(cfg, logger, nil, handlers, authService)

	// 1. Healthz Endpoint Test
	req, _ := http.NewRequest("GET", "/api/healthz", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("GET /api/healthz status = %d, want 200", w.Code)
	}

	// 2. Send OTP Test
	sendOtpBody, _ := json.Marshal(map[string]string{"email": "alice@kinjo.world"})
	req, _ = http.NewRequest("POST", "/api/auth/send-otp", bytes.NewBuffer(sendOtpBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("POST /api/auth/send-otp status = %d, want 200", w.Code)
	}
	var sendOtpResp domain.SendOTPResponse
	_ = json.NewDecoder(w.Body).Decode(&sendOtpResp)
	if !sendOtpResp.Success {
		t.Fatalf("SendOTP failed: %+v", sendOtpResp)
	}

	otp := mockEmail.SentOTPs["alice@kinjo.world"]
	if otp == "" {
		t.Fatalf("No OTP sent to mock email service")
	}

	// 3. Verify OTP Test
	verifyOtpBody, _ := json.Marshal(map[string]string{
		"email": "alice@kinjo.world",
		"otp":   otp,
	})
	req, _ = http.NewRequest("POST", "/api/auth/verify-otp", bytes.NewBuffer(verifyOtpBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("POST /api/auth/verify-otp status = %d, want 200", w.Code)
	}
	var verifyOtpResp domain.VerifyOTPResponse
	_ = json.NewDecoder(w.Body).Decode(&verifyOtpResp)
	token := verifyOtpResp.VerificationToken
	if token == "" {
		t.Fatalf("VerificationToken is empty")
	}

	// 4. Create Profile Test
	profileBody, _ := json.Marshal(map[string]string{
		"name":  "Alice Kinjo",
		"about": "Building AI products",
	})
	req, _ = http.NewRequest("POST", "/api/profiles", bytes.NewBuffer(profileBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("POST /api/profiles status = %d, body = %s", w.Code, w.Body.String())
	}

	// 5. Get My Profile Test
	req, _ = http.NewRequest("GET", "/api/profiles/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/profiles/me status = %d", w.Code)
	}
	var myProfile domain.MyProfileResponse
	_ = json.NewDecoder(w.Body).Decode(&myProfile)
	if myProfile.Name != "Alice Kinjo" {
		t.Errorf("GetMyProfile name = %q, want 'Alice Kinjo'", myProfile.Name)
	}

	// 6. Update Location Test
	locBody, _ := json.Marshal(map[string]float64{
		"latitude":  37.7749,
		"longitude": -122.4194,
	})
	req, _ = http.NewRequest("POST", "/api/profiles/location", bytes.NewBuffer(locBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("POST /api/profiles/location status = %d", w.Code)
	}

	// 7. Heartbeat Test
	req, _ = http.NewRequest("POST", "/api/profiles/heartbeat", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("POST /api/profiles/heartbeat status = %d", w.Code)
	}

	// 8. Nearby Discovery Test
	req, _ = http.NewRequest("GET", "/api/profiles/nearby", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/profiles/nearby status = %d", w.Code)
	}

	// 9. Offline Beacon Test
	offlineBody, _ := json.Marshal(map[string]string{"token": token})
	req, _ = http.NewRequest("POST", "/api/profiles/offline", bytes.NewBuffer(offlineBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("POST /api/profiles/offline status = %d", w.Code)
	}
}

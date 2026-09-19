package unit

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/atharvix/kinjo-backend/internal/config"
	"github.com/atharvix/kinjo-backend/internal/domain"
	"github.com/atharvix/kinjo-backend/internal/profile"
	"github.com/atharvix/kinjo-backend/internal/security"
)

type MockProfileRepo struct {
	profiles     map[string]*domain.Profile
	faceVerified map[string]bool
}

func (m *MockProfileRepo) Upsert(ctx context.Context, p *domain.Profile) error {
	m.profiles[p.Email] = p
	return nil
}

func (m *MockProfileRepo) GetByEmail(ctx context.Context, email string) (*domain.Profile, error) {
	p, ok := m.profiles[email]
	if !ok {
		return nil, domain.ErrProfileNotFound
	}
	return p, nil
}

func (m *MockProfileRepo) MarkFaceVerified(ctx context.Context, email string, faceScanPhotoURL string) error {
	m.faceVerified[email] = true
	return nil
}

func (m *MockProfileRepo) IsFaceVerified(ctx context.Context, email string) (bool, error) {
	return m.faceVerified[email], nil
}

func (m *MockProfileRepo) BackfillEncryption(ctx context.Context, c *security.Crypto) (int, error) {
	return 0, nil
}

func (m *MockProfileRepo) UpdateLocation(ctx context.Context, email string, lat, lon float64) error {
	if p, ok := m.profiles[email]; ok {
		p.Latitude = &lat
		p.Longitude = &lon
	}
	return nil
}

type MockStorage struct{}

func (m *MockStorage) Save(ctx context.Context, data []byte, contentType string) (string, error) {
	return "/uploads/mock-photo.jpg", nil
}

func (m *MockStorage) Delete(ctx context.Context, fileURL string) error {
	return nil
}

func newTestService(t *testing.T) (*profile.Service, *MockProfileRepo) {
	t.Helper()
	repo := &MockProfileRepo{
		profiles:     make(map[string]*domain.Profile),
		faceVerified: make(map[string]bool),
	}
	cfg := &config.Config{MaxPhotoBytes: 5000000, PhotoStorage: "db"}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cryptoSvc, err := security.New("unit-test-key-that-is-long-enough-for-256-bits!!")
	if err != nil {
		t.Fatalf("security.New() error = %v", err)
	}
	svc := profile.NewService(repo, &MockStorage{}, cfg, cryptoSvc, logger)
	return svc, repo
}

func TestProfileServiceRequiresFaceVerification(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Profile must NOT be savable before face verification
	req := &domain.UpsertProfileRequest{Name: "Alice"}
	_, err := svc.UpsertProfile(ctx, "alice@example.com", req)
	if err == nil {
		t.Fatal("expected error saving profile before face verification, got nil")
	}

	// Record face verification server-side, then save must succeed
	if _, err := svc.VerifyFaceScan(ctx, "alice@example.com", &domain.VerifyFaceRequest{Photo: "data:image/jpeg;base64,ZmFrZQ=="}); err != nil {
		t.Fatalf("VerifyFaceScan() error = %v", err)
	}
	res, err := svc.UpsertProfile(ctx, "alice@example.com", req)
	if err != nil {
		t.Fatalf("UpsertProfile after face verification error = %v", err)
	}
	if !res.Success {
		t.Error("expected success true after face verification")
	}
}

func TestProfileServiceUpsertAndSanitize(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Verify face first so the save is permitted
	if _, err := svc.VerifyFaceScan(ctx, "john@example.com", &domain.VerifyFaceRequest{Photo: "data:image/jpeg;base64,ZmFrZQ=="}); err != nil {
		t.Fatalf("VerifyFaceScan() error = %v", err)
	}

	// Empty name should return 400
	reqEmptyName := &domain.UpsertProfileRequest{Name: "   "}
	_, err := svc.UpsertProfile(ctx, "test@example.com", reqEmptyName)
	if err == nil {
		t.Fatalf("expected error for empty name, got nil")
	}

	// Name & Bio sanitization (HTML escaping)
	bioText := "Hello <script>alert(1)</script> World"
	req := &domain.UpsertProfileRequest{
		Name: "John <Doe>",
		Bio:  &bioText,
	}

	res, err := svc.UpsertProfile(ctx, "john@example.com", req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !res.Success {
		t.Errorf("expected success true, got false")
	}

	// Check stored values are sanitized
	p, err := svc.GetMyProfile(ctx, "john@example.com")
	if err != nil {
		t.Fatalf("failed to fetch stored profile: %v", err)
	}

	if p.Name != "John &lt;Doe&gt;" {
		t.Errorf("name sanitization failed: got %q, want %q", p.Name, "John &lt;Doe&gt;")
	}

	if p.Bio != "Hello &lt;script&gt;alert(1)&lt;/script&gt; World" {
		t.Errorf("bio sanitization failed: got %q", p.Bio)
	}
}

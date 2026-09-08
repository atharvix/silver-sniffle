package unit

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/atharvix/kinjo-backend/internal/config"
	"github.com/atharvix/kinjo-backend/internal/domain"
	"github.com/atharvix/kinjo-backend/internal/profile"
)

type MockProfileRepo struct {
	profiles map[string]*domain.Profile
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

func TestProfileServiceUpsertAndSanitize(t *testing.T) {
	repo := &MockProfileRepo{profiles: make(map[string]*domain.Profile)}
	mockStorage := &MockStorage{}
	cfg := &config.Config{MaxPhotoBytes: 5000000}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	svc := profile.NewService(repo, mockStorage, cfg, logger)
	ctx := context.Background()

	// Test 1: Empty name should return 400
	reqEmptyName := &domain.UpsertProfileRequest{Name: "   "}
	_, err := svc.UpsertProfile(ctx, "test@example.com", reqEmptyName)
	if err == nil {
		t.Fatalf("expected error for empty name, got nil")
	}

	// Test 2: Name & Bio sanitization (HTML escaping)
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

package unit

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/atharvix/kinjo-backend/internal/config"
	"github.com/atharvix/kinjo-backend/internal/domain"
	"github.com/atharvix/kinjo-backend/internal/profile"
	"github.com/atharvix/kinjo-backend/internal/storage"
)

// A tiny valid WebP, same shape the frontend sends as a data URL.
const inlinePhotoDataURL = "data:image/webp;base64,UklGRiQAAABXRUJQVlA4IBgAAAAwAQCdASoBAAEAAQAcJaQAA3AA/v38gAA="

// selfieDataURL builds a real JPEG. Face verification rejects arbitrary
// strings, so tests need actual image bytes; flat=true produces a solid fill.
func selfieDataURL(t *testing.T, size int, flat bool) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if flat {
				img.Set(x, y, color.RGBA{R: 200, G: 200, B: 200, A: 255})
				continue
			}
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 128, A: 255})
		}
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatalf("failed to encode test selfie: %v", err)
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

// testSelfie is a live-capture-sized, non-blank JPEG.
func testSelfie(t *testing.T) string {
	t.Helper()
	return selfieDataURL(t, storage.MinFaceScanDimension, false)
}

// verifyFace runs the two-step handshake the client performs: request a
// challenge, then submit the scan with it.
func verifyFace(t *testing.T, svc *profile.Service, email, photo string) error {
	t.Helper()
	challenge, err := svc.IssueFaceChallenge(context.Background(), email)
	if err != nil {
		t.Fatalf("IssueFaceChallenge() error = %v", err)
	}
	_, err = svc.VerifyFaceScan(context.Background(), email, &domain.VerifyFaceRequest{
		Photo:     photo,
		Challenge: challenge.Challenge,
	})
	return err
}

type MockProfileRepo struct {
	profiles      map[string]*domain.Profile
	faceVerified  map[string]bool
	challenges    map[string]string
	challengeExp  map[string]time.Time
	scanHashOwner map[string]string
}

func (m *MockProfileRepo) SaveFaceChallenge(ctx context.Context, email, challengeHash string, expiresAt time.Time) error {
	m.challenges[email] = challengeHash
	m.challengeExp[email] = expiresAt
	return nil
}

func (m *MockProfileRepo) ConsumeFaceChallenge(ctx context.Context, email, challengeHash string) error {
	stored, ok := m.challenges[email]
	if !ok || stored != challengeHash || !time.Now().Before(m.challengeExp[email]) {
		return domain.ErrForbidden
	}
	delete(m.challenges, email)
	return nil
}

func (m *MockProfileRepo) SetFaceScanHash(ctx context.Context, email, faceScanHash string) error {
	if owner, ok := m.scanHashOwner[faceScanHash]; ok && owner != email {
		return domain.ErrFaceScanReused
	}
	m.scanHashOwner[faceScanHash] = email
	return nil
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

func newMockProfileRepo() *MockProfileRepo {
	return &MockProfileRepo{
		profiles:      make(map[string]*domain.Profile),
		faceVerified:  make(map[string]bool),
		challenges:    make(map[string]string),
		challengeExp:  make(map[string]time.Time),
		scanHashOwner: make(map[string]string),
	}
}

func newTestService(t *testing.T) (*profile.Service, *MockProfileRepo) {
	t.Helper()
	repo := newMockProfileRepo()
	cfg := &config.Config{MaxPhotoBytes: 5000000, PhotoStorage: "db"}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := profile.NewService(repo, &MockStorage{}, cfg, nil, logger)
	return svc, repo
}

func TestProfileServiceRequiresFaceVerification(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Profile must NOT be savable before face verification
	bio := "Software Engineer · Looking for collaborators"
	req := &domain.UpsertProfileRequest{Name: "Alice", Bio: &bio}
	_, err := svc.UpsertProfile(ctx, "alice@example.com", req)
	if err == nil {
		t.Fatal("expected error saving profile before face verification, got nil")
	}

	// Record face verification server-side, then save must succeed
	if err := verifyFace(t, svc, "alice@example.com", testSelfie(t)); err != nil {
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

// Legacy rows kept photos inline as base64, which bloats every profile and
// discovery response. Reading such a row once must move it onto real storage.
func TestProfileServiceMigratesInlinePhotos(t *testing.T) {
	repo := newMockProfileRepo()
	repo.profiles["inline@example.com"] = &domain.Profile{
		Email:            "inline@example.com",
		Name:             "Inline",
		Bio:              "Legacy row",
		PhotoURL:         inlinePhotoDataURL,
		FaceScanPhotoURL: inlinePhotoDataURL,
	}
	repo.faceVerified["inline@example.com"] = true
	cfg := &config.Config{MaxPhotoBytes: 5000000, PhotoStorage: "local"}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := profile.NewService(repo, &MockStorage{}, cfg, nil, logger)

	p, err := svc.GetMyProfile(context.Background(), "inline@example.com")
	if err != nil {
		t.Fatalf("GetMyProfile() error = %v", err)
	}
	if p.Photo != "/uploads/mock-photo.jpg" {
		t.Errorf("inline profile photo not migrated: got %q", p.Photo)
	}
	if p.FaceScanPhoto != "/uploads/mock-photo.jpg" {
		t.Errorf("inline face scan photo not migrated: got %q", p.FaceScanPhoto)
	}
	if strings.HasPrefix(repo.profiles["inline@example.com"].PhotoURL, "data:") {
		t.Error("migrated profile photo was not persisted back to the repository")
	}
}

// Accounts verified before the liveness flow (migration 000004) have no scan
// hash. They must be reported as unverified so the client sends them back
// through the live face scan instead of keeping the weaker verification.
func TestLegacyFaceVerificationWithoutScanHashIsUnverified(t *testing.T) {
	repo := newMockProfileRepo()
	verifiedAt := time.Now().Add(-24 * time.Hour)
	repo.profiles["legacy@example.com"] = &domain.Profile{
		Email:          "legacy@example.com",
		Name:           "Legacy",
		Bio:            "Verified before the liveness flow",
		FaceVerifiedAt: &verifiedAt,
	}
	cfg := &config.Config{MaxPhotoBytes: 5000000, PhotoStorage: "db"}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := profile.NewService(repo, &MockStorage{}, cfg, nil, logger)

	p, err := svc.GetMyProfile(context.Background(), "legacy@example.com")
	if err != nil {
		t.Fatalf("GetMyProfile() error = %v", err)
	}
	if p.FaceVerified {
		t.Error("legacy verification without a scan hash must report face_verified=false")
	}
}

// A scan is only accepted when it is a real image that carries the challenge
// this server issued for the session.
func TestFaceScanRequiresRealPhotoAndFreshChallenge(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	email := "scanner@example.com"

	rejected := []struct {
		name  string
		photo string
	}{
		{"arbitrary payload", "data:image/jpeg;base64,ZmFrZWZha2VmYWtl"},
		{"hosted URL instead of a capture", "https://example.com/face.jpg"},
		{"solid fill", selfieDataURL(t, storage.MinFaceScanDimension, true)},
		{"thumbnail", selfieDataURL(t, storage.MinFaceScanDimension/4, false)},
	}

	for _, tc := range rejected {
		t.Run(tc.name, func(t *testing.T) {
			challenge, err := svc.IssueFaceChallenge(ctx, email)
			if err != nil {
				t.Fatalf("IssueFaceChallenge() error = %v", err)
			}
			_, err = svc.VerifyFaceScan(ctx, email, &domain.VerifyFaceRequest{
				Photo:     tc.photo,
				Challenge: challenge.Challenge,
			})
			if err == nil {
				t.Fatal("expected the scan to be rejected, got nil error")
			}
		})
	}

	// Missing challenge is refused even for a perfect photo.
	if _, err := svc.VerifyFaceScan(ctx, email, &domain.VerifyFaceRequest{Photo: testSelfie(t)}); err == nil {
		t.Error("expected a scan without a challenge to be rejected")
	}

	// The challenge is single-use: the same one cannot be replayed.
	challenge, err := svc.IssueFaceChallenge(ctx, email)
	if err != nil {
		t.Fatalf("IssueFaceChallenge() error = %v", err)
	}
	if _, err := svc.VerifyFaceScan(ctx, email, &domain.VerifyFaceRequest{Photo: testSelfie(t), Challenge: challenge.Challenge}); err != nil {
		t.Fatalf("first scan should succeed, got %v", err)
	}
	if _, err := svc.VerifyFaceScan(ctx, email, &domain.VerifyFaceRequest{Photo: testSelfie(t), Challenge: challenge.Challenge}); err == nil {
		t.Error("expected a replayed challenge to be rejected")
	}
}

// One captured selfie must not be able to verify somebody else's account.
func TestFaceScanCannotVerifyTwoAccounts(t *testing.T) {
	svc, _ := newTestService(t)
	photo := testSelfie(t)

	if err := verifyFace(t, svc, "first@example.com", photo); err != nil {
		t.Fatalf("first account verification error = %v", err)
	}

	challenge, err := svc.IssueFaceChallenge(context.Background(), "second@example.com")
	if err != nil {
		t.Fatalf("IssueFaceChallenge() error = %v", err)
	}
	_, err = svc.VerifyFaceScan(context.Background(), "second@example.com", &domain.VerifyFaceRequest{
		Photo:     photo,
		Challenge: challenge.Challenge,
	})
	if !errors.Is(err, domain.ErrFaceScanReused) {
		t.Errorf("reused selfie error = %v, want ErrFaceScanReused", err)
	}
}

func TestProfileServiceUpsertAndSanitize(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()

	// Verify face first so the save is permitted
	if err := verifyFace(t, svc, "john@example.com", testSelfie(t)); err != nil {
		t.Fatalf("VerifyFaceScan() error = %v", err)
	}

	// Empty name should return 400
	reqEmptyName := &domain.UpsertProfileRequest{Name: "   "}
	_, err := svc.UpsertProfile(ctx, "test@example.com", reqEmptyName)
	if err == nil {
		t.Fatalf("expected error for empty name, got nil")
	}

	// Empty bio should return 400
	reqEmptyBio := &domain.UpsertProfileRequest{Name: "Valid Name", Bio: nil}
	_, err = svc.UpsertProfile(ctx, "test@example.com", reqEmptyBio)
	if err == nil {
		t.Fatalf("expected error for empty bio, got nil")
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

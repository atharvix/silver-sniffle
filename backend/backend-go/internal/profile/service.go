package profile

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"html"
	"log/slog"
	"strings"
	"time"

	"github.com/atharvix/kinjo-backend/internal/config"
	"github.com/atharvix/kinjo-backend/internal/domain"
	"github.com/atharvix/kinjo-backend/internal/email"
	"github.com/atharvix/kinjo-backend/internal/storage"
)

type Service struct {
	repo         Repository
	storage      storage.Storage
	cfg          *config.Config
	emailService email.Service
	logger       *slog.Logger
}

func NewService(repo Repository, store storage.Storage, cfg *config.Config, emailService email.Service, logger *slog.Logger) *Service {
	return &Service{
		repo:         repo,
		storage:      store,
		cfg:          cfg,
		emailService: emailService,
		logger:       logger,
	}
}

// SavePhoto persists a photo according to cfg.PhotoStorage:
//   - "db":       keep the data URL / URL string as-is inside the DB row
//     (legacy mode; works everywhere, including read-only FS).
//   - "local":    decode base64 data URLs, validate, compress to JPEG and
//     store on disk under /uploads; DB keeps a relative URL.
//   - "supabase": same decode/validate/compress pipeline, upload to a
//     Supabase Storage bucket; DB keeps the public URL.
//
// In all modes inputs are validated for size and MIME type; only image/* is
// accepted, and plaintext URLs are passed through untouched.
func (s *Service) SavePhoto(ctx context.Context, input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", nil
	}

	// Already a hosted URL or legacy demo path: nothing to store.
	if strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://") ||
		strings.HasPrefix(input, "/demo-") || strings.Contains(input, "/uploads/") {
		return input, nil
	}

	switch s.cfg.PhotoStorage {
	case "local", "supabase":
		url, err := storage.ProcessImage(ctx, s.storage, input, s.cfg.MaxPhotoBytes)
		if err != nil {
			switch {
			case errors.Is(err, storage.ErrImageTooLarge):
				return "", domain.NewAppError(413, "Photo is too large. Please choose an image under 8MB.", err)
			case errors.Is(err, storage.ErrUnsupportedFormat):
				return "", domain.NewAppError(415, "Unsupported image format. Please upload a JPG, PNG or WebP photo.", err)
			default:
				return "", domain.NewAppError(400, "Invalid photo data. Please try a different image.", err)
			}
		}
		return url, nil

	default: // "db" (default) — validate size for base64 payloads, then store inline
		if len(input) > int(s.cfg.MaxPhotoBytes*4/3+1024) { // base64 overhead ≈ 4/3
			return "", domain.NewAppError(413, "Photo is too large. Please choose an image under 8MB.", nil)
		}
		return input, nil
	}
}

func (s *Service) UpsertProfile(ctx context.Context, email string, req *domain.UpsertProfileRequest) (*domain.ProfileResponse, error) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, domain.NewAppError(400, "Name is required.", domain.ErrBadRequest)
	}
	if len(name) > 100 {
		name = name[:100]
	}
	name = html.EscapeString(name)

	bio := ""
	if req.Bio != nil {
		bio = strings.TrimSpace(*req.Bio)
		if len(bio) > 1000 {
			bio = bio[:1000]
		}
		bio = html.EscapeString(bio)
	}
	if bio == "" {
		return nil, domain.NewAppError(400, "What you do & what you are looking for is required.", domain.ErrBadRequest)
	}

	// Enforce the server-side face verification gate: profiles can only be
	// saved after the live liveness scan has been recorded for this account.
	verified, err := s.repo.IsFaceVerified(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrProfileNotFound) {
			return nil, domain.NewAppError(404, "Profile not found. Please create a profile first.", domain.ErrProfileNotFound)
		}
		s.logger.ErrorContext(ctx, "failed to check face verification", slog.String("email", email), slog.String("error", err.Error()))
		return nil, domain.NewAppError(500, "Failed to save profile. Please try again.", domain.ErrInternal)
	}
	if !verified {
		return nil, domain.NewAppError(403, "Face verification required. Please complete the live face scan before saving your profile.", domain.ErrForbidden)
	}

	// Check if this is initial onboarding
	isInitialOnboarding := false
	existing, getErr := s.repo.GetByEmail(ctx, email)
	if getErr != nil || existing == nil || existing.Bio == "" {
		isInitialOnboarding = true
	}

	photoURL := ""
	if req.Photo != nil && *req.Photo != "" {
		photoURL, err = s.SavePhoto(ctx, *req.Photo)
		if err != nil {
			return nil, err
		}
	} else if existing != nil {
		photoURL = existing.PhotoURL
	}

	p := &domain.Profile{
		Email:     email,
		Name:      name,
		Bio:       bio,
		PhotoURL:  photoURL,
		Latitude:  req.Latitude,
		Longitude: req.Longitude,
	}

	if err := s.repo.Upsert(ctx, p); err != nil {
		s.logger.ErrorContext(ctx, "failed to upsert profile", slog.String("error", err.Error()))
		return nil, domain.NewAppError(500, "Failed to save profile. Please try again.", domain.ErrInternal)
	}

	s.logger.InfoContext(ctx, "profile upserted successfully")

	// Trigger welcome email asynchronously upon successful initial onboarding
	if isInitialOnboarding && s.emailService != nil && s.emailService.IsConfigured() {
		go func(toEmail, toName, toBio string) {
			bgCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			if err := s.emailService.SendWelcome(bgCtx, toEmail, toName, toBio); err != nil {
				s.logger.WarnContext(bgCtx, "failed to send welcome email on profile onboarding",
					slog.String("email", toEmail),
					slog.String("error", err.Error()),
				)
			} else {
				s.logger.InfoContext(bgCtx, "welcome email sent successfully on onboarding", slog.String("email", toEmail))
			}
		}(email, name, bio)
	}

	return &domain.ProfileResponse{
		Success:  true,
		Message:  "Profile saved.",
		PhotoURL: photoURL,
	}, nil
}

func (s *Service) GetMyProfile(ctx context.Context, email string) (*domain.MyProfileResponse, error) {
	p, err := s.repo.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, domain.ErrProfileNotFound) {
			return nil, domain.NewAppError(404, "Profile not found.", domain.ErrProfileNotFound)
		}
		s.logger.ErrorContext(ctx, "failed to query profile", slog.String("error", err.Error()))
		return nil, domain.NewAppError(500, "Failed to retrieve profile.", domain.ErrInternal)
	}

	s.migrateInlinePhotos(ctx, p)

	return &domain.MyProfileResponse{
		Email: p.Email,
		Name:  p.Name,
		Bio:   p.Bio,
		Photo: p.PhotoURL,
		// Legacy verifications have no scan hash and are reported as unverified,
		// so the client sends those users back through the face scan.
		FaceVerified:  p.FaceVerifiedAt != nil && p.FaceScanHash != "",
		FaceScanPhoto: p.FaceScanPhotoURL,
	}, nil
}

// migrateInlinePhotos moves photos stored inline as base64 data URLs onto real
// storage. Rows written before PHOTO_STORAGE moved off "db" bloat every
// profile and discovery response by megabytes; this heals them on first read
// and is a no-op for every row written afterwards.
func (s *Service) migrateInlinePhotos(ctx context.Context, p *domain.Profile) {
	if s.cfg.PhotoStorage == "db" {
		return
	}

	if strings.HasPrefix(p.PhotoURL, "data:") {
		if url, err := s.SavePhoto(ctx, p.PhotoURL); err == nil && url != "" {
			if err := s.repo.Upsert(ctx, &domain.Profile{Email: p.Email, Name: p.Name, Bio: p.Bio, PhotoURL: url}); err == nil {
				p.PhotoURL = url
			} else {
				s.logger.WarnContext(ctx, "failed to migrate inline profile photo", slog.String("error", err.Error()))
			}
		}
	}

	if strings.HasPrefix(p.FaceScanPhotoURL, "data:") {
		if url, err := s.SavePhoto(ctx, p.FaceScanPhotoURL); err == nil && url != "" {
			if err := s.repo.MarkFaceVerified(ctx, p.Email, url); err == nil {
				p.FaceScanPhotoURL = url
			} else {
				s.logger.WarnContext(ctx, "failed to migrate inline face scan photo", slog.String("error", err.Error()))
			}
		}
	}
}

// faceChallengeTTL is how long a client has to finish a scan once the challenge
// has been issued. Short enough that a pre-recorded clip can't be queued up for
// later, long enough for a user to steady the camera.
const faceChallengeTTL = 3 * time.Minute

// IssueFaceChallenge hands the client a single-use nonce to include with its
// live selfie. Without this, any previously captured scan could be replayed.
func (s *Service) IssueFaceChallenge(ctx context.Context, email string) (*domain.FaceChallengeResponse, error) {
	nonceBytes := make([]byte, 32)
	if _, err := rand.Read(nonceBytes); err != nil {
		return nil, domain.NewAppError(500, "Failed to start face verification. Please try again.", err)
	}
	nonce := hex.EncodeToString(nonceBytes)
	expiresAt := time.Now().Add(faceChallengeTTL)

	// Only the digest is stored, so reading the table never yields a live nonce.
	if err := s.repo.SaveFaceChallenge(ctx, email, hashHex(nonce), expiresAt); err != nil {
		s.logger.ErrorContext(ctx, "failed to save face challenge", slog.String("error", err.Error()))
		return nil, domain.NewAppError(500, "Failed to start face verification. Please try again.", domain.ErrInternal)
	}

	return &domain.FaceChallengeResponse{Success: true, Challenge: nonce, ExpiresAt: expiresAt}, nil
}

// VerifyFaceScan records a successful live face scan server-side. The photo is
// validated as a real, non-blank image of usable resolution, must match the
// challenge this server issued, and is fingerprinted so the same capture cannot
// verify a second account.
func (s *Service) VerifyFaceScan(ctx context.Context, email string, req *domain.VerifyFaceRequest) (*domain.VerifyFaceResponse, error) {
	photo := strings.TrimSpace(req.Photo)
	if photo == "" {
		return nil, domain.NewAppError(400, "Face scan photo is required. Please retake the live scan.", domain.ErrBadRequest)
	}

	challenge := strings.TrimSpace(req.Challenge)
	if challenge == "" {
		return nil, domain.NewAppError(400, "Face scan challenge is missing. Please restart face verification.", domain.ErrBadRequest)
	}

	photoHash, err := storage.ValidateFaceScan(photo)
	if err != nil {
		return nil, domain.NewAppError(400, faceScanMessage(err), err)
	}

	if err := s.repo.SetFaceScanHash(ctx, email, photoHash); err != nil {
		if errors.Is(err, domain.ErrFaceScanReused) {
			return nil, domain.NewAppError(409, "This photo has already verified another account. Please scan your own face live.", domain.ErrFaceScanReused)
		}
		s.logger.ErrorContext(ctx, "failed to record face scan hash", slog.String("error", err.Error()))
		return nil, domain.NewAppError(500, "Failed to record face verification. Please try again.", domain.ErrInternal)
	}

	if err := s.repo.ConsumeFaceChallenge(ctx, email, hashHex(challenge)); err != nil {
		if !errors.Is(err, domain.ErrForbidden) {
			s.logger.ErrorContext(ctx, "failed to consume face challenge", slog.String("error", err.Error()))
		}
		return nil, domain.NewAppError(403, "This scan took too long or was already used. Please scan again.", domain.ErrForbidden)
	}

	photoURL, err := s.SavePhoto(ctx, photo)
	if err != nil {
		return nil, err
	}

	if err := s.repo.MarkFaceVerified(ctx, email, photoURL); err != nil {
		if errors.Is(err, domain.ErrProfileNotFound) {
			return nil, domain.NewAppError(404, "Profile not found. Please sign up first.", domain.ErrProfileNotFound)
		}
		s.logger.ErrorContext(ctx, "failed to record face verification", slog.String("error", err.Error()))
		return nil, domain.NewAppError(500, "Failed to record face verification. Please try again.", domain.ErrInternal)
	}

	s.logger.InfoContext(ctx, "face verification recorded")

	return &domain.VerifyFaceResponse{
		Success: true,
		Message: "Face verified successfully.",
	}, nil
}

// faceScanMessage turns a storage validation failure into something a user can
// act on.
func faceScanMessage(err error) string {
	switch {
	case errors.Is(err, storage.ErrImageTooSmall):
		return "Your scan was too low resolution. Please hold the camera closer and try again."
	case errors.Is(err, storage.ErrImageIsBlank):
		return "The scan looked blank. Please make sure your face is visible and well lit."
	case errors.Is(err, storage.ErrImageTooLarge):
		return "The scan was too large. Please try again."
	default:
		return "We could not read that scan. Please scan your face again."
	}
}

// hashHex is the SHA-256 digest of a challenge nonce, hex encoded. Challenges
// are stored hashed so a leaked row is not a usable nonce.
func hashHex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

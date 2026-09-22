package profile

import (
	"context"
	"errors"
	"html"
	"log/slog"
	"strings"

	"github.com/atharvix/kinjo-backend/internal/config"
	"github.com/atharvix/kinjo-backend/internal/domain"
	"github.com/atharvix/kinjo-backend/internal/storage"
)

type Service struct {
	repo    Repository
	storage storage.Storage
	cfg     *config.Config
	logger  *slog.Logger
}

func NewService(repo Repository, store storage.Storage, cfg *config.Config, logger *slog.Logger) *Service {
	return &Service{
		repo:    repo,
		storage: store,
		cfg:     cfg,
		logger:  logger,
	}
}

// SavePhoto persists a photo according to cfg.PhotoStorage:
//   - "db":       keep the data URL / URL string as-is inside the DB row
//                 (legacy mode; works everywhere, including read-only FS).
//   - "local":    decode base64 data URLs, validate, compress to JPEG and
//                 store on disk under /uploads; DB keeps a relative URL.
//   - "supabase": same decode/validate/compress pipeline, upload to a
//                 Supabase Storage bucket; DB keeps the public URL.
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

	photoURL := ""
	if req.Photo != nil && *req.Photo != "" {
		photoURL, err = s.SavePhoto(ctx, *req.Photo)
		if err != nil {
			return nil, err
		}
	} else if existing, err := s.repo.GetByEmail(ctx, email); err == nil && existing != nil {
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

	return &domain.MyProfileResponse{
		Email:         p.Email,
		Name:          p.Name,
		Bio:           p.Bio,
		Photo:         p.PhotoURL,
		FaceVerified:  p.FaceVerifiedAt != nil,
		FaceScanPhoto: p.FaceScanPhotoURL,
	}, nil
}

// VerifyFaceScan records a successful live face scan server-side. The photo
// is stored according to the same pipeline as profile photos and kept as the
// reference selfie for future photo-match checks.
func (s *Service) VerifyFaceScan(ctx context.Context, email string, req *domain.VerifyFaceRequest) (*domain.VerifyFaceResponse, error) {
	photo := strings.TrimSpace(req.Photo)
	if photo == "" {
		return nil, domain.NewAppError(400, "Face scan photo is required. Please retake the live scan.", domain.ErrBadRequest)
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

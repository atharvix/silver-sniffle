package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"net/http"
	"strings"

	"github.com/atharvix/kinjo-backend/internal/config"
	"github.com/google/uuid"
)

var (
	ErrInvalidImageData  = errors.New("invalid image data")
	ErrImageTooLarge     = errors.New("image exceeds maximum allowed size")
	ErrUnsupportedFormat = errors.New("unsupported image format")
	ErrImageTooSmall     = errors.New("image resolution is too low")
	ErrImageIsBlank      = errors.New("image appears to be a solid colour")
)

// MinFaceScanDimension is the smallest side a live selfie may have. The client
// captures 400x400; anything under this is a thumbnail or an icon, not a face.
const MinFaceScanDimension = 200

// ValidateFaceScan checks that a submitted live-scan payload is a real,
// non-trivial photograph and returns its SHA-256 fingerprint (hex), which the
// caller stores to stop the same capture verifying more than one account.
//
// ponytail: this rejects arbitrary payloads, blank fills and replays, but it
// does NOT detect presentation attacks — a photo of a screen passes. Add a
// liveness vendor (FaceTec/Onfido) when spoofing becomes a real problem.
func ValidateFaceScan(input string) (string, error) {
	raw, _, err := decodeImagePayload(input)
	if err != nil {
		return "", err
	}

	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return "", fmt.Errorf("%w: not a decodable image", ErrInvalidImageData)
	}

	bounds := img.Bounds()
	if bounds.Dx() < MinFaceScanDimension || bounds.Dy() < MinFaceScanDimension {
		return "", fmt.Errorf("%w: scan is %dx%d, need at least %dpx per side",
			ErrImageTooSmall, bounds.Dx(), bounds.Dy(), MinFaceScanDimension)
	}

	if isFlatColour(img) {
		return "", ErrImageIsBlank
	}

	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// isFlatColour samples the image and reports whether it has no meaningful tonal
// range — a solid fill dressed up as a JPEG.
func isFlatColour(img image.Image) bool {
	bounds := img.Bounds()
	minLuma, maxLuma := 255, 0

	stepX := max(bounds.Dx()/32, 1)
	stepY := max(bounds.Dy()/32, 1)

	for y := bounds.Min.Y; y < bounds.Max.Y; y += stepY {
		for x := bounds.Min.X; x < bounds.Max.X; x += stepX {
			r, g, b, _ := img.At(x, y).RGBA()
			luma := int((299*int(r>>8) + 587*int(g>>8) + 114*int(b>>8)) / 1000)
			minLuma = min(minLuma, luma)
			maxLuma = max(maxLuma, luma)
		}
	}

	return maxLuma-minLuma < 25
}

type Storage interface {
	Save(ctx context.Context, data []byte, contentType string) (string, error)
	Delete(ctx context.Context, fileURL string) error
}

// NewFromConfig builds the configured storage backend. The API server and the
// admin CLI both go through here so they can never disagree about where
// uploaded bytes live.
func NewFromConfig(cfg *config.Config) (Storage, error) {
	if (cfg.StorageDriver == "supabase" || cfg.StorageDriver == "") &&
		cfg.SupabaseURL != "" && cfg.SupabaseServiceRoleKey != "" {
		return NewSupabaseStorage(cfg.SupabaseURL, cfg.SupabaseServiceRoleKey, cfg.SupabaseBucket)
	}
	return NewLocalStorage(cfg.StorageDir)
}

// decodeImagePayload turns a data URL (or a bare base64 string) into bytes and
// the declared content type.
func decodeImagePayload(input string) ([]byte, string, error) {
	if strings.HasPrefix(input, "data:") {
		parts := strings.SplitN(input, ",", 2)
		if len(parts) != 2 {
			return nil, "", ErrInvalidImageData
		}

		contentType := strings.TrimPrefix(strings.Split(parts[0], ";")[0], "data:")
		decoded, err := base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			return nil, "", fmt.Errorf("%w: base64 decode failed", ErrInvalidImageData)
		}
		return decoded, contentType, nil
	}

	decoded, err := base64.StdEncoding.DecodeString(input)
	if err != nil {
		return nil, "", fmt.Errorf("%w: not a valid URL or base64 data", ErrInvalidImageData)
	}
	return decoded, "", nil
}

// ProcessImage checks if input is already a URL or a base64 data URL.
// If base64, it decodes, validates format/size, saves to storage, and returns the URL.
func ProcessImage(ctx context.Context, storage Storage, input string, maxBytes int64) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", nil
	}

	// If contains /uploads/, store as relative path so domain changes (ngrok) don't break image URLs
	if idx := strings.Index(input, "/uploads/"); idx != -1 {
		return input[idx:], nil
	}

	// If already a regular URL, return as-is
	if strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://") || strings.HasPrefix(input, "/demo-") {
		return input, nil
	}

	rawData, contentType, err := decodeImagePayload(input)
	if err != nil {
		return "", err
	}

	// Validate size
	if int64(len(rawData)) > maxBytes {
		return "", ErrImageTooLarge
	}

	// Detect and validate MIME type via magic bytes
	detectedType := http.DetectContentType(rawData)
	if !strings.HasPrefix(detectedType, "image/") {
		return "", ErrUnsupportedFormat
	}

	// Verify image decodability & compress for storage efficiency
	img, format, err := image.Decode(bytes.NewReader(rawData))
	if err == nil && (format == "jpeg" || format == "png" || format == "webp" || format == "gif") {
		var compressedBuf bytes.Buffer
		if err := jpeg.Encode(&compressedBuf, img, &jpeg.Options{Quality: 98}); err == nil {
			rawData = compressedBuf.Bytes()
			contentType = "image/jpeg"
		}
	} else if err != nil && detectedType != "image/webp" {
		return "", fmt.Errorf("%w: corrupt image file", ErrInvalidImageData)
	}
	if contentType == "" {
		contentType = detectedType
	}

	// Save to storage
	return storage.Save(ctx, rawData, contentType)
}

func GenerateFilename(contentType string) string {
	ext := ".jpg"
	switch contentType {
	case "image/png":
		ext = ".png"
	case "image/jpeg", "image/jpg":
		ext = ".jpg"
	case "image/webp":
		ext = ".webp"
	case "image/gif":
		ext = ".gif"
	}
	return uuid.NewString() + ext
}

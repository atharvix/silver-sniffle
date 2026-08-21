package storage

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

var (
	ErrInvalidImageData = errors.New("invalid image data")
	ErrImageTooLarge    = errors.New("image exceeds maximum allowed size")
	ErrUnsupportedFormat = errors.New("unsupported image format")
)

type Storage interface {
	Save(ctx context.Context, data []byte, contentType string) (string, error)
	Delete(ctx context.Context, fileURL string) error
}

// ProcessBase64OrURL checks if input is already a URL or a base64 data URL.
// If base64, it decodes, validates format/size, saves to storage, and returns the URL.
func ProcessImage(ctx context.Context, storage Storage, input string, maxBytes int64) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", nil
	}

	// If already a regular URL, return as-is
	if strings.HasPrefix(input, "http://") || strings.HasPrefix(input, "https://") || strings.HasPrefix(input, "/uploads/") || strings.HasPrefix(input, "/demo-") {
		return input, nil
	}

	// Parse Data URL format: data:<mime>;base64,<data>
	var rawData []byte
	var contentType string

	if strings.HasPrefix(input, "data:") {
		parts := strings.SplitN(input, ",", 2)
		if len(parts) != 2 {
			return "", ErrInvalidImageData
		}

		meta := parts[0]
		dataStr := parts[1]

		// Extract content type
		metaParts := strings.Split(meta, ";")
		if len(metaParts) > 0 {
			contentType = strings.TrimPrefix(metaParts[0], "data:")
		}

		decoded, err := base64.StdEncoding.DecodeString(dataStr)
		if err != nil {
			return "", fmt.Errorf("%w: base64 decode failed", ErrInvalidImageData)
		}
		rawData = decoded
	} else {
		// Try raw base64 string
		decoded, err := base64.StdEncoding.DecodeString(input)
		if err != nil {
			return "", fmt.Errorf("%w: not a valid URL or base64 data", ErrInvalidImageData)
		}
		rawData = decoded
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

	// Verify image decodability
	_, format, err := image.DecodeConfig(bytes.NewReader(rawData))
	if err != nil && detectedType != "image/webp" {
		return "", fmt.Errorf("%w: corrupt image file", ErrInvalidImageData)
	}
	if contentType == "" {
		contentType = detectedType
	}
	_ = format

	// Save to storage
	return storage.Save(ctx, rawData, contentType)
}

func GenerateFilename(contentType string) string {
	ext := ".jpg"
	switch contentType {
	case "image/png":
		ext = ".png"
	case "image/webp":
		ext = ".webp"
	case "image/gif":
		ext = ".gif"
	}
	return uuid.NewString() + ext
}

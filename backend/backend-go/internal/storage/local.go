package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// LocalStorage writes uploads under baseDir and stores a relative /uploads/...
// path in the database, so the host serving them stays a deployment concern
// rather than something baked into stored rows.
type LocalStorage struct {
	baseDir string
}

func NewLocalStorage(baseDir string) (*LocalStorage, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create upload directory: %w", err)
	}

	return &LocalStorage{baseDir: baseDir}, nil
}

func (s *LocalStorage) Save(ctx context.Context, data []byte, contentType string) (string, error) {
	filename := GenerateFilename(contentType)
	fullPath := filepath.Join(s.baseDir, filename)

	if err := os.WriteFile(fullPath, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write local file: %w", err)
	}

	return fmt.Sprintf("/uploads/%s", filename), nil
}

func (s *LocalStorage) Delete(ctx context.Context, fileURL string) error {
	filename := filepath.Base(fileURL)
	fullPath := filepath.Join(s.baseDir, filename)
	return os.Remove(fullPath)
}

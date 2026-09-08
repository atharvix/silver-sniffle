package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type SupabaseStorage struct {
	supabaseURL    string
	serviceRoleKey string
	bucket         string
	client         *http.Client
}

func NewSupabaseStorage(supabaseURL, serviceRoleKey, bucket string) (*SupabaseStorage, error) {
	if bucket == "" {
		bucket = "profiles"
	}
	supabaseURL = strings.TrimRight(supabaseURL, "/")
	s := &SupabaseStorage{
		supabaseURL:    supabaseURL,
		serviceRoleKey: serviceRoleKey,
		bucket:         bucket,
		client:         &http.Client{Timeout: 30 * time.Second},
	}
	if err := s.ensureBucket(context.Background()); err != nil {
		fmt.Printf("Warning: failed to ensure Supabase bucket exists: %v\n", err)
	}
	return s, nil
}

func (s *SupabaseStorage) ensureBucket(ctx context.Context) error {
	createBucketURL := fmt.Sprintf("%s/storage/v1/bucket", s.supabaseURL)
	payload := fmt.Sprintf(`{"id":"%s","name":"%s","public":true}`, s.bucket, s.bucket)
	req, err := http.NewRequestWithContext(ctx, "POST", createBucketURL, strings.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.serviceRoleKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (s *SupabaseStorage) Save(ctx context.Context, data []byte, contentType string) (string, error) {
	filename := GenerateFilename(contentType)
	uploadURL := fmt.Sprintf("%s/storage/v1/object/%s/%s", s.supabaseURL, s.bucket, filename)

	req, err := http.NewRequestWithContext(ctx, "POST", uploadURL, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("failed to create Supabase upload request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+s.serviceRoleKey)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("x-upsert", "true")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("Supabase upload request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Supabase storage upload error (%d): %s", resp.StatusCode, string(body))
	}

	publicURL := fmt.Sprintf("%s/storage/v1/object/public/%s/%s", s.supabaseURL, s.bucket, strings.TrimPrefix(filename, "/"))
	return publicURL, nil
}

func (s *SupabaseStorage) Delete(ctx context.Context, fileURL string) error {
	idx := strings.Index(fileURL, "/object/public/"+s.bucket+"/")
	if idx == -1 {
		return nil
	}
	filename := fileURL[idx+len("/object/public/"+s.bucket+"/"):]

	deleteURL := fmt.Sprintf("%s/storage/v1/object/%s/%s", s.supabaseURL, s.bucket, filename)
	req, err := http.NewRequestWithContext(ctx, "DELETE", deleteURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create Supabase delete request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+s.serviceRoleKey)
	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("Supabase delete request failed: %w", err)
	}
	defer resp.Body.Close()
	return nil
}

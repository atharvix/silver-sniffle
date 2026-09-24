package notification

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type FCMClient interface {
	Send(ctx context.Context, token, title, body string, data map[string]string) error
	IsConfigured() bool
}

type GoogleServiceAccount struct {
	Type         string `json:"type"`
	ProjectID    string `json:"project_id"`
	PrivateKeyID string `json:"private_key_id"`
	PrivateKey   string `json:"private_key"`
	ClientEmail  string `json:"client_email"`
	ClientID     string `json:"client_id"`
	TokenURI     string `json:"token_uri"`
}

type FCMService struct {
	projectID string
	sa        *GoogleServiceAccount
	parsedKey *rsa.PrivateKey
	serverKey string // optional legacy fallback
	client    *http.Client
	logger    *slog.Logger

	mu          sync.RWMutex
	accessToken string
	tokenExpiry time.Time
}

func NewFCMService(projectID, accountKey, serverKey string, logger *slog.Logger) *FCMService {
	svc := &FCMService{
		projectID: strings.TrimSpace(projectID),
		serverKey: strings.TrimSpace(serverKey),
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger: logger,
	}

	trimmedKey := strings.TrimSpace(accountKey)
	if trimmedKey != "" {
		var saData []byte
		if strings.HasPrefix(trimmedKey, "{") {
			saData = []byte(trimmedKey)
		} else {
			// Try reading as file path
			content, err := os.ReadFile(trimmedKey)
			if err == nil {
				saData = content
			} else {
				logger.Warn("failed to read FCM service account key file", slog.String("path", trimmedKey), slog.String("error", err.Error()))
			}
		}

		if len(saData) > 0 {
			var sa GoogleServiceAccount
			if err := json.Unmarshal(saData, &sa); err == nil {
				svc.sa = &sa
				if svc.projectID == "" {
					svc.projectID = sa.ProjectID
				}
				if privKey, err := parseRSAPrivateKey(sa.PrivateKey); err == nil {
					svc.parsedKey = privKey
					logger.Info("FCM service account initialized successfully", slog.String("project_id", svc.projectID), slog.String("client_email", sa.ClientEmail))
				} else {
					logger.Error("failed to parse FCM RSA private key", slog.String("error", err.Error()))
				}
			} else {
				logger.Error("failed to parse FCM service account JSON", slog.String("error", err.Error()))
			}
		}
	}

	return svc
}

// maskToken keeps device registration tokens out of logs in full.
func maskToken(token string) string {
	if len(token) <= 12 {
		return "***"
	}
	return token[:6] + "…" + token[len(token)-4:]
}

func (s *FCMService) IsConfigured() bool {
	return (s.parsedKey != nil && s.projectID != "" && s.sa != nil) || s.serverKey != ""
}

func (s *FCMService) Send(ctx context.Context, token, title, body string, data map[string]string) error {
	if !s.IsConfigured() {
		s.logger.InfoContext(ctx, "[FCM Mock] Push notification dispatched (credentials not configured)",
			slog.String("token", maskToken(token)),
			slog.String("title", title),
			slog.String("body", body),
		)
		return nil
	}

	// 1. Prefer FCM HTTP v1 API
	if s.parsedKey != nil && s.projectID != "" {
		return s.sendHTTPv1(ctx, token, title, body, data)
	}

	// 2. Legacy FCM server key fallback
	if s.serverKey != "" {
		return s.sendLegacy(ctx, token, title, body, data)
	}

	return fmt.Errorf("fcm is not properly configured")
}

func (s *FCMService) sendHTTPv1(ctx context.Context, token, title, body string, data map[string]string) error {
	accessToken, err := s.getAccessToken(ctx)
	if err != nil {
		return fmt.Errorf("failed to obtain FCM access token: %w", err)
	}

	url := fmt.Sprintf("https://fcm.googleapis.com/v1/projects/%s/messages:send", s.projectID)

	messagePayload := map[string]any{
		"token": token,
		"notification": map[string]string{
			"title": title,
			"body":  body,
		},
	}
	if len(data) > 0 {
		messagePayload["data"] = data
	}

	reqBody, err := json.Marshal(map[string]any{
		"message": messagePayload,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal FCM payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("failed to create FCM request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("fcm http request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		s.logger.InfoContext(ctx, "FCM push notification sent successfully", slog.String("token", maskToken(token)))
		return nil
	}

	respBytes, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("fcm returned status %d: %s", resp.StatusCode, string(respBytes))
}

func (s *FCMService) sendLegacy(ctx context.Context, token, title, body string, data map[string]string) error {
	payload := map[string]any{
		"to": token,
		"notification": map[string]string{
			"title": title,
			"body":  body,
		},
	}
	if len(data) > 0 {
		payload["data"] = data
	}

	reqBody, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal legacy FCM payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://fcm.googleapis.com/fcm/send", bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("failed to create legacy FCM request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "key="+s.serverKey)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("legacy fcm request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		s.logger.InfoContext(ctx, "Legacy FCM push notification sent successfully", slog.String("token", maskToken(token)))
		return nil
	}

	respBytes, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("legacy fcm returned status %d: %s", resp.StatusCode, string(respBytes))
}

func (s *FCMService) getAccessToken(ctx context.Context) (string, error) {
	s.mu.RLock()
	if s.accessToken != "" && time.Now().Before(s.tokenExpiry) {
		token := s.accessToken
		s.mu.RUnlock()
		return token, nil
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	// Double-check under write lock
	if s.accessToken != "" && time.Now().Before(s.tokenExpiry) {
		return s.accessToken, nil
	}

	now := time.Now()
	tokenURI := s.sa.TokenURI
	if tokenURI == "" {
		tokenURI = "https://oauth2.googleapis.com/token"
	}

	// Create JWT Header and Claims
	headerJSON := `{"alg":"RS256","typ":"JWT"}`
	claimsJSON := fmt.Sprintf(`{"iss":%q,"scope":"https://www.googleapis.com/auth/firebase.messaging","aud":%q,"exp":%d,"iat":%d}`,
		s.sa.ClientEmail,
		tokenURI,
		now.Add(1*time.Hour).Unix(),
		now.Unix(),
	)

	encHeader := base64.RawURLEncoding.EncodeToString([]byte(headerJSON))
	encClaims := base64.RawURLEncoding.EncodeToString([]byte(claimsJSON))
	signingInput := encHeader + "." + encClaims

	hashed := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, s.parsedKey, crypto.SHA256, hashed[:])
	if err != nil {
		return "", fmt.Errorf("failed to sign JWT: %w", err)
	}

	signedJWT := signingInput + "." + base64.RawURLEncoding.EncodeToString(signature)

	// Exchange JWT for access token
	form := strings.NewReader("grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Ajwt-bearer&assertion=" + signedJWT)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURI, form)
	if err != nil {
		return "", fmt.Errorf("failed to create oauth request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to exchange JWT for oauth token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("oauth token request returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("failed to decode oauth response: %w", err)
	}

	s.accessToken = tokenResp.AccessToken
	// Buffer expiry by 5 minutes
	expirySec := tokenResp.ExpiresIn
	if expirySec <= 0 {
		expirySec = 3600
	}
	s.tokenExpiry = now.Add(time.Duration(expirySec-300) * time.Second)

	return s.accessToken, nil
}

func parseRSAPrivateKey(keyPEM string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(keyPEM))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block for private key")
	}

	// Try PKCS8 first (standard for Google service accounts)
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err == nil {
		if rsaKey, ok := key.(*rsa.PrivateKey); ok {
			return rsaKey, nil
		}
		return nil, fmt.Errorf("PKCS8 key is not an RSA private key")
	}

	// Try PKCS1
	rsaKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err == nil {
		return rsaKey, nil
	}

	return nil, fmt.Errorf("failed to parse private key as PKCS8 or PKCS1: %w", err)
}

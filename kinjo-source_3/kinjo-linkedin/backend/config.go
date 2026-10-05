package main

import (
	"fmt"
	"os"
	"strings"
)

// Config is loaded entirely from environment variables so no secrets live in
// the repo. On Utho these go in the systemd unit / .env file (see README).
type Config struct {
	Port         string
	DatabaseURL  string
	FrontendURL  string // where the SPA is served; used for OAuth redirect-back + CORS
	AllowOrigins []string

	LinkedInClientID     string
	LinkedInClientSecret string
	LinkedInRedirectURL  string // must exactly match the redirect registered on LinkedIn

	// Shared secret for the admin panel. If unset, the whole /admin API is off.
	AdminToken string

	// Optional SMTP for verification emails. If unset, the backend logs the
	// verification link instead of sending it (fine for development).
	SMTPHost string
	SMTPPort string
	SMTPUser string
	SMTPPass string
	SMTPFrom string

	// Optional FCM (Firebase Cloud Messaging) for background push. If unset,
	// notifications are still stored + delivered live over the WebSocket, but
	// not pushed to closed apps. Provide a service-account key as inline JSON
	// or a file path; the project id is read from it if not set explicitly.
	FCMProjectID       string
	FCMCredentialsJSON string
	FCMCredentialsFile string
}

func (c Config) smtpConfigured() bool {
	return c.SMTPHost != "" && c.SMTPFrom != ""
}

func (c Config) fcmCredentials() ([]byte, error) {
	if c.FCMCredentialsJSON != "" {
		return []byte(c.FCMCredentialsJSON), nil
	}
	if c.FCMCredentialsFile != "" {
		return os.ReadFile(c.FCMCredentialsFile)
	}
	return nil, nil // not configured
}

func loadConfig() (Config, error) {
	c := Config{
		Port:                 envOr("PORT", "8080"),
		DatabaseURL:          os.Getenv("DATABASE_URL"),
		FrontendURL:          envOr("FRONTEND_URL", "kinjo://auth"),
		LinkedInClientID:     os.Getenv("LINKEDIN_CLIENT_ID"),
		LinkedInClientSecret: os.Getenv("LINKEDIN_CLIENT_SECRET"),
		LinkedInRedirectURL:  os.Getenv("LINKEDIN_REDIRECT_URL"),
		AdminToken:           os.Getenv("ADMIN_TOKEN"),
		SMTPHost:             os.Getenv("SMTP_HOST"),
		SMTPPort:             envOr("SMTP_PORT", "587"),
		SMTPUser:             os.Getenv("SMTP_USER"),
		SMTPPass:             os.Getenv("SMTP_PASS"),
		SMTPFrom:             os.Getenv("SMTP_FROM"),
		FCMProjectID:         os.Getenv("FCM_PROJECT_ID"),
		FCMCredentialsJSON:   os.Getenv("FCM_SERVICE_ACCOUNT_JSON"),
		FCMCredentialsFile:   os.Getenv("FCM_CREDENTIALS_FILE"),
	}

	// CORS allow-list (comma-separated). Defaults to the Capacitor Android
	// webview origin, since the bundled app is the primary client. Add a website
	// origin here too if you serve one.
	origins := os.Getenv("ALLOW_ORIGINS")
	if origins == "" {
		origins = "https://localhost"
	}
	for _, o := range strings.Split(origins, ",") {
		if o = strings.TrimSpace(o); o != "" {
			c.AllowOrigins = append(c.AllowOrigins, o)
		}
	}

	// Fail fast on missing required config rather than 500-ing at request time.
	var missing []string
	if c.DatabaseURL == "" {
		missing = append(missing, "DATABASE_URL")
	}
	if c.LinkedInClientID == "" {
		missing = append(missing, "LINKEDIN_CLIENT_ID")
	}
	if c.LinkedInClientSecret == "" {
		missing = append(missing, "LINKEDIN_CLIENT_SECRET")
	}
	if c.LinkedInRedirectURL == "" {
		missing = append(missing, "LINKEDIN_REDIRECT_URL")
	}
	if len(missing) > 0 {
		return c, fmt.Errorf("missing required env: %s", strings.Join(missing, ", "))
	}
	return c, nil
}

func (c Config) originAllowed(origin string) bool {
	for _, o := range c.AllowOrigins {
		if o == origin || o == "*" {
			return true
		}
	}
	return false
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

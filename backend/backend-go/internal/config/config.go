package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	// Server
	Port            int           `json:"port"`
	Environment     string        `json:"environment"`
	LogLevel        string        `json:"log_level"`
	ReadTimeout     time.Duration `json:"read_timeout"`
	WriteTimeout    time.Duration `json:"write_timeout"`
	IdleTimeout     time.Duration `json:"idle_timeout"`
	ShutdownTimeout time.Duration `json:"shutdown_timeout"`

	// Database
	DatabaseURL     string        `json:"database_url"`
	DBMaxConns      int32         `json:"db_max_conns"`
	DBMinConns      int32         `json:"db_min_conns"`
	DBMaxConnIdle   time.Duration `json:"db_max_conn_idle"`
	DBMaxConnLife   time.Duration `json:"db_max_conn_life"`
	DBHealthTimeout time.Duration `json:"db_health_timeout"`

	// Security & Auth
	AllowedOrigins []string      `json:"allowed_origins"`
	TokenTTL       time.Duration `json:"token_ttl"`
	OtpTTL         time.Duration `json:"otp_ttl"`
	MaxOtpAttempts int           `json:"max_otp_attempts"`
	RateLimitEmail int           `json:"rate_limit_email"`
	RateLimitIP    int           `json:"rate_limit_ip"`
	PresenceTTL    time.Duration `json:"presence_ttl"`
	MaxPhotoBytes  int64         `json:"max_photo_bytes"`
	AESEncryptionKey string      `json:"-"`

	// External Services - Email (SMTP)
	SMTPHost        string `json:"smtp_host"`
	SMTPPort        int    `json:"smtp_port"`
	SMTPUsername    string `json:"smtp_username"`
	SMTPPassword    string `json:"-"`
	SMTPSenderEmail string `json:"smtp_sender_email"`
	SMTPSenderName  string `json:"smtp_sender_name"`
	SMTPEncryption  string `json:"smtp_encryption"` // "tls", "ssl", or "none"

	// External Services - Push Notifications (FCM)
	FCMProjectID  string `json:"fcm_project_id"`
	FCMAccountKey string `json:"-"` // Path to service account JSON or raw JSON
	FCMServerKey  string `json:"-"` // Optional legacy server key fallback

	GoogleClientID     string `json:"google_client_id"`
	GoogleClientSecret string `json:"google_client_secret"`
	OpenAIAPIKey       string `json:"-"`
	OpenAIBaseURL      string `json:"openai_base_url"`

	// Storage
	StorageDriver          string `json:"storage_driver"` // "local", "supabase", or "s3"
	StorageDir             string `json:"storage_dir"`
	BaseURL                string `json:"base_url"`
	SupabaseURL            string `json:"supabase_url"`
	SupabaseServiceRoleKey string `json:"-"`
	SupabaseBucket         string `json:"supabase_bucket"`

	// PhotoStorage controls where profile/face photos are persisted:
	// "db" (default, base64 inline), "local" (disk /uploads), "supabase".
	PhotoStorage string `json:"photo_storage"`
}

func Load() (*Config, error) {
	cfg := &Config{
		Port:            getEnvInt("PORT", 8080),
		Environment:     getEnv("NODE_ENV", getEnv("ENVIRONMENT", "development")),
		LogLevel:        getEnv("LOG_LEVEL", "info"),
		ReadTimeout:     getEnvDuration("HTTP_READ_TIMEOUT", 10*time.Second),
		WriteTimeout:    getEnvDuration("HTTP_WRITE_TIMEOUT", 30*time.Second),
		IdleTimeout:     getEnvDuration("HTTP_IDLE_TIMEOUT", 120*time.Second),
		ShutdownTimeout: getEnvDuration("SHUTDOWN_TIMEOUT", 10*time.Second),

		DatabaseURL:     getEnv("DATABASE_URL", "postgres://postgres@localhost:5432/kinjo?sslmode=disable"),
		DBMaxConns:      int32(getEnvInt("DB_MAX_CONNS", 25)),
		DBMinConns:      int32(getEnvInt("DB_MIN_CONNS", 5)),
		DBMaxConnIdle:   getEnvDuration("DB_MAX_CONN_IDLE", 5*time.Minute),
		DBMaxConnLife:   getEnvDuration("DB_MAX_CONN_LIFE", 1*time.Hour),
		DBHealthTimeout: getEnvDuration("DB_HEALTH_TIMEOUT", 2*time.Second),

		AllowedOrigins: parseCommaSeparated(getEnv("ALLOWED_ORIGINS", "*")),
		TokenTTL:       getEnvDuration("TOKEN_TTL", 30*24*time.Hour),
		OtpTTL:         getEnvDuration("OTP_TTL", 10*time.Minute),
		MaxOtpAttempts: getEnvInt("MAX_OTP_ATTEMPTS", 5),
		RateLimitEmail: getEnvInt("RATE_LIMIT_EMAIL", 3), // max 3 per 10 mins
		RateLimitIP:    getEnvInt("RATE_LIMIT_IP", 10),   // max 10 per 1 min
		PresenceTTL:    getEnvDuration("PRESENCE_TTL", 30*24*time.Hour),
		MaxPhotoBytes:  int64(getEnvInt("MAX_PHOTO_BYTES", 8*1024*1024)), // 8 MB
		AESEncryptionKey: getEnv("AES_ENCRYPTION_KEY", "kinjo-master-aes-encryption-key-256bit-default-secret-key"),

		SMTPHost:        getEnv("SMTP_HOST", ""),
		SMTPPort:        getEnvInt("SMTP_PORT", 587),
		SMTPUsername:    getEnv("SMTP_USERNAME", getEnv("SMTP_USER", "")),
		SMTPPassword:    getEnv("SMTP_PASSWORD", getEnv("SMTP_PASS", "")),
		SMTPSenderEmail: getEnv("SMTP_SENDER_EMAIL", getEnv("SMTP_FROM_EMAIL", "hello@kinjo.world")),
		SMTPSenderName:  getEnv("SMTP_SENDER_NAME", "Kinjo"),
		SMTPEncryption:  getEnv("SMTP_ENCRYPTION", "tls"),

		FCMProjectID:  getEnv("FCM_PROJECT_ID", ""),
		FCMAccountKey: getEnv("FCM_SERVICE_ACCOUNT_KEY", getEnv("GOOGLE_APPLICATION_CREDENTIALS", "")),
		FCMServerKey:  getEnv("FCM_SERVER_KEY", ""),

		GoogleClientID:     getEnv("GOOGLE_CLIENT_ID", "599627705479-os5q2be0jnrjcbftfkatv75nd5idmhsk.apps.googleusercontent.com"),
		GoogleClientSecret: getEnv("GOOGLE_CLIENT_SECRET", ""),
		OpenAIAPIKey:       getEnv("OPENAI_API_KEY", getEnv("AI_INTEGRATIONS_OPENAI_API_KEY", "")),
		OpenAIBaseURL:      getEnv("AI_INTEGRATIONS_OPENAI_BASE_URL", "https://api.openai.com/v1"),

		StorageDriver:          getEnv("STORAGE_DRIVER", "supabase"),
		StorageDir:             getEnv("STORAGE_DIR", "./uploads"),
		BaseURL:                getEnv("BASE_URL", "https://kinjo.world"),
		SupabaseURL:            getEnv("SUPABASE_URL", ""),
		SupabaseServiceRoleKey: getEnv("SUPABASE_SERVICE_ROLE_KEY", ""),
		SupabaseBucket:         getEnv("SUPABASE_BUCKET", "profiles"),

		PhotoStorage: getEnv("PHOTO_STORAGE", "db"),
	}

	if cfg.IsProduction() {
		if cfg.DatabaseURL == "" {
			return nil, fmt.Errorf("DATABASE_URL is required in production")
		}
		if cfg.AESEncryptionKey == "kinjo-master-aes-encryption-key-256bit-default-secret-key" || len(cfg.AESEncryptionKey) < 32 {
			return nil, fmt.Errorf("AES_ENCRYPTION_KEY must be set to a unique 32+ char secret in production")
		}
		if cfg.AllowedOrigins[0] == "*" {
			cfg.AllowedOrigins = []string{"https://kinjo.world", "https://www.kinjo.world"}
		}
	}

	return cfg, nil
}

func (c *Config) IsProduction() bool {
	return strings.ToLower(c.Environment) == "production"
}

func getEnv(key, defaultVal string) string {
	if val, ok := os.LookupEnv(key); ok && val != "" {
		return val
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val, ok := os.LookupEnv(key); ok {
		if intVal, err := strconv.Atoi(val); err == nil {
			return intVal
		}
	}
	return defaultVal
}

func getEnvDuration(key string, defaultVal time.Duration) time.Duration {
	if val, ok := os.LookupEnv(key); ok {
		if d, err := time.ParseDuration(val); err == nil {
			return d
		}
	}
	return defaultVal
}

func parseCommaSeparated(val string) []string {
	if val == "" {
		return nil
	}
	parts := strings.Split(val, ",")
	res := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			res = append(res, trimmed)
		}
	}
	return res
}

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

	// External Services
	BrevoAPIKey     string `json:"-"`
	BrevoSenderMail string `json:"brevo_sender_email"`
	OpenAIAPIKey    string `json:"-"`
	OpenAIBaseURL   string `json:"openai_base_url"`

	// Storage
	StorageDriver string `json:"storage_driver"` // "local" or "s3"
	StorageDir    string `json:"storage_dir"`
	BaseURL       string `json:"base_url"`
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

		DatabaseURL:     getEnv("DATABASE_URL", ""),
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
		PresenceTTL:    getEnvDuration("PRESENCE_TTL", 20*time.Second),
		MaxPhotoBytes:  int64(getEnvInt("MAX_PHOTO_BYTES", 8*1024*1024)), // 8 MB

		BrevoAPIKey:     getEnv("BREVO_API_KEY", ""),
		BrevoSenderMail: getEnv("BREVO_SENDER_EMAIL", "hello@kinjo.world"),
		OpenAIAPIKey:    getEnv("OPENAI_API_KEY", getEnv("AI_INTEGRATIONS_OPENAI_API_KEY", "")),
		OpenAIBaseURL:   getEnv("AI_INTEGRATIONS_OPENAI_BASE_URL", "https://api.openai.com/v1"),

		StorageDriver: getEnv("STORAGE_DRIVER", "local"),
		StorageDir:    getEnv("STORAGE_DIR", "./uploads"),
		BaseURL:       getEnv("BASE_URL", "https://kinjo.world"),
	}

	if cfg.IsProduction() {
		if cfg.DatabaseURL == "" {
			return nil, fmt.Errorf("DATABASE_URL is required in production")
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

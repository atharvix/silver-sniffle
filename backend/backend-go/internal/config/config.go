package config

import (
	"fmt"
	"os"
	"slices"
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

	// NearbyRadiusMeters is how far apart two people can be and still see each
	// other. Kept in config because real GPS drifts 15-40m indoors.
	NearbyRadiusMeters float64 `json:"nearby_radius_meters"`

	// MetricsToken gates /metrics. Empty disables the endpoint entirely.
	MetricsToken string `json:"-"`

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

	// AdminEmails lists the accounts allowed to use privileged endpoints such
	// as broadcast push notifications. Empty means the endpoint is disabled.
	AdminEmails []string `json:"admin_emails"`

	GoogleClientID     string `json:"google_client_id"`
	GoogleClientSecret string `json:"google_client_secret"`

	// Storage
	StorageDriver          string `json:"storage_driver"` // "local" or "supabase"
	StorageDir             string `json:"storage_dir"`
	SupabaseURL            string `json:"supabase_url"`
	SupabaseServiceRoleKey string `json:"-"`
	SupabaseBucket         string `json:"supabase_bucket"`

	// PhotoStorage controls where profile/face photos are persisted:
	// "db" (base64 inline), "local" (disk /uploads), "supabase". Defaults to
	// StorageDriver when that driver is usable, otherwise "db".
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

		NearbyRadiusMeters: getEnvFloat("NEARBY_RADIUS_METERS", 30),
		MetricsToken:       getEnv("METRICS_TOKEN", ""),

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

		AdminEmails: parseCommaSeparated(getEnv("ADMIN_EMAILS", "")),

		GoogleClientID:     getEnv("GOOGLE_CLIENT_ID", "469545347988-vsu4c3rvqh6tcelvm8c1sce13ea5dopc.apps.googleusercontent.com"),
		GoogleClientSecret: getEnv("GOOGLE_CLIENT_SECRET", ""),

		StorageDriver:          getEnv("STORAGE_DRIVER", "supabase"),
		StorageDir:             getEnv("STORAGE_DIR", "./uploads"),
		SupabaseURL:            getEnv("SUPABASE_URL", ""),
		SupabaseServiceRoleKey: getEnv("SUPABASE_SERVICE_ROLE_KEY", ""),
		SupabaseBucket:         getEnv("SUPABASE_BUCKET", "profiles"),
	}

	// Photos default to the configured storage driver when it can store them,
	// and to inline "db" otherwise. Keeping photos out of the database matters:
	// base64 rows turn every profile and discovery response into multiple MB.
	cfg.PhotoStorage = getEnv("PHOTO_STORAGE", "")
	if cfg.PhotoStorage == "" {
		switch {
		case cfg.StorageDriver == "local":
			cfg.PhotoStorage = "local"
		case cfg.StorageDriver == "supabase" && cfg.SupabaseServiceRoleKey != "":
			cfg.PhotoStorage = "supabase"
		default:
			cfg.PhotoStorage = "db"
		}
	}

	if cfg.IsProduction() {
		if cfg.DatabaseURL == "" {
			return nil, fmt.Errorf("DATABASE_URL is required in production")
		}
	}

	cfg.AllowedOrigins = withFirstPartyOrigins(cfg.AllowedOrigins)

	return cfg, nil
}

// withFirstPartyOrigins guarantees the origins the app itself runs from are
// allowed. Capacitor ships as https://localhost on Android, so a .env that
// lists only a dev tunnel would otherwise block every request from the
// installed app. "*" is left alone.
func withFirstPartyOrigins(origins []string) []string {
	if slices.Contains(origins, "*") {
		return origins
	}

	for _, origin := range []string{
		"capacitor://localhost",
		"https://localhost",
		"https://kinjo.world",
		"https://www.kinjo.world",
	} {
		if !slices.Contains(origins, origin) {
			origins = append(origins, origin)
		}
	}
	return origins
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

func getEnvFloat(key string, defaultVal float64) float64 {
	if val, ok := os.LookupEnv(key); ok {
		if f, err := strconv.ParseFloat(val, 64); err == nil && f > 0 {
			return f
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

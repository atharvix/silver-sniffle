package observability

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"
)

type contextKey string

const (
	RequestIDKey contextKey = "request_id"
	EmailKey     contextKey = "user_email"
)

func NewLogger(level string, isProduction bool) *slog.Logger {
	var logLevel slog.Level
	switch strings.ToLower(level) {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: logLevel,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// Redact sensitive keys
			key := strings.ToLower(a.Key)
			if key == "authorization" || key == "token" || key == "otp" || key == "password" || key == "cookie" || key == "set-cookie" || key == "api-key" {
				return slog.String(a.Key, "[REDACTED]")
			}
			return a
		},
	}

	var handler slog.Handler
	if isProduction {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	return slog.New(&ContextHandler{Handler: handler})
}

// ContextHandler automatically extracts request_id and email from context
type ContextHandler struct {
	slog.Handler
}

func (h *ContextHandler) Handle(ctx context.Context, r slog.Record) error {
	if reqID, ok := ctx.Value(RequestIDKey).(string); ok && reqID != "" {
		r.AddAttrs(slog.String("request_id", reqID))
	}
	if email, ok := ctx.Value(EmailKey).(string); ok && email != "" {
		r.AddAttrs(slog.String("email", email))
	}
	return h.Handler.Handle(ctx, r)
}

func NewNopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

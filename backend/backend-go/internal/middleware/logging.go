package middleware

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/atharvix/kinjo-backend/internal/observability"
)

type responseWriterWrapper struct {
	http.ResponseWriter
	statusCode int
	bytesWritten int64
}

func (rw *responseWriterWrapper) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriterWrapper) Write(b []byte) (int, error) {
	if rw.statusCode == 0 {
		rw.statusCode = http.StatusOK
	}
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesWritten += int64(n)
	return n, err
}

func Logger(logger *slog.Logger, metrics *observability.Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			wrapper := &responseWriterWrapper{ResponseWriter: w, statusCode: http.StatusOK}

			next.ServeHTTP(wrapper, r)

			duration := time.Since(start)
			path := r.URL.Path

			// Log request completion
			logger.InfoContext(r.Context(), "http_request",
				slog.String("method", r.Method),
				slog.String("path", path),
				slog.Int("status", wrapper.statusCode),
				slog.Duration("duration", duration),
				slog.Int64("bytes", wrapper.bytesWritten),
				slog.String("remote_addr", r.RemoteAddr),
				slog.String("user_agent", r.UserAgent()),
			)

			// Record Prometheus metrics if configured
			if metrics != nil && path != "/metrics" && path != "/api/healthz" && path != "/api/readyz" {
				metrics.HTTPRequestsTotal.WithLabelValues(r.Method, path, strconv.Itoa(wrapper.statusCode)).Inc()
				metrics.HTTPRequestDuration.WithLabelValues(r.Method, path).Observe(duration.Seconds())
			}
		})
	}
}

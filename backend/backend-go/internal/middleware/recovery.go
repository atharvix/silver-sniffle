package middleware

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
)

func Recovery(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rvr := recover(); rvr != nil {
					stack := string(debug.Stack())
					logger.ErrorContext(r.Context(), "panic recovered in HTTP handler",
						slog.Any("error", rvr),
						slog.String("stack", stack),
						slog.String("url", r.URL.String()),
					)

					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					_ = json.NewEncoder(w).Encode(map[string]string{
						"error": fmt.Sprintf("Internal Server Error: %v", rvr),
					})
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}

package middleware

import (
	"encoding/json"
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

					// The panic value and stack are logged server-side only. Echoing
					// them to the client leaks internal types, paths and state.
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusInternalServerError)
					_ = json.NewEncoder(w).Encode(map[string]string{
						"error": "Something went wrong. Please try again.",
					})
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}

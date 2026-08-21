package middleware

import (
	"net/http"
	"strings"
)

func SecurityHeaders(isProduction bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Canonical domain redirect for Replit
			host := r.Host
			if isProduction && strings.HasSuffix(host, ".replit.app") {
				target := "https://kinjo.world" + r.RequestURI
				http.Redirect(w, r, target, http.StatusMovedPermanently)
				return
			}

			// Security headers
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
			w.Header().Set("X-XSS-Protection", "1; mode=block")

			if isProduction {
				w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}

			next.ServeHTTP(w, r)
		})
	}
}

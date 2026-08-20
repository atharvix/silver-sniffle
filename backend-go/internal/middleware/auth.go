package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/atharvix/kinjo-backend/internal/domain"
	"github.com/atharvix/kinjo-backend/internal/observability"
)

type TokenValidator interface {
	GetEmailFromToken(ctx context.Context, token string) (string, error)
}

func RequireAuth(validator TokenValidator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			token := ""
			if strings.HasPrefix(authHeader, "Bearer ") {
				token = strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
			}

			if token == "" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error": "Authorization token required.",
				})
				return
			}

			email, err := validator.GetEmailFromToken(r.Context(), token)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				msg := "Verification token is invalid or has expired. Please verify your email again."
				if errors.Is(err, domain.ErrTokenExpired) {
					msg = "Verification token is invalid or has expired. Please verify your email again."
				}
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error": msg,
				})
				return
			}

			ctx := context.WithValue(r.Context(), observability.EmailKey, email)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func GetUserEmail(ctx context.Context) (string, bool) {
	email, ok := ctx.Value(observability.EmailKey).(string)
	return email, ok && email != ""
}

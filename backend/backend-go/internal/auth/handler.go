package auth

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/atharvix/kinjo-backend/internal/domain"
	"github.com/atharvix/kinjo-backend/internal/middleware"
	"github.com/atharvix/kinjo-backend/internal/observability"
)

type Handler struct {
	service           *Service
	emailLimiter      *middleware.RateLimiter
	ipLimiter         *middleware.RateLimiter
	welcomeLimiter    *middleware.RateLimiter
	checkEmailLimiter *middleware.RateLimiter
	metrics           *observability.Metrics
}

func NewHandler(service *Service, metrics *observability.Metrics) *Handler {
	// Honour the RATE_LIMIT_EMAIL / RATE_LIMIT_IP environment configuration;
	// fall back to defaults only when they are unset or invalid.
	emailLimit := 3
	ipLimit := 10
	if service != nil && service.cfg != nil {
		if service.cfg.RateLimitEmail > 0 {
			emailLimit = service.cfg.RateLimitEmail
		}
		if service.cfg.RateLimitIP > 0 {
			ipLimit = service.cfg.RateLimitIP
		}
	}

	return &Handler{
		service:      service,
		emailLimiter: middleware.NewRateLimiter(10*time.Minute, emailLimit, metrics), // sends per 10 mins per email
		ipLimiter:    middleware.NewRateLimiter(1*time.Minute, ipLimit, metrics),     // sends per 1 min per IP
		// Generous per-IP cap that still blunts scripted email enumeration.
		checkEmailLimiter: middleware.NewRateLimiter(1*time.Minute, 30, metrics),
		welcomeLimiter:    middleware.NewRateLimiter(1*time.Hour, 2, metrics), // 2 welcomes per hour per email
		metrics:           metrics,
	}
}

func (h *Handler) SendOTP(w http.ResponseWriter, r *http.Request) {
	var req domain.SendOTPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	cleanEmail, err := ValidateEmail(req.Email)
	if err != nil {
		respondError(w, err)
		return
	}

	// Email rate limit
	if !h.emailLimiter.Allow(cleanEmail) {
		respondJSON(w, http.StatusTooManyRequests, map[string]string{
			"error": "Too many OTP requests for this email. Please wait 10 minutes.",
		})
		return
	}

	// IP rate limit
	ip := middleware.GetClientIP(r)
	if !h.ipLimiter.Allow(ip) {
		respondJSON(w, http.StatusTooManyRequests, map[string]string{
			"error": "Too many requests. Please slow down.",
		})
		return
	}

	resp, err := h.service.SendOTP(r.Context(), cleanEmail)
	if err != nil {
		// Refund rate limits on delivery failure
		h.emailLimiter.Refund(cleanEmail)
		h.ipLimiter.Refund(ip)
		respondError(w, err)
		return
	}

	respondJSON(w, http.StatusOK, resp)
}

func (h *Handler) SignUp(w http.ResponseWriter, r *http.Request) {
	var req domain.SignUpRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	// Sign-up dispatches an OTP email, so it must be rate limited exactly like
	// SendOTP — otherwise the OTP limits can be bypassed entirely.
	cleanEmail, err := ValidateEmail(req.Email)
	if err != nil {
		respondError(w, err)
		return
	}
	if !h.emailLimiter.Allow(cleanEmail) {
		respondJSON(w, http.StatusTooManyRequests, map[string]string{
			"error": "Too many OTP requests for this email. Please wait 10 minutes.",
		})
		return
	}
	ip := middleware.GetClientIP(r)
	if !h.ipLimiter.Allow(ip) {
		respondJSON(w, http.StatusTooManyRequests, map[string]string{
			"error": "Too many requests. Please slow down.",
		})
		return
	}

	resp, err := h.service.SignUp(r.Context(), cleanEmail, req.Password)
	if err != nil {
		// Refund rate limits on failure (e.g. account already exists)
		h.emailLimiter.Refund(cleanEmail)
		h.ipLimiter.Refund(ip)
		respondError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, resp)
}

func (h *Handler) SignIn(w http.ResponseWriter, r *http.Request) {
	var req domain.SignInRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}
	resp, err := h.service.SignIn(r.Context(), req.Email, req.Password)
	if err != nil {
		respondError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, resp)
}

func (h *Handler) VerifyOTP(w http.ResponseWriter, r *http.Request) {
	var req domain.VerifyOTPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	resp, err := h.service.VerifyOTP(r.Context(), req.Email, req.OTP)
	if err != nil {
		respondError(w, err)
		return
	}

	respondJSON(w, http.StatusOK, resp)
}

func (h *Handler) GoogleSignIn(w http.ResponseWriter, r *http.Request) {
	var req domain.GoogleSignInRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}
	if strings.TrimSpace(req.IDToken) == "" {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Google ID token is required."})
		return
	}

	resp, err := h.service.GoogleSignIn(r.Context(), req.IDToken)
	if err != nil {
		respondError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, resp)
}

func (h *Handler) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	email, ok := middleware.GetUserEmail(r.Context())
	if !ok {
		respondJSON(w, http.StatusUnauthorized, map[string]string{"error": "Authorization token required."})
		return
	}
	if err := h.service.DeleteAccount(r.Context(), email); err != nil {
		respondError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"success": true, "message": "Account deleted."})
}

func (h *Handler) SendWelcome(w http.ResponseWriter, r *http.Request) {
	var req domain.SendWelcomeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondJSON(w, http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
		return
	}

	cleanEmail, err := ValidateEmail(req.Email)
	if err != nil {
		respondError(w, err)
		return
	}

	if !h.welcomeLimiter.Allow(cleanEmail) {
		respondJSON(w, http.StatusTooManyRequests, map[string]string{
			"error": "Too many requests for this email. Please try again later.",
		})
		return
	}

	resp, err := h.service.SendWelcome(r.Context(), cleanEmail, req.Name, req.About)
	if err != nil {
		respondError(w, err)
		return
	}

	respondJSON(w, http.StatusOK, resp)
}

func (h *Handler) CheckEmail(w http.ResponseWriter, r *http.Request) {
	// This endpoint reveals whether an account exists, so keep it rate limited
	// to make bulk email enumeration impractical.
	if !h.checkEmailLimiter.Allow(middleware.GetClientIP(r)) {
		respondJSON(w, http.StatusTooManyRequests, map[string]string{
			"error": "Too many requests. Please slow down.",
		})
		return
	}

	email := strings.TrimSpace(r.URL.Query().Get("email"))
	if email == "" && r.Body != nil {
		var req domain.CheckEmailRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		email = req.Email
	}

	cleanEmail, err := ValidateEmail(email)
	if err != nil {
		respondError(w, err)
		return
	}

	resp, err := h.service.CheckEmail(r.Context(), cleanEmail)
	if err != nil {
		respondError(w, err)
		return
	}

	respondJSON(w, http.StatusOK, resp)
}

func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func respondError(w http.ResponseWriter, err error) {
	status := domain.ErrToStatus(err)
	msg := err.Error()

	var appErr *domain.AppError
	if errors.As(err, &appErr) && appErr.Message != "" {
		msg = appErr.Message
	}

	respondJSON(w, status, map[string]string{"error": msg})
}

package auth

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/atharvix/kinjo-backend/internal/domain"
	"github.com/atharvix/kinjo-backend/internal/middleware"
	"github.com/atharvix/kinjo-backend/internal/observability"
)

type Handler struct {
	service        *Service
	emailLimiter   *middleware.RateLimiter
	ipLimiter      *middleware.RateLimiter
	welcomeLimiter *middleware.RateLimiter
	metrics        *observability.Metrics
}

func NewHandler(service *Service, metrics *observability.Metrics) *Handler {
	return &Handler{
		service:        service,
		emailLimiter:   middleware.NewRateLimiter(10*time.Minute, 3, metrics), // 3 sends per 10 mins per email
		ipLimiter:      middleware.NewRateLimiter(1*time.Minute, 10, metrics),  // 10 sends per 1 min per IP
		welcomeLimiter: middleware.NewRateLimiter(1*time.Hour, 2, metrics),   // 2 welcomes per hour per email
		metrics:        metrics,
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

func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func respondError(w http.ResponseWriter, err error) {
	status := domain.ErrToStatus(err)
	msg := err.Error()
	var appErr *domain.AppError
	if json.Unmarshal([]byte(msg), &appErr) == nil && appErr.Message != "" {
		msg = appErr.Message
	}

	respondJSON(w, status, map[string]string{"error": msg})
}

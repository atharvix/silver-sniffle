package domain

import (
	"errors"
	"fmt"
	"net/http"
)

var (
	ErrNotFound          = errors.New("resource not found")
	ErrUnauthorized      = errors.New("unauthorized")
	ErrForbidden         = errors.New("forbidden")
	ErrBadRequest        = errors.New("bad request")
	ErrConflict          = errors.New("conflict")
	ErrRateLimited       = errors.New("too many requests")
	ErrInternal          = errors.New("internal server error")
	ErrInvalidOTP        = errors.New("invalid or expired OTP")
	ErrTooManyAttempts   = errors.New("too many incorrect attempts")
	ErrTokenExpired      = errors.New("token expired")
	ErrProfileNotFound   = errors.New("profile not found")
	ErrLocationExpired   = errors.New("location sharing has expired")
	ErrNoLocation        = errors.New("no location stored for profile")
	ErrEmailNotVerified  = errors.New("email not verified")
	ErrServiceUnavailable = errors.New("service unavailable")
)

type AppError struct {
	Err        error  `json:"-"`
	Message    string `json:"error"`
	StatusCode int    `json:"-"`
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

func (e *AppError) Unwrap() error {
	return e.Err
}

func NewAppError(statusCode int, message string, underlying error) *AppError {
	return &AppError{
		Err:        underlying,
		Message:    message,
		StatusCode: statusCode,
	}
}

func ErrToStatus(err error) int {
	if err == nil {
		return http.StatusOK
	}

	var appErr *AppError
	if errors.As(err, &appErr) {
		return appErr.StatusCode
	}

	switch {
	case errors.Is(err, ErrBadRequest), errors.Is(err, ErrInvalidOTP), errors.Is(err, ErrTooManyAttempts), errors.Is(err, ErrNoLocation), errors.Is(err, ErrLocationExpired):
		return http.StatusBadRequest
	case errors.Is(err, ErrUnauthorized), errors.Is(err, ErrTokenExpired):
		return http.StatusUnauthorized
	case errors.Is(err, ErrForbidden), errors.Is(err, ErrEmailNotVerified):
		return http.StatusForbidden
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrProfileNotFound):
		return http.StatusNotFound
	case errors.Is(err, ErrConflict):
		return http.StatusConflict
	case errors.Is(err, ErrRateLimited):
		return http.StatusTooManyRequests
	case errors.Is(err, ErrServiceUnavailable):
		return http.StatusServiceUnavailable
	default:
		return http.StatusInternalServerError
	}
}

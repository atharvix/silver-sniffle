package http

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/atharvix/kinjo-backend/internal/domain"
)

// RespondJSON writes a JSON response with status code.
func RespondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// RespondError extracts status code and user-facing error message from AppError or standard error.
func RespondError(w http.ResponseWriter, err error) {
	status := domain.ErrToStatus(err)
	msg := err.Error()

	var appErr *domain.AppError
	if errors.As(err, &appErr) && appErr.Message != "" {
		msg = appErr.Message
	}

	RespondJSON(w, status, map[string]string{"error": msg})
}

package domain

import (
	"time"
)

type OTPCode struct {
	Email     string    `json:"email"`
	OTPHash   string    `json:"-"`
	ExpiresAt time.Time `json:"expires_at"`
	Attempts  int       `json:"attempts"`
}

type VerificationToken struct {
	TokenHash string    `json:"-"`
	Email     string    `json:"email"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

type VerifiedEmail struct {
	Email     string    `json:"email"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Request & Response DTOs
type SendOTPRequest struct {
	Email string `json:"email"`
}

type SendOTPResponse struct {
	Success bool    `json:"success"`
	Message string  `json:"message"`
	DevOTP  *string `json:"devOtp,omitempty"`
}

type VerifyOTPRequest struct {
	Email string `json:"email"`
	OTP   string `json:"otp"`
}

type VerifyOTPResponse struct {
	Success           bool   `json:"success"`
	Message           string `json:"message"`
	VerificationToken string `json:"verificationToken"`
}

type SendWelcomeRequest struct {
	Email string `json:"email"`
	Name  string `json:"name"`
	About string `json:"about,omitempty"`
}

type SendWelcomeResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

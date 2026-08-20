package email

import (
	"context"
)

type Service interface {
	SendOTP(ctx context.Context, toEmail, otp string) error
	SendWelcome(ctx context.Context, toEmail, name, about string) error
	IsConfigured() bool
}

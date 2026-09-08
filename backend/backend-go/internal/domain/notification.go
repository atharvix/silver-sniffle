package domain

import "time"

type DeviceToken struct {
	Email     string    `json:"email"`
	Token     string    `json:"token"`
	Platform  string    `json:"platform"`
	UpdatedAt time.Time `json:"updated_at"`
}

type RegisterTokenRequest struct {
	Token    string `json:"token"`
	Platform string `json:"platform"`
}

type SendCustomNotificationRequest struct {
	TargetEmail string         `json:"target_email"`
	Title       string         `json:"title"`
	Body        string         `json:"body"`
	Data        map[string]any `json:"data,omitempty"`
}

type NotificationResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

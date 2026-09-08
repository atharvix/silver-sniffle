package domain

import (
	"time"
)

type Profile struct {
	Email      string     `json:"email"`
	Name       string     `json:"name"`
	Bio        string     `json:"bio"`
	PhotoURL   string     `json:"photo"`
	Latitude   *float64   `json:"latitude,omitempty"`
	Longitude  *float64   `json:"longitude,omitempty"`
	LastSeenAt *time.Time `json:"last_seen_at,omitempty"`
	AISummary  *string    `json:"ai_summary,omitempty"`
	Headline   *string    `json:"headline,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

type UpsertProfileRequest struct {
	Name      string   `json:"name"`
	Bio       *string  `json:"bio,omitempty"`
	Photo     *string  `json:"photo,omitempty"` // Can be Base64 data URL or HTTP URL
	Latitude  *float64 `json:"latitude,omitempty"`
	Longitude *float64 `json:"longitude,omitempty"`
}

type ProfileResponse struct {
	Success  bool   `json:"success"`
	Message  string `json:"message"`
	PhotoURL string `json:"photo_url,omitempty"`
}

type MyProfileResponse struct {
	Email string `json:"email"`
	Name  string `json:"name"`
	Bio   string `json:"bio"`
	Photo string `json:"photo"`
}

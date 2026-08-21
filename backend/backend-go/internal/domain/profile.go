package domain

import (
	"time"
)

type Profile struct {
	Email          string     `json:"email"`
	Name           string     `json:"name"`
	About          string     `json:"about"`
	PhotoURL       string     `json:"photo"`
	Latitude       *float64   `json:"latitude,omitempty"`
	Longitude      *float64   `json:"longitude,omitempty"`
	LastSeenAt     *time.Time `json:"last_seen_at,omitempty"`
	AISummary      *string    `json:"ai_summary,omitempty"`
	AISummaryAbout *string    `json:"ai_summary_about,omitempty"`
	Headline       *string    `json:"headline,omitempty"`
	HeadlineAbout  *string    `json:"headline_about,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type UpsertProfileRequest struct {
	Name  string  `json:"name"`
	About *string `json:"about,omitempty"`
	Photo *string `json:"photo,omitempty"` // Can be Base64 data URL or HTTP URL
}

type ProfileResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type MyProfileResponse struct {
	Email string `json:"email"`
	Name  string `json:"name"`
	About string `json:"about"`
	Photo string `json:"photo"`
}

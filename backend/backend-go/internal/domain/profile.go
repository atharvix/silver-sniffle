package domain

import (
	"time"
)

// Profile is the application-layer representation. Fields are stored as
// plaintext; no field-level encryption is applied by the repositories.
type Profile struct {
	Email            string     `json:"email"`
	Name             string     `json:"name"`
	Bio              string     `json:"bio"`
	PhotoURL         string     `json:"photo"`
	Latitude         *float64   `json:"latitude,omitempty"`
	Longitude        *float64   `json:"longitude,omitempty"`
	LastSeenAt       *time.Time `json:"last_seen_at,omitempty"`
	AISummary        *string    `json:"ai_summary,omitempty"`
	Headline         *string    `json:"headline,omitempty"`
	FaceVerifiedAt   *time.Time `json:"face_verified_at,omitempty"`
	FaceScanPhotoURL string     `json:"-"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
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
	Email         string `json:"email"`
	Name          string `json:"name"`
	Bio           string `json:"bio"`
	Photo         string `json:"photo"`
	FaceVerified  bool   `json:"face_verified"`
	FaceScanPhoto string `json:"face_scan_photo,omitempty"`
}

// VerifyFaceRequest is submitted by the client right after the live liveness
// scan completes. The backend records verification state server-side so the
// gate cannot be bypassed by tampering with the client.
type VerifyFaceRequest struct {
	// Photo is the live-captured selfie as a base64 data URL (or raw base64).
	Photo string `json:"photo"`
}

type VerifyFaceResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

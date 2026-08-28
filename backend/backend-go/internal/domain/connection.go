package domain

import "time"

type ConnectionRequest struct {
	RecipientEmail string `json:"recipientEmail"`
}

type ConnectionResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type ConnectionNotification struct {
	RequesterEmail string    `json:"requesterEmail"`
	RequesterName  string    `json:"requesterName"`
	CreatedAt      time.Time `json:"createdAt"`
}
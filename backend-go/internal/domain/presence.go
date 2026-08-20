package domain

type UpdateLocationRequest struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type UpdateLocationResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type HeartbeatResponse struct {
	Success bool `json:"success"`
}

type GoOfflineRequest struct {
	Token string `json:"token"`
}

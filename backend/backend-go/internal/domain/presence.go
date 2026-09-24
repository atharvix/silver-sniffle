package domain

// UpdateLocationRequest uses pointers so a missing field is rejected instead of
// silently defaulting to 0,0 (which would pin the user off the coast of Africa).
type UpdateLocationRequest struct {
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
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

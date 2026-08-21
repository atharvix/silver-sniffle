package domain

type NearbyProfileCard struct {
	Name                string  `json:"name"`
	Photo               string  `json:"photo"`
	DistanceMeters      float64 `json:"distanceMeters"`
	Headline            string  `json:"headline"`
	ConversationStarter string  `json:"conversationStarter"`
}

type NearbyProfilesResponse struct {
	Profiles []NearbyProfileCard `json:"profiles"`
}

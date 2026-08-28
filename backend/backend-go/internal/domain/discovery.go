package domain

type NearbyProfileCard struct {
	Email               string  `json:"email"`
	Name                string  `json:"name"`
	Photo               string  `json:"photo"`
	DistanceMeters      float64 `json:"distanceMeters"`
	Headline            string  `json:"headline"`
	ConversationStarter string  `json:"conversationStarter"`
	SocialLinks         map[string]string `json:"socialLinks,omitempty"`
}

type NearbyProfilesResponse struct {
	Profiles []NearbyProfileCard `json:"profiles"`
}

export interface UserProfile {
  id: string;
  name: string;
  handle: string;
  avatar: string;
  quotePrompt: string;
  distanceMeters: number; // e.g. 8, 18, 42
  category: 'Tech' | 'Health' | 'Design' | 'Finance' | 'AI' | 'Creative' | 'Other';
  tags: string[];
  bio: string;
  locationName: string;
  online: boolean;
  verified?: boolean;
}

export type SwipeDirection = 'left' | 'right' | 'up';

export interface FilterState {
  searchQuery: string;
  maxRadiusMeters: number; // e.g. 10, 50, 100, 250, 500
  selectedCategory: string; // 'All' | category
  onlyOnline: boolean;
}

export interface MatchSignal {
  id: string;
  profile: UserProfile;
  timestamp: string;
  matchedAtDistance: number;
}

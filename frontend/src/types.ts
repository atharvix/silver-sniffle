export interface UserProfile {
  id: string;
  name: string;
  handle: string;
  avatar: string; // Profile photo URL
  quotePrompt: string;
  distanceMeters: number; // Strictly within 30m
  category: 'Tech' | 'Health' | 'Design' | 'Finance' | 'AI' | 'Creative' | 'Other';
  tags: string[];
  bio: string;
  locationName: string;
  online: boolean;
  verified?: boolean;
  faceVerified?: boolean; // Verified real human face via ML Kit
  latitude?: number;
  longitude?: number;
}

export type SwipeDirection = 'left' | 'right' | 'up';

export interface FilterState {
  searchQuery: string;
  maxRadiusMeters: number; // Locked at 30m
  selectedCategory: string; // 'All' | category
  onlyOnline: boolean;
}

export interface MatchSignal {
  id: string;
  profile: UserProfile;
  timestamp: string;
  matchedAtDistance: number;
}


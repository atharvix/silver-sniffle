export interface UserProfile {
  id: string;
  email: string;
  name: string;
  avatar: string;
  bio: string;             // "What you do & What you are looking for" (Max 50 words)
  profession?: string;     // Legacy support
  lookingFor?: string;     // Legacy support
  distanceMeters?: number;
  locationName?: string;
  online?: boolean;
  latitude?: number;
  longitude?: number;
}

export type SwipeDirection = 'left' | 'right';

export interface MatchSignal {
  id: string;
  profile: UserProfile;
  timestamp: string;
  matchedAtDistance: number;
}

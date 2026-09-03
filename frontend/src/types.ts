export interface UserProfile {
  id: string;
  email: string;
  name: string;
  avatar: string;
  profession: string;      // "What you do"
  lookingFor: string;      // "What you are looking for"
  distanceMeters: number;
  locationName: string;
  online: boolean;
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

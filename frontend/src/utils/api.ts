import type { UserProfile } from '../types';

const API_BASE = (import.meta.env.VITE_API_URL || '/api').replace(/\/$/, '');

async function request<T>(path: string, init: RequestInit = {}, token?: string): Promise<T> {
  const response = await fetch(`${API_BASE}${path}`, {
    ...init,
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...init.headers,
    },
  });

  const body = (await response.json().catch(() => ({}))) as T & { error?: string };
  if (!response.ok) throw new Error(body.error || `Request failed (${response.status})`);
  return body;
}

export function sendOtp(email: string) {
  return request<{ success: boolean; message: string; devOtp?: string }>('/auth/send-otp', {
    method: 'POST',
    body: JSON.stringify({ email }),
  });
}

export function verifyOtp(email: string, otp: string) {
  return request<{ success: boolean; message: string; verificationToken: string }>('/auth/verify-otp', {
    method: 'POST',
    body: JSON.stringify({ email, otp }),
  });
}

export function saveProfile(profile: UserProfile, token: string) {
  return request('/profiles', {
    method: 'POST',
    body: JSON.stringify({ name: profile.name, about: profile.lookingFor || profile.profession, photo: profile.avatar }),
  }, token);
}

export function updateLocation(latitude: number, longitude: number, token: string) {
  return request('/profiles/location', {
    method: 'POST',
    body: JSON.stringify({ latitude, longitude }),
  }, token);
}

export function createConnection(recipientEmail: string, token: string) {
  return request<{ success: boolean; message: string }>('/connections', {
    method: 'POST',
    body: JSON.stringify({ recipientEmail }),
  }, token);
}

interface NearbyProfileResponse {
  profiles: Array<{
    email: string;
    name: string;
    photo: string;
    distanceMeters: number;
    headline: string;
    conversationStarter: string;
    socialLinks?: Record<string, string>;
  }>;
}

export async function getNearbyProfiles(token: string): Promise<UserProfile[]> {
  const response = await request<NearbyProfileResponse>('/profiles/nearby', {}, token);
  return response.profiles.map((profile, index) => ({
    id: `remote-${profile.name}-${index}`,
    email: profile.email,
    name: profile.name,
    avatar: profile.photo,
    profession: profile.headline || '',
    lookingFor: profile.conversationStarter || profile.headline || 'Open to a nearby conversation',
    distanceMeters: Math.round(profile.distanceMeters),
    locationName: `${Math.round(profile.distanceMeters)}m away`,
    online: true,
  }));
}

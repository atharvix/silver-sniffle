import type { UserProfile } from '../types';
import { Capacitor } from '@capacitor/core';
import { Http } from '@capacitor-community/http';

const DEFAULT_DEV_API = 'http://localhost:8080/api';
const API_BASE = (import.meta.env.VITE_API_URL || DEFAULT_DEV_API).trim().replace(/\/$/, '');

export const DEFAULT_AVATAR_WEBP = 'data:image/webp;base64,UklGRiQAAABXRUJQVlA4IBgAAAAwAQCdASoBAAEAAQAcJaQAA3AA/v38gAA=';

export function resolvePhotoUrl(url?: string): string {
  if (!url) return DEFAULT_AVATAR_WEBP;
  let fullUrl = url;
  if (!url.startsWith('http://') && !url.startsWith('https://') && !url.startsWith('data:')) {
    const base = API_BASE.replace(/\/api\/?$/, '');
    fullUrl = `${base}${url.startsWith('/') ? '' : '/'}${url}`;
  }
  if (fullUrl.includes('ngrok') && !fullUrl.includes('ngrok-skip-browser-warning')) {
    const sep = fullUrl.includes('?') ? '&' : '?';
    fullUrl = `${fullUrl}${sep}ngrok-skip-browser-warning=true`;
  }
  return fullUrl;
}

export function setAuthToken(token: string) {
  if (token) {
    localStorage.setItem('kinjo_auth_token', token);
  } else {
    localStorage.removeItem('kinjo_auth_token');
  }
}

export function compressImage(file: File, maxDimension = 1080, quality = 0.88): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onerror = (err) => reject(err);
    reader.onload = (e) => {
      const img = new Image();
      img.onerror = (err) => reject(err);
      img.onload = () => {
        let width = img.width;
        let height = img.height;
        if (width > maxDimension || height > maxDimension) {
          if (width > height) {
            height = Math.round((height * maxDimension) / width);
            width = maxDimension;
          } else {
            width = Math.round((width * maxDimension) / height);
            height = maxDimension;
          }
        }
        const canvas = document.createElement('canvas');
        canvas.width = width;
        canvas.height = height;
        const ctx = canvas.getContext('2d');
        if (!ctx) {
          resolve(e.target?.result as string);
          return;
        }
        ctx.drawImage(img, 0, 0, width, height);
        let dataUrl = canvas.toDataURL('image/webp', quality);
        if (!dataUrl.startsWith('data:image/webp')) {
          dataUrl = canvas.toDataURL('image/jpeg', quality);
        }
        resolve(dataUrl);
      };
      img.src = e.target?.result as string;
    };
    reader.readAsDataURL(file);
  });
}

export function compressBase64DataUrl(dataUrl: string, maxDimension = 1080, quality = 0.92): Promise<string> {
  if (!dataUrl || !dataUrl.startsWith('data:image')) return Promise.resolve(dataUrl);
  return new Promise((resolve) => {
    const img = new Image();
    img.crossOrigin = 'anonymous';
    img.onerror = () => resolve(dataUrl);
    img.onload = () => {
      let width = img.width;
      let height = img.height;
      if (width > maxDimension || height > maxDimension) {
        if (width > height) {
          height = Math.round((height * maxDimension) / width);
          width = maxDimension;
        } else {
          width = Math.round((width * maxDimension) / height);
          height = maxDimension;
        }
      }
      const canvas = document.createElement('canvas');
      canvas.width = width;
      canvas.height = height;
      const ctx = canvas.getContext('2d');
      if (!ctx) {
        resolve(dataUrl);
        return;
      }
      ctx.drawImage(img, 0, 0, width, height);
      resolve(canvas.toDataURL('image/jpeg', quality));
    };
    img.src = dataUrl;
  });
}

export class ApiError extends Error {
  readonly status: number;

  constructor(status: number, message: string) {
    super(message);
    this.status = status;
    this.name = 'ApiError';
  }
}

async function request<T>(path: string, init: RequestInit = {}, token?: string): Promise<T> {
  const headers = {
    'Content-Type': 'application/json',
    'bypass-tunnel-reminder': 'true',
    'ngrok-skip-browser-warning': 'true',
    ...(token ? { Authorization: `Bearer ${token}` } : {}),
    ...init.headers,
  } as Record<string, string>;

  if (Capacitor.isNativePlatform()) {
    try {
      const httpPromise = Http.request({
        url: `${API_BASE}${path}`,
        method: init.method || 'GET',
        headers,
        params: {},
        data: init.body ? JSON.parse(init.body as string) : undefined,
        responseType: 'json',
        connectTimeout: 30000,
        readTimeout: 30000,
      });
      const timeoutPromise = new Promise<never>((_, reject) =>
        setTimeout(() => reject(new Error('Request timed out after 30 seconds.')), 30000)
      );
      const response = await Promise.race([httpPromise, timeoutPromise]);
      const body = response.data as T & { error?: string };
      if (response.status < 200 || response.status >= 300) {
        throw new ApiError(response.status, body?.error || `Request failed (${response.status})`);
      }
      return body;
    } catch (error) {
      if (error instanceof ApiError) throw error;
      const detail = error instanceof Error ? error.message : String(error);
      throw new Error(`Kinjo API request failed at ${API_BASE}: ${detail}`);
    }
  }

  let response: Response;
  try {
    response = await fetch(`${API_BASE}${path}`, {
      ...init,
      headers,
    });
  } catch {
    throw new Error(`Cannot reach the Kinjo API at ${API_BASE}. Keep the backend running and run adb reverse tcp:8080 tcp:8080.`);
  }

  const body = (await response.json().catch(() => ({}))) as T & { error?: string };
  if (!response.ok) throw new ApiError(response.status, body.error || `Request failed (${response.status})`);
  return body;
}

export function sendOtp(email: string) {
  return request<{ success: boolean; message: string; devOtp?: string }>('/auth/send-otp', {
    method: 'POST',
    body: JSON.stringify({ email }),
  });
}

export function signUp(email: string, password: string) {
  return request<{ success: boolean; message: string; devOtp?: string }>('/auth/sign-up', {
    method: 'POST',
    body: JSON.stringify({ email, password }),
  });
}

export function signIn(email: string, password: string) {
  return request<{ success: boolean; message: string; verificationToken: string; email: string }>('/auth/sign-in', {
    method: 'POST',
    body: JSON.stringify({ email, password }),
  });
}

export function verifyOtp(email: string, otp: string) {
  return request<{ success: boolean; message: string; verificationToken: string }>('/auth/verify-otp', {
    method: 'POST',
    body: JSON.stringify({ email, otp }),
  });
}

export function googleSignIn(idToken: string) {
  return request<{ success: boolean; message: string; verificationToken: string; email: string }>('/auth/google', {
    method: 'POST',
    body: JSON.stringify({ idToken }),
  });
}

export function getMyProfile(token: string) {
  return request<{
    id?: string;
    email: string;
    name: string;
    bio: string;
    photo: string;
  }>('/profiles/me', {}, token);
}

export function deleteAccount(token: string) {
  return request<{ success: boolean; message: string }>('/auth/account', { method: 'DELETE' }, token);
}

export function saveProfile(profile: UserProfile, token: string, lat?: number | null, lon?: number | null) {
  const bioContent = profile.bio || [profile.profession, profile.lookingFor].filter(Boolean).join(' · ');
  return request<{ success: boolean; message: string; photo_url?: string }>('/profiles', {
    method: 'POST',
    body: JSON.stringify({
      name: profile.name,
      bio: bioContent,
      photo: profile.avatar,
      latitude: lat ?? undefined,
      longitude: lon ?? undefined,
    }),
  }, token);
}

export function updateLocation(latitude: number, longitude: number, token: string) {
  return request('/profiles/location', {
    method: 'POST',
    body: JSON.stringify({ latitude, longitude }),
  }, token);
}

export function recordHeartbeat(token: string) {
  return request<{ success: boolean }>('/profiles/heartbeat', {
    method: 'POST',
  }, token);
}

export function sendOffline(token: string) {
  return request<{ success: boolean }>('/profiles/offline', {
    method: 'POST',
    body: JSON.stringify({ token }),
  });
}

interface NearbyProfileResponse {
  profiles: Array<{
    email: string;
    name: string;
    photo: string;
    distanceMeters: number;
    headline: string;
    conversationStarter: string;
  }>;
}

export async function getNearbyProfiles(token: string, lat?: number | null, lon?: number | null): Promise<UserProfile[]> {
  const query = (lat != null && lon != null) ? `?lat=${lat}&lon=${lon}` : '';
  const response = await request<NearbyProfileResponse>(`/profiles/nearby${query}`, {}, token);
  return (response.profiles || [])
    .filter((profile) => profile.distanceMeters <= 30)
    .map((profile) => {
    const bioText = profile.headline || profile.conversationStarter || '';
    const parts = bioText.split(' · ');
    return {
      id: `remote-${profile.email}`,
      email: profile.email,
      name: profile.name,
      avatar: profile.photo,
      bio: bioText,
      profession: parts[0] || bioText,
      lookingFor: parts[1] || '',
      distanceMeters: Math.round(profile.distanceMeters),
      locationName: `${Math.round(profile.distanceMeters)}m away`,
      online: true,
    };
  });
}

export function registerDeviceToken(token: string, deviceToken: string, platform: string = 'android') {
  return request<{ success: boolean; message: string }>('/notifications/register-token', {
    method: 'POST',
    body: JSON.stringify({ token: deviceToken, platform }),
  }, token);
}

export function sendCustomNotification(token: string, targetEmail: string, title: string, body: string, data?: Record<string, any>) {
  return request<{ success: boolean; message: string }>('/notifications/send-custom', {
    method: 'POST',
    body: JSON.stringify({ target_email: targetEmail, title, body, data }),
  }, token);
}


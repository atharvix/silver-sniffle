import React, { createContext, useContext, useState, useEffect, useCallback } from 'react';
import type { UserProfile } from '../types';
import { getMyProfile, setAuthToken as setGlobalAuthToken } from '../utils/api';

interface AuthContextType {
  authToken: string;
  userEmail: string;
  userProfile: UserProfile | null;
  isAuthenticated: boolean;
  setAuth: (token: string, email: string) => void;
  setUserProfile: React.Dispatch<React.SetStateAction<UserProfile | null>>;
  logout: () => void;
  refreshMyProfile: () => Promise<UserProfile | null>;
}

const AuthContext = createContext<AuthContextType | undefined>(undefined);

export const AuthProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [authToken, setAuthTokenState] = useState<string>(() => localStorage.getItem('kinjo_auth_token') || '');
  const [userEmail, setUserEmailState] = useState<string>(() => localStorage.getItem('kinjo_user_email') || '');
  const [userProfile, setUserProfile] = useState<UserProfile | null>(() => {
    const saved = localStorage.getItem('kinjo_user_profile');
    if (saved) {
      try {
        return JSON.parse(saved);
      } catch {}
    }
    return null;
  });

  const setAuth = useCallback((token: string, email: string) => {
    setAuthTokenState(token);
    setUserEmailState(email);
    setGlobalAuthToken(token);
    localStorage.setItem('kinjo_auth_token', token);
    localStorage.setItem('kinjo_user_email', email);
  }, []);

  const logout = useCallback(() => {
    setAuthTokenState('');
    setUserEmailState('');
    setUserProfile(null);
    setGlobalAuthToken('');
    localStorage.removeItem('kinjo_auth_token');
    localStorage.removeItem('kinjo_user_email');
    localStorage.removeItem('kinjo_user_profile');
    localStorage.removeItem('kinjo_cached_nearby_profiles');
  }, []);

  const refreshMyProfile = useCallback(async () => {
    if (!authToken) return null;
    try {
      const data = await getMyProfile(authToken);
      if (data && data.email) {
        const profile: UserProfile = {
          id: data.email,
          name: data.name,
          email: data.email,
          avatar: data.photo || '',
          bio: data.bio || '',
          profession: data.bio || '',
          lookingFor: data.bio || '',
          distanceMeters: 0,
          locationName: 'Current Location',
          online: true,
        };
        setUserProfile(profile);
        localStorage.setItem('kinjo_user_profile', JSON.stringify(profile));
        return profile;
      }
    } catch {}
    return null;
  }, [authToken]);

  useEffect(() => {
    if (authToken) {
      setGlobalAuthToken(authToken);
      void refreshMyProfile();
    }
  }, [authToken, refreshMyProfile]);

  return (
    <AuthContext.Provider
      value={{
        authToken,
        userEmail,
        userProfile,
        isAuthenticated: Boolean(authToken),
        setAuth,
        setUserProfile,
        logout,
        refreshMyProfile,
      }}
    >
      {children}
    </AuthContext.Provider>
  );
};

export const useAuth = (): AuthContextType => {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error('useAuth must be used within an AuthProvider');
  }
  return context;
};

import { useState, useEffect } from 'react';
import type { UserProfile, SwipeDirection } from './types';
import { useGPSLocation } from './hooks/useGPSLocation';
import { deleteAccount, getMyProfile, resolvePhotoUrl, saveProfile, sendOffline } from './utils/api';

import { App as CapApp } from '@capacitor/app';
import { Capacitor } from '@capacitor/core';
import { GoogleAuth } from '@shardev/capacitor-google-auth';

import { Header } from './components/Header';
import { ProfilePopover } from './components/ProfilePopover';
import { CardDeck } from './components/CardDeck';
import { ProfileView } from './components/ProfileView';
import { ProfileDetailScreen } from './components/ProfileDetailScreen';
import { OnboardingModal } from './components/OnboardingModal';
import { OnboardingTutorial } from './components/OnboardingTutorial';

type Screen = 'home' | 'profile' | 'details';

export function App() {
  const [userProfile, setUserProfile] = useState<UserProfile | null>(null);

  const [isOnboarding, setIsOnboarding] = useState(() => {
    const onboarded = localStorage.getItem('kinjo_onboarded');
    const token = localStorage.getItem('kinjo_auth_token');
    return !(onboarded === 'true' && Boolean(token));
  });
  const [showTutorial, setShowTutorial] = useState(false);
  const [isEditingProfileFromSettings, setIsEditingProfileFromSettings] = useState(false);
  const [authToken, setAuthToken] = useState(() =>
    localStorage.getItem('kinjo_auth_token') || ''
  );
  const [showOpening, setShowOpening] = useState(true);
  const [isPopoverOpen, setIsPopoverOpen] = useState(false);
  const [theme, setTheme] = useState<'dark' | 'light' | 'system'>(() =>
    (localStorage.getItem('kinjo_theme') as 'dark' | 'light' | 'system') || 'dark'
  );

  const handleToggleTheme = (newTheme: 'dark' | 'light' | 'system') => {
    setTheme(newTheme);
    localStorage.setItem('kinjo_theme', newTheme);
  };

  useEffect(() => {
    const applyTheme = () => {
      let isDark = true;
      if (theme === 'system') {
        isDark = window.matchMedia('(prefers-color-scheme: dark)').matches;
      } else if (theme === 'light') {
        isDark = false;
      } else {
        isDark = true;
      }

      if (isDark) {
        document.documentElement.classList.remove('light-theme');
      } else {
        document.documentElement.classList.add('light-theme');
      }
    };

    applyTheme();

    if (theme === 'system') {
      const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)');
      const listener = () => applyTheme();
      mediaQuery.addEventListener('change', listener);
      return () => mediaQuery.removeEventListener('change', listener);
    }
  }, [theme]);



  useEffect(() => {
    const token = localStorage.getItem('kinjo_auth_token');
    if (!token) return;

    let cancelled = false;
    void getMyProfile(token)
      .then((profile) => {
        const finalAvatar = resolvePhotoUrl(profile.photo);
        const bioText = profile.bio || '';
        const bioParts = bioText.split(' · ');
        const restored: UserProfile = {
          id: profile.email,
          email: profile.email,
          name: profile.name || 'User',
          avatar: finalAvatar,
          bio: bioText,
          profession: bioParts[0] || bioText,
          lookingFor: bioParts[1] || '',
          distanceMeters: 0,
          locationName: 'Current Location',
          online: true,
        };
        setAuthToken(token);
        setUserProfile(restored);
        localStorage.setItem('kinjo_onboarded', 'true');
        setIsOnboarding(false);
      })
      .catch(() => {
        if (cancelled) return;
        localStorage.removeItem('kinjo_auth_token');
        localStorage.removeItem('kinjo_onboarded');
        localStorage.removeItem('kinjo_user_profile');
        setAuthToken('');
        setUserProfile(null);
        setIsOnboarding(true);
      });

    return () => {
      cancelled = true;
    };
  }, []);

  // ─── Navigation ─────────────────────────────────────────────────────────────
  const [screen, setScreen] = useState<Screen>('home');
  const [detailProfile, setDetailProfile] = useState<UserProfile | null>(null);

  // ─── Hardware Back Button / Native Gesture Handling ──────────────────────────
  useEffect(() => {
    if (!Capacitor.isNativePlatform()) return;
    // When onboarding is active, the OnboardingModal registers its own back button handler
    if (isOnboarding) return;

    const backListener = CapApp.addListener('backButton', () => {
      if (detailProfile || screen === 'details') {
        setDetailProfile(null);
        setScreen('home');
        return;
      }
      if (screen === 'profile') {
        setScreen('home');
        return;
      }
      if (isPopoverOpen) {
        setIsPopoverOpen(false);
        return;
      }
      if (showTutorial) {
        setShowTutorial(false);
        return;
      }
      void CapApp.minimizeApp();
    });

    return () => {
      void backListener.then((l) => l.remove());
    };
  }, [screen, detailProfile, isEditingProfileFromSettings, isPopoverOpen, showTutorial, isOnboarding]);

  const navigate = (s: Screen) => {
    window.history.pushState({ screen: s }, '');
    setScreen(s);
  };

  const goHome = () => {
    if (screen !== 'home') {
      setScreen('home');
      setDetailProfile(null);
    }
  };

  const { gps, profiles, isLoadingProfiles, refreshProfiles } = useGPSLocation(authToken);

  // Splash
  useEffect(() => {
    const t = setTimeout(() => setShowOpening(false), 2400);
    return () => clearTimeout(t);
  }, []);

  // ─── Browser History & Popstate Navigation ──────────────────────────────────────────
  useEffect(() => {
    const handlePopState = () => {
      if (screen !== 'home') {
        goHome();
      }
    };

    window.addEventListener('popstate', handlePopState);
    return () => {
      window.removeEventListener('popstate', handlePopState);
    };
  }, [screen]);

  // ─── Auth Handlers ───────────────────────────────────────────────────────────
  const handleSaveProfile = async (updated: UserProfile) => {
    setUserProfile(updated);
    localStorage.setItem('kinjo_user_profile', JSON.stringify(updated));
    const token = authToken || localStorage.getItem('kinjo_auth_token');
    if (token) {
      try {
        let lat = gps.latitude;
        let lon = gps.longitude;
        if (lat === null || lon === null) {
          try {
            const savedLoc = localStorage.getItem('kinjo_user_location');
            if (savedLoc) {
              const parsed = JSON.parse(savedLoc);
              if (parsed.latitude && parsed.longitude) {
                lat = parsed.latitude;
                lon = parsed.longitude;
              }
            }
          } catch {}
        }
        const res = await saveProfile(updated, token, lat, lon);
        if (res.photo_url) {
          const finalUrl = resolvePhotoUrl(res.photo_url);
          const newProfile = { ...updated, avatar: finalUrl };
          setUserProfile(newProfile);
          localStorage.setItem('kinjo_user_profile', JSON.stringify(newProfile));
        }
      } catch (err) {
        console.error('Failed to sync profile to server:', err);
      }
    }
  };

  const handleAuthenticated = async (
    token: string,
    emailArg: string,
    fallbackPhoto?: string,
    skipAutoClose = false
  ): Promise<boolean> => {
    localStorage.setItem('kinjo_auth_token', token);
    setAuthToken(token);
    try {
      const profile = await getMyProfile(token);
      if (profile && profile.name) {
        let finalAvatar = resolvePhotoUrl(profile.photo);
        if (!finalAvatar && fallbackPhoto) {
          finalAvatar = fallbackPhoto;
          void saveProfile({
            id: profile.id || 'current_user',
            email: profile.email || emailArg,
            name: profile.name,
            avatar: fallbackPhoto,
            bio: profile.bio || '',
            profession: (profile.bio || '').split(' · ')[0] || '',
            lookingFor: (profile.bio || '').split(' · ')[1] || '',
          }, token).then(res => {
            if (res.photo_url) {
              const url = resolvePhotoUrl(res.photo_url);
              setUserProfile(prev => prev ? { ...prev, avatar: url } : null);
            }
          }).catch(() => {});
        }
        const bioText = profile.bio || '';
        const bioParts = bioText.split(' · ');
        const restored: UserProfile = {
          id: profile.id || 'current_user',
          email: profile.email || emailArg,
          name: profile.name,
          avatar: finalAvatar || '',
          bio: bioText,
          profession: bioParts[0] || bioText,
          lookingFor: bioParts[1] || '',
        };
        setUserProfile(restored);
        localStorage.setItem('kinjo_user_profile', JSON.stringify(restored));
        if (!skipAutoClose) {
          localStorage.setItem('kinjo_onboarded', 'true');
          setIsOnboarding(false);
          setShowTutorial(false);
        }
        return true;
      }
    } catch {}

    return false;
  };

  const handleOnboardingComplete = (_email?: string, token?: string, isGuest?: boolean) => {
    if (token) {
      localStorage.setItem('kinjo_auth_token', token);
      setAuthToken(token);
    }
    // Guest bypass does NOT persist kinjo_onboarded so login/onboarding shows on next refresh
    if (!isGuest) {
      localStorage.setItem('kinjo_onboarded', 'true');
      setShowTutorial(true);
    }
    setIsOnboarding(false);
  };

  const handleSwipe = (_direction: SwipeDirection, _profile: UserProfile) => {
    // Card swiped
  };

  const handleLogout = () => {
    if (authToken) {
      void sendOffline(authToken).catch(() => {});
    }
    if (Capacitor.isNativePlatform()) {
      GoogleAuth.logout().catch(() => {});
    }
    localStorage.clear();
    setAuthToken('');
    setUserProfile(null);
    goHome();
    setIsOnboarding(true);
  };

  const handleDeleteAccount = async () => {
    if (Capacitor.isNativePlatform()) {
      GoogleAuth.logout().catch(() => {});
    }
    if (authToken) {
      try {
        await deleteAccount(authToken);
      } catch (err) {
        console.warn('Backend account deletion API warning:', err);
      }
    }
    localStorage.clear();
    setAuthToken('');
    setUserProfile(null);
    goHome();
    setIsOnboarding(true);
  };

  // Re-validate session whenever app resumes from background or regains focus
  useEffect(() => {
    const token = authToken || localStorage.getItem('kinjo_auth_token');
    if (!token) return;

    const validateSessionOnResume = async () => {
      try {
        await getMyProfile(token);
      } catch {
        localStorage.clear();
        setAuthToken('');
        setUserProfile(null);
        setIsOnboarding(true);
      }
    };

    const handleVisibility = () => {
      if (document.visibilityState === 'visible') {
        void validateSessionOnResume();
      }
    };
    document.addEventListener('visibilitychange', handleVisibility);

    let appStateListener: any = null;
    if (Capacitor.isNativePlatform()) {
      appStateListener = CapApp.addListener('appStateChange', (state) => {
        if (state.isActive) {
          void validateSessionOnResume();
        }
      });
    }

    return () => {
      document.removeEventListener('visibilitychange', handleVisibility);
      if (appStateListener) appStateListener.then((l: any) => l?.remove?.());
    };
  }, [authToken]);

  const handleProfileSetupComplete = (data: { name: string; avatar: string; bio?: string; profession: string; lookingFor: string }) => {
    const bioValue = data.bio || data.profession || userProfile?.bio || '';
    const updated: UserProfile = {
      id: userProfile?.id || 'current_user',
      email: userProfile?.email || '',
      name: data.name || userProfile?.name || 'User',
      avatar: data.avatar || userProfile?.avatar || '',
      bio: bioValue,
      profession: bioValue,
      lookingFor: '',
    };
    setUserProfile(updated);
    localStorage.setItem('kinjo_user_profile', JSON.stringify(updated));
    setIsEditingProfileFromSettings(false);
  };

  return (
    <div className={`app-shell relative min-h-screen w-full flex flex-col overflow-hidden font-sans transition-colors duration-300 ${theme === 'light' ? 'bg-[#f5f5f7] text-neutral-900' : 'bg-[#08080a] text-white'}`}>

      {/* Onboarding / Login Modal */}
      {isOnboarding && (
        <OnboardingModal
          isOpen={isOnboarding}
          onClose={() => setIsOnboarding(false)}
          onComplete={handleOnboardingComplete}
          onAuthenticated={handleAuthenticated}
          onProfileSetupComplete={handleProfileSetupComplete}
        />
      )}



      {/* Interactive Onboarding Tutorial Overlay */}
      <OnboardingTutorial
        isOpen={showTutorial}
        onClose={() => setShowTutorial(false)}
      />

      {/* Main App */}
      {!isOnboarding && (
        <>
          {/* Header with Circular Profile Photo */}
          <Header
            userProfile={userProfile}
            onOpenMenu={() => setIsPopoverOpen(true)}
          />

          {/* Floating Profile Popover Box */}
          {userProfile && (
            <ProfilePopover
              isOpen={isPopoverOpen}
              onClose={() => setIsPopoverOpen(false)}
              userProfile={userProfile}
              onOpenSettings={() => navigate('profile')}
            />
          )}

          {/* Main Card Deck Area */}
          <main className="flex-1 flex items-center justify-center px-3 py-2">
            <CardDeck
              profiles={profiles}
              isLoading={isLoadingProfiles}
              onRefresh={refreshProfiles}
              onSwipe={handleSwipe}
              onOpenDetails={(p) => {
                setDetailProfile(p);
                navigate('details');
              }}
            />
          </main>

          {/* Full-screen Profile Settings */}
          {screen === 'profile' && userProfile && (
            <div
              className="fixed inset-0 z-50 bg-[#060606] overflow-y-auto"
              style={{ animation: 'screen-slide-in-right 260ms cubic-bezier(0.32,0.72,0,1) both' }}
            >
              <ProfileView
                userProfile={userProfile}
                onSave={handleSaveProfile}
                onLogout={handleLogout}
                onDeleteAccount={handleDeleteAccount}
                onClose={goHome}
                currentTheme={theme}
                onToggleTheme={handleToggleTheme}
              />
            </div>
          )}

          {/* Full-screen Card Details */}
          {screen === 'details' && detailProfile && (
            <ProfileDetailScreen
              profile={detailProfile}
              onClose={goHome}
            />
          )}
        </>
      )}

      {/* Splash Screen */}
      {showOpening && (
        <div className="opening-screen">
          <div className="opening-logo-wrap">
            <div className="opening-mark">k<span>.</span></div>
          </div>
        </div>
      )}
    </div>
  );
}

export default App;



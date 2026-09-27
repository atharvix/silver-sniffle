import { useState, useEffect, useCallback } from 'react';
import type { UserProfile, SwipeDirection } from './types';
import { useGPSLocation } from './hooks/useGPSLocation';
import { deleteAccount, getMyProfile, resolvePhotoUrl, saveProfile, sendOffline } from './utils/api';
import { initializeFCM } from './utils/fcm';
import { startBackgroundLocation, stopBackgroundLocation } from './utils/backgroundLocation';
import { syncStatusBar } from './utils/nativeUi';

import { App as CapApp } from '@capacitor/app';
import { Capacitor } from '@capacitor/core';
import { GoogleAuth } from '@shardev/capacitor-google-auth';

import { Header } from './components/Header';
import { ProfilePopover } from './components/ProfilePopover';
import { CardDeck } from './components/CardDeck';
import { ProfileView } from './components/ProfileView';
import { OnboardingModal, type AuthStep } from './components/OnboardingModal';
import { OnboardingTutorial } from './components/OnboardingTutorial';
import { useToast } from './components/Toast';

type Screen = 'home' | 'profile';

// Storage keys owned by the session. Cleared on logout so the next user starts
// clean, while unrelated preferences (e.g. the chosen theme) survive.
const SESSION_STORAGE_KEYS = [
  'kinjo_auth_token',
  'kinjo_user_email',
  'kinjo_onboarded',
  'kinjo_cached_nearby',
  'kinjo_face_photo',
  'kinjo_face_features',
  'kinjo_fcm_registered_token',
];

function clearSessionStorage() {
  SESSION_STORAGE_KEYS.forEach((key) => localStorage.removeItem(key));
}

export function App() {
  const [userProfile, setUserProfile] = useState<UserProfile | null>(null);

  const [isOnboarding, setIsOnboarding] = useState(() => {
    const onboarded = localStorage.getItem('kinjo_onboarded');
    const token = localStorage.getItem('kinjo_auth_token');
    return !(onboarded === 'true' && Boolean(token));
  });
  const [onboardingInitialStep, setOnboardingInitialStep] = useState<AuthStep>('email');
  const [showTutorial, setShowTutorial] = useState(false);
  const [tutorialStep, setTutorialStep] = useState<'swipe' | 'settings'>('swipe');
  const [authToken, setAuthToken] = useState(() =>
    localStorage.getItem('kinjo_auth_token') || ''
  );
  const [isPopoverOpen, setIsPopoverOpen] = useState(false);
  const [theme, setTheme] = useState<'dark' | 'light' | 'system'>(() =>
    (localStorage.getItem('kinjo_theme') as 'dark' | 'light' | 'system') || 'dark'
  );
  const toast = useToast();

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

      // The native bar (clock, battery, signal) does not follow CSS — it follows
      // the device's colour scheme unless we push the app's choice to it.
      void syncStatusBar(isDark);
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
        if (cancelled) return;
        if (profile.face_scan_photo) {
          localStorage.setItem('kinjo_face_photo', profile.face_scan_photo);
        }
        // Only resolve a real photo: resolvePhotoUrl() substitutes a 1x1
        // placeholder, which must never be treated as the user's photo.
        const finalAvatar = profile.photo ? resolvePhotoUrl(profile.photo) : '';
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
        if (profile.face_verified === false) {
          setOnboardingInitialStep('face_verification');
          setIsOnboarding(true);
        } else {
          localStorage.setItem('kinjo_onboarded', 'true');
          setIsOnboarding(false);
        }
      })
      .catch(() => {
        if (cancelled) return;
        clearSessionStorage();
        setAuthToken('');
        setUserProfile(null);
        setIsOnboarding(true);
      });

    return () => {
      cancelled = true;
    };
  }, []);

  // ─── Initialize Push Notifications (FCM) on Native Mobile ──────────────────
  useEffect(() => {
    if (authToken) {
      void initializeFCM(authToken);
    }
  }, [authToken]);

  // ─── Keep presence alive in the background (native only) ───────────────────
  // Started once signed in and past onboarding, so people nearby still find you
  // after the app is backgrounded or swiped away.
  useEffect(() => {
    if (!authToken || isOnboarding) return;
    void startBackgroundLocation(authToken);
  }, [authToken, isOnboarding]);

  // ─── Face verification required (backend returned 403 during discovery) ─────
  useEffect(() => {
    const handleFaceVerificationRequired = () => {
      setOnboardingInitialStep('face_verification');
      setIsOnboarding(true);
    };
    window.addEventListener('kinjo:face_verification_required', handleFaceVerificationRequired);
    return () => {
      window.removeEventListener('kinjo:face_verification_required', handleFaceVerificationRequired);
    };
  }, []);

  // ─── Navigation ─────────────────────────────────────────────────────────────
  const [screen, setScreen] = useState<Screen>('home');

  // ─── Hardware Back Button / Native Gesture Handling ──────────────────────────
  useEffect(() => {
    if (!Capacitor.isNativePlatform()) return;
    // When onboarding is active, the OnboardingModal registers its own back button handler
    if (isOnboarding) return;

    const backListener = CapApp.addListener('backButton', () => {
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
  }, [screen, isPopoverOpen, showTutorial, isOnboarding]);

  const navigate = (s: Screen) => {
    window.history.pushState({ screen: s }, '');
    setScreen(s);
  };

  const goHome = useCallback(() => {
    setScreen('home');
  }, []);

  const { gps, profiles, isLoadingProfiles, refreshProfiles } = useGPSLocation(!isOnboarding ? authToken : '');

  const [showSplash, setShowSplash] = useState(true);

  // Splash Screen: plays the GIF once through, then hides
  useEffect(() => {
    const t = setTimeout(() => setShowSplash(false), 2400);
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
  }, [screen, goHome]);

  // ─── Auth Handlers ───────────────────────────────────────────────────────────
  // Applies the edit locally at once, then syncs in the background. The avatar
  // is never swapped for the stored URL afterwards: the image is already on
  // screen, and re-pointing it at the server would force a fresh download.
  const handleSaveProfile = (updated: UserProfile, photoChanged = true) => {
    setUserProfile(updated);

    const token = authToken || localStorage.getItem('kinjo_auth_token');
    if (!token) return;

    void saveProfile(updated, token, gps.latitude, gps.longitude, photoChanged)
      .then(() => toast.success('Profile updated ✓'))
      .catch((err: any) => {
        console.error('Failed to sync profile to server:', err);
        toast.error(err?.message || 'Could not save your profile. Please check your connection and try again.');
      });
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
        if (profile.face_scan_photo) {
          localStorage.setItem('kinjo_face_photo', profile.face_scan_photo);
        }
        // resolvePhotoUrl never returns an empty string (it falls back to a
        // placeholder), so test the raw value to know whether a real photo exists.
        const hasServerPhoto = Boolean(profile.photo);
        let finalAvatar = hasServerPhoto ? resolvePhotoUrl(profile.photo) : '';
        if (!hasServerPhoto && fallbackPhoto) {
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
          }).catch(() => { });
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
        if (restored.email) localStorage.setItem('kinjo_user_email', restored.email);
        if (profile.face_verified === false) {
          setOnboardingInitialStep('face_verification');
          return false;
        }
        if (!skipAutoClose) {
          localStorage.setItem('kinjo_onboarded', 'true');
          setIsOnboarding(false);
          setShowTutorial(false);
        }
        return true;
      }
    } catch { }

    return false;
  };

  const handleOnboardingComplete = (email?: string, token?: string) => {
    if (token) {
      localStorage.setItem('kinjo_auth_token', token);
      setAuthToken(token);
    }
    if (email) localStorage.setItem('kinjo_user_email', email);
    localStorage.setItem('kinjo_onboarded', 'true');
    setIsOnboarding(false);
  };

  // First time the main app becomes visible on this device — ever, regardless
  // of whether that happened via fresh signup, an existing-account sign-in, or
  // a restored session — show the guided tutorial exactly once.
  useEffect(() => {
    if (isOnboarding) return;
    let seen = false;
    try { seen = localStorage.getItem('kinjo_tutorial_seen') === '1'; } catch { /* ignore */ }
    if (seen) return;
    setTutorialStep('swipe');
    setShowTutorial(true);
  }, [isOnboarding]);

  const handleCloseTutorial = () => {
    try { localStorage.setItem('kinjo_tutorial_seen', '1'); } catch { /* ignore */ }
    setShowTutorial(false);
    setIsPopoverOpen(false);
    setTutorialStep('swipe');
  };

  const advanceTutorialToSettings = () => {
    setTutorialStep('settings');
    if (userProfile) {
      setIsPopoverOpen(true);
    }
  };

  const handleSwipe = (_direction: SwipeDirection, _profile: UserProfile) => {
    if (showTutorial && tutorialStep === 'swipe') {
      advanceTutorialToSettings();
    }
  };

  const handleLogout = () => {
    void stopBackgroundLocation();
    if (authToken) {
      void sendOffline(authToken).catch(() => { });
    }
    if (Capacitor.isNativePlatform()) {
      GoogleAuth.logout().catch(() => { });
    }
    clearSessionStorage();
    setAuthToken('');
    setUserProfile(null);
    setOnboardingInitialStep('email');
    goHome();
    setIsOnboarding(true);
  };

  const handleDeleteAccount = async () => {
    void stopBackgroundLocation();
    if (Capacitor.isNativePlatform()) {
      GoogleAuth.logout().catch(() => { });
    }
    if (authToken) {
      try {
        await deleteAccount(authToken);
        toast.info('Your account has been permanently deleted.');
      } catch (err: any) {
        console.warn('Backend account deletion API warning:', err);
        toast.error(err?.message || 'Could not delete your account on the server. Please try again later.');
      }
    }
    clearSessionStorage();
    setAuthToken('');
    setUserProfile(null);
    setOnboardingInitialStep('email');
    goHome();
    setIsOnboarding(true);
  };

  // Re-validate session whenever app resumes from background or regains focus
  useEffect(() => {
    const token = authToken || localStorage.getItem('kinjo_auth_token');
    if (!token) return;

    const validateSessionOnResume = async () => {
      // Re-register the (possibly rotated) FCM token with the backend. Token
      // rotation has no in-app listener, so this is what keeps push working.
      void initializeFCM(token);
      try {
        await getMyProfile(token);
      } catch {
        clearSessionStorage();
        setAuthToken('');
        setUserProfile(null);
        setIsOnboarding(true);
        toast.error('Your session expired. Please sign in again.');
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

  const handleProfileSetupComplete = (data: { name: string; avatar: string; email?: string; bio?: string; profession: string; lookingFor: string }) => {
    const bioValue = data.bio || data.profession || userProfile?.bio || '';
    const updated: UserProfile = {
      id: userProfile?.id || 'current_user',
      email: data.email || userProfile?.email || '',
      name: data.name || userProfile?.name || 'User',
      avatar: data.avatar || userProfile?.avatar || '',
      bio: bioValue,
      profession: bioValue,
      lookingFor: '',
    };
    setUserProfile(updated);
  };

  return (
    <div
      className="app-shell relative min-h-screen w-full flex flex-col overflow-hidden transition-colors duration-300"
      style={{ background: 'var(--bg)', color: 'var(--fg)' }}
    >

      {/* Onboarding / Login Modal */}
      {isOnboarding && (
        <OnboardingModal
          isOpen={isOnboarding}
          onClose={() => setIsOnboarding(false)}
          onComplete={handleOnboardingComplete}
          onAuthenticated={handleAuthenticated}
          onProfileSetupComplete={handleProfileSetupComplete}
          initialStep={onboardingInitialStep}
          initialProfile={userProfile || undefined}
        />
      )}



      {/* Interactive Onboarding Tutorial Overlay */}
      <OnboardingTutorial
        isOpen={showTutorial}
        step={tutorialStep}
        onAdvance={advanceTutorialToSettings}
        onClose={handleCloseTutorial}
      />

      {/* Main App */}
      {!isOnboarding && (
        <>
          {/* Header with Wordmark and Hamburger Menu */}
          <Header
            onOpenMenu={() => {
              if (showTutorial) {
                setTutorialStep('settings');
              }
              if (userProfile) {
                setIsPopoverOpen(true);
              } else {
                navigate('profile');
              }
            }}
          />

          {/* Floating Profile Popover Box */}
          {userProfile && (
            <ProfilePopover
              isOpen={isPopoverOpen}
              onClose={() => {
                setIsPopoverOpen(false);
                if (showTutorial && tutorialStep === 'settings') {
                  handleCloseTutorial();
                }
              }}
              userProfile={userProfile}
              onOpenSettings={() => {
                if (showTutorial) {
                  handleCloseTutorial();
                }
                navigate('profile');
              }}
            />
          )}

          {/* Main Card Deck Area — top-anchored layout */}
          <main className="flex-1 flex flex-col w-full max-w-md mx-auto relative overflow-hidden">
            <CardDeck
              profiles={profiles}
              isLoading={isLoadingProfiles}
              onRefresh={refreshProfiles}
              onSwipe={handleSwipe}
            />
          </main>

          {/* Full-screen Profile Settings */}
          {screen === 'profile' && userProfile && (
            <div
              className="fixed inset-0 z-50 overflow-y-auto"
              style={{ background: 'var(--bg)', animation: 'screen-slide-in-right 480ms var(--ease) both' }}
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
        </>
      )}

      {/* Splash Screen — Pure black (#000000) */}
      {showSplash && (
        <div
          className="fixed inset-0 z-[9999] flex items-center justify-center select-none"
          style={{ backgroundColor: '#000000' }}
        >
          <img
            src="/kinjo-approach.gif"
            alt="Kinjo"
            className="w-[80vw] max-w-[360px] aspect-square object-contain"
            style={{
              filter: 'contrast(1.15) brightness(0.98)',
            }}
          />
        </div>
      )}
    </div>
  );
}

export default App;



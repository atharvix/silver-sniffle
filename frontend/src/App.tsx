import { useState, useEffect } from 'react';
import type { UserProfile, SwipeDirection } from './types';
import { useGPSLocation } from './hooks/useGPSLocation';

import { App as CapApp } from '@capacitor/app';
import { Capacitor } from '@capacitor/core';

import { Header } from './components/Header';
import { ProfilePopover } from './components/ProfilePopover';
import { CardDeck } from './components/CardDeck';
import { ProfileView } from './components/ProfileView';
import { ProfileDetailScreen } from './components/ProfileDetailScreen';
import { OnboardingModal } from './components/OnboardingModal';
import { OnboardingTutorial } from './components/OnboardingTutorial';

type Screen = 'home' | 'profile' | 'details';

const DEFAULT_USER: UserProfile = {
  id: 'current_user',
  email: 'user@kinjo.local',
  name: 'Kinjo User',
  avatar: 'https://images.unsplash.com/photo-1534528741775-53994a69daeb?auto=format&fit=crop&q=80&w=1000',
  profession: 'Product Builder',
  lookingFor: 'Exploring nearby innovators and creators',
  distanceMeters: 0,
  locationName: '',
  online: true,
};

export function App() {
  const [userProfile, setUserProfile] = useState<UserProfile>(() => {
    const saved = localStorage.getItem('kinjo_user_profile');
    if (saved) {
      try { return { ...DEFAULT_USER, ...JSON.parse(saved) }; } catch (_) {}
    }
    return DEFAULT_USER;
  });

  const [isOnboarding, setIsOnboarding] = useState(() =>
    localStorage.getItem('kinjo_onboarded') !== 'true'
  );
  const [showTutorial, setShowTutorial] = useState(false);
  const [isEditingProfileFromSettings, setIsEditingProfileFromSettings] = useState(false);
  const [authToken, setAuthToken] = useState(() =>
    localStorage.getItem('kinjo_auth_token') || ''
  );
  const [showOpening, setShowOpening] = useState(true);
  const [isPopoverOpen, setIsPopoverOpen] = useState(false);

  // ─── Navigation ─────────────────────────────────────────────────────────────
  const [screen, setScreen] = useState<Screen>('home');
  const [detailProfile, setDetailProfile] = useState<UserProfile | null>(null);

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

  const { profiles: allProfiles } = useGPSLocation(authToken, userProfile);
  const profiles = allProfiles.filter((p) => p.distanceMeters <= 30);

  // Splash
  useEffect(() => {
    const t = setTimeout(() => setShowOpening(false), 2400);
    return () => clearTimeout(t);
  }, []);

  // ─── Hardware & Browser Back Button ──────────────────────────────────────────
  useEffect(() => {
    let listener: any = null;

    if (Capacitor.isNativePlatform()) {
      listener = CapApp.addListener('backButton', () => {
        if (screen !== 'home') {
          goHome();
        } else {
          CapApp.exitApp();
        }
      });
    }

    const handlePopState = () => {
      if (screen !== 'home') {
        goHome();
      }
    };

    window.addEventListener('popstate', handlePopState);
    return () => {
      window.removeEventListener('popstate', handlePopState);
      if (listener) listener.then((h: any) => h?.remove?.());
    };
  }, [screen]);

  // ─── Auth Handlers ───────────────────────────────────────────────────────────
  const handleSaveProfile = (updated: UserProfile) => {
    setUserProfile(updated);
    localStorage.setItem('kinjo_user_profile', JSON.stringify(updated));
  };

  const handleAuthenticated = (token: string, emailArg: string) => {
    localStorage.setItem('kinjo_auth_token', token);
    setAuthToken(token);
    const name = emailArg
      .split('@')[0]
      .replace(/[._-]+/g, ' ')
      .replace(/\b\w/g, (l) => l.toUpperCase());
    const updated = { ...userProfile, email: emailArg, name };
    setUserProfile(updated);
    localStorage.setItem('kinjo_user_profile', JSON.stringify(updated));
  };

  const handleOnboardingComplete = (_email?: string, token?: string, isGuest?: boolean) => {
    if (token) {
      localStorage.setItem('kinjo_auth_token', token);
      setAuthToken(token);
    }
    // Guest bypass does NOT persist kinjo_onboarded so login/onboarding shows on next refresh
    if (!isGuest) {
      localStorage.setItem('kinjo_onboarded', 'true');
    }
    setIsOnboarding(false);
    setShowTutorial(true);
  };

  const handleSwipe = (_direction: SwipeDirection, _profile: UserProfile) => {
    // Card cycling
  };

  const handleLogout = () => {
    localStorage.removeItem('kinjo_auth_token');
    setAuthToken('');
    setUserProfile(DEFAULT_USER);
    goHome();
    setIsOnboarding(true);
  };

  const handleDeleteAccount = () => {
    localStorage.clear();
    setAuthToken('');
    setUserProfile(DEFAULT_USER);
    goHome();
    setIsOnboarding(true);
  };

  const handleProfileSetupComplete = (data: { name: string; avatar: string; profession: string; lookingFor: string }) => {
    const updated = {
      ...userProfile,
      name: data.name || userProfile.name,
      avatar: data.avatar || userProfile.avatar,
      profession: data.profession || userProfile.profession,
      lookingFor: data.lookingFor || userProfile.lookingFor,
    };
    setUserProfile(updated);
    localStorage.setItem('kinjo_user_profile', JSON.stringify(updated));
    setIsEditingProfileFromSettings(false);
  };

  return (
    <div className="app-shell relative min-h-screen w-full bg-[#08080a] text-white flex flex-col overflow-hidden font-sans">

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

      {/* Profile Editing Modal from Settings (Launches Creation Setup Screen) */}
      {isEditingProfileFromSettings && (
        <OnboardingModal
          isOpen={isEditingProfileFromSettings}
          onClose={() => setIsEditingProfileFromSettings(false)}
          onComplete={() => setIsEditingProfileFromSettings(false)}
          onAuthenticated={handleAuthenticated}
          onProfileSetupComplete={handleProfileSetupComplete}
          initialStep="profile_setup"
          initialProfile={{
            name: userProfile.name,
            avatar: userProfile.avatar,
            profession: userProfile.profession,
            lookingFor: userProfile.lookingFor,
          }}
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
          <ProfilePopover
            isOpen={isPopoverOpen}
            onClose={() => setIsPopoverOpen(false)}
            userProfile={userProfile}
            onOpenSettings={() => navigate('profile')}
          />

          {/* Main Card Deck Area */}
          <main className="flex-1 flex items-center justify-center px-3 py-2">
            <CardDeck
              profiles={profiles}
              onSwipe={handleSwipe}
              onOpenDetails={(p) => {
                setDetailProfile(p);
                navigate('details');
              }}
            />
          </main>

          {/* Full-screen Profile Settings */}
          {screen === 'profile' && (
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
                onOpenEditProfile={() => setIsEditingProfileFromSettings(true)}
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



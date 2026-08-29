import { useState, useEffect } from 'react';
import type { UserProfile, SwipeDirection } from './types';
import { useGPSLocation } from './hooks/useGPSLocation';

import { Header } from './components/Header';
import { CardDeck } from './components/CardDeck';
import { ProfileView } from './components/ProfileView';
import { ProfileDetailScreen } from './components/ProfileDetailScreen';
import { OnboardingModal } from './components/OnboardingModal';

// ─── Navigation screens ────────────────────────────────────────────────────────
type Screen = 'home' | 'profile' | 'details';

const DEFAULT_USER: UserProfile = {
  id: 'current_user',
  email: '',
  name: '',
  avatar: '',
  profession: '',
  lookingFor: '',
  bio: '',
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
  const [authToken, setAuthToken] = useState(() =>
    localStorage.getItem('kinjo_auth_token') || ''
  );
  const [showOpening, setShowOpening] = useState(true);

  // ─── Screen navigation ───────────────────────────────────────────────────────
  const [screen, setScreen] = useState<Screen>('home');
  const [detailProfile, setDetailProfile] = useState<UserProfile | null>(null);

  const navigate = (s: Screen) => setScreen(s);


  const { profiles: allProfiles } = useGPSLocation(authToken, userProfile);
  const profiles = allProfiles.filter((p) => p.distanceMeters <= 30);

  // Splash
  useEffect(() => {
    const t = setTimeout(() => setShowOpening(false), 2800);
    return () => clearTimeout(t);
  }, []);

// ─── Screen navigation & hardware back button ──────────────────────────────
  useEffect(() => {
    if (screen === 'home') return;

    window.history.pushState({ screen }, '');
    const handlePopState = () => {
      setScreen('home');
      setDetailProfile(null);
    };

    window.addEventListener('popstate', handlePopState);
    return () => {
      window.removeEventListener('popstate', handlePopState);
    };
  }, [screen]);

  // ─── Auth handlers ───────────────────────────────────────────────────────────
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

  const handleOnboardingComplete = (_email?: string, token?: string) => {
    if (token) { localStorage.setItem('kinjo_auth_token', token); setAuthToken(token); }
    localStorage.setItem('kinjo_onboarded', 'true');
    setIsOnboarding(false);
  };

  const handleSwipe = (_direction: SwipeDirection, _profile: UserProfile) => {
    // Both directions cycle card — no save
  };

  const handleLogout = () => {
    localStorage.removeItem('kinjo_onboarded');
    localStorage.removeItem('kinjo_auth_token');
    localStorage.removeItem('kinjo_user_profile');
    setAuthToken('');
    setUserProfile(DEFAULT_USER);
    setScreen('home');
    setIsOnboarding(true);
  };

  const handleDeleteAccount = () => {
    localStorage.clear();
    setAuthToken('');
    setUserProfile(DEFAULT_USER);
    setScreen('home');
    setIsOnboarding(true);
  };

  return (
    <div className="app-shell relative min-h-screen w-full bg-black text-white flex flex-col overflow-hidden font-sans">

      {/* ── ONBOARDING (full-screen auth) ── */}
      {isOnboarding && (
        <OnboardingModal
          isOpen={isOnboarding}
          onClose={() => setIsOnboarding(false)}
          onComplete={handleOnboardingComplete}
          onAuthenticated={handleAuthenticated}
        />
      )}

      {/* ── MAIN APP ── */}
      {!isOnboarding && (
        <>
          {/* HOME SCREEN */}
          <Header onOpenMenu={() => navigate('profile')} />
          <main className="flex-1 flex items-center justify-center px-3 py-4">
            <CardDeck
              profiles={profiles}
              onSwipe={handleSwipe}
              onOpenDetails={(p) => {
                setDetailProfile(p);
                navigate('details');
              }}
            />
          </main>

          {/* PROFILE SCREEN — full-screen page, slides in from right */}
          {screen === 'profile' && (
            <div
              className="fixed inset-0 z-60 bg-[#060606] overflow-y-auto"
              style={{ animation: 'screen-slide-in-right 260ms cubic-bezier(0.32,0.72,0,1) both' }}
            >
              <ProfileView
                userProfile={userProfile}
                onSave={handleSaveProfile}
                onLogout={handleLogout}
                onDeleteAccount={handleDeleteAccount}
                onClose={() => navigate('home')}
              />
            </div>
          )}

          {/* DETAIL SCREEN — full-screen page, slides up from bottom */}
          {screen === 'details' && detailProfile && (
            <ProfileDetailScreen
              profile={detailProfile}
              onClose={() => { setDetailProfile(null); navigate('home'); }}
            />
          )}
        </>
      )}

      {/* SPLASH */}
      {showOpening && (
        <div className="opening-screen">
          <div className="opening-logo-wrap">
            <div className="opening-mark">k<span>.</span></div>
          </div>
          <p className="opening-tagline">getting into the world</p>
        </div>
      )}
    </div>
  );
}

export default App;

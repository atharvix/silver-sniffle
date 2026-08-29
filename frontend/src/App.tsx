import { useState, useEffect } from 'react';
import type { UserProfile, SwipeDirection } from './types';
import { useGPSLocation } from './hooks/useGPSLocation';

import { Header } from './components/Header';
import { CardDeck } from './components/CardDeck';
import { ProfileView } from './components/ProfileView';
import { ProfileDetailsModal } from './components/ProfileDetailsModal';
import { OnboardingModal } from './components/OnboardingModal';

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

  // Hamburger side drawer state
  const [isMenuOpen, setIsMenuOpen] = useState(false);

  // Card detail modal
  const [detailProfile, setDetailProfile] = useState<UserProfile | null>(null);

  const { profiles: allProfiles } = useGPSLocation(authToken, userProfile);

  // Filter to ≤30m
  const profiles = allProfiles.filter((p) => p.distanceMeters <= 30);

  // Splash: visible for 2800ms then fades out
  useEffect(() => {
    const t = setTimeout(() => setShowOpening(false), 2800);
    return () => clearTimeout(t);
  }, []);

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
    // Both directions just cycle card — no save, no action
  };

  const handleLogout = () => {
    localStorage.removeItem('kinjo_onboarded');
    localStorage.removeItem('kinjo_auth_token');
    localStorage.removeItem('kinjo_user_profile');
    setAuthToken('');
    setUserProfile(DEFAULT_USER);
    setIsMenuOpen(false);
    setIsOnboarding(true);
  };

  const handleDeleteAccount = () => {
    localStorage.clear();
    setAuthToken('');
    setUserProfile(DEFAULT_USER);
    setIsMenuOpen(false);
    setIsOnboarding(true);
  };

  return (
    <div className="app-shell relative min-h-screen w-full bg-black text-white flex flex-col overflow-x-hidden font-sans">

      {/* Onboarding Auth (full-screen, no backdrop) */}
      {isOnboarding && (
        <OnboardingModal
          isOpen={isOnboarding}
          onClose={() => setIsOnboarding(false)}
          onComplete={handleOnboardingComplete}
          onAuthenticated={handleAuthenticated}
        />
      )}

      {/* ──────────────────────────────────────
          MAIN APP UI
          ────────────────────────────────────── */}
      {!isOnboarding && (
        <>
          <Header onOpenMenu={() => setIsMenuOpen(true)} />

          <main className="flex-1 flex items-center justify-center px-3 py-4">
            <CardDeck
              profiles={profiles}
              onSwipe={handleSwipe}
              onOpenDetails={(p) => setDetailProfile(p)}
            />
          </main>

          {/* Profile Details Bottom Sheet */}
          <ProfileDetailsModal
            profile={detailProfile}
            isOpen={!!detailProfile}
            onClose={() => setDetailProfile(null)}
          />

          {/* Side Drawer (Hamburger Menu) */}
          {isMenuOpen && (
            <>
              {/* Backdrop */}
              <div
                className="side-drawer-overlay"
                onClick={() => setIsMenuOpen(false)}
              />
              {/* Drawer */}
              <div className="side-drawer">
                <ProfileView
                  userProfile={userProfile}
                  onSave={handleSaveProfile}
                  onLogout={handleLogout}
                  onDeleteAccount={handleDeleteAccount}
                  onClose={() => setIsMenuOpen(false)}
                />
              </div>
            </>
          )}
        </>
      )}

      {/* Splash Screen */}
      {showOpening && (
        <div className="opening-screen">
          <div className="opening-logo-wrap">
            <div className="opening-mark">
              k<span>.</span>
            </div>
          </div>
          <p className="opening-tagline">getting into the world</p>
        </div>
      )}
    </div>
  );
}

export default App;

import { useState, useEffect, useMemo } from 'react';
import type { UserProfile, MatchSignal, SwipeDirection } from './types';
import { INITIAL_PROFILES, INITIAL_USER_PROFILE } from './data/mockProfiles';
import { useGPSLocation } from './hooks/useGPSLocation';

import { ConstellationBackground } from './components/ConstellationBackground';
import { Header } from './components/Header';
import { CardDeck } from './components/CardDeck';
import { ProfileView } from './components/ProfileView';
import { BottomBar } from './components/BottomBar';
import { ProfileDetailsModal } from './components/ProfileDetailsModal';
import { ConnectedDrawer } from './components/ConnectedDrawer';
import { LocationPickerModal } from './components/LocationPickerModal';
import { OnboardingModal } from './components/OnboardingModal';
import { MatchNotificationModal } from './components/MatchNotificationModal';
import Lenis from 'lenis';

export function App() {
  const [userProfile, setUserProfile] = useState<UserProfile>(() => {
    const saved = localStorage.getItem('kinjo_user_profile');
    if (saved) {
      try {
        return JSON.parse(saved);
      } catch (e) {}
    }
    return INITIAL_USER_PROFILE;
  });

  // Onboarding & Account State (Hidden by default for instant app testing & shipping)
  const [isOnboardingOpen, setIsOnboardingOpen] = useState<boolean>(() => {
    return localStorage.getItem('kinjo_onboarded') !== 'true';
  });
  const [showOpening, setShowOpening] = useState(true);
  const [authToken, setAuthToken] = useState(() => localStorage.getItem('kinjo_auth_token') || '');

  const [isDeactivated, setIsDeactivated] = useState(false);
  const [isLocationPickerOpen, setIsLocationPickerOpen] = useState(false);

  // Device GPS Location tracking & 30m distance calculations (with Location Override)
  const { gps, profiles: allProfiles, setCustomLocation, resetToAutoGPS } = useGPSLocation(INITIAL_PROFILES, authToken, userProfile);

  const [, setSwipeHistory] = useState<{ profile: UserProfile; direction: SwipeDirection }[]>([]);
  const [matches, setMatches] = useState<MatchSignal[]>(() => {
    const saved = localStorage.getItem('kinjo_matches');
    if (saved) {
      try {
        return JSON.parse(saved);
      } catch (e) {}
    }
    return [];
  });

  // Active Navigation Tab ('cards' | 'profile')
  const [activeTab, setActiveTab] = useState<'cards' | 'profile'>('cards');
  const [selectedDetailProfile, setSelectedDetailProfile] = useState<UserProfile | null>(null);
  const [recentMatchProfile, setRecentMatchProfile] = useState<UserProfile | null>(null);
  const [isConnectedDrawerOpen, setIsConnectedDrawerOpen] = useState(false);

  // Initialize Lenis smooth scroll
  useEffect(() => {
    const lenis = new Lenis({
      duration: 1.2,
      easing: (t) => Math.min(1, 1.001 - Math.pow(2, -10 * t)),
    });

    function raf(time: number) {
      lenis.raf(time);
      requestAnimationFrame(raf);
    }

    const rafId = requestAnimationFrame(raf);
    return () => {
      cancelAnimationFrame(rafId);
      lenis.destroy();
    };
  }, []);

  useEffect(() => {
    const timer = window.setTimeout(() => setShowOpening(false), 1800);
    return () => window.clearTimeout(timer);
  }, []);

  // Filter profiles strictly within 30m radius
  const filteredProfiles = useMemo(() => {
    if (isDeactivated) return [];
    return allProfiles.filter((p) => p.distanceMeters <= 30);
  }, [allProfiles, isDeactivated]);

  // Extract right-swiped profiles for Hamburg drawer
  const connectedProfiles = useMemo(() => {
    const matchedProfileIds = new Set(matches.map((m) => m.profile.id));
    return allProfiles.filter((p) => matchedProfileIds.has(p.id));
  }, [allProfiles, matches]);

  const handleSwipe = (direction: SwipeDirection, profile: UserProfile) => {
    setSwipeHistory((prev) => [...prev, { profile, direction }]);

    if (direction === 'right') {
      const newMatch: MatchSignal = {
        id: `match_${Date.now()}`,
        profile,
        timestamp: new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }),
        matchedAtDistance: profile.distanceMeters,
      };

      const updatedMatches = [newMatch, ...matches];
      setMatches(updatedMatches);
      localStorage.setItem('kinjo_matches', JSON.stringify(updatedMatches));
      setRecentMatchProfile(profile);
    }
  };

  const handleRemoveMatch = (profileId: string) => {
    const updated = matches.filter((m) => m.profile.id !== profileId);
    setMatches(updated);
    localStorage.setItem('kinjo_matches', JSON.stringify(updated));
  };

  const handleSaveUserProfile = (updated: UserProfile) => {
    setUserProfile(updated);
    localStorage.setItem('kinjo_user_profile', JSON.stringify(updated));
  };

  const handleCompleteOnboarding = (_userEmail?: string, token?: string) => {
    if (token) {
      localStorage.setItem('kinjo_auth_token', token);
      setAuthToken(token);
    }
    localStorage.setItem('kinjo_onboarded', 'true');
    setIsOnboardingOpen(false);
  };

  const handleLogout = () => {
    localStorage.removeItem('kinjo_onboarded');
    localStorage.removeItem('kinjo_auth_token');
    setAuthToken('');
    setIsOnboardingOpen(true);
    setActiveTab('cards');
  };

  const handleDeactivateAccount = () => {
    setIsDeactivated((prev) => !prev);
  };

  const handleDeleteAccount = () => {
    localStorage.clear();
    setAuthToken('');
    setUserProfile(INITIAL_USER_PROFILE);
    setMatches([]);
    setIsOnboardingOpen(true);
    setActiveTab('cards');
  };

  return (
    <div className="relative min-h-screen w-full bg-[#05040a] text-white flex flex-col justify-between overflow-x-hidden select-none font-sans">
      {/* Ambient Cosmic Indigo/Violet Glow Spheres Background */}
      <ConstellationBackground />

      {/* Top Header displaying "kinjo.", Area & City location pill, and Hamburg Menu button */}
      <Header
        gps={gps}
        connectedCount={connectedProfiles.length}
        onOpenConnectedDrawer={() => setIsConnectedDrawerOpen(true)}
        onOpenLocationPicker={() => setIsLocationPickerOpen(true)}
      />

      {/* Main Screen Content Switching: Cards Deck vs. Full-Page Profile Dashboard */}
      <main className="relative z-10 flex-1 flex items-center justify-center py-6 px-4">
        {activeTab === 'cards' ? (
          <CardDeck
            profiles={filteredProfiles}
            onSwipe={handleSwipe}
            onOpenDetails={(p) => setSelectedDetailProfile(p)}
          />
        ) : (
          <ProfileView
            userProfile={userProfile}
            onSave={handleSaveUserProfile}
            onLogout={handleLogout}
            onDeactivateAccount={handleDeactivateAccount}
            onDeleteAccount={handleDeleteAccount}
          />
        )}
      </main>

      {/* Bottom Glass Navigation Bar */}
      <BottomBar
        activeTab={activeTab}
        onTabChange={(tab) => setActiveTab(tab)}
      />

      {/* Location Picker & Search Modal */}
      <LocationPickerModal
        isOpen={isLocationPickerOpen}
        onClose={() => setIsLocationPickerOpen(false)}
        currentLocation={gps.formattedLocation}
        onSelectLocation={setCustomLocation}
        onUseAutoGPS={resetToAutoGPS}
      />

      {/* Onboarding Authentication & Feature Tour Modal */}
      <OnboardingModal
        isOpen={isOnboardingOpen}
        onClose={() => setIsOnboardingOpen(false)}
        onComplete={handleCompleteOnboarding}
        onAuthenticated={(token) => {
          localStorage.setItem('kinjo_auth_token', token);
          setAuthToken(token);
        }}
      />

      {/* Hamburg Menu Drawer displaying Right-Swiped Connected Profiles */}
      <ConnectedDrawer
        isOpen={isConnectedDrawerOpen}
        onClose={() => setIsConnectedDrawerOpen(false)}
        connectedProfiles={connectedProfiles}
        onOpenDetails={(p) => setSelectedDetailProfile(p)}
        onRemoveMatch={handleRemoveMatch}
      />

      {/* Profile Details Modal for inspecting cards */}
      <ProfileDetailsModal
        profile={selectedDetailProfile}
        isOpen={!!selectedDetailProfile}
        onClose={() => setSelectedDetailProfile(null)}
        onConnect={(p) => handleSwipe('right', p)}
      />

      {/* Match Notification */}
      <MatchNotificationModal
        matchedProfile={recentMatchProfile}
        onClose={() => setRecentMatchProfile(null)}
      />

      {showOpening && (
        <div className="opening-screen" aria-label="Opening kinjo">
          <div className="opening-mark">kinjo<span>.</span></div>
          <div className="opening-line" />
          <p>meet nearby. make it matter.</p>
        </div>
      )}
    </div>
  );
}

export default App;

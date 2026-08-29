import { useState, useEffect, useMemo } from 'react';
import type { UserProfile, MatchSignal, SwipeDirection } from './types';
import { useGPSLocation } from './hooks/useGPSLocation';

import { ConstellationBackground } from './components/ConstellationBackground';
import { Header } from './components/Header';
import { CardDeck } from './components/CardDeck';
import { ProfileView } from './components/ProfileView';
import { BottomBar, type TabType } from './components/BottomBar';
import { ProfileDetailsModal } from './components/ProfileDetailsModal';
import { ConnectedDrawer } from './components/ConnectedDrawer';
import { LocationPickerModal } from './components/LocationPickerModal';
import { OnboardingModal } from './components/OnboardingModal';
import { MatchNotificationModal } from './components/MatchNotificationModal';
import Lenis from 'lenis';
import { createConnection } from './utils/api';

export function App() {
  const [userProfile, setUserProfile] = useState<UserProfile>(() => {
    const saved = localStorage.getItem('kinjo_user_profile');
    if (saved) {
      try {
        return JSON.parse(saved);
      } catch (e) {}
    }
    return {
      id: 'current_user',
      name: 'Heston Mogotlane',
      handle: '@heston_m',
      subtitle: 'Co-founder, Medical Startup',
      avatar: 'https://images.unsplash.com/photo-1534528741775-53994a69daeb?auto=format&fit=crop&q=80&w=1000',
      quotePrompt: 'Looking for a co-founder for my medical startup',
      distanceMeters: 0,
      category: 'Health',
      tags: ['HealthTech', 'Startups', 'AI'],
      interests: ['Tech', 'Startups', 'AI', 'Hiking', 'Coffee'],
      statuses: [
        'Looking for a co-founder for my medical startup',
        'Excited for the new project launch!',
      ],
      bio: 'Passionate about healthcare innovation. Building a team to make a real difference.',
      locationName: 'Jaipur, Malviya Nagar',
      online: true,
    };
  });

  // Onboarding & Account State
  const [isOnboardingOpen, setIsOnboardingOpen] = useState<boolean>(() => {
    return localStorage.getItem('kinjo_onboarded') !== 'true';
  });
  const [showOpening, setShowOpening] = useState(true);
  const [authToken, setAuthToken] = useState(() => localStorage.getItem('kinjo_auth_token') || '');

  const [isDeactivated, setIsDeactivated] = useState(false);
  const [isLocationPickerOpen, setIsLocationPickerOpen] = useState(false);

  // Device GPS Location tracking
  const { gps, profiles: allProfiles, setCustomLocation, resetToAutoGPS } = useGPSLocation(authToken, userProfile);

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

  // Navigation Tab ('radar' | 'cards' | 'saved' | 'profile')
  const [activeTab, setActiveTab] = useState<TabType>('cards');
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
    const timer = window.setTimeout(() => setShowOpening(false), 2800);
    return () => window.clearTimeout(timer);
  }, []);

  // Filter profiles strictly within 30m radius
  const filteredProfiles = useMemo(() => {
    if (isDeactivated) return [];
    return allProfiles.filter((p) => p.distanceMeters <= 30);
  }, [allProfiles, isDeactivated]);

  // Extract saved profiles for Connected Drawer
  const connectedProfiles = useMemo(() => {
    const matchedProfileIds = new Set(matches.map((m) => m.profile.id));
    return allProfiles.filter((p) => matchedProfileIds.has(p.id));
  }, [allProfiles, matches]);

  const handleTabChange = (tab: TabType) => {
    setActiveTab(tab);
    if (tab === 'radar') {
      setIsLocationPickerOpen(true);
    }
  };

  // SWIPE LOGIC: LEFT SWIPE SAVES PROFILE & NOTIFIES, RIGHT SWIPE DOES NOTHING (PUSHES BACK TO STACK)
  const handleSwipe = (direction: SwipeDirection, profile: UserProfile) => {
    setSwipeHistory((prev) => [...prev, { profile, direction }]);

    if (direction === 'left') {
      if (authToken && profile.email && !profile.email.endsWith('@kinjo.local')) {
        void createConnection(profile.email, authToken).catch(() => {});
      }

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
    // Right swipe does nothing (just cycles card back to stack)
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
    if (_userEmail === 'test@kinjo.local') {
      setUserProfile((profile) => ({ ...profile, name: 'Heston Mogotlane', handle: '@heston_m' }));
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
    setUserProfile({
      id: 'current_user', name: '', handle: '', avatar: '', quotePrompt: '', distanceMeters: 0,
      category: 'Other', tags: [], bio: '', locationName: '', online: true,
    });
    setMatches([]);
    setIsOnboardingOpen(true);
    setActiveTab('cards');
  };

  return (
    <div className="app-shell relative min-h-screen w-full bg-black text-white flex flex-col justify-between overflow-x-hidden select-none font-sans">
      {/* Pure Black Background */}
      <ConstellationBackground />

      {/* Top Header displaying "k.", Area & City location pill, and Hamburg Menu button */}
      <Header
        gps={gps}
        connectedCount={connectedProfiles.length}
        onOpenConnectedDrawer={() => setIsConnectedDrawerOpen(true)}
        onOpenLocationPicker={() => setIsLocationPickerOpen(true)}
      />

      {/* Main Screen Content */}
      <main className="app-main relative z-10 flex-1 flex items-center justify-center px-3 pb-24 sm:py-6 sm:px-4">
        {activeTab === 'profile' ? (
          <ProfileView
            userProfile={userProfile}
            onSave={handleSaveUserProfile}
            onLogout={handleLogout}
            onDeactivateAccount={handleDeactivateAccount}
            onDeleteAccount={handleDeleteAccount}
          />
        ) : (
          <CardDeck
            profiles={filteredProfiles}
            onSwipe={handleSwipe}
            onOpenDetails={(p) => setSelectedDetailProfile(p)}
          />
        )}
      </main>

      {/* Floating Bottom Navigation Bar */}
      <BottomBar
        activeTab={activeTab}
        onTabChange={handleTabChange}
      />

      {/* Location Picker Modal */}
      <LocationPickerModal
        isOpen={isLocationPickerOpen}
        onClose={() => setIsLocationPickerOpen(false)}
        currentLocation={gps.formattedLocation}
        onSelectLocation={setCustomLocation}
        onUseAutoGPS={resetToAutoGPS}
      />

      {/* Onboarding Authentication Modal */}
      <OnboardingModal
        isOpen={isOnboardingOpen}
        onClose={() => setIsOnboardingOpen(false)}
        onComplete={handleCompleteOnboarding}
        onAuthenticated={(token, email) => {
          localStorage.setItem('kinjo_auth_token', token);
          setAuthToken(token);
          const name = email.split('@')[0].replace(/[._-]+/g, ' ').replace(/\b\w/g, (letter) => letter.toUpperCase());
          setUserProfile((profile) => ({ ...profile, name, handle: `@${email.split('@')[0]}` }));
        }}
      />

      {/* Inward Flush Saved Profiles Drawer */}
      <ConnectedDrawer
        isOpen={isConnectedDrawerOpen}
        onClose={() => setIsConnectedDrawerOpen(false)}
        connectedProfiles={connectedProfiles}
        onOpenDetails={(p) => setSelectedDetailProfile(p)}
        onRemoveMatch={handleRemoveMatch}
      />

      {/* Profile Details Modal */}
      <ProfileDetailsModal
        profile={selectedDetailProfile}
        isOpen={!!selectedDetailProfile}
        onClose={() => setSelectedDetailProfile(null)}
        onConnect={(p) => handleSwipe('left', p)}
      />

      {/* Match Notification */}
      <MatchNotificationModal
        matchedProfile={recentMatchProfile}
        onClose={() => setRecentMatchProfile(null)}
      />

      {showOpening && (
        <div className="opening-screen" aria-label="Opening k.">
          <div className="opening-mark">k<span>.</span></div>
          <div className="opening-line" />
          <p>meet nearby. make it matter.</p>
        </div>
      )}
    </div>
  );
}

export default App;

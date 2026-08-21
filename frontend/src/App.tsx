import { useState, useEffect, useMemo } from 'react';
import type { UserProfile, FilterState, MatchSignal, SwipeDirection } from './types';
import { INITIAL_PROFILES, INITIAL_USER_PROFILE } from './data/mockProfiles';
import { INITIAL_EVENTS } from './data/mockEvents';
import type { SocialEvent } from './data/mockEvents';

import { ConstellationBackground } from './components/ConstellationBackground';
import { Header } from './components/Header';
import { CardDeck } from './components/CardDeck';
import { BottomBar } from './components/BottomBar';
import { ProfileEditorModal } from './components/ProfileEditorModal';
import { ProfileDetailsModal } from './components/ProfileDetailsModal';
import { FilterModal } from './components/FilterModal';
import { EventsModal } from './components/EventsModal';
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

  const [allProfiles] = useState<UserProfile[]>(INITIAL_PROFILES);
  const [swipeHistory, setSwipeHistory] = useState<{ profile: UserProfile; direction: SwipeDirection }[]>([]);
  const [matches, setMatches] = useState<MatchSignal[]>(() => {
    const saved = localStorage.getItem('kinjo_matches');
    if (saved) {
      try {
        return JSON.parse(saved);
      } catch (e) {}
    }
    return [];
  });

  // Social Events State
  const [events, setEvents] = useState<SocialEvent[]>(() => {
    const saved = localStorage.getItem('kinjo_events');
    if (saved) {
      try {
        return JSON.parse(saved);
      } catch (e) {}
    }
    return INITIAL_EVENTS;
  });

  // Filter & Search State (Default 100m)
  const [filters, setFilters] = useState<FilterState>({
    searchQuery: '',
    maxRadiusMeters: 100,
    selectedCategory: 'All',
    onlyOnline: false,
  });

  // Active Navigation & Modal States
  const [activeTab, setActiveTab] = useState<'cards' | 'events' | 'profile'>('cards');
  const [selectedDetailProfile, setSelectedDetailProfile] = useState<UserProfile | null>(null);
  const [recentMatchProfile, setRecentMatchProfile] = useState<UserProfile | null>(null);
  const [isFilterModalOpen, setIsFilterModalOpen] = useState(false);

  // Initialize Lenis smooth scroll for global window
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

  // Filter profiles based on radius (e.g. 10m Event / 50m Building / 100m Block), bio search, category
  const filteredProfiles = useMemo(() => {
    const swipedIds = new Set(swipeHistory.map((h) => h.profile.id));
    return allProfiles.filter((p) => {
      if (swipedIds.has(p.id)) return false;
      if (p.distanceMeters > filters.maxRadiusMeters) return false;
      if (filters.selectedCategory !== 'All' && p.category !== filters.selectedCategory) return false;
      if (filters.onlyOnline && !p.online) return false;

      if (filters.searchQuery.trim()) {
        const query = filters.searchQuery.toLowerCase();
        const bioMatch = p.bio.toLowerCase().includes(query);
        const quoteMatch = p.quotePrompt.toLowerCase().includes(query);
        const tagMatch = p.tags.some((t) => t.toLowerCase().includes(query));
        const nameMatch = p.name.toLowerCase().includes(query);
        if (!bioMatch && !quoteMatch && !tagMatch && !nameMatch) return false;
      }

      return true;
    });
  }, [allProfiles, swipeHistory, filters]);

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

  const handleUndo = () => {
    if (swipeHistory.length === 0) return;
    const last = swipeHistory[swipeHistory.length - 1];
    setSwipeHistory((prev) => prev.slice(0, -1));

    if (last.direction === 'right') {
      const updatedMatches = matches.filter((m) => m.profile.id !== last.profile.id);
      setMatches(updatedMatches);
      localStorage.setItem('kinjo_matches', JSON.stringify(updatedMatches));
    }
  };

  const handleSaveUserProfile = (updated: UserProfile) => {
    setUserProfile(updated);
    localStorage.setItem('kinjo_user_profile', JSON.stringify(updated));
  };

  // Join or leave an event
  const handleJoinEvent = (eventId: string) => {
    const updatedEvents = events.map((ev) => {
      if (ev.id === eventId) {
        const isJoined = !ev.joined;
        return {
          ...ev,
          joined: isJoined,
          attendeesCount: isJoined ? ev.attendeesCount + 1 : ev.attendeesCount - 1,
        };
      }
      return ev;
    });
    setEvents(updatedEvents);
    localStorage.setItem('kinjo_events', JSON.stringify(updatedEvents));
  };

  // Create a new event
  const handleCreateEvent = (newEventData: Omit<SocialEvent, 'id' | 'attendeesCount'>) => {
    const newEv: SocialEvent = {
      ...newEventData,
      id: `ev_${Date.now()}`,
      attendeesCount: 1,
      joined: true,
    };
    const updatedEvents = [newEv, ...events];
    setEvents(updatedEvents);
    localStorage.setItem('kinjo_events', JSON.stringify(updatedEvents));
  };

  return (
    <div className="relative min-h-screen w-full bg-[#070709] text-white flex flex-col justify-between overflow-x-hidden select-none font-sans">
      {/* Subtle Constellation Line Background */}
      <ConstellationBackground />

      {/* Top Header with Instant Radius Presets (10m Event, 50m Building, 100m Block) & Filter Modal */}
      <Header
        filters={filters}
        onUpdateRadius={(r) => setFilters((prev) => ({ ...prev, maxRadiusMeters: r }))}
        onOpenFilterModal={() => setIsFilterModalOpen(true)}
      />

      {/* Main Container: Stacked Card Deck */}
      <main className="relative z-10 flex-1 flex items-center justify-center py-6 px-4">
        <CardDeck
          profiles={filteredProfiles}
          onSwipe={handleSwipe}
          onUndo={handleUndo}
          canUndo={swipeHistory.length > 0}
          onOpenDetails={(p) => setSelectedDetailProfile(p)}
        />
      </main>

      {/* Bottom Glass Navigation Bar (3 Tabs: Cards, Events, Profile) */}
      <BottomBar
        activeTab={activeTab}
        onTabChange={(tab) => setActiveTab(tab)}
        eventsCount={events.length}
      />

      {/* User Profile & Settings Drawer Modal (Strict 50-100 Word Count Limit) */}
      <ProfileEditorModal
        userProfile={userProfile}
        isOpen={activeTab === 'profile'}
        onClose={() => setActiveTab('cards')}
        onSave={handleSaveUserProfile}
      />

      {/* Events & Meets Drawer Modal */}
      <EventsModal
        isOpen={activeTab === 'events'}
        onClose={() => setActiveTab('cards')}
        events={events}
        onJoinEvent={handleJoinEvent}
        onCreateEvent={handleCreateEvent}
      />

      {/* Profile Details Modal */}
      <ProfileDetailsModal
        profile={selectedDetailProfile}
        isOpen={!!selectedDetailProfile}
        onClose={() => setSelectedDetailProfile(null)}
        onConnect={(p) => handleSwipe('right', p)}
      />

      {/* Search & Radius Filter Modal Drawer */}
      <FilterModal
        isOpen={isFilterModalOpen}
        onClose={() => setIsFilterModalOpen(false)}
        filters={filters}
        onUpdateFilters={setFilters}
        onResetFilters={() =>
          setFilters({
            searchQuery: '',
            maxRadiusMeters: 100,
            selectedCategory: 'All',
            onlyOnline: false,
          })
        }
      />

      {/* Match Notification */}
      <MatchNotificationModal
        matchedProfile={recentMatchProfile}
        onClose={() => setRecentMatchProfile(null)}
      />
    </div>
  );
}

export default App;

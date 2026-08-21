import React from 'react';
import { Layers, Calendar, User } from 'lucide-react';

interface BottomBarProps {
  activeTab: 'cards' | 'events' | 'profile';
  onTabChange: (tab: 'cards' | 'events' | 'profile') => void;
  eventsCount: number;
}

export const BottomBar: React.FC<BottomBarProps> = ({
  activeTab,
  onTabChange,
  eventsCount,
}) => {
  return (
    <nav className="fixed bottom-6 left-1/2 -translate-x-1/2 z-40 w-auto">
      <div className="bottom-glass-bar rounded-full px-5 py-2.5 flex items-center justify-between gap-8 md:gap-10">
        {/* Tab 1: Stacked Cards (Center Active Pill) */}
        <button
          onClick={() => onTabChange('cards')}
          className={`w-11 h-10 rounded-full flex items-center justify-center transition-all active:scale-95 ${
            activeTab === 'cards'
              ? 'bg-[#f2ece1] text-neutral-950 shadow-md shadow-black/40'
              : 'text-neutral-400 hover:text-white'
          }`}
          title="Cards Stack"
        >
          <Layers className="w-5 h-5 stroke-[2]" />
        </button>

        {/* Tab 2: Nearby Events & Social Meets */}
        <button
          onClick={() => onTabChange('events')}
          className={`p-2.5 rounded-full relative transition-all active:scale-90 ${
            activeTab === 'events' ? 'text-white' : 'text-neutral-400 hover:text-white'
          }`}
          title="Nearby Events & Social Meets"
        >
          <Calendar className="w-5 h-5 stroke-[1.8]" />
          {eventsCount > 0 && (
            <span className="absolute top-1.5 right-1.5 w-2 h-2 rounded-full bg-emerald-400 animate-pulse" />
          )}
        </button>

        {/* Tab 3: Profile & Settings */}
        <button
          onClick={() => onTabChange('profile')}
          className={`p-2.5 rounded-full transition-all active:scale-90 ${
            activeTab === 'profile' ? 'text-white' : 'text-neutral-400 hover:text-white'
          }`}
          title="User Profile & Settings"
        >
          <User className="w-5 h-5 stroke-[1.8]" />
        </button>
      </div>
    </nav>
  );
};

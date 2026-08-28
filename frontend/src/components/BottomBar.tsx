import React from 'react';
import { Layers, User } from 'lucide-react';

interface BottomBarProps {
  activeTab: 'cards' | 'profile';
  onTabChange: (tab: 'cards' | 'profile') => void;
}

export const BottomBar: React.FC<BottomBarProps> = ({
  activeTab,
  onTabChange,
}) => {
  return (
    <nav className="fixed bottom-4 sm:bottom-6 left-1/2 -translate-x-1/2 z-40 w-auto select-none">
      <div className="bottom-glass-bar rounded-full px-3.5 py-1.5 sm:px-4 sm:py-2 flex items-center justify-between gap-4 sm:gap-6">
        {/* Tab 1: Stacked Cards */}
        <button
          onClick={() => onTabChange('cards')}
          className={`flex items-center gap-2 px-3.5 py-1.5 sm:px-4 sm:py-2 rounded-full transition-all active:scale-95 ${
            activeTab === 'cards'
              ? 'bg-white text-black font-extrabold shadow-lg'
              : 'text-neutral-400 hover:text-white font-medium'
          }`}
          title="Cards Stack (30m)"
        >
          <Layers className="w-4 h-4 stroke-[2]" />
          <span className="text-xs">Cards</span>
        </button>

        {/* Tab 2: User Profile & Settings */}
        <button
          onClick={() => onTabChange('profile')}
          className={`flex items-center gap-2 px-3.5 py-1.5 sm:px-4 sm:py-2 rounded-full transition-all active:scale-95 ${
            activeTab === 'profile'
              ? 'bg-white text-black font-extrabold shadow-lg'
              : 'text-neutral-400 hover:text-white font-medium'
          }`}
          title="User Profile & Liveness Detector"
        >
          <User className="w-4 h-4 stroke-[2]" />
          <span className="text-xs">Profile</span>
        </button>

      </div>
    </nav>
  );
};

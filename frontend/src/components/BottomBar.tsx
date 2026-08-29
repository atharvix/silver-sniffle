import React from 'react';
import { Navigation, Layers, User } from 'lucide-react';

export type TabType = 'radar' | 'cards' | 'profile';

interface BottomBarProps {
  activeTab: TabType;
  onTabChange: (tab: TabType) => void;
}

export const BottomBar: React.FC<BottomBarProps> = ({
  activeTab,
  onTabChange,
}) => {
  const navItems: { id: TabType; label: string; icon: React.FC<{ className?: string }> }[] = [
    { id: 'radar', label: 'Radar', icon: Navigation },
    { id: 'cards', label: 'Cards Stack', icon: Layers },
    { id: 'profile', label: 'Profile', icon: User },
  ];

  return (
    <nav className="fixed bottom-5 left-1/2 -translate-x-1/2 z-40 w-auto select-none">
      <div className="bg-[#0a0a0d]/90 backdrop-blur-2xl border border-white/18 rounded-full p-1.5 sm:p-2 flex items-center gap-3 sm:gap-4 shadow-[0_24px_60px_rgba(0,0,0,0.9)]">
        {navItems.map((item) => {
          const Icon = item.icon;
          const isActive = activeTab === item.id;

          return (
            <button
              key={item.id}
              onClick={() => onTabChange(item.id)}
              className={`relative flex items-center justify-center transition-all duration-200 active:scale-90 ${
                isActive
                  ? 'w-10 h-10 sm:w-11 sm:h-11 rounded-full bg-white text-black font-extrabold shadow-[0_4px_20px_rgba(255,255,255,0.35)] scale-105'
                  : 'w-10 h-10 sm:w-11 sm:h-11 rounded-full text-neutral-400 hover:text-white hover:bg-white/10'
              }`}
              title={item.label}
            >
              <Icon className={`w-5 h-5 ${isActive ? 'text-black stroke-[2.5]' : 'stroke-[1.8]'}`} />
            </button>
          );
        })}
      </div>
    </nav>
  );
};

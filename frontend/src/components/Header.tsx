import React from 'react';
import type { UserProfile } from '../types';

interface HeaderProps {
  userProfile?: UserProfile;
  onOpenMenu: () => void;
}

export const Header: React.FC<HeaderProps> = ({ userProfile, onOpenMenu }) => {
  return (
    <header className="app-header relative z-30 w-full max-w-md mx-auto px-5 py-3 flex items-center justify-between select-none">
      {/* Top Corner Logo */}
      <span className="text-[28px] sm:text-3xl font-extrabold tracking-tight text-white font-sans leading-none">
        k<span className="text-white/30">.</span>
      </span>

      {/* User Profile Circle Photo (Replaces 3 bars) */}
      <button
        onClick={onOpenMenu}
        className="relative w-9 h-9 rounded-full overflow-hidden border border-white/25 bg-white/10 hover:border-white/60 transition-all active:scale-95 shadow-md flex items-center justify-center shrink-0"
        title="Account & Settings"
      >
        {userProfile?.avatar ? (
          <img
            src={userProfile.avatar}
            alt={userProfile.name || 'User'}
            className="w-full h-full object-cover"
          />
        ) : (
          <span className="text-xs font-bold text-white">
            {userProfile?.name ? userProfile.name.charAt(0).toUpperCase() : 'U'}
          </span>
        )}
      </button>
    </header>
  );
};


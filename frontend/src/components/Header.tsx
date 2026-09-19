import React from 'react';
import type { UserProfile } from '../types';
import { UserAvatar } from './UserAvatar';

interface HeaderProps {
  userProfile?: UserProfile | null;
  onOpenMenu: () => void;
}

export const Header: React.FC<HeaderProps> = ({ userProfile, onOpenMenu }) => {
  return (
    <header className="app-header relative z-30 w-full max-w-md mx-auto px-5 py-3 flex items-center justify-between select-none">
      {/* Top Corner Logo */}
      <div className="flex items-center gap-2.5">
        <div className="app-logo-box w-8 h-8 rounded-xl overflow-hidden flex items-center justify-center shadow-sm">
          <img
            src="/kinjo-logo-dark.png"
            alt="Kinjo"
            className="app-logo-dark w-full h-full object-contain"
          />
          <img
            src="/kinjo-logo-light.png"
            alt="Kinjo"
            className="app-logo-light w-full h-full object-contain hidden"
          />
        </div>
        <span className="app-brand-title text-2xl font-extrabold tracking-tight font-sans leading-none">
          Kinjo<span className="app-brand-dot">.</span>
        </span>
      </div>

      {/* User Profile Circle Photo (Replaces 3 bars) */}
      <div id="header-profile-avatar" className="shrink-0">
        <UserAvatar
          avatar={userProfile?.avatar}
          name={userProfile?.name}
          className="w-9 h-9 text-xs border-white/25 hover:border-white/60 cursor-pointer transition-all active:scale-95 shadow-md"
          onClick={onOpenMenu}
        />
      </div>
    </header>
  );
};


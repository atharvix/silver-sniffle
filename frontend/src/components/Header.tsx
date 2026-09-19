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
        <img
          src="/kinjo-app-icon-store-1024_1.png"
          alt="Kinjo"
          className="w-8 h-8 rounded-xl object-contain shadow-sm ring-1 ring-white/10"
        />
        <span className="text-2xl font-extrabold tracking-tight text-white font-sans leading-none">
          Kinjo<span className="text-white/30">.</span>
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


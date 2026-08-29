import React from 'react';
import { Menu } from 'lucide-react';

interface HeaderProps {
  onOpenMenu: () => void;
}

export const Header: React.FC<HeaderProps> = ({ onOpenMenu }) => {
  return (
    <header className="app-header relative z-30 w-full max-w-md mx-auto px-5 flex items-center justify-between select-none">
      {/* Logo */}
      <span className="text-[28px] sm:text-3xl font-extrabold tracking-tight text-white font-sans leading-none">
        k<span className="text-white/30">.</span>
      </span>

      {/* Hamburger — opens profile/settings drawer */}
      <button
        onClick={onOpenMenu}
        className="p-2 -mr-1 rounded-xl text-white/50 hover:text-white transition-colors active:scale-95"
        title="Open Menu"
      >
        <Menu className="w-5 h-5" strokeWidth={1.8} />
      </button>
    </header>
  );
};

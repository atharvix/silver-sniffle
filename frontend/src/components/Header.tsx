import React from 'react';

interface HeaderProps {
  onOpenMenu: () => void;
}

export const Header: React.FC<HeaderProps> = ({ onOpenMenu }) => {
  return (
    <header className="app-header relative z-30 w-full max-w-md mx-auto px-6 flex items-center justify-between select-none">
      {/* Wordmark */}
      <span className="wordmark">
        <svg width="18" height="18" viewBox="0 0 100 100" aria-hidden="true">
          <path fill="currentColor" d="M28 0H63.2V45.3H27.8V100A28 28 0 0 1 0 72V28A28 28 0 0 1 28 0Z"/>
          <path fill="currentColor" d="M71.6 0H72A28 28 0 0 1 100 28V72A28 28 0 0 1 72 100H36.2V53.7H71.6Z"/>
        </svg>
        <span>KINJO</span>
      </span>

      {/* 3-Lines Menu Icon matching reference images (top/bottom full width, middle shorter & right-aligned) */}
      <button
        type="button"
        id="header-profile-menu-button"
        onClick={onOpenMenu}
        className="p-1 -mr-1 hover:opacity-75 transition-opacity cursor-pointer flex items-center justify-center"
        style={{ color: 'var(--fg)' }}
        aria-label="Open menu"
      >
        <svg width="20" height="13" viewBox="0 0 20 13" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
          <line x1="0" y1="1" x2="20" y2="1" stroke="currentColor" strokeWidth="1.6" />
          <line x1="6" y1="6.5" x2="20" y2="6.5" stroke="currentColor" strokeWidth="1.6" />
          <line x1="0" y1="12" x2="20" y2="12" stroke="currentColor" strokeWidth="1.6" />
        </svg>
      </button>
    </header>
  );
};

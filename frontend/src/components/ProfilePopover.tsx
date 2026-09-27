import React from 'react';
import type { UserProfile } from '../types';
import { X } from 'lucide-react';
import { UserAvatar } from './UserAvatar';

interface ProfilePopoverProps {
  isOpen: boolean;
  onClose: () => void;
  userProfile: UserProfile;
  onOpenSettings: () => void;
}

export const ProfilePopover: React.FC<ProfilePopoverProps> = ({
  isOpen,
  onClose,
  userProfile,
  onOpenSettings,
}) => {
  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-50 select-none">
      {/* Scrim */}
      <div className="side-drawer-overlay" onClick={onClose} />

      {/* Drawer */}
      <aside className="side-drawer flex flex-col p-6" style={{ paddingTop: 'max(36px, calc(env(safe-area-inset-top) + 28px))' }}>
        <button
          onClick={onClose}
          aria-label="Close menu"
          className="self-end -mr-2.5 w-10 h-10 grid place-items-center transition-transform cursor-pointer"
          style={{ color: 'var(--fg)', borderRadius: 0 }}
        >
          <X className="w-5 h-5" />
        </button>

        <div className="flex flex-col items-start gap-3.5 py-6" style={{ borderBottom: '1px solid var(--soft)' }}>
          <UserAvatar
            avatar={userProfile.avatar}
            name={userProfile.name}
            className="w-[70px] h-[70px] text-2xl"
          />
          <div className="text-left min-w-0">
            <p className="text-2xl font-normal leading-tight truncate" style={{ letterSpacing: '-.03em' }}>
              {userProfile.name || 'User'}
            </p>
            <p className="text-sm mt-1 truncate" style={{ color: 'var(--muted)' }}>
              {userProfile.email || 'No email'}
            </p>
          </div>
        </div>

        <button
          id="sidebar-settings-button"
          className="row"
          onClick={() => {
            onClose();
            onOpenSettings();
          }}
        >
          <span>Settings</span>
          <svg viewBox="0 0 24 24" className="chev" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round"><path d="M9 6l6 6-6 6" /></svg>
        </button>
      </aside>
    </div>
  );
};

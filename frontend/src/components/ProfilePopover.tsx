import React from 'react';
import type { UserProfile } from '../types';
import { Settings, X } from 'lucide-react';

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
    <div className="fixed inset-0 z-50 flex items-start justify-end p-4 pt-16 select-none animate-in fade-in duration-150">
      {/* Backdrop (invisible clickable area to close box) */}
      <div className="fixed inset-0 bg-black/30 backdrop-blur-[2px]" onClick={onClose} />

      {/* Floating Invisible Box Container */}
      <div className="relative z-10 w-72 bg-[#121212]/95 border border-white/15 rounded-3xl p-5 shadow-2xl backdrop-blur-xl text-white space-y-4">
        {/* Header / Close button */}
        <div className="flex items-center justify-between">
          <span className="text-[11px] font-semibold text-white/40 uppercase tracking-wider">Account</span>
          <button
            onClick={onClose}
            className="p-1 rounded-full text-white/40 hover:text-white hover:bg-white/10 transition-colors"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        {/* User Info (Photo, Name, Email) */}
        <div className="flex items-center gap-3.5 pb-1">
          <div className="w-12 h-12 rounded-full overflow-hidden bg-white/10 shrink-0 border border-white/20 flex items-center justify-center text-lg font-bold text-white shadow-inner">
            {userProfile.avatar ? (
              <img src={userProfile.avatar} alt={userProfile.name} className="w-full h-full object-cover" />
            ) : (
              userProfile.name ? userProfile.name.charAt(0).toUpperCase() : 'U'
            )}
          </div>
          <div className="flex-1 min-w-0">
            <h3 className="text-base font-bold text-white leading-tight truncate">
              {userProfile.name || 'User'}
            </h3>
            <p className="text-xs text-white/50 truncate mt-0.5">
              {userProfile.email || 'No email attached'}
            </p>
          </div>
        </div>

        {/* Options */}
        <div className="pt-2 border-t border-white/10">
          <button
            onClick={() => { onClose(); onOpenSettings(); }}
            className="w-full flex items-center gap-3 px-3.5 py-2.5 rounded-2xl hover:bg-white/10 text-white/90 hover:text-white transition-colors text-xs font-medium text-left"
          >
            <Settings className="w-4 h-4 text-white/50" />
            <span>Settings</span>
          </button>
        </div>
      </div>
    </div>
  );
};

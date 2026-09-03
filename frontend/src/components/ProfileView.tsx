import React, { useState } from 'react';
import type { UserProfile } from '../types';
import {
  LogOut,
  Trash2,
  Eye,
  ArrowLeft,
  ChevronRight,
  X
} from 'lucide-react';
import { ProfileCard } from './ProfileCard';

interface ProfileViewProps {
  userProfile: UserProfile;
  onSave?: (updated: UserProfile) => void;
  onLogout?: () => void;
  onDeleteAccount?: () => void;
  onClose: () => void;
  onOpenEditProfile?: () => void;
}

type ModalType = 'none' | 'theme' | 'terms' | 'privacy';

export const ProfileView: React.FC<ProfileViewProps> = ({
  userProfile,
  onLogout,
  onDeleteAccount,
  onClose,
  onOpenEditProfile,
}) => {
  const form = userProfile;
  const [showPreview, setShowPreview] = useState(false);
  const [activeModal, setActiveModal] = useState<ModalType>('none');
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);

  if (showPreview) {
    return (
      <div className="h-full flex flex-col bg-[#060606] text-white">
        <div className="flex items-center justify-between p-5 border-b border-white/5">
          <span className="text-sm font-medium text-white/60">Card Preview</span>
          <button
            onClick={() => setShowPreview(false)}
            className="text-xs text-white/40 hover:text-white transition-colors"
          >
            ← Back
          </button>
        </div>
        <div className="flex-1 flex items-center justify-center p-6">
          <div style={{ width: '270px', height: '400px' }}>
            <ProfileCard profile={form} />
          </div>
        </div>
        <p className="text-center text-[11px] text-white/25 pb-6">
          How others see your card within 30m
        </p>
      </div>
    );
  }

  return (
    <div className="h-full flex flex-col bg-[#060606] text-white overflow-y-auto select-none">
      {/* Top Header */}
      <div className="flex items-center justify-between px-5 pt-[max(20px,env(safe-area-inset-top))] pb-4 border-b border-white/5 shrink-0">
        <div className="flex items-center gap-3">
          <button
            onClick={onClose}
            className="p-1.5 -ml-1.5 rounded-full text-white/60 hover:text-white transition-colors"
            title="Go back"
          >
            <ArrowLeft className="w-5 h-5" strokeWidth={1.8} />
          </button>
          <h2 className="text-lg font-bold text-white tracking-tight">Profile & Settings</h2>
        </div>
        <button
          onClick={onClose}
          className="text-xs text-white/40 hover:text-white/80 transition-colors font-medium"
        >
          Done
        </button>
      </div>

      <div className="flex-1 overflow-y-auto">
        {/* Photo & Profile Header Section */}
        <div className="p-5 flex items-center gap-4 border-b border-white/5">
          <div className="w-16 h-16 rounded-full overflow-hidden bg-white/5 shrink-0 border border-white/10 flex items-center justify-center text-xl font-bold text-white">
            {form.avatar ? (
              <img src={form.avatar} alt={form.name} className="w-full h-full object-cover" />
            ) : (
              form.name ? form.name.charAt(0).toUpperCase() : 'U'
            )}
          </div>
          <div className="flex-1 min-w-0">
            <h3 className="text-base font-bold text-white truncate">{form.name || 'Kinjo User'}</h3>
            <p className="text-xs text-white/40 truncate mt-0.5">{form.email || 'user@kinjo.local'}</p>
          </div>

          {/* Edit Profile Button -> Takes user to the creation setup page */}
          <button
            onClick={onOpenEditProfile}
            className="px-3.5 py-1.5 rounded-full bg-white/10 hover:bg-white/20 text-white font-medium text-xs transition-all shrink-0"
          >
            Edit Profile
          </button>

          {/* Card Preview Toggle */}
          <button
            onClick={() => setShowPreview(true)}
            className="p-2 text-white/30 hover:text-white/70 transition-colors"
            title="Preview Card"
          >
            <Eye className="w-4 h-4" strokeWidth={1.5} />
          </button>
        </div>

        {/* Profile Details Section */}
        <div className="divide-y divide-white/[0.04]">
          {/* Full Name */}
          <div className="px-5 py-4">
            <label className="text-[11px] font-semibold text-white/35 uppercase tracking-wider block mb-1">
              Full Name
            </label>
            <p className="text-sm font-medium text-white">{form.name || 'Kinjo User'}</p>
          </div>

          {/* Email */}
          <div className="px-5 py-4">
            <label className="text-[11px] font-semibold text-white/35 uppercase tracking-wider block mb-1">
              Email
            </label>
            <p className="text-sm font-medium text-white/70">{form.email || 'user@kinjo.local'}</p>
          </div>

          {/* What you do */}
          <div className="px-5 py-4">
            <label className="text-[11px] font-semibold text-white/35 uppercase tracking-wider block mb-1">
              What you do
            </label>
            <p className="text-sm font-medium text-white/90">{form.profession || '—'}</p>
          </div>

          {/* What you are looking for */}
          <div className="px-5 py-4">
            <label className="text-[11px] font-semibold text-white/35 uppercase tracking-wider block mb-1">
              What you are looking for
            </label>
            <p className="text-sm font-medium text-white/85 leading-relaxed">{form.lookingFor || '—'}</p>
          </div>
        </div>

        {/* Settings & Preferences Section */}
        <div className="mt-4 border-t border-white/[0.06]">
          <div className="px-5 py-3">
            <p className="text-[11px] font-medium text-white/25 uppercase tracking-wider">
              Settings & Preferences
            </p>
          </div>

          <div className="divide-y divide-white/[0.04]">
            <button
              onClick={() => setActiveModal('theme')}
              className="w-full px-5 py-3.5 flex items-center justify-between text-left hover:bg-white/[0.02] transition-colors"
            >
              <span className="text-sm text-white/70 font-normal">Theme</span>
              <ChevronRight className="w-4 h-4 text-white/20" strokeWidth={1.5} />
            </button>

            <button
              onClick={() => setActiveModal('terms')}
              className="w-full px-5 py-3.5 flex items-center justify-between text-left hover:bg-white/[0.02] transition-colors"
            >
              <span className="text-sm text-white/70 font-normal">Terms & conditions</span>
              <ChevronRight className="w-4 h-4 text-white/20" strokeWidth={1.5} />
            </button>

            <button
              onClick={() => setActiveModal('privacy')}
              className="w-full px-5 py-3.5 flex items-center justify-between text-left hover:bg-white/[0.02] transition-colors"
            >
              <span className="text-sm text-white/70 font-normal">Privacy policy</span>
              <ChevronRight className="w-4 h-4 text-white/20" strokeWidth={1.5} />
            </button>
          </div>
        </div>

        {/* Account Actions Section */}
        <div className="mt-4 border-t border-white/[0.06] pb-12">
          <div className="px-5 py-3">
            <p className="text-[11px] font-medium text-white/25 uppercase tracking-wider">
              Account
            </p>
          </div>

          <button
            onClick={onLogout}
            className="w-full px-5 py-3.5 flex items-center justify-between text-left hover:bg-white/[0.02] transition-colors"
          >
            <span className="text-sm text-white/60 font-normal">Log Out</span>
            <LogOut className="w-4 h-4 text-white/25" strokeWidth={1.5} />
          </button>

          {showDeleteConfirm ? (
            <div className="mx-5 my-3 p-4 rounded-2xl bg-red-500/8 border border-red-500/15 space-y-3">
              <p className="text-xs font-medium text-red-400/80">Permanently delete your account?</p>
              <div className="flex gap-2">
                <button
                  onClick={onDeleteAccount}
                  className="flex-1 py-2 rounded-xl bg-red-500/80 text-white text-xs font-semibold"
                >
                  Delete
                </button>
                <button
                  onClick={() => setShowDeleteConfirm(false)}
                  className="px-4 py-2 rounded-xl bg-white/8 text-white/60 text-xs"
                >
                  Cancel
                </button>
              </div>
            </div>
          ) : (
            <button
              onClick={() => setShowDeleteConfirm(true)}
              className="w-full px-5 py-3.5 flex items-center justify-between text-left hover:bg-white/[0.02] transition-colors"
            >
              <span className="text-sm text-red-400/70 font-normal">Delete Account</span>
              <Trash2 className="w-4 h-4 text-red-400/30" strokeWidth={1.5} />
            </button>
          )}
        </div>
      </div>

      {/* Sub-Modals for Settings options */}
      {activeModal !== 'none' && (
        <div className="fixed inset-0 z-50 bg-black/70 backdrop-blur-sm flex items-center justify-center p-5 select-none">
          <div className="w-full max-w-sm bg-[#111111] border border-white/10 rounded-3xl p-6 shadow-2xl text-white space-y-4">
            <div className="flex items-center justify-between border-b border-white/10 pb-3">
              <h3 className="text-sm font-bold text-white capitalize">
                {activeModal === 'terms' ? 'Terms & Conditions' :
                 activeModal === 'privacy' ? 'Privacy Policy' : 'Theme Settings'}
              </h3>
              <button
                onClick={() => setActiveModal('none')}
                className="p-1 rounded-full text-white/40 hover:text-white hover:bg-white/10"
              >
                <X className="w-4 h-4" />
              </button>
            </div>

            <div className="text-xs text-white/60 leading-relaxed max-h-60 overflow-y-auto space-y-2 font-normal">
              {activeModal === 'theme' && (
                <p>App theme is currently set to Dark Mode to conserve battery and enhance nighttime visibility.</p>
              )}
              {activeModal === 'terms' && (
                <p>By using Kinjo, you agree to treat nearby members with respect and maintain valid profile information.</p>
              )}
              {activeModal === 'privacy' && (
                <p>GPS locations are used exclusively for computing local 30m proximity and are never sold or broadcasted.</p>
              )}
            </div>

            <button
              onClick={() => setActiveModal('none')}
              className="w-full py-2.5 rounded-2xl bg-white/10 hover:bg-white/20 text-white font-medium text-xs transition-all"
            >
              Close
            </button>
          </div>
        </div>
      )}
    </div>
  );
};

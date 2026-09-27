import React, { useState } from 'react';
import type { UserProfile } from '../types';
import {
  LogOut,
  Trash2,
  Eye,
  ArrowLeft,
  X,
  Edit3,
  Moon,
  Shield,
  FileText
} from 'lucide-react';
import { ProfileCard } from './ProfileCard';
import { ProfileEditorModal } from './ProfileEditorModal';
import { UserAvatar } from './UserAvatar';
import { useMountedReveal } from '../hooks/useMountedReveal';

interface ProfileViewProps {
  userProfile: UserProfile;
  onSave?: (updated: UserProfile, photoChanged?: boolean) => void;
  onLogout?: () => void;
  onDeleteAccount?: () => void;
  onClose: () => void;
  currentTheme?: 'dark' | 'light' | 'system';
  onToggleTheme?: (theme: 'dark' | 'light' | 'system') => void;
}

type ModalType = 'none' | 'theme' | 'terms' | 'privacy';

/** "Your card" viewer — matches the reference design system's card-viewer screen. */
const ProfileCardViewer: React.FC<{ profile: UserProfile; onClose: () => void; onEdit: () => void }> = ({
  profile,
  onClose,
  onEdit,
}) => {
  const mounted = useMountedReveal([]);

  return (
    <div
      className={`viewer-fade${mounted ? ' in' : ''} h-full flex flex-col select-none`}
      style={{ background: 'var(--bg)', color: 'var(--fg)', padding: '0 22px', paddingTop: 'max(32px, calc(env(safe-area-inset-top) + 16px))', paddingBottom: 'max(20px, env(safe-area-inset-bottom))' }}
    >
      <div className="flex items-center justify-between shrink-0" style={{ minHeight: 48 }}>
        <span className="eyebrow">Your card</span>
        <button onClick={onClose} aria-label="Close" className="x-btn -mr-2.5">
          <X className="w-5 h-5" />
        </button>
      </div>

      <div className="flex-1 min-h-0 flex items-center justify-center py-3">
        <div className={`viewer-card-in${mounted ? ' in' : ''}`} style={{ width: 'min(88vw, 320px)', height: 'min(64vh, 460px)' }}>
          <ProfileCard profile={profile} />
        </div>
      </div>

      <p className="text-sm text-center pb-4" style={{ color: 'var(--muted)' }}>
        This is how people nearby see you.
      </p>

      <button onClick={onEdit} className="btn btn--line shrink-0">
        <span>Edit profile</span>
      </button>
    </div>
  );
};

export const ProfileView: React.FC<ProfileViewProps> = ({
  userProfile,
  onSave,
  onLogout,
  onDeleteAccount,
  onClose,
  currentTheme = 'dark',
  onToggleTheme,
}) => {
  const form = userProfile;
  const [showPreview, setShowPreview] = useState(false);
  const [activeModal, setActiveModal] = useState<ModalType>('none');
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [isEditingModalOpen, setIsEditingModalOpen] = useState(false);

  if (showPreview) {
    return (
      <ProfileCardViewer
        profile={form}
        onClose={() => setShowPreview(false)}
        onEdit={() => {
          setShowPreview(false);
          setIsEditingModalOpen(true);
        }}
      />
    );
  }

  return (
    <div className="h-full flex flex-col overflow-y-auto select-none" style={{ background: 'var(--bg)', color: 'var(--fg)' }}>
      {/* Top Header */}
      <div
        className="flex items-center justify-between px-6 pt-[max(42px,calc(env(safe-area-inset-top)+26px))] pb-4 shrink-0 sticky top-0 z-10 backdrop-blur-md"
        style={{ borderBottom: '1px solid var(--soft)', background: 'color-mix(in srgb, var(--bg) 80%, transparent)' }}
      >
        <div className="flex items-center gap-3">
          <button
            onClick={onClose}
            className="p-1.5 -ml-1.5 transition-colors cursor-pointer"
            style={{ color: 'var(--muted)', borderRadius: 0 }}
            title="Go back"
          >
            <ArrowLeft className="w-5 h-5" strokeWidth={1.8} />
          </button>
          <h2 className="text-base font-medium tracking-tight">Settings</h2>
        </div>
        <button
          onClick={onClose}
          className="text-xs font-semibold px-3 py-1.5 transition-all cursor-pointer"
          style={{ color: 'var(--fg)' }}
        >
          Done
        </button>
      </div>

      <div className="flex-1 overflow-y-auto px-5 py-6 max-w-md w-full mx-auto space-y-8">
        {/* User Identity */}
        <div className="flex items-center gap-4 pb-6" style={{ borderBottom: '1px solid var(--soft)' }}>
          <UserAvatar
            avatar={form.avatar}
            name={form.name}
            className="w-16 h-16 text-xl"
          />
          <div className="flex-1 min-w-0 text-left">
            <h3 className="text-lg font-medium truncate leading-tight" style={{ letterSpacing: '-.02em' }}>
              {form.name || 'Kinjo User'}
            </h3>
            {form.email && (
              <p className="text-xs truncate mt-1" style={{ color: 'var(--muted)' }}>
                {form.email}
              </p>
            )}
          </div>
        </div>

        <div className="space-y-1 -mt-4">
          <button className="row" onClick={() => setIsEditingModalOpen(true)}>
            <span className="flex items-center gap-3">
              <Edit3 className="w-4 h-4" style={{ color: 'var(--muted)' }} />
              <span>Edit profile</span>
            </span>
            <svg viewBox="0 0 24 24" className="chev" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round"><path d="M9 6l6 6-6 6" /></svg>
          </button>
          <button className="row" onClick={() => setShowPreview(true)}>
            <span className="flex items-center gap-3">
              <Eye className="w-4 h-4" style={{ color: 'var(--muted)' }} />
              <span>View my card</span>
            </span>
            <svg viewBox="0 0 24 24" className="chev" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round"><path d="M9 6l6 6-6 6" /></svg>
          </button>
        </div>

        {/* Bio */}
        <div className="text-left space-y-1.5">
          <span className="eyebrow">What you do &amp; what you're looking for</span>
          <p className="text-sm leading-relaxed font-normal pt-1">
            {form.bio || form.profession || 'No bio entered yet.'}
          </p>
        </div>

        {/* Preferences Section */}
        <div className="text-left">
          <p className="eyebrow mb-1">Preferences</p>
          <div className="row theme-row" style={{ cursor: 'default' }}>
            <span className="flex items-center gap-3">
              <Moon className="w-4 h-4" style={{ color: 'var(--muted)' }} />
              <span>Theme</span>
            </span>
            <div className="seg" role="radiogroup" aria-label="Theme" style={{ borderRadius: 0 }}>
              {(['light', 'dark', 'system'] as const).map((t) => (
                <button
                  key={t}
                  type="button"
                  role="radio"
                  style={{ borderRadius: 0 }}
                  aria-checked={currentTheme === t}
                  onClick={() => onToggleTheme?.(t)}
                >
                  {t === 'light' ? 'Light' : t === 'dark' ? 'Dark' : 'System'}
                </button>
              ))}
            </div>
          </div>

          <p className="eyebrow mt-6 mb-1">Legal</p>
          <button className="row" onClick={() => setActiveModal('terms')}>
            <span className="flex items-center gap-3">
              <FileText className="w-4 h-4" style={{ color: 'var(--muted)' }} />
              <span>Terms of Service</span>
            </span>
            <svg viewBox="0 0 24 24" className="chev" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round"><path d="M9 6l6 6-6 6" /></svg>
          </button>
          <button className="row" onClick={() => setActiveModal('privacy')}>
            <span className="flex items-center gap-3">
              <Shield className="w-4 h-4" style={{ color: 'var(--muted)' }} />
              <span>Privacy Policy</span>
            </span>
            <svg viewBox="0 0 24 24" className="chev" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round"><path d="M9 6l6 6-6 6" /></svg>
          </button>
        </div>

        {/* Account Section */}
        <div className="text-left">
          <p className="eyebrow mb-1">Account</p>
          <button className="row" onClick={onLogout}>
            <span className="flex items-center gap-3">
              <LogOut className="w-4 h-4" style={{ color: 'var(--muted)' }} />
              <span>Log out</span>
            </span>
            <svg viewBox="0 0 24 24" className="chev" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round"><path d="M9 6l6 6-6 6" /></svg>
          </button>

          {showDeleteConfirm ? (
            <div className="py-4 space-y-3">
              <p className="text-xs font-medium" style={{ color: 'var(--danger)' }}>Permanently delete your account? This can't be undone.</p>
              <div className="flex gap-2">
                <button onClick={onDeleteAccount} className="btn btn--danger btn--compact flex-1">
                  <span>Delete account</span>
                </button>
                <button onClick={() => setShowDeleteConfirm(false)} className="btn btn--line btn--compact">
                  <span>Cancel</span>
                </button>
              </div>
            </div>
          ) : (
            <button className="row danger" onClick={() => setShowDeleteConfirm(true)}>
              <span className="flex items-center gap-3">
                <Trash2 className="w-4 h-4" />
                <span>Delete account</span>
              </span>
              <svg viewBox="0 0 24 24" className="chev" style={{ color: 'currentColor' }} fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round"><path d="M9 6l6 6-6 6" /></svg>
            </button>
          )}
        </div>

        {/* Footer Brand Info */}
        <div className="pt-4 pb-8 flex flex-col items-center justify-center gap-2 text-center">
          <svg width="18" height="18" viewBox="0 0 100 100" style={{ color: 'var(--fg)' }} aria-hidden="true">
            <path fill="currentColor" d="M28 0H63.2V45.3H27.8V100A28 28 0 0 1 0 72V28A28 28 0 0 1 28 0Z" />
            <path fill="currentColor" d="M71.6 0H72A28 28 0 0 1 100 28V72A28 28 0 0 1 72 100H36.2V53.7H71.6Z" />
          </svg>
          <p className="text-[11px] tracking-wider uppercase" style={{ color: 'var(--faint)' }}>
            Kinjo · version 2.0.1
          </p>
        </div>
      </div>

      {/* Profile Editor Modal */}
      {isEditingModalOpen && (
        <ProfileEditorModal
          isOpen={isEditingModalOpen}
          onClose={() => setIsEditingModalOpen(false)}
          userProfile={form}
          onSave={(updated, photoChanged) => {
            if (onSave) onSave(updated, photoChanged);
            setIsEditingModalOpen(false);
          }}
        />
      )}

      {/* Sub-Modals for Settings */}
      {activeModal !== 'none' && (
        <div className="fixed inset-0 z-[110] flex items-center justify-center p-4" style={{ background: 'var(--scrim)' }}>
          <div className="max-w-sm w-full p-6 space-y-4" style={{ background: 'var(--bg)', border: '1px solid var(--hairline)', borderRadius: 0, color: 'var(--fg)' }}>
            <div className="flex items-center justify-between pb-3" style={{ borderBottom: '1px solid var(--soft)' }}>
              <h3 className="text-base font-medium tracking-tight" style={{ letterSpacing: '-.02em' }}>
                {activeModal === 'terms' && 'Terms of Service'}
                {activeModal === 'privacy' && 'Privacy Policy'}
              </h3>
              <button
                onClick={() => setActiveModal('none')}
                className="p-1 transition-colors cursor-pointer"
                style={{ color: 'var(--muted)', borderRadius: 0 }}
              >
                <X className="w-4 h-4" />
              </button>
            </div>

            {(activeModal === 'terms' || activeModal === 'privacy') && (
              <div className="text-sm leading-relaxed max-h-60 overflow-y-auto pr-1 space-y-3" style={{ color: 'var(--muted)' }}>
                <p>
                  Kinjo operates strictly within an optical 30-meter radius using local GPS coordinates and liveness verification.
                </p>
                <p>
                  Your location is processed only when the app is active to calculate immediate physical proximity and is never shared with third parties.
                </p>
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
};

import React, { useState, useRef } from 'react';
import type { UserProfile } from '../types';
import { Upload, LogOut, Trash2, Eye, ArrowLeft } from 'lucide-react';
import { ProfileCard } from './ProfileCard';

interface ProfileViewProps {
  userProfile: UserProfile;
  onSave: (updated: UserProfile) => void;
  onLogout?: () => void;
  onDeleteAccount?: () => void;
  onClose: () => void;
}

export const ProfileView: React.FC<ProfileViewProps> = ({
  userProfile,
  onSave,
  onLogout,
  onDeleteAccount,
  onClose,
}) => {
  const [form, setForm] = useState<UserProfile>({ ...userProfile });
  const [showPreview, setShowPreview] = useState(false);
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const wordCount = (form.bio || '').trim().split(/\s+/).filter(Boolean).length;
  const bioOverLimit = wordCount > 50;

  const update = (field: keyof UserProfile, value: string) => {
    const updated = { ...form, [field]: value };
    setForm(updated);
    onSave(updated);
  };

  const handlePhotoUpload = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    const reader = new FileReader();
    reader.onload = (ev) => {
      const result = ev.target?.result as string;
      const updated = { ...form, avatar: result };
      setForm(updated);
      onSave(updated);
    };
    reader.readAsDataURL(file);
  };

  if (showPreview) {
    return (
      <div className="h-full flex flex-col">
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
          <div style={{ width: '260px', height: '380px' }}>
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
    <div className="h-full flex flex-col overflow-y-auto">
      {/* Header */}
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
        {/* Photo Section */}
        <div className="p-5 flex items-center gap-4 border-b border-white/5">
          <div
            className="relative w-16 h-16 rounded-full overflow-hidden bg-white/5 shrink-0 cursor-pointer"
            onClick={() => fileInputRef.current?.click()}
          >
            {form.avatar ? (
              <img src={form.avatar} alt={form.name} className="w-full h-full object-cover" />
            ) : (
              <div className="w-full h-full flex items-center justify-center text-white/20">
                <Upload className="w-5 h-5" strokeWidth={1.5} />
              </div>
            )}
            <div className="absolute inset-0 bg-black/20 flex items-center justify-center opacity-0 hover:opacity-100 transition-opacity">
              <Upload className="w-4 h-4 text-white" strokeWidth={1.5} />
            </div>
          </div>
          <div>
            <button
              onClick={() => fileInputRef.current?.click()}
              className="text-sm font-medium text-white/70 hover:text-white transition-colors"
            >
              Change photo
            </button>
            <p className="text-xs text-white/25 mt-0.5">Tap photo to upload</p>
          </div>
          <input ref={fileInputRef} type="file" accept="image/*" onChange={handlePhotoUpload} className="hidden" />

          {/* Card Preview Toggle */}
          <button
            onClick={() => setShowPreview(true)}
            className="ml-auto text-white/30 hover:text-white/70 transition-colors"
            title="Preview Card"
          >
            <Eye className="w-4 h-4" strokeWidth={1.5} />
          </button>
        </div>

        {/* Fields */}
        <div className="divide-y divide-white/[0.04]">
          {/* Full Name */}
          <div className="px-5 py-4">
            <label className="text-[11px] font-medium text-white/35 uppercase tracking-wider block mb-2">
              Full Name
            </label>
            <input
              type="text"
              value={form.name}
              onChange={(e) => update('name', e.target.value)}
              placeholder="Your full name"
              className="w-full bg-transparent text-sm text-white placeholder:text-white/20 outline-none"
            />
          </div>

          {/* Email (read-only) */}
          <div className="px-5 py-4">
            <label className="text-[11px] font-medium text-white/35 uppercase tracking-wider block mb-2">
              Email
            </label>
            <p className="text-sm text-white/40">{form.email || '—'}</p>
          </div>

          {/* What you do */}
          <div className="px-5 py-4">
            <label className="text-[11px] font-medium text-white/35 uppercase tracking-wider block mb-2">
              What you do
            </label>
            <input
              type="text"
              value={form.profession}
              onChange={(e) => update('profession', e.target.value)}
              placeholder="Co-founder, Medical Startup"
              className="w-full bg-transparent text-sm text-white placeholder:text-white/20 outline-none"
            />
          </div>

          {/* What you're looking for */}
          <div className="px-5 py-4">
            <label className="text-[11px] font-medium text-white/35 uppercase tracking-wider block mb-2">
              What you are looking for
            </label>
            <input
              type="text"
              value={form.lookingFor}
              onChange={(e) => update('lookingFor', e.target.value)}
              placeholder="Looking for a co-founder…"
              className="w-full bg-transparent text-sm text-white placeholder:text-white/20 outline-none"
            />
          </div>

          {/* Bio */}
          <div className="px-5 py-4">
            <div className="flex items-center justify-between mb-2">
              <label className="text-[11px] font-medium text-white/35 uppercase tracking-wider">
                Bio
              </label>
              <span className={`text-[11px] font-mono ${bioOverLimit ? 'text-red-400/70' : 'text-white/20'}`}>
                {wordCount}/50
              </span>
            </div>
            <textarea
              rows={3}
              value={form.bio}
              onChange={(e) => update('bio', e.target.value)}
              placeholder="Brief bio about yourself…"
              className="w-full bg-transparent text-sm text-white placeholder:text-white/20 outline-none resize-none leading-relaxed"
            />
          </div>
        </div>

        {/* Account Settings */}
        <div className="mt-2 border-t border-white/[0.04]">
          <div className="px-5 py-3">
            <p className="text-[11px] font-medium text-white/25 uppercase tracking-wider mb-1">
              Account
            </p>
          </div>

          <button
            onClick={onLogout}
            className="w-full px-5 py-3.5 flex items-center justify-between text-left hover:bg-white/[0.02] transition-colors"
          >
            <span className="text-sm text-white/60">Log Out</span>
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
              <span className="text-sm text-red-400/70">Delete Account</span>
              <Trash2 className="w-4 h-4 text-red-400/30" strokeWidth={1.5} />
            </button>
          )}
        </div>

        <div className="h-8" />
      </div>
    </div>
  );
};

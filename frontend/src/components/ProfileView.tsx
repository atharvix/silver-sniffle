import React, { useState, useRef, useEffect } from 'react';
import type { UserProfile } from '../types';
import {
  Upload,
  LogOut,
  Trash2,
  Check,
  Eye,
  X,
  Plus,
} from 'lucide-react';
import { ProfileCard } from './ProfileCard';

interface ProfileViewProps {
  userProfile: UserProfile;
  onSave: (updatedProfile: UserProfile) => void;
  onLogout?: () => void;
  onDeactivateAccount?: () => void;
  onDeleteAccount?: () => void;
}

export const ProfileView: React.FC<ProfileViewProps> = ({
  userProfile,
  onSave,
  onLogout,
  onDeactivateAccount,
  onDeleteAccount,
}) => {
  const [formData, setFormData] = useState<UserProfile>({ ...userProfile });
  const [showPreview, setShowPreview] = useState(false);
  const [newInterestTag, setNewInterestTag] = useState('');
  const [bioError, setBioError] = useState<string | null>(null);

  // Account States
  const [isDeactivated, setIsDeactivated] = useState(false);
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [resetMessage, setResetMessage] = useState<string | null>(null);

  const fileInputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    setFormData({ ...userProfile });
  }, [userProfile]);

  // Calculate word count for bio limit (MAX 50 WORDS)
  const bioWordCount = formData.bio ? formData.bio.trim().split(/\s+/).filter(Boolean).length : 0;

  const handleBioChange = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
    const val = e.target.value;
    const words = val.trim().split(/\s+/).filter(Boolean).length;
    if (words > 50) {
      setBioError('Bio is strictly limited to 50 words maximum.');
    } else {
      setBioError(null);
    }
    const updated = { ...formData, bio: val };
    setFormData(updated);
    if (words <= 50) onSave(updated);
  };

  const handleFileUpload = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;

    const reader = new FileReader();
    reader.onload = (event) => {
      const result = event.target?.result as string;
      const updated = { ...formData, avatar: result };
      setFormData(updated);
      onSave(updated);
    };
    reader.readAsDataURL(file);
  };

  const handleAddInterest = (e?: React.FormEvent) => {
    if (e) e.preventDefault();
    const tag = newInterestTag.trim();
    if (!tag) return;

    const existing = formData.interests || [];
    if (existing.includes(tag)) return;

    const updatedInterests = [...existing, tag];
    const updated = { ...formData, interests: updatedInterests, tags: updatedInterests };
    setFormData(updated);
    onSave(updated);
    setNewInterestTag('');
  };

  const handleRemoveInterest = (tagToRemove: string) => {
    const updatedInterests = (formData.interests || []).filter((t) => t !== tagToRemove);
    const updated = { ...formData, interests: updatedInterests, tags: updatedInterests };
    setFormData(updated);
    onSave(updated);
  };

  const handleUpdateField = (field: keyof UserProfile, value: any) => {
    const updated = { ...formData, [field]: value };
    setFormData(updated);
    onSave(updated);
  };

  const handleSocialLinkUpdate = (network: string, handle: string) => {
    const updatedLinks = { ...(formData.socialLinks || {}), [network]: handle };
    const updated = { ...formData, socialLinks: updatedLinks };
    setFormData(updated);
    onSave(updated);
  };

  const handleForgotPassword = () => {
    setResetMessage('Password reset link sent to your email.');
    setTimeout(() => setResetMessage(null), 4000);
  };

  return (
    <div className="w-full max-w-md mx-auto space-y-6 pb-32 px-4 select-none font-sans">
      {/* HEADER TITLE */}
      <div className="flex items-center justify-between pt-2">
        <h2 className="text-xl sm:text-2xl font-bold text-white tracking-tight">
          Profile & Settings
        </h2>
        <button
          type="button"
          onClick={() => setShowPreview(!showPreview)}
          className="px-3.5 py-1.5 rounded-full bg-white text-black font-extrabold text-xs flex items-center gap-1.5 shadow-lg active:scale-95 transition-all"
        >
          <Eye className="w-3.5 h-3.5" />
          <span>{showPreview ? 'Hide Card' : 'Preview Card'}</span>
        </button>
      </div>

      {/* 1. CARD PREVIEW */}
      {showPreview && (
        <div className="space-y-2">
          <span className="text-[11px] font-mono uppercase tracking-wider text-neutral-400 block">
            Card Preview (30m Discovery)
          </span>
          <div className="w-full h-[460px] mx-auto rounded-[36px] overflow-hidden shadow-2xl">
            <ProfileCard profile={formData} />
          </div>
        </div>
      )}

      {resetMessage && (
        <div className="p-3 rounded-xl bg-white/10 border border-white/20 text-white text-xs font-medium flex items-center justify-center gap-2">
          <Check className="w-4 h-4 text-emerald-400" />
          <span>{resetMessage}</span>
        </div>
      )}

      {/* 2. UPDATE PROFILE DETAILS */}
      <div className="space-y-4">
        <span className="text-[11px] font-mono uppercase tracking-wider text-neutral-400 block">
          Edit Profile Details
        </span>

        <div className="p-4 rounded-[28px] bg-white/[0.03] border border-white/12 space-y-4 backdrop-blur-xl">
          {/* Avatar Upload */}
          <div className="flex items-center gap-4">
            <img
              src={formData.avatar || 'https://images.unsplash.com/photo-1534528741775-53994a69daeb?auto=format&fit=crop&q=80&w=1000'}
              alt={formData.name}
              className="w-16 h-16 rounded-full object-cover border-2 border-white/20 shrink-0"
            />
            <div>
              <button
                type="button"
                onClick={() => fileInputRef.current?.click()}
                className="px-3.5 py-2 rounded-xl bg-white/10 hover:bg-white/20 text-xs font-bold text-white flex items-center gap-1.5 transition-all border border-white/15 active:scale-95"
              >
                <Upload className="w-3.5 h-3.5" />
                <span>Upload New Photo</span>
              </button>
              <input ref={fileInputRef} type="file" accept="image/*" onChange={handleFileUpload} className="hidden" />
            </div>
          </div>

          {/* Name */}
          <div>
            <label className="text-xs font-semibold text-neutral-400 block mb-1">Full Name</label>
            <input
              type="text"
              value={formData.name}
              onChange={(e) => handleUpdateField('name', e.target.value)}
              className="w-full bg-black/40 border border-white/15 rounded-xl px-3.5 py-2.5 text-xs text-white focus:outline-none"
            />
          </div>

          {/* Subtitle / Role */}
          <div>
            <label className="text-xs font-semibold text-neutral-400 block mb-1">Subtitle / Role</label>
            <input
              type="text"
              value={formData.subtitle || ''}
              onChange={(e) => handleUpdateField('subtitle', e.target.value)}
              placeholder="e.g. Co-founder, Medical Startup"
              className="w-full bg-black/40 border border-white/15 rounded-xl px-3.5 py-2.5 text-xs text-white focus:outline-none"
            />
          </div>

          {/* Status / Quote Prompt */}
          <div>
            <label className="text-xs font-semibold text-neutral-400 block mb-1">Status Prompt / Quote</label>
            <input
              type="text"
              value={formData.quotePrompt}
              onChange={(e) => handleUpdateField('quotePrompt', e.target.value)}
              placeholder="e.g. Looking for a co-founder for my medical startup"
              className="w-full bg-black/40 border border-white/15 rounded-xl px-3.5 py-2.5 text-xs text-white focus:outline-none"
            />
          </div>

          {/* Bio (Strict 50-Word Limit) */}
          <div>
            <div className="flex items-center justify-between mb-1">
              <label className="text-xs font-semibold text-neutral-400">Bio (Max 50 Words)</label>
              <span className={`text-[11px] font-mono font-bold ${bioWordCount > 50 ? 'text-rose-400' : 'text-neutral-400'}`}>
                {bioWordCount}/50 words
              </span>
            </div>
            <textarea
              rows={3}
              value={formData.bio}
              onChange={handleBioChange}
              placeholder="Write a brief bio about yourself..."
              className="w-full bg-black/40 border border-white/15 rounded-xl p-3 text-xs text-white focus:outline-none"
            />
            {bioError && <p className="text-[11px] text-rose-400 mt-1">{bioError}</p>}
          </div>

          {/* USER-TYPED INTERESTS */}
          <div>
            <label className="text-xs font-semibold text-neutral-400 block mb-1">User-Typed Interests</label>
            <form onSubmit={handleAddInterest} className="flex items-center gap-2 mb-2">
              <input
                type="text"
                value={newInterestTag}
                onChange={(e) => setNewInterestTag(e.target.value)}
                placeholder="Type custom interest tag & press enter..."
                className="flex-1 bg-black/40 border border-white/15 rounded-xl px-3.5 py-2 text-xs text-white focus:outline-none"
              />
              <button
                type="submit"
                className="px-3.5 py-2 rounded-xl bg-white text-black font-bold text-xs flex items-center gap-1 shrink-0"
              >
                <Plus className="w-3.5 h-3.5" />
                <span>Add</span>
              </button>
            </form>

            <div className="flex flex-wrap gap-2 pt-1">
              {(formData.interests || []).map((interest, idx) => (
                <span
                  key={idx}
                  className="px-3 py-1 rounded-full bg-white/10 border border-white/15 text-xs text-neutral-200 flex items-center gap-1.5"
                >
                  <span>#{interest}</span>
                  <button
                    type="button"
                    onClick={() => handleRemoveInterest(interest)}
                    className="w-3.5 h-3.5 rounded-full hover:bg-white/20 flex items-center justify-center text-neutral-400 hover:text-white"
                  >
                    <X className="w-2.5 h-2.5" />
                  </button>
                </span>
              ))}
            </div>
          </div>

          {/* Social Media Handles Editing */}
          <div className="space-y-2 pt-2 border-t border-white/10">
            <label className="text-xs font-semibold text-neutral-400 block">Social Media Handles</label>
            {['Instagram', 'LinkedIn', 'Twitter', 'GitHub'].map((net) => (
              <div key={net} className="flex items-center gap-2">
                <span className="w-20 text-[11px] font-medium text-neutral-400">{net}</span>
                <input
                  type="text"
                  value={formData.socialLinks?.[net] || ''}
                  onChange={(e) => handleSocialLinkUpdate(net, e.target.value)}
                  placeholder={`@username`}
                  className="flex-1 bg-black/40 border border-white/15 rounded-xl px-3 py-1.5 text-xs text-white focus:outline-none"
                />
              </div>
            ))}
          </div>
        </div>
      </div>

      {/* 3. BASIC ACCOUNT SETTINGS */}
      <div className="space-y-2">
        <span className="text-[11px] font-mono uppercase tracking-wider text-neutral-400 block">
          Basic Account Settings
        </span>

        <div className="bg-white/[0.03] border border-white/10 rounded-[24px] overflow-hidden divide-y divide-white/[0.08]">
          <button
            type="button"
            onClick={handleForgotPassword}
            className="w-full p-3.5 flex items-center justify-between text-left hover:bg-white/[0.03] transition-colors"
          >
            <span className="text-xs font-semibold text-neutral-300">Reset Password</span>
            <span className="text-xs text-neutral-400">Send Link</span>
          </button>

          <button
            type="button"
            onClick={() => {
              setIsDeactivated(!isDeactivated);
              if (onDeactivateAccount) onDeactivateAccount();
            }}
            className="w-full p-3.5 flex items-center justify-between text-left hover:bg-white/[0.03] transition-colors"
          >
            <span className="text-xs font-semibold text-neutral-300">Deactivate Account</span>
            <span className="text-xs font-bold text-neutral-400">
              {isDeactivated ? 'Deactivated' : 'Active'}
            </span>
          </button>

          <button
            type="button"
            onClick={() => {
              if (onLogout) onLogout();
            }}
            className="w-full p-3.5 flex items-center justify-between text-left hover:bg-white/[0.03] transition-colors"
          >
            <span className="text-xs font-semibold text-neutral-300">Log Out</span>
            <LogOut className="w-3.5 h-3.5 text-neutral-400" />
          </button>
        </div>

        {/* Delete Account */}
        <div className="pt-2">
          {showDeleteConfirm ? (
            <div className="p-4 rounded-2xl bg-rose-500/10 border border-rose-500/30 space-y-3">
              <p className="text-xs font-bold text-rose-400">Permanently delete account?</p>
              <div className="flex items-center gap-2 pt-1">
                <button
                  type="button"
                  onClick={() => {
                    if (onDeleteAccount) onDeleteAccount();
                  }}
                  className="flex-1 py-2 rounded-xl bg-rose-500 hover:bg-rose-600 text-white font-bold text-xs"
                >
                  Delete
                </button>
                <button
                  type="button"
                  onClick={() => setShowDeleteConfirm(false)}
                  className="px-4 py-2 rounded-xl bg-white/10 text-white text-xs"
                >
                  Cancel
                </button>
              </div>
            </div>
          ) : (
            <button
              type="button"
              onClick={() => setShowDeleteConfirm(true)}
              className="w-full p-3.5 rounded-2xl bg-rose-500/10 hover:bg-rose-500/20 border border-rose-500/20 text-rose-400 text-xs font-semibold flex items-center justify-between transition-all active:scale-95"
            >
              <div className="flex items-center gap-2">
                <Trash2 className="w-4 h-4 text-rose-400" />
                <span>Delete Account</span>
              </div>
              <span className="text-xs font-bold">Delete</span>
            </button>
          )}
        </div>
      </div>
    </div>
  );
};

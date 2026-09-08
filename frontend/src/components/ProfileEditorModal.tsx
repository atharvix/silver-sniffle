import React, { useState, useRef } from 'react';
import type { UserProfile } from '../types';
import { Camera, ArrowRight, X, Upload } from 'lucide-react';
import { compressImage, resolvePhotoUrl } from '../utils/api';

interface ProfileEditorModalProps {
  isOpen: boolean;
  onClose: () => void;
  userProfile: UserProfile;
  onSave: (updated: UserProfile) => void;
}

const countWords = (str: string) => {
  const trimmed = str.trim();
  return trimmed ? trimmed.split(/\s+/).length : 0;
};

const limitWords = (str: string, max: number) => {
  const words = str.trim().split(/\s+/);
  if (words.length <= max) return str;
  return words.slice(0, max).join(' ');
};

export const ProfileEditorModal: React.FC<ProfileEditorModalProps> = ({
  isOpen,
  onClose,
  userProfile,
  onSave,
}) => {
  const [form, setForm] = useState<UserProfile>({ ...userProfile });
  const [isVerifyingPhoto, setIsVerifyingPhoto] = useState(false);
  const [photoError, setPhotoError] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

  if (!isOpen) return null;

  const resolvedAvatar = resolvePhotoUrl(form.avatar);

  const update = (field: keyof UserProfile, value: string) => {
    setForm((prev) => ({ ...prev, [field]: value }));
  };

  const handlePhotoUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    setIsVerifyingPhoto(true);
    setPhotoError(false);

    try {
      const compressed = await compressImage(file);
      setForm((prev) => ({ ...prev, avatar: compressed }));
    } catch {
      const reader = new FileReader();
      reader.onload = (ev) => {
        setForm((prev) => ({ ...prev, avatar: ev.target?.result as string }));
      };
      reader.readAsDataURL(file);
    } finally {
      setIsVerifyingPhoto(false);
    }
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    const currentBio = form.bio || form.profession || '';
    if (countWords(currentBio) > 50) return;
    onSave({
      ...form,
      bio: currentBio,
      profession: currentBio,
    });
    onClose();
  };

  const currentBio = form.bio || form.profession || '';

  return (
    <div className="fixed inset-0 z-50 bg-black flex flex-col select-none overflow-y-auto animate-in fade-in duration-200">
      {/* Top Header */}
      <div className="px-6 pt-12 pb-4 flex items-center justify-between border-b border-white/10 max-w-md w-full mx-auto">
        <div className="flex items-center gap-2">
          <span className="text-2xl font-bold tracking-tight text-white">
            k<span className="text-white/25">.</span>
          </span>
          <span className="text-xs font-semibold text-white/50 uppercase tracking-wider ml-2">
            Bio Setup
          </span>
        </div>
        <button
          onClick={onClose}
          className="p-1.5 rounded-full text-white/40 hover:text-white hover:bg-white/10 transition-colors"
        >
          <X className="w-5 h-5" />
        </button>
      </div>

      {/* Form Content */}
      <div className="flex-1 px-6 py-6 max-w-md w-full mx-auto flex flex-col justify-between">
        <form onSubmit={handleSubmit} className="space-y-6">

          {/* Photo Upload Area */}
          <div className="flex flex-col items-center justify-center py-2 space-y-2 text-center">
            <div
              className="relative w-24 h-24 rounded-full overflow-hidden bg-white/5 border border-white/20 cursor-pointer shadow-xl flex items-center justify-center group transition-transform active:scale-95"
              onClick={() => fileInputRef.current?.click()}
            >
              {resolvedAvatar && !photoError ? (
                <img
                  src={resolvedAvatar}
                  alt=""
                  onError={() => setPhotoError(true)}
                  className="w-full h-full object-cover"
                />
              ) : (
                <Camera className="w-8 h-8 text-white/40" />
              )}
              <div className="absolute inset-0 bg-black/60 flex flex-col items-center justify-center opacity-0 group-hover:opacity-100 transition-opacity">
                <Upload className="w-5 h-5 text-white" />
              </div>
            </div>
            <p className="text-[11px] text-white/40 font-medium">
              {isVerifyingPhoto ? 'Updating photo…' : 'Tap photo to upload image'}
            </p>
            <input ref={fileInputRef} type="file" accept="image/*" onChange={handlePhotoUpload} className="hidden" />
          </div>

          {/* Full Name */}
          <div className="space-y-2">
            <label className="text-[11px] font-semibold text-white/40 uppercase tracking-wider block">
              Full Name
            </label>
            <input
              type="text"
              required
              value={form.name}
              onChange={(e) => update('name', e.target.value)}
              placeholder="Your full name"
              className="w-full px-4 py-3.5 rounded-2xl bg-white/[0.06] border border-white/10 text-white placeholder:text-white/25 text-sm outline-none focus:border-white/30 transition-colors"
            />
          </div>

          {/* Combined Field: What you do & What you are looking for (Max 50 Words) */}
          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <label className="text-[11px] font-semibold text-white/40 uppercase tracking-wider block">
                What you do & What you are looking for
              </label>
              <span className={`text-[10px] font-mono ${countWords(currentBio) > 50 ? 'text-red-400 font-bold' : 'text-white/40'}`}>
                {countWords(currentBio)} / 50 words
              </span>
            </div>
            <textarea
              rows={4}
              required
              value={currentBio}
              onChange={(e) => {
                const updated = limitWords(e.target.value, 50);
                setForm((prev) => ({ ...prev, bio: updated, profession: updated }));
              }}
              placeholder="e.g. Co-founder at a medical startup. Looking for an AI engineer to build real-time clinical tools..."
              className="w-full px-4 py-3.5 rounded-2xl bg-white/[0.06] border border-white/10 text-white placeholder:text-white/25 text-sm outline-none focus:border-white/30 transition-colors resize-none leading-relaxed"
            />
          </div>

          <button
            type="submit"
            disabled={countWords(currentBio) > 50}
            className="w-full flex items-center justify-center gap-2 py-4 px-4 rounded-2xl bg-white hover:bg-neutral-200 text-black font-bold text-sm transition-all active:scale-[0.98] shadow-lg mt-4 disabled:opacity-50"
          >
            <span>Save Profile</span>
            <ArrowRight className="w-4 h-4" strokeWidth={2.5} />
          </button>
        </form>
      </div>
    </div>
  );
};

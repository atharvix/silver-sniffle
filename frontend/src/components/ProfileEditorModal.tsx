import React, { useState, useRef } from 'react';
import type { UserProfile } from '../types';
import { Camera, ArrowRight, X, Upload } from 'lucide-react';

interface ProfileEditorModalProps {
  isOpen: boolean;
  onClose: () => void;
  userProfile: UserProfile;
  onSave: (updated: UserProfile) => void;
}

export const ProfileEditorModal: React.FC<ProfileEditorModalProps> = ({
  isOpen,
  onClose,
  userProfile,
  onSave,
}) => {
  const [form, setForm] = useState<UserProfile>({ ...userProfile });
  const [isVerifyingPhoto, setIsVerifyingPhoto] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

  if (!isOpen) return null;

  const update = (field: keyof UserProfile, value: string) => {
    setForm((prev) => ({ ...prev, [field]: value }));
  };

  const handlePhotoUpload = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    setIsVerifyingPhoto(true);

    const reader = new FileReader();
    reader.onload = (ev) => {
      const result = ev.target?.result as string;
      setTimeout(() => {
        setForm((prev) => ({ ...prev, avatar: result }));
        setIsVerifyingPhoto(false);
      }, 700);
    };
    reader.readAsDataURL(file);
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    onSave(form);
    onClose();
  };

  return (
    <div className="fixed inset-0 z-50 bg-black flex flex-col select-none overflow-y-auto animate-in fade-in duration-200">
      {/* Top Header */}
      <div className="px-6 pt-12 pb-4 flex items-center justify-between border-b border-white/10 max-w-md w-full mx-auto">
        <div className="flex items-center gap-2">
          <span className="text-2xl font-bold tracking-tight text-white">
            k<span className="text-white/25">.</span>
          </span>
          <span className="text-xs font-semibold text-white/50 uppercase tracking-wider ml-2">
            Profile Setup
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

          {/* Real Person Check / Photo Verification */}
          <div className="flex flex-col items-center justify-center p-6 rounded-3xl bg-white/[0.04] border border-white/10 text-center space-y-3">
            <div
              className="relative w-24 h-24 rounded-full overflow-hidden bg-white/10 border-2 border-white/20 cursor-pointer shadow-xl flex items-center justify-center group"
              onClick={() => fileInputRef.current?.click()}
            >
              {form.avatar ? (
                <img src={form.avatar} alt={form.name} className="w-full h-full object-cover" />
              ) : (
                <Camera className="w-8 h-8 text-white/40" />
              )}
              <div className="absolute inset-0 bg-black/40 flex flex-col items-center justify-center opacity-0 group-hover:opacity-100 transition-opacity">
                <Upload className="w-5 h-5 text-white" />
                <span className="text-[10px] text-white font-medium mt-1">Upload</span>
              </div>
            </div>
            <p className="text-xs text-white/40">
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

          {/* Question 1: What you do */}
          <div className="space-y-2">
            <label className="text-[11px] font-semibold text-white/40 uppercase tracking-wider block">
              What you do
            </label>
            <input
              type="text"
              required
              value={form.profession}
              onChange={(e) => update('profession', e.target.value)}
              placeholder="e.g. Co-founder, Medical Startup"
              className="w-full px-4 py-3.5 rounded-2xl bg-white/[0.06] border border-white/10 text-white placeholder:text-white/25 text-sm outline-none focus:border-white/30 transition-colors"
            />
          </div>

          {/* Question 2: What are you looking for */}
          <div className="space-y-2">
            <label className="text-[11px] font-semibold text-white/40 uppercase tracking-wider block">
              What are you looking for
            </label>
            <textarea
              rows={3}
              required
              value={form.lookingFor}
              onChange={(e) => update('lookingFor', e.target.value)}
              placeholder="e.g. Looking for a co-founder for my startup…"
              className="w-full px-4 py-3.5 rounded-2xl bg-white/[0.06] border border-white/10 text-white placeholder:text-white/25 text-sm outline-none focus:border-white/30 transition-colors resize-none leading-relaxed"
            />
          </div>

          <button
            type="submit"
            className="w-full flex items-center justify-center gap-2 py-4 px-4 rounded-2xl bg-white hover:bg-neutral-200 text-black font-bold text-sm transition-all active:scale-[0.98] shadow-lg mt-4"
          >
            <span>Save Profile</span>
            <ArrowRight className="w-4 h-4" strokeWidth={2.5} />
          </button>
        </form>
      </div>
    </div>
  );
};

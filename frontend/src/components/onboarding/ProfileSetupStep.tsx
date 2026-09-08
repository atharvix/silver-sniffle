import React from 'react';
import { ArrowRight, Camera, Upload } from 'lucide-react';
import { resolvePhotoUrl } from '../../utils/api';

const countWords = (str: string) => {
  const trimmed = str.trim();
  return trimmed ? trimmed.split(/\s+/).length : 0;
};

const limitWords = (str: string, max: number) => {
  const words = str.trim().split(/\s+/);
  if (words.length <= max) return str;
  return words.slice(0, max).join(' ');
};

interface ProfileSetupStepProps {
  name: string;
  setName: (name: string) => void;
  avatar: string;
  setAvatar: (avatar: string) => void;
  bio: string;
  setBio: (bio: string) => void;
  authError: string;
  isSubmitting: boolean;
  fileInputRef: React.RefObject<HTMLInputElement | null>;
  handlePhotoUpload: (e: React.ChangeEvent<HTMLInputElement>) => void;
  onSubmit: (e: React.FormEvent) => void;
}

export const ProfileSetupStep: React.FC<ProfileSetupStepProps> = ({
  name,
  setName,
  avatar,
  setAvatar,
  bio,
  setBio,
  authError,
  isSubmitting,
  fileInputRef,
  handlePhotoUpload,
  onSubmit,
}) => {
  return (
    <div className="space-y-5">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-extrabold text-white tracking-tight leading-snug">
            Bio Setup
          </h2>
          <p className="text-xs text-white/50 mt-1 font-normal">
            Personalize your identity for nearby cards
          </p>
        </div>
      </div>

      <form onSubmit={onSubmit} className="space-y-4" noValidate>
        {/* Photo Upload Area */}
        <div className="flex flex-col items-center justify-center py-3 space-y-2">
          <div
            className="relative w-24 h-24 rounded-full overflow-hidden bg-white/5 border border-white/20 cursor-pointer shadow-xl flex items-center justify-center group transition-transform active:scale-95"
            onClick={() => fileInputRef.current?.click()}
          >
            {avatar ? (
              <img
                src={resolvePhotoUrl(avatar)}
                alt=""
                onError={() => setAvatar('')}
                className="w-full h-full object-cover"
              />
            ) : (
              <Camera className="w-8 h-8 text-white/40" />
            )}
            <div className="absolute inset-0 bg-black/60 flex flex-col items-center justify-center opacity-0 group-hover:opacity-100 transition-opacity">
              <Upload className="w-5 h-5 text-white" />
            </div>
          </div>
          <p className="text-[11px] text-white/40 font-medium">Tap photo to upload image</p>
          <input ref={fileInputRef} type="file" accept="image/*" onChange={handlePhotoUpload} className="hidden" />
        </div>

        {/* Full Name */}
        <div className="space-y-1.5">
          <label className="text-[11px] font-bold text-white/50 uppercase tracking-wider block">
            Full Name
          </label>
          <input
            type="text"
            required
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="Your full name"
            className="w-full px-4 py-3.5 rounded-2xl bg-white/[0.06] border border-white/12 text-white placeholder:text-white/20 text-sm outline-none focus:border-white/40 transition-colors font-medium shadow-inner"
          />
        </div>

        {/* Single Field: What you do & What you are looking for (Max 50 Words) */}
        <div className="space-y-1.5">
          <div className="flex items-center justify-between">
            <label className="text-[11px] font-bold text-white/50 uppercase tracking-wider block">
              What you do & What you are looking for
            </label>
            <span className={`text-[10px] font-mono ${countWords(bio) > 50 ? 'text-red-400 font-bold' : 'text-white/40'}`}>
              {countWords(bio)} / 50 words
            </span>
          </div>
          <textarea
            rows={4}
            required
            value={bio}
            onChange={(e) => setBio(limitWords(e.target.value, 50))}
            placeholder="e.g. Co-founder at a medical startup. Looking for an AI engineer to build real-time clinical tools..."
            className="w-full px-4 py-3.5 rounded-2xl bg-white/[0.06] border border-white/12 text-white placeholder:text-white/20 text-sm outline-none focus:border-white/40 transition-colors font-medium resize-none leading-relaxed shadow-inner"
          />
        </div>

        <button
          type="submit"
          disabled={isSubmitting || countWords(bio) > 50}
          className="w-full flex items-center justify-center gap-2 py-4 px-4 rounded-2xl bg-white hover:bg-neutral-200 text-black font-extrabold text-sm transition-all active:scale-[0.98] shadow-2xl mt-4 disabled:opacity-50"
        >
          <span>{isSubmitting ? 'Saving profile...' : 'Save profile'}</span>
          <ArrowRight className="w-4 h-4" strokeWidth={2.5} />
        </button>

        {authError && <p className="text-xs text-red-400 font-medium text-center mt-2">{authError}</p>}
      </form>
    </div>
  );
};

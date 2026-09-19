import React from 'react';
import { ArrowRight, Camera, Upload, CheckCircle2, AlertTriangle, RefreshCw, X } from 'lucide-react';
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

export interface PhotoVerificationStatus {
  isChecking: boolean;
  isVerified: boolean;
  errorMessage?: string;
  matchScore?: number;
}

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
  verifiedFaceSnapshot?: string;
  photoVerification?: PhotoVerificationStatus;
  onUseVerifiedSnapshot?: () => void;
  isEditMode?: boolean;
  onCancelEdit?: () => void;
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
  verifiedFaceSnapshot,
  photoVerification,
  onUseVerifiedSnapshot,
  isEditMode = false,
  onCancelEdit,
}) => {
  const isSnapshotActive = verifiedFaceSnapshot && avatar === verifiedFaceSnapshot;

  return (
    <div className="space-y-5">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-extrabold text-white tracking-tight leading-snug">
            {isEditMode ? 'Update Profile' : 'Bio Setup'}
          </h2>
          <p className="text-xs text-white/50 mt-1 font-normal">
            {isEditMode
              ? 'Update how you appear to others within 30 meters'
              : 'Personalize your identity for nearby discovery'}
          </p>
        </div>
        {isEditMode && onCancelEdit && (
          <button
            type="button"
            onClick={onCancelEdit}
            className="p-1.5 rounded-full text-white/50 hover:text-white hover:bg-white/10 transition-colors"
            title="Cancel"
          >
            <X className="w-5 h-5" />
          </button>
        )}
      </div>

      <form onSubmit={onSubmit} className="space-y-4" noValidate>
        {/* Photo Upload & Face Verification Match Area */}
        <div className="flex flex-col items-center justify-center py-2 space-y-2.5">
          <div
            className="relative w-24 h-24 rounded-full overflow-hidden bg-white/5 border-2 border-white/20 cursor-pointer shadow-2xl flex items-center justify-center group transition-transform active:scale-95"
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

          {/* Photo Verification Status Badge */}
          {photoVerification?.isChecking ? (
            <div className="flex items-center gap-1.5 px-3 py-1 rounded-full bg-blue-500/15 border border-blue-500/30 text-blue-300 text-[11px] font-medium">
              <RefreshCw className="w-3 h-3 animate-spin" />
              <span>Verifying face match…</span>
            </div>
          ) : photoVerification?.isVerified || isSnapshotActive ? (
            <div className="flex items-center gap-1.5 px-3 py-1 rounded-full bg-emerald-500/15 border border-emerald-500/30 text-emerald-300 text-[11px] font-medium">
              <CheckCircle2 className="w-3.5 h-3.5 text-emerald-400" />
              <span>
                {isSnapshotActive
                  ? 'Live Face Verified (100% Real Human)'
                  : `Verified Face Match (${photoVerification?.matchScore ? Math.round(photoVerification.matchScore * 100) : 95}%)`}
              </span>
            </div>
          ) : photoVerification?.errorMessage ? (
            <div className="flex flex-col items-center gap-1.5 max-w-xs text-center">
              <div className="flex items-center gap-1.5 px-3 py-1 rounded-full bg-rose-500/15 border border-rose-500/30 text-rose-300 text-[11px] font-medium">
                <AlertTriangle className="w-3.5 h-3.5 text-rose-400 shrink-0" />
                <span>Face mismatch detected</span>
              </div>
              <p className="text-[11px] text-rose-300/90 leading-tight">
                {photoVerification.errorMessage}
              </p>
              {verifiedFaceSnapshot && onUseVerifiedSnapshot && (
                <button
                  type="button"
                  onClick={onUseVerifiedSnapshot}
                  className="mt-1 px-3 py-1 rounded-full bg-white/10 hover:bg-white/20 text-white text-[11px] font-semibold transition-all"
                >
                  Use Live Scan Photo Instead
                </button>
              )}
            </div>
          ) : (
            <p className="text-[11px] text-white/40 font-medium">
              Tap photo to upload or take a new picture
            </p>
          )}

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
            placeholder="e.g. Co-founder building an AI health app. Looking for a mobile developer or designer nearby..."
            className="w-full px-4 py-3.5 rounded-2xl bg-white/[0.06] border border-white/12 text-white placeholder:text-white/20 text-sm outline-none focus:border-white/40 transition-colors font-medium resize-none leading-relaxed shadow-inner"
          />
        </div>

        <button
          type="submit"
          disabled={isSubmitting || countWords(bio) > 50 || photoVerification?.isChecking}
          className="w-full flex items-center justify-center gap-2 py-4 px-4 rounded-2xl bg-white hover:bg-neutral-200 text-black font-extrabold text-sm transition-all active:scale-[0.98] shadow-2xl mt-4 disabled:opacity-50"
        >
          <span>
            {isSubmitting
              ? 'Saving profile…'
              : isEditMode
              ? 'Save Changes'
              : 'Save profile'}
          </span>
          <ArrowRight className="w-4 h-4" strokeWidth={2.5} />
        </button>

        {authError && (
          <div className="p-3 rounded-xl bg-rose-500/10 border border-rose-500/20 text-rose-300 text-xs font-medium text-center mt-2">
            {authError}
          </div>
        )}
      </form>
    </div>
  );
};

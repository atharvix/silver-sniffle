import React, { useState } from 'react';
import { ArrowRight, Loader2 } from 'lucide-react';
import { ProfileCard } from '../ProfileCard';
import { PhotoSourceModal } from '../PhotoSourceModal';

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
  isEditMode?: boolean;
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
  isEditMode = false,
}) => {
  const [showPhotoModal, setShowPhotoModal] = useState(false);
  const wordCount = countWords(bio);
  const isOverWordLimit = wordCount > 50;

  return (
    <div className="space-y-5 text-left">
      <div className="space-y-1">
        <h2 className="text-[34px] font-normal leading-[1.04]" style={{ letterSpacing: '-.045em' }}>
          {isEditMode ? 'Update profile.' : 'Create your profile.'}
        </h2>
        <p className="text-sm leading-relaxed" style={{ color: 'var(--muted)' }}>
          This is exactly what people will see.
        </p>
      </div>

      <form onSubmit={onSubmit} className="space-y-4" noValidate>
        {/* Live Profile Card Preview using the unified ProfileCard component */}
        <div className="flex flex-col items-center justify-center pt-1 pb-1">
          <div
            className="relative w-[215px] h-[305px] cursor-pointer group select-none transition-transform active:scale-98"
            onClick={() => setShowPhotoModal(true)}
          >
            <ProfileCard
              profile={{
                id: 'preview',
                email: '',
                name: name.trim() || 'Your Name',
                avatar: avatar,
                bio: bio.trim() || 'What you do and what you want',
                profession: bio.trim() || '',
                lookingFor: '',
              }}
            />
          </div>

          {/* Under-card Actions */}
          <div className="flex items-center justify-center gap-3 pt-2.5">
            <button
              type="button"
              onClick={() => setShowPhotoModal(true)}
              className="text-xs font-medium underline cursor-pointer hover:opacity-80"
              style={{ color: 'var(--fg)' }}
            >
              Change photo
            </button>
            <span className="text-xs" style={{ color: 'var(--faint)' }}>
              Portrait works best
            </span>
          </div>

          <input ref={fileInputRef} type="file" accept="image/*" onChange={handlePhotoUpload} className="hidden" />
        </div>

        {/* Full Name Field */}
        <div className="field">
          <input
            id="setupName"
            type="text"
            required
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder=" "
          />
          <label htmlFor="setupName">Name</label>
        </div>

        {/* Single Question: What you do and what you want (Max 50 Words) */}
        <div>
          <div className="field">
            <textarea
              id="setupBio"
              rows={3}
              required
              value={bio}
              onChange={(e) => setBio(limitWords(e.target.value, 50))}
              placeholder=" "
            />
            <label htmlFor="setupBio">What you do and what you want</label>
          </div>
          <div className="field-foot justify-end">
            <span className={`count${isOverWordLimit ? ' over' : ''}`}>{wordCount} / 50 words</span>
          </div>
        </div>

        {authError && (
          <p className="text-sm font-normal leading-relaxed text-left" style={{ color: 'var(--danger)' }}>
            {authError}
          </p>
        )}

        <button
          type="submit"
          disabled={isSubmitting || !bio.trim() || isOverWordLimit || !name.trim()}
          className="btn"
        >
          {isSubmitting ? (
            <>
              <Loader2 className="w-4 h-4 animate-spin" />
              <span>Saving profile…</span>
            </>
          ) : (
            <>
              <span>{isEditMode ? 'Save changes' : 'Continue'}</span>
              <ArrowRight className="stroke" strokeWidth={2} />
            </>
          )}
        </button>
      </form>

      {/* Camera vs Gallery Photo Source Modal */}
      <PhotoSourceModal
        isOpen={showPhotoModal}
        onClose={() => setShowPhotoModal(false)}
        onPhotoSelected={(dataUrl: string) => {
          setAvatar(dataUrl);
          setShowPhotoModal(false);
        }}
        title="Profile Photo"
      />
    </div>
  );
};

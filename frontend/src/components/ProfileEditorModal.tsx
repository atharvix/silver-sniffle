import React, { useState, useEffect, useRef } from 'react';
import type { UserProfile } from '../types';
import { ProfileSetupStep } from './onboarding/ProfileSetupStep';
import { compressImage } from '../utils/api';
import { useToast } from './Toast';

interface ProfileEditorModalProps {
  isOpen: boolean;
  onClose: () => void;
  userProfile: UserProfile;
  /** photoChanged tells the caller whether the avatar needs re-uploading. */
  onSave: (updated: UserProfile, photoChanged?: boolean) => void;
}

export const ProfileEditorModal: React.FC<ProfileEditorModalProps> = ({
  isOpen,
  onClose,
  userProfile,
  onSave,
}) => {
  const toast = useToast();
  const [name, setName] = useState(userProfile.name || '');
  const [avatar, setAvatar] = useState(userProfile.avatar || '');
  const [bio, setBio] = useState(userProfile.bio || userProfile.profession || '');
  const [authError, setAuthError] = useState('');
  const fileInputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (isOpen) {
      setName(userProfile.name || '');
      setAvatar(userProfile.avatar || '');
      setBio(userProfile.bio || userProfile.profession || '');
      setAuthError('');
    }
  }, [isOpen, userProfile]);

  if (!isOpen) return null;

  const handlePhotoUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    try {
      const compressed = await compressImage(file);
      setAvatar(compressed);
    } catch {
      toast.error('Could not process photo');
    }
  };

  // Hand-off only: the caller owns the network write and applies the change
  // optimistically, so the sheet closes instantly instead of waiting on it.
  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim()) {
      setAuthError('Please enter your name.');
      return;
    }
    if (!bio.trim()) {
      setAuthError('Please tell people what you do and what you want.');
      return;
    }

    setAuthError('');

    const updatedProfile: UserProfile = {
      ...userProfile,
      name: name.trim(),
      avatar: avatar || userProfile.avatar,
      bio: bio.trim(),
      profession: bio.trim().split('\n')[0] || bio.trim(),
      lookingFor: bio.trim().split('\n').slice(1).join('\n') || '',
    };

    onSave(updatedProfile, updatedProfile.avatar !== userProfile.avatar);
    onClose();
  };

  return (
    <div
      className="fixed inset-0 z-[120] flex flex-col justify-between p-6 sm:p-10 overflow-y-auto min-h-screen select-none"
      style={{ background: 'var(--bg)', color: 'var(--fg)' }}
    >
      {/* Top Header: Wordmark & Back Button */}
      <div className="flex items-center justify-between py-4 max-w-md w-full mx-auto">
        <span className="wordmark">
          <svg width="18" height="18" viewBox="0 0 100 100" aria-hidden="true">
            <path fill="currentColor" d="M28 0H63.2V45.3H27.8V100A28 28 0 0 1 0 72V28A28 28 0 0 1 28 0Z"/>
            <path fill="currentColor" d="M71.6 0H72A28 28 0 0 1 100 28V72A28 28 0 0 1 72 100H36.2V53.7H71.6Z"/>
          </svg>
          <span>KINJO</span>
        </span>
        <button
          type="button"
          onClick={onClose}
          className="text-xs font-medium hover:opacity-75 transition-opacity cursor-pointer px-1 py-1"
          style={{ background: 'transparent', border: 0, color: 'var(--fg)' }}
        >
          <span>← Back</span>
        </button>
      </div>

      <div className="my-auto w-full max-w-md mx-auto py-4">
        <ProfileSetupStep
          name={name}
          setName={setName}
          avatar={avatar}
          setAvatar={setAvatar}
          bio={bio}
          setBio={setBio}
          authError={authError}
          isSubmitting={false}
          fileInputRef={fileInputRef}
          handlePhotoUpload={handlePhotoUpload}
          onSubmit={handleSubmit}
          isEditMode={true}
        />
      </div>
    </div>
  );
};

import React, { useState, useEffect } from 'react';
import type { UserProfile } from '../types';
import { ArrowLeft, User } from 'lucide-react';
import { resolvePhotoUrl } from '../utils/api';

interface ProfileDetailScreenProps {
  profile: UserProfile;
  onClose: () => void;
}

export const ProfileDetailScreen: React.FC<ProfileDetailScreenProps> = ({
  profile,
  onClose,
}) => {
  const [hasError, setHasError] = useState(false);
  const photoUrl = resolvePhotoUrl(profile.avatar);

  useEffect(() => {
    setHasError(false);
  }, [profile.avatar]);

  const showImage = Boolean(photoUrl && !hasError);
  const initial = profile.name ? profile.name.trim().charAt(0).toUpperCase() : '';

  return (
    <div
      className="fixed inset-0 z-50 bg-black overflow-y-auto select-none"
      style={{ animation: 'screen-slide-up 280ms cubic-bezier(0.32,0.72,0,1) both' }}
    >
      {/* Hero Photo or Default Banner */}
      <div className="relative w-full" style={{ height: '65vh', minHeight: 360 }}>
        {showImage ? (
          <img
            src={photoUrl}
            alt={profile.name}
            onError={() => setHasError(true)}
            className="w-full h-full object-cover"
          />
        ) : (
          <div className="w-full h-full bg-gradient-to-b from-neutral-800 via-neutral-900 to-black flex items-center justify-center">
            <div className="w-32 h-32 rounded-full bg-white/10 border border-white/20 flex items-center justify-center shadow-2xl">
              {initial ? (
                <span className="text-6xl font-extrabold text-white">{initial}</span>
              ) : (
                <User className="w-16 h-16 text-white/50" />
              )}
            </div>
          </div>
        )}
        <div className="absolute inset-0 bg-gradient-to-t from-black via-black/20 to-transparent pointer-events-none" />

        <button
          onClick={onClose}
          className="absolute top-[max(18px,env(safe-area-inset-top))] left-4 p-2.5 rounded-full bg-black/40 backdrop-blur-md border border-white/10 text-white/80 hover:text-white transition-colors"
        >
          <ArrowLeft className="w-4 h-4" strokeWidth={2} />
        </button>
      </div>

      <div className="px-5 pt-6 pb-16 space-y-6">
        <div>
          <h1 className="text-3xl font-bold text-white tracking-tight leading-tight">
            {profile.name}
          </h1>
        </div>
        {(profile.bio || profile.profession || profile.lookingFor) && (
          <div className="space-y-2">
            <p className="text-[10px] font-semibold text-white/40 uppercase tracking-widest">
              What you do & What you are looking for
            </p>
            <p className="text-base font-medium text-white/90 leading-relaxed">
              {profile.bio || [profile.profession, profile.lookingFor].filter(Boolean).join(' · ')}
            </p>
          </div>
        )}
      </div>
    </div>
  );
};

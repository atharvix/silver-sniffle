import React, { useState, useEffect } from 'react';
import type { UserProfile } from '../types';
import { MapPin, User } from 'lucide-react';
import { resolvePhotoUrl } from '../utils/api';

interface ProfileCardProps {
  profile: UserProfile;
  isBackCard?: boolean;
}

export const ProfileCard: React.FC<ProfileCardProps> = ({
  profile,
  isBackCard = false,
}) => {
  const [hasError, setHasError] = useState(false);
  const photoUrl = resolvePhotoUrl(profile.avatar);

  useEffect(() => {
    setHasError(false);
  }, [profile.avatar]);

  const showImage = Boolean(photoUrl && !hasError);
  const initial = profile.name ? profile.name.trim().charAt(0).toUpperCase() : '';

  if (isBackCard) {
    return (
      <div className="profile-card relative w-full h-full rounded-[32px] overflow-hidden bg-[#111111] select-none shadow-xl flex items-center justify-center border border-white/5">
        {showImage ? (
          <img
            src={photoUrl}
            alt={profile.name}
            onError={() => setHasError(true)}
            className="w-full h-full object-cover object-center opacity-70"
          />
        ) : (
          <div className="w-full h-full bg-gradient-to-b from-neutral-800 to-neutral-950 flex flex-col items-center justify-center p-6 text-center">
            {initial ? (
              <span className="text-6xl font-bold text-white/40">{initial}</span>
            ) : (
              <User className="w-20 h-20 text-white/30" />
            )}
          </div>
        )}
        <div className="absolute inset-0 bg-gradient-to-b from-black/30 via-transparent to-black/80" />
      </div>
    );
  }

  return (
    <div className="profile-card relative w-full h-full rounded-[32px] overflow-hidden select-none bg-[#0d0d0d] shadow-2xl border border-white/15">
      {/* Background Image or Gradient Avatar */}
      <div className="absolute inset-0 z-0">
        {showImage ? (
          <img
            src={photoUrl}
            alt={profile.name}
            loading="eager"
            decoding="async"
            onError={() => setHasError(true)}
            className="w-full h-full object-cover object-center scale-[1.01] image-rendering-high-quality"
          />
        ) : (
          <div className="w-full h-full bg-gradient-to-b from-neutral-800 via-neutral-900 to-black flex flex-col items-center justify-center pb-24 text-center">
            <div className="w-28 h-28 rounded-full bg-white/10 border border-white/20 flex items-center justify-center shadow-2xl mb-2">
              {initial ? (
                <span className="text-5xl font-extrabold text-white tracking-tight">{initial}</span>
              ) : (
                <User className="w-14 h-14 text-white/50" />
              )}
            </div>
          </div>
        )}
        {/* Top subtle vignette */}
        <div className="absolute inset-0 bg-gradient-to-b from-black/40 via-transparent to-transparent pointer-events-none" />
        {/* Bottom dark section for content */}
        <div className="absolute bottom-0 left-0 right-0 h-[50%] bg-gradient-to-t from-black/95 via-black/60 to-transparent pointer-events-none" />
      </div>

      {/* Top Right Distance Badge */}
      <div className="absolute top-4 right-4 z-10 px-3 py-1 rounded-full bg-black/60 backdrop-blur-md border border-white/20 text-[11px] font-semibold text-white flex items-center gap-1 shadow-lg">
        <MapPin className="w-3.5 h-3.5 text-white/90 shrink-0" strokeWidth={2} />
        <span>{profile.distanceMeters}m</span>
      </div>

      {/* Bottom Content Box */}
      <div className="absolute bottom-3.5 left-3.5 right-3.5 z-10 p-4 space-y-1 bg-black/60 backdrop-blur-xl border border-white/20 rounded-[24px] shadow-2xl">
        <h2 className="text-xl font-bold tracking-tight text-white leading-tight">
          {profile.name}
        </h2>
        {(profile.bio || profile.profession || profile.lookingFor) && (
          <p className="text-xs font-medium text-white/90 leading-relaxed line-clamp-3 pt-0.5">
            {profile.bio || [profile.profession, profile.lookingFor].filter(Boolean).join(' · ')}
          </p>
        )}
      </div>
    </div>
  );
};

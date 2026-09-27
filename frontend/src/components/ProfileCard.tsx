import React, { useState, useEffect } from 'react';
import type { UserProfile } from '../types';
import { resolvePhotoUrl, DEFAULT_AVATAR_WEBP } from '../utils/api';

export const formatDistance = (meters?: number | null) => {
  if (meters === undefined || meters === null || isNaN(meters) || meters <= 0) return 'Nearby';
  return `${Math.round(meters)}m`;
};

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

  const showImage = photoUrl !== DEFAULT_AVATAR_WEBP && !hasError;
  const initial = profile.name ? profile.name.trim().charAt(0).toUpperCase() : '';

  const bioBody = profile.bio?.includes(' · ')
    ? profile.bio.split(' · ').slice(1).join(' · ')
    : profile.bio || profile.lookingFor || '';

  if (isBackCard) {
    return (
      <div className="profile-card relative w-full h-full rounded-[24px] overflow-hidden select-none" style={{ background: '#1b1b1b' }}>
        {showImage ? (
          <img
            src={photoUrl}
            alt=""
            onError={() => setHasError(true)}
            className="w-full h-full object-cover object-center opacity-85"
          />
        ) : (
          <div className="w-full h-full flex flex-col items-center justify-center p-6 text-center" style={{ background: 'radial-gradient(120% 80% at 50% 25%, #2e2e2d, #131313 75%)' }}>
            {initial && (
              <span className="text-5xl font-bold" style={{ color: 'rgba(255,255,255,0.2)' }}>{initial}</span>
            )}
          </div>
        )}
        <div className="absolute inset-0" style={{ background: 'rgba(0,0,0,0.35)' }} />
      </div>
    );
  }

  return (
    <div className="profile-card relative w-full h-full rounded-[24px] overflow-hidden select-none text-left" style={{ background: '#1b1b1b' }}>
      {/* Full-bleed Portrait Image */}
      <div className="absolute inset-0 z-0">
        {showImage ? (
          <img
            src={photoUrl}
            alt={profile.name}
            loading="eager"
            decoding="async"
            onError={() => setHasError(true)}
            className="w-full h-full object-cover object-center"
          />
        ) : (
          <div className="w-full h-full flex flex-col items-center justify-center pb-24 text-center" style={{ background: 'radial-gradient(120% 80% at 50% 25%, #2e2e2d, #131313 75%)' }}>
            <div className="w-24 h-24 flex items-center justify-center" style={{ background: 'rgba(255,255,255,0.06)', border: '1px solid rgba(255,255,255,0.12)', borderRadius: 0 }}>
              <span className="text-4xl font-semibold" style={{ color: 'rgba(255,255,255,0.5)' }}>{initial || 'K'}</span>
            </div>
          </div>
        )}

        {/* Bottom gradient — softly lightens and softens towards base */}
        <div
          className="absolute inset-0 pointer-events-none"
          style={{
            background: 'linear-gradient(180deg, rgba(0,0,0,0) 55%, rgba(0,0,0,0.15) 75%, rgba(0,0,0,0.45) 100%)',
          }}
        />
      </div>

      {/* Extremely subtle, super smooth frosted blur layer behind text */}
      <div
        className="absolute bottom-0 left-0 right-0 h-44 z-0 pointer-events-none"
        style={{
          backdropFilter: 'blur(3.5px)',
          WebkitBackdropFilter: 'blur(3.5px)',
          background: 'linear-gradient(180deg, rgba(0,0,0,0) 0%, rgba(0,0,0,0.12) 32%, rgba(0,0,0,0.42) 70%, rgba(0,0,0,0.65) 100%)',
          maskImage: 'linear-gradient(to bottom, transparent 0%, rgba(0,0,0,0.4) 30%, black 80%)',
          WebkitMaskImage: 'linear-gradient(to bottom, transparent 0%, rgba(0,0,0,0.4) 30%, black 80%)',
          borderBottomLeftRadius: 24,
          borderBottomRightRadius: 24,
        }}
      />

      {/* Elegant Distance Badge at Top Right */}
      <div
        className="absolute top-3.5 right-3.5 z-20 px-2.5 py-1 text-[11px] font-medium tracking-wider text-zinc-300 flex items-center shadow-sm select-none"
        style={{
          background: 'rgba(0, 0, 0, 0.45)',
          backdropFilter: 'blur(10px)',
          WebkitBackdropFilter: 'blur(10px)',
          border: '1px solid rgba(255, 255, 255, 0.16)',
          borderRadius: 9999,
        }}
      >
        <span>{formatDistance(profile.distanceMeters)}</span>
      </div>

      {/* Editorial Content Overlay directly on image vignette */}
      <div className="absolute bottom-0 left-0 right-0 z-10 p-5 space-y-1 text-left" style={{ color: '#f1f1f1' }}>
        <h2 className="text-xl font-bold tracking-tight leading-tight" style={{ letterSpacing: '-.02em', color: '#ffffff' }}>
          {profile.name}
        </h2>

        {bioBody && (
          <p className="text-[14px] font-normal leading-snug line-clamp-3 pt-1" style={{ color: '#d4d4d8' }}>
            {bioBody}
          </p>
        )}
      </div>
    </div>
  );
};

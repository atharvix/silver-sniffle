import React from 'react';
import type { UserProfile } from '../types';

interface ProfileCardProps {
  profile: UserProfile;
  isBackCard?: boolean;
}

export const ProfileCard: React.FC<ProfileCardProps> = ({
  profile,
  isBackCard = false,
}) => {
  if (isBackCard) {
    return (
      <div className="profile-card relative w-full h-full rounded-[28px] overflow-hidden bg-[#111111] select-none">
        <img
          src={profile.avatar}
          alt={profile.name}
          className="w-full h-full object-cover object-center opacity-70"
        />
        <div className="absolute inset-0 bg-gradient-to-b from-black/30 via-transparent to-black/80" />
      </div>
    );
  }

  return (
    <div className="profile-card relative w-full h-full rounded-[28px] overflow-hidden select-none bg-[#0d0d0d]">
      {/* Full Portrait Background */}
      <div className="absolute inset-0 z-0">
        <img
          src={profile.avatar}
          alt={profile.name}
          className="w-full h-full object-cover object-center"
        />
        {/* Top subtle vignette */}
        <div className="absolute inset-0 bg-gradient-to-b from-black/20 via-transparent to-transparent" />
        {/* Bottom dark section for content (translucent gradient) */}
        <div className="absolute bottom-0 left-0 right-0 h-[45%] bg-gradient-to-t from-black/70 via-black/40 to-transparent" />
      </div>

      {/* Bottom Content with Translucent Layer */}
      <div className="absolute bottom-3 left-3 right-3 z-10 p-4 space-y-1 bg-black/40 backdrop-blur-xl border border-white/10 rounded-[22px]">
        {/* Heading: Name */}
        <h2 className="text-xl font-bold tracking-tight text-white leading-tight">
          {profile.name}
        </h2>

        {/* Subheading: Profession */}
        {profile.profession && (
          <p className="text-xs font-medium text-white/75 tracking-wide">
            {profile.profession}
          </p>
        )}

        {/* Content */}
        {profile.lookingFor && (
          <p className="text-sm font-normal text-white/90 leading-snug line-clamp-3 pt-0.5">
            {profile.lookingFor}
          </p>
        )}
      </div>
    </div>
  );
};

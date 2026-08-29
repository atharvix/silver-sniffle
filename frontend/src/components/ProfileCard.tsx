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

      {/* Bottom Content — 65-70% translucent */}
      <div className="absolute bottom-0 left-0 right-0 z-10 p-5 space-y-2.5 opacity-70">
        {/* Author row: avatar + "{name} shared" */}
        <div className="flex items-center gap-2">
          <img
            src={profile.avatar}
            alt={profile.name}
            className="w-6 h-6 rounded-full object-cover opacity-90"
          />
          <span className="text-[13px] text-white/80 font-normal">
            {profile.name} shared
          </span>
        </div>

        {/* Main Headline */}
        <h3 className="text-[22px] font-bold tracking-tight text-white leading-[1.2] line-clamp-3">
          {profile.lookingFor}
        </h3>
      </div>
    </div>
  );
};

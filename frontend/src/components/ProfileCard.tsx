import React from 'react';
import type { UserProfile } from '../types';
import { MapPin } from 'lucide-react';

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
      <div className="profile-card relative w-full h-full rounded-[28px] overflow-hidden bg-[#111111] select-none shadow-xl">
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
    <div className="profile-card relative w-full h-full rounded-[28px] overflow-hidden select-none bg-[#0d0d0d] shadow-2xl border border-white/10">
      {/* Full Portrait Background */}
      <div className="absolute inset-0 z-0">
        <img
          src={profile.avatar}
          alt={profile.name}
          className="w-full h-full object-cover object-center"
        />
        {/* Top subtle vignette */}
        <div className="absolute inset-0 bg-gradient-to-b from-black/30 via-transparent to-transparent" />
        {/* Bottom dark section for content */}
        <div className="absolute bottom-0 left-0 right-0 h-[45%] bg-gradient-to-t from-black/80 via-black/45 to-transparent" />
      </div>

      {/* Top Right Distance Badge */}
      <div className="absolute top-3.5 right-3.5 z-10 px-3 py-1 rounded-full bg-black/50 backdrop-blur-md border border-white/20 text-[11px] font-semibold text-white/90 flex items-center gap-1 shadow-lg">
        <MapPin className="w-3.5 h-3.5 text-white/80 shrink-0" strokeWidth={2} />
        <span>{profile.distanceMeters}m</span>
      </div>

      {/* Bottom Content Box */}
      <div className="absolute bottom-3 left-3 right-3 z-10 p-4 space-y-1 bg-black/50 backdrop-blur-xl border border-white/15 rounded-[22px]">
        {/* Name */}
        <h2 className="text-xl font-bold tracking-tight text-white leading-tight">
          {profile.name}
        </h2>

        {/* What you do / Profession */}
        {profile.profession && (
          <p className="text-xs font-semibold text-white/80 tracking-wide">
            {profile.profession}
          </p>
        )}

        {/* What you're looking for (truncated to 2 lines on card stack) */}
        {profile.lookingFor && (
          <p className="text-xs font-normal text-white/85 leading-snug line-clamp-2 pt-0.5 opacity-90">
            {profile.lookingFor.length > 90 ? `${profile.lookingFor.slice(0, 88)}...` : profile.lookingFor}
          </p>
        )}
      </div>
    </div>
  );
};


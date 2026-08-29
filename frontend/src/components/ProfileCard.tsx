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
        <div className="absolute inset-0 bg-gradient-to-b from-black/30 via-transparent to-transparent" />
        {/* Bottom dark section for content (translucent gradient) */}
        <div className="absolute bottom-0 left-0 right-0 h-[45%] bg-gradient-to-t from-black/70 via-black/40 to-transparent" />
      </div>

      {/* Top Right Distance Badge */}
      <div className="absolute top-3.5 right-3.5 z-10 px-3 py-1 rounded-full bg-black/40 backdrop-blur-md border border-white/15 text-[11px] font-medium text-white/80 flex items-center gap-1 shadow-md">
        <MapPin className="w-3 h-3 text-white/60 shrink-0" strokeWidth={1.8} />
        <span>{profile.distanceMeters}m away</span>
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

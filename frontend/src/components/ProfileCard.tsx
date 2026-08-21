import React from 'react';
import type { UserProfile } from '../types';

interface ProfileCardProps {
  profile: UserProfile;
  isBackCard?: boolean;
  onClick?: () => void;
}

export const ProfileCard: React.FC<ProfileCardProps> = ({
  profile,
  isBackCard = false,
  onClick,
}) => {
  return (
    <div
      onClick={onClick}
      className={`w-full h-full rounded-[36px] p-4 md:p-5 flex flex-col justify-between select-none transition-all duration-300 ${
        isBackCard
          ? 'bg-[#e5dfd4] text-neutral-800 back-card-shadow border border-white/20'
          : 'bg-[#f4efe6] text-[#1a1a1a] cream-card-shadow border border-white/50 cursor-pointer'
      }`}
    >
      {/* Top Rounded Photo Container */}
      <div className="relative w-full h-[62%] rounded-[26px] overflow-hidden bg-neutral-200">
        <img
          src={profile.avatar}
          alt={profile.name}
          className="w-full h-full object-cover"
        />
      </div>

      {/* Bottom Content Area (Matches Screenshot Layout Exactly) */}
      <div className="flex-1 flex flex-col justify-center px-2 py-3 space-y-2">
        {/* Author Line: Small avatar + Name shared */}
        <div className="flex items-center gap-2">
          <img
            src={profile.avatar}
            alt={profile.name}
            className="w-6 h-6 rounded-full object-cover border border-black/10"
          />
          <span className="text-xs font-semibold text-neutral-700 truncate">
            {profile.name} <span className="font-normal text-neutral-500">shared</span>
          </span>
        </div>

        {/* Large Quoted Headline Prompt */}
        <h3 className="text-xl md:text-2xl font-bold tracking-tight text-neutral-900 leading-snug line-clamp-3">
          {profile.quotePrompt}
        </h3>
      </div>
    </div>
  );
};

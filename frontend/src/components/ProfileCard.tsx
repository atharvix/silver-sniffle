import React from 'react';
import type { UserProfile } from '../types';
import { MapPin, ShieldCheck } from 'lucide-react';

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
      className={`relative w-full h-full rounded-[28px] overflow-hidden flex flex-col justify-between select-none transition-all duration-300 border ${
        isBackCard
          ? 'bg-[#0f1016] border-white/10 text-white shadow-2xl opacity-90'
          : 'bg-[#090a0f] border-white/15 text-white shadow-2xl hover:border-white/25 cursor-pointer'
      }`}
    >
      {/* Background Portrait Photo with Dark Vignette Gradient */}
      <div className="absolute inset-0 z-0">
        <img
          src={profile.avatar}
          alt={profile.name}
          className="w-full h-full object-cover object-top filter brightness-[0.92] contrast-[1.05]"
        />
        <div className="absolute inset-0 bg-gradient-to-b from-black/50 via-black/20 to-[#070709] opacity-95" />
      </div>

      {/* Top Header inside Card */}
      <div className="relative z-10 p-4 sm:p-5 flex items-center justify-between">
        <span className="text-base sm:text-lg font-bold tracking-tight text-white/90 font-sans drop-shadow-md">
          kinjo<span className="text-white/40">.</span>
        </span>

        {/* Distance Tag */}
        <span className="px-3 py-1 rounded-full bg-black/60 border border-white/15 text-white/90 text-xs font-medium flex items-center gap-1 backdrop-blur-md">
          <MapPin className="w-3 h-3 text-emerald-400" />
          <span>{profile.distanceMeters}m away</span>
        </span>
      </div>

      {/* Bottom Translucent Container (60% translucent with backdrop blur) */}
      <div className="relative z-10 p-3.5 sm:p-4 m-3 sm:m-3.5 bg-[#14151c]/60 backdrop-blur-xl border border-white/15 rounded-[22px] shadow-2xl flex flex-col gap-2">
        {/* Author Line */}
        <div className="flex items-center gap-2.5">
          <div className="relative">
            <img
              src={profile.avatar}
              alt={profile.name}
              className="w-7 h-7 rounded-full object-cover border border-white/20"
            />
            {profile.online && (
              <span className="absolute bottom-0 right-0 w-2 h-2 rounded-full bg-emerald-400 border border-black" />
            )}
          </div>

          <div className="flex items-center gap-1 text-xs font-semibold text-white/90 truncate">
            <span className="text-white font-medium">{profile.name}</span>
            <span className="text-neutral-400 font-normal opacity-80">shared</span>
          </div>

          {profile.verified && (
            <ShieldCheck className="w-3.5 h-3.5 text-emerald-400 shrink-0 ml-auto" />
          )}
        </div>

        {/* Quoted Headline Prompt */}
        <h3 className="text-sm sm:text-base md:text-lg font-medium tracking-tight text-white/95 leading-snug line-clamp-3">
          &ldquo;{profile.quotePrompt}&rdquo;
        </h3>

        {/* Tags */}
        {profile.tags && profile.tags.length > 0 && (
          <div className="flex flex-wrap gap-1 pt-0.5">
            {profile.tags.slice(0, 3).map((tag, idx) => (
              <span
                key={idx}
                className="px-2 py-0.5 rounded-md bg-white/10 border border-white/10 text-[10px] font-medium text-neutral-200 backdrop-blur-sm"
              >
                #{tag}
              </span>
            ))}
          </div>
        )}
      </div>
    </div>
  );
};

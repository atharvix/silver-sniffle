import React from 'react';
import type { UserProfile } from '../types';
import { MapPin } from 'lucide-react';

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
      className={`profile-card relative w-full h-full rounded-[36px] overflow-hidden flex flex-col justify-between select-none transition-all duration-300 border ${
        isBackCard
          ? 'bg-[#000000] border-white/15 text-white shadow-2xl opacity-90'
          : 'bg-[#000000] border-white/20 text-white shadow-[0_30px_80px_rgba(0,0,0,0.9)] hover:border-white/35 cursor-pointer'
      }`}
    >
      {/* 1. Full Portrait Background Photo */}
      <div className="absolute inset-0 z-0">
        <img
          src={profile.avatar}
          alt={profile.name}
          className="w-full h-full object-cover object-center filter brightness-[0.98] contrast-[1.02]"
        />
        {/* Vignette Gradients */}
        <div className="absolute inset-0 bg-gradient-to-b from-black/50 via-transparent to-black/80" />
      </div>

      {/* 2. Top Header inside Card */}
      <div className="relative z-10 p-5 flex items-center justify-between">
        {/* Left: k. Branding Badge */}
        <div className="flex items-center gap-1.5 px-3 py-1 rounded-full bg-black/30 backdrop-blur-md border border-white/15 shadow-md">
          <span className="text-lg font-black tracking-tight text-white font-sans">
            k<span className="text-white/40">.</span>
          </span>
        </div>

        {/* Right: Distance Badge (Pure Translucent Neutral, No Green) */}
        <span className="px-3.5 py-1.5 rounded-full bg-black/35 backdrop-blur-xl border border-white/20 text-white text-xs font-semibold flex items-center gap-1.5 shadow-lg">
          <MapPin className="w-3.5 h-3.5 text-white/70 shrink-0" />
          <span>{profile.distanceMeters}m away</span>
        </span>
      </div>

      {/* 3. Ultra-Translucent Glassmorphic Bottom Text Box */}
      <div className="relative z-10 p-4 sm:p-5 m-3.5 sm:m-4 bg-black/25 backdrop-blur-2xl border border-white/18 rounded-[28px] shadow-[0_20px_50px_rgba(0,0,0,0.5)] flex flex-col gap-2.5">
        {/* Author Line */}
        <div className="flex items-center gap-3">
          <div className="relative shrink-0">
            <img
              src={profile.avatar}
              alt={profile.name}
              className="w-9 h-9 rounded-full object-cover border border-white/30"
            />
          </div>

          <div className="flex flex-col min-w-0 flex-1">
            <div className="flex items-center gap-1.5 text-xs font-bold text-white truncate">
              <span className="truncate">{profile.name}</span>
            </div>
            <span className="text-[11px] text-neutral-300 font-medium truncate opacity-85">
              {profile.subtitle || profile.handle}
            </span>
          </div>
        </div>

        {/* Quoted Headline Prompt */}
        <h3 className="text-base sm:text-lg font-bold tracking-tight text-white leading-snug line-clamp-3">
          &ldquo;{profile.quotePrompt}&rdquo;
        </h3>

        {/* Category & Tags Row */}
        <div className="flex flex-wrap items-center gap-1.5 pt-0.5">
          <span className="px-2.5 py-0.5 rounded-full bg-white/15 border border-white/20 text-[11px] font-bold text-white backdrop-blur-md">
            {profile.category}
          </span>
          {profile.tags && profile.tags.slice(0, 2).map((tag, idx) => (
            <span
              key={idx}
              className="px-2.5 py-0.5 rounded-full bg-white/10 border border-white/15 text-[11px] font-medium text-neutral-200 backdrop-blur-md"
            >
              #{tag}
            </span>
          ))}
        </div>
      </div>
    </div>
  );
};

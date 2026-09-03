import React from 'react';
import type { UserProfile } from '../types';
import { ArrowLeft } from 'lucide-react';

interface ProfileDetailScreenProps {
  profile: UserProfile;
  onClose: () => void;
}

export const ProfileDetailScreen: React.FC<ProfileDetailScreenProps> = ({
  profile,
  onClose,
}) => {
  return (
    <div
      className="fixed inset-0 z-50 bg-black overflow-y-auto select-none"
      style={{ animation: 'screen-slide-up 280ms cubic-bezier(0.32,0.72,0,1) both' }}
    >
      {/* Full-bleed Hero Photo */}
      <div className="relative w-full" style={{ height: '65vh', minHeight: 360 }}>
        <img
          src={profile.avatar}
          alt={profile.name}
          className="w-full h-full object-cover"
        />
        {/* Gradient overlay */}
        <div className="absolute inset-0 bg-gradient-to-t from-black via-black/20 to-transparent" />

        {/* Back button */}
        <button
          onClick={onClose}
          className="absolute top-[max(18px,env(safe-area-inset-top))] left-4 p-2.5 rounded-full bg-black/40 backdrop-blur-md border border-white/10 text-white/80 hover:text-white transition-colors"
        >
          <ArrowLeft className="w-4 h-4" strokeWidth={2} />
        </button>
      </div>

      {/* Details */}
      <div className="px-5 pt-6 pb-16 space-y-6">
        {/* Name + Profession ("What you do") */}
        <div>
          <h1 className="text-3xl font-bold text-white tracking-tight leading-tight">
            {profile.name}
          </h1>
          {profile.profession && (
            <p className="text-sm font-semibold text-white/70 mt-1.5">{profile.profession}</p>
          )}
        </div>

        {/* Looking For ("What are you looking for") */}
        {profile.lookingFor && (
          <div className="space-y-2">
            <p className="text-[10px] font-semibold text-white/40 uppercase tracking-widest">
              Looking for
            </p>
            <p className="text-base font-medium text-white/90 leading-relaxed">
              {profile.lookingFor}
            </p>
          </div>
        )}
      </div>
    </div>
  );
};


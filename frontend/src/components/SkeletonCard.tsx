import React from 'react';

export const SkeletonCard: React.FC = () => {
  return (
    <div
      className="relative flex flex-col justify-end w-full h-full rounded-3xl overflow-hidden shadow-2xl border border-white/10 bg-slate-900/90 animate-pulse select-none"
      style={{ width: 'min(92vw, 360px)', height: 'min(70vh, 550px)', minHeight: 440 }}
    >
      {/* Background Avatar Skeleton */}
      <div className="absolute inset-0 bg-gradient-to-t from-slate-950 via-slate-900/80 to-slate-800/50 animate-pulse" />

      {/* Shimmer Overlay */}
      <div className="absolute inset-0 bg-gradient-to-r from-transparent via-white/5 to-transparent animate-shimmer" />

      {/* Card Content Skeleton */}
      <div className="relative z-10 p-6 space-y-4">
        {/* Distance Badge Skeleton */}
        <div className="w-28 h-6 rounded-full bg-white/10" />

        {/* Name Skeleton */}
        <div className="w-3/4 h-8 rounded-lg bg-white/20" />

        {/* Bio / Profession Skeleton */}
        <div className="space-y-2 pt-2">
          <div className="w-full h-4 rounded bg-white/10" />
          <div className="w-5/6 h-4 rounded bg-white/10" />
        </div>

        {/* Footer Badge Skeleton */}
        <div className="flex gap-2 pt-2">
          <div className="w-20 h-6 rounded-md bg-white/10" />
          <div className="w-24 h-6 rounded-md bg-white/10" />
        </div>
      </div>
    </div>
  );
};

import React, { useEffect } from 'react';
import type { UserProfile } from '../types';
import confetti from 'canvas-confetti';
import { Heart, X } from 'lucide-react';

interface MatchNotificationModalProps {
  matchedProfile: UserProfile | null;
  onClose: () => void;
}

export const MatchNotificationModal: React.FC<MatchNotificationModalProps> = ({
  matchedProfile,
  onClose,
}) => {
  useEffect(() => {
    if (matchedProfile) {
      confetti({
        particleCount: 100,
        spread: 70,
        origin: { y: 0.6 },
        colors: ['#e63946', '#10b981', '#f59e0b', '#8b5cf6'],
      });
    }
  }, [matchedProfile]);

  if (!matchedProfile) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/85 backdrop-blur-md animate-fade-in">
      <div className="relative w-full max-w-sm bg-[#121319] rounded-[32px] p-8 border border-white/15 text-center flex flex-col items-center gap-6 shadow-2xl my-auto">
        <button
          onClick={onClose}
          className="absolute top-4 right-4 w-8 h-8 rounded-full bg-white/5 hover:bg-white/10 text-neutral-400 hover:text-white flex items-center justify-center transition-colors"
        >
          <X className="w-4 h-4" />
        </button>

        <div className="w-16 h-16 rounded-full bg-emerald-500/10 text-emerald-400 flex items-center justify-center border border-emerald-500/30">
          <Heart className="w-8 h-8 fill-emerald-400" />
        </div>

        <div>
          <span className="text-xs font-extrabold uppercase tracking-widest text-emerald-400 block mb-1">
            New Proximity Match!
          </span>
          <h3 className="text-2xl font-extrabold text-white">You & {matchedProfile.name}</h3>
          <p className="text-xs text-neutral-300 mt-2">
            You are within <span className="text-emerald-400 font-bold">{matchedProfile.distanceMeters} meters</span> of each other.
          </p>
        </div>

        <div className="w-full p-4 rounded-2xl bg-white/[0.04] border border-white/10 flex items-center gap-3 text-left">
          <img
            src={matchedProfile.avatar}
            alt={matchedProfile.name}
            className="w-12 h-12 rounded-full object-cover border border-white/20"
          />
          <div>
            <p className="text-xs font-bold text-white">{matchedProfile.name}</p>
            <p className="text-[11px] text-neutral-300 italic line-clamp-1">
              &ldquo;{matchedProfile.quotePrompt}&rdquo;
            </p>
          </div>
        </div>

        <div className="flex flex-col gap-2.5 w-full">
          <button
            onClick={onClose}
            className="w-full py-2.5 text-xs text-neutral-400 hover:text-white transition-colors"
          >
            Keep Swiping
          </button>
        </div>
      </div>
    </div>
  );
};

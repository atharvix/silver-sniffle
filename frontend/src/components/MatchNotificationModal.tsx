import React, { useEffect } from 'react';
import type { UserProfile } from '../types';
import confetti from 'canvas-confetti';
import { X, Bell } from 'lucide-react';

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
        particleCount: 80,
        spread: 60,
        origin: { y: 0.6 },
        colors: ['#ffffff', '#10b981', '#6366f1', '#f59e0b'],
      });
    }
  }, [matchedProfile]);

  if (!matchedProfile) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/90 backdrop-blur-xl animate-fade-in select-none">
      <div className="relative w-full max-w-sm bg-[#08080a] rounded-[36px] p-7 border border-white/20 text-center flex flex-col items-center gap-5 shadow-[0_30px_90px_rgba(0,0,0,0.95)] my-auto">
        <button
          onClick={onClose}
          className="absolute top-4 right-4 w-8 h-8 rounded-full bg-white/5 hover:bg-white/10 text-neutral-400 hover:text-white flex items-center justify-center transition-colors border border-white/10"
        >
          <X className="w-4 h-4" />
        </button>

        <div className="w-16 h-16 rounded-full bg-white/10 text-white flex items-center justify-center border border-white/20 shadow-lg">
          <Bell className="w-7 h-7 text-emerald-400" />
        </div>

        <div>
          <span className="text-xs font-extrabold uppercase tracking-widest text-emerald-400 block mb-1">
            Profile Saved & Notified!
          </span>
          <h3 className="text-xl font-extrabold text-white">Saved {matchedProfile.name}</h3>
          <p className="text-xs text-neutral-300 mt-1.5 leading-relaxed">
            A notification has been sent to <span className="text-white font-bold">{matchedProfile.name}</span> letting them know you saved their profile within {matchedProfile.distanceMeters}m.
          </p>
        </div>

        <div className="w-full p-3.5 rounded-2xl bg-white/[0.05] border border-white/12 flex items-center gap-3 text-left">
          <img
            src={matchedProfile.avatar}
            alt={matchedProfile.name}
            className="w-11 h-11 rounded-full object-cover border border-white/20 shrink-0"
          />
          <div className="min-w-0 flex-1">
            <p className="text-xs font-bold text-white truncate">{matchedProfile.name}</p>
            <p className="text-[11px] text-neutral-400 truncate">{matchedProfile.subtitle || matchedProfile.handle}</p>
          </div>
        </div>

        <button
          onClick={onClose}
          className="w-full py-3 rounded-2xl bg-white text-black font-extrabold text-xs shadow-lg active:scale-95 transition-all"
        >
          Continue Discovering
        </button>
      </div>
    </div>
  );
};

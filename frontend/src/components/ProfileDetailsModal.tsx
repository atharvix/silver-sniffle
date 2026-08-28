import React, { useEffect, useRef } from 'react';
import type { UserProfile } from '../types';
import { X, MapPin, CheckCircle, Heart } from 'lucide-react';
import Lenis from 'lenis';

interface ProfileDetailsModalProps {
  profile: UserProfile | null;
  isOpen: boolean;
  onClose: () => void;
  onConnect: (profile: UserProfile) => void;
}

export const ProfileDetailsModal: React.FC<ProfileDetailsModalProps> = ({
  profile,
  isOpen,
  onClose,
  onConnect,
}) => {
  const scrollContainerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!isOpen || !scrollContainerRef.current) return;

    const lenis = new Lenis({
      wrapper: scrollContainerRef.current,
      content: scrollContainerRef.current.firstElementChild as HTMLElement,
      duration: 1.2,
      easing: (t) => Math.min(1, 1.001 - Math.pow(2, -10 * t)),
    });

    function raf(time: number) {
      lenis.raf(time);
      requestAnimationFrame(raf);
    }

    const rafId = requestAnimationFrame(raf);

    return () => {
      cancelAnimationFrame(rafId);
      lenis.destroy();
    };
  }, [isOpen]);

  if (!isOpen || !profile) return null;

  const wordCount = profile.bio.trim().split(/\s+/).filter(Boolean).length;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/80 backdrop-blur-md overflow-hidden">
      <div className="relative w-full max-w-lg h-[82vh] bg-[#121319] rounded-[32px] overflow-hidden flex flex-col border border-white/10 my-auto shadow-2xl">
        <div className="sticky top-0 z-20 p-5 bg-[#121319]/90 backdrop-blur-xl border-b border-white/10 flex items-center justify-between">
          <div className="flex items-center gap-3">
            <img
              src={profile.avatar}
              alt={profile.name}
              className="w-10 h-10 rounded-full object-cover border border-white/20"
            />
            <div>
              <div className="flex items-center gap-1.5 font-bold text-white text-sm">
                <span>{profile.name}</span>
                {profile.verified && <CheckCircle className="w-4 h-4 text-emerald-400 fill-emerald-400/20" />}
              </div>
              <p className="text-xs text-neutral-400">{profile.handle}</p>
            </div>
          </div>

          <button
            onClick={onClose}
            className="w-8 h-8 rounded-full bg-white/5 hover:bg-white/10 text-neutral-400 hover:text-white flex items-center justify-center transition-colors"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        <div ref={scrollContainerRef} className="flex-1 overflow-y-auto p-6 space-y-6">
          <div>
            <div className="relative w-full h-64 rounded-2xl overflow-hidden mb-6 card-shadow">
              <img src={profile.avatar} alt={profile.name} className="w-full h-full object-cover" />
              <div className="absolute inset-0 bg-gradient-to-t from-[#121319] via-transparent to-transparent" />
              <div className="absolute bottom-4 left-4 right-4 flex items-center justify-between text-xs text-white">
                <div className="glass-pill px-3 py-1.5 rounded-full flex items-center gap-1.5">
                  <MapPin className="w-3.5 h-3.5 text-rose-400" />
                  <span>{profile.locationName}</span>
                </div>
                <div className="glass-pill px-3 py-1.5 rounded-full text-emerald-400 font-semibold">
                  {profile.distanceMeters}m radius
                </div>
              </div>
            </div>

            <div className="p-4 rounded-2xl bg-white/[0.04] border border-white/10 mb-6">
              <span className="text-xs font-semibold text-rose-400 uppercase tracking-wider block mb-1">
                Headline Prompt
              </span>
              <p className="text-xl font-bold text-white leading-snug">
                &ldquo;{profile.quotePrompt}&rdquo;
              </p>
            </div>

            <div className="mb-6">
              <span className="text-xs font-semibold text-neutral-400 block mb-2">Category & Tags</span>
              <div className="flex flex-wrap gap-2">
                <span className="bg-rose-500/20 text-rose-300 border border-rose-500/30 px-3 py-1 rounded-full text-xs font-semibold">
                  {profile.category}
                </span>
                {profile.tags.map((tag, idx) => (
                  <span key={idx} className="glass-pill px-3 py-1 rounded-full text-xs text-neutral-300">
                    #{tag}
                  </span>
                ))}
              </div>
            </div>

            <div className="mb-6 space-y-2">
              <div className="flex items-center justify-between">
                <span className="text-xs font-semibold text-neutral-400">Complete Bio</span>
                <span className="text-xs font-semibold text-emerald-400 bg-emerald-500/10 px-2.5 py-0.5 rounded-full border border-emerald-500/20">
                  {wordCount} words
                </span>
              </div>
              <p className="text-sm text-neutral-200 leading-relaxed bg-white/[0.04] p-4 rounded-2xl border border-white/10 font-normal">
                {profile.bio}
              </p>
            </div>
          </div>
        </div>

        <div className="p-4 bg-[#121319] border-t border-white/10 flex items-center gap-3">
          <button
            onClick={() => {
              onConnect(profile);
              onClose();
            }}
            className="flex-1 flex items-center justify-center gap-2 bg-[#f2ece1] hover:bg-white text-neutral-950 font-bold text-xs py-3 px-4 rounded-xl shadow-lg transition-all"
          >
            <Heart className="w-4 h-4 fill-neutral-950" />
            <span>Connect</span>
          </button>
        </div>
      </div>
    </div>
  );
};

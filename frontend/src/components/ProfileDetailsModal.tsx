import React, { useEffect, useRef, useState } from 'react';
import type { UserProfile } from '../types';
import { X, MapPin, Bookmark, Globe, AtSign, Share2 } from 'lucide-react';
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
  const [copiedNetwork, setCopiedNetwork] = useState<string | null>(null);

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

  const handleCopy = (network: string, handle: string) => {
    navigator.clipboard.writeText(handle);
    setCopiedNetwork(network);
    setTimeout(() => setCopiedNetwork(null), 2000);
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-3 sm:p-5 bg-black/90 backdrop-blur-2xl overflow-hidden select-none">
      <div className="relative w-full max-w-md h-[86vh] bg-[#08080a] rounded-[36px] overflow-hidden flex flex-col border border-white/20 my-auto shadow-[0_30px_90px_rgba(0,0,0,0.95)]">
        {/* Sticky Header */}
        <div className="sticky top-0 z-20 p-4 bg-[#08080a]/90 backdrop-blur-xl border-b border-white/10 flex items-center justify-between">
          <div className="flex items-center gap-3">
            <img
              src={profile.avatar}
              alt={profile.name}
              className="w-10 h-10 rounded-full object-cover border border-white/20"
            />
            <div>
              <h2 className="font-bold text-white text-sm">{profile.name}</h2>
              <p className="text-xs text-neutral-400">{profile.subtitle || profile.handle}</p>
            </div>
          </div>

          <button
            onClick={onClose}
            className="w-8 h-8 rounded-full bg-white/10 hover:bg-white/20 text-neutral-300 hover:text-white flex items-center justify-center transition-colors border border-white/15 active:scale-95"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        {/* Scrollable Full Profile Details Body */}
        <div ref={scrollContainerRef} className="flex-1 overflow-y-auto p-5 sm:p-6 space-y-6">
          <div>
            {/* Hero Image Section */}
            <div className="relative w-full h-64 rounded-[28px] overflow-hidden mb-6 shadow-2xl">
              <img src={profile.avatar} alt={profile.name} className="w-full h-full object-cover" />
              <div className="absolute inset-0 bg-gradient-to-t from-[#08080a] via-transparent to-transparent" />
              <div className="absolute bottom-4 left-4 right-4 flex items-center justify-between text-xs text-white">
                <div className="px-3 py-1.5 rounded-full bg-black/60 backdrop-blur-md border border-white/15 flex items-center gap-1.5">
                  <MapPin className="w-3.5 h-3.5 text-white/70" />
                  <span>{profile.locationName}</span>
                </div>
                <div className="px-3 py-1.5 rounded-full bg-white/10 border border-white/20 text-white font-semibold backdrop-blur-md">
                  {profile.distanceMeters}m radius
                </div>
              </div>
            </div>

            {/* Quoted Headline Prompt */}
            <div className="p-4 rounded-[22px] bg-white/[0.04] border border-white/12 mb-6">
              <span className="text-[11px] font-semibold text-neutral-400 uppercase tracking-wider block mb-1">
                Headline Prompt
              </span>
              <p className="text-lg font-bold text-white leading-snug">
                &ldquo;{profile.quotePrompt}&rdquo;
              </p>
            </div>

            {/* Category & Tags */}
            <div className="mb-6">
              <span className="text-xs font-semibold text-neutral-400 block mb-2">Category & Tags</span>
              <div className="flex flex-wrap gap-2">
                <span className="bg-white/20 text-white border border-white/25 px-3.5 py-1 rounded-full text-xs font-bold">
                  {profile.category}
                </span>
                {profile.tags.map((tag, idx) => (
                  <span key={idx} className="px-3 py-1 rounded-full bg-white/10 border border-white/15 text-xs text-neutral-200">
                    #{tag}
                  </span>
                ))}
              </div>
            </div>

            {/* Complete Bio */}
            <div className="mb-6 space-y-2">
              <span className="text-xs font-semibold text-neutral-400 block">Complete Bio</span>
              <p className="text-sm text-neutral-200 leading-relaxed bg-white/[0.04] p-4 rounded-[20px] border border-white/10 font-normal">
                {profile.bio}
              </p>
            </div>

            {/* Social Media Links Addition */}
            {profile.socialLinks && Object.keys(profile.socialLinks).length > 0 && (
              <div className="mb-6 space-y-2">
                <span className="text-xs font-semibold text-neutral-400 block">Social Media Handles</span>
                <div className="space-y-2.5">
                  {Object.entries(profile.socialLinks).map(([network, handle]) => {
                    const isCopied = copiedNetwork === network;
                    return (
                      <div
                        key={network}
                        onClick={() => handleCopy(network, handle)}
                        className="w-full p-3 rounded-2xl bg-white/[0.06] border border-white/15 flex items-center justify-between transition-all hover:bg-white/[0.1] cursor-pointer"
                      >
                        <div className="flex items-center gap-3">
                          <div className="w-8 h-8 rounded-xl bg-white/10 flex items-center justify-center text-white border border-white/15">
                            {network === 'Instagram' || network === 'Twitter' ? (
                              <AtSign className="w-4 h-4" />
                            ) : (
                              <Globe className="w-4 h-4" />
                            )}
                          </div>
                          <div className="text-left">
                            <span className="text-[11px] font-medium text-neutral-400 block">{network}</span>
                            <span className="text-xs font-bold text-white">{handle}</span>
                          </div>
                        </div>

                        <button
                          type="button"
                          className="px-3 py-1 rounded-lg bg-white/10 hover:bg-white/20 text-[11px] font-bold text-white border border-white/15 flex items-center gap-1 transition-all"
                        >
                          {isCopied ? (
                            <span className="text-emerald-400">Copied</span>
                          ) : (
                            <>
                              <Share2 className="w-3 h-3" />
                              <span>Copy</span>
                            </>
                          )}
                        </button>
                      </div>
                    );
                  })}
                </div>
              </div>
            )}
          </div>
        </div>

        {/* Bottom Connect Action */}
        <div className="p-4 bg-[#08080a] border-t border-white/10">
          <button
            onClick={() => {
              onConnect(profile);
              onClose();
            }}
            className="w-full flex items-center justify-center gap-2 bg-white hover:bg-neutral-200 text-black font-extrabold text-xs py-3.5 px-4 rounded-2xl shadow-lg transition-all active:scale-95"
          >
            <Bookmark className="w-4 h-4 fill-black" />
            <span>Connect & Save</span>
          </button>
        </div>
      </div>
    </div>
  );
};

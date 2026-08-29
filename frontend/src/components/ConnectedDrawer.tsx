import React from 'react';
import type { UserProfile } from '../types';
import { X, MapPin, Trash2, ArrowUpRight } from 'lucide-react';

interface ConnectedDrawerProps {
  isOpen: boolean;
  onClose: () => void;
  connectedProfiles: UserProfile[];
  onOpenDetails: (profile: UserProfile) => void;
  onRemoveMatch: (profileId: string) => void;
}

export const ConnectedDrawer: React.FC<ConnectedDrawerProps> = ({
  isOpen,
  onClose,
  connectedProfiles,
  onOpenDetails,
  onRemoveMatch,
}) => {
  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-3 sm:p-5 bg-black/80 backdrop-blur-xl transition-opacity select-none">
      {/* Backdrop overlay click to close */}
      <div className="absolute inset-0" onClick={onClose} />

      {/* Floating Inward Flush Container (Moved Inward with rounded margins) */}
      <div className="relative z-10 w-full max-w-md h-[88vh] bg-[#080911]/92 border border-white/18 rounded-[36px] p-5 flex flex-col justify-between shadow-[0_28px_80px_rgba(0,0,0,0.85)] overflow-hidden">
        {/* Top Header */}
        <div className="flex items-center justify-between pb-4 border-b border-white/10 shrink-0">
          <div>
            <div className="flex items-center gap-2">
              <h2 className="text-xl font-bold text-white tracking-tight">Connected Profiles</h2>
              <span className="px-2.5 py-0.5 rounded-full bg-emerald-400/10 border border-emerald-400/30 text-emerald-300 text-xs font-bold font-mono">
                {connectedProfiles.length}
              </span>
            </div>
            <p className="text-xs text-neutral-400 mt-0.5">Nearby connections within 30m radius</p>
          </div>

          <button
            onClick={onClose}
            className="w-9 h-9 rounded-full bg-white/5 hover:bg-white/10 text-neutral-400 hover:text-white flex items-center justify-center transition-colors border border-white/10 active:scale-95"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        {/* List of Connected Profiles */}
        <div className="flex-1 my-3 space-y-3.5 overflow-y-auto pr-1">
          {connectedProfiles.length === 0 ? (
            <div className="h-64 flex flex-col items-center justify-center text-center p-6 space-y-3 border border-dashed border-white/12 rounded-[28px] bg-white/[0.02]">
              <div className="w-12 h-12 rounded-full bg-white/10 text-white flex items-center justify-center text-xl font-black font-sans border border-white/15">
                k<span className="text-white/40">.</span>
              </div>
              <h3 className="text-sm font-bold text-white">No Connections Yet</h3>
              <p className="text-xs text-neutral-400 max-w-xs leading-relaxed">
                Swipe right on card profiles to add them to your connected stack.
              </p>
            </div>
          ) : (
            connectedProfiles.map((profile) => (
              <div
                key={profile.id}
                className="p-4 rounded-[24px] bg-white/[0.04] border border-white/12 hover:border-white/20 transition-all flex flex-col gap-3 group shadow-lg backdrop-blur-xl"
              >
                {/* User Info Row */}
                <div className="flex items-center gap-3">
                  <img
                    src={profile.avatar}
                    alt={profile.name}
                    className="w-11 h-11 rounded-full object-cover border border-white/20 shrink-0"
                  />

                  <div className="flex-1 min-w-0">
                    <div className="flex items-center justify-between">
                      <h4 className="text-sm font-bold text-white truncate">{profile.name}</h4>
                      <span className="text-[11px] font-semibold text-emerald-400 bg-emerald-400/10 px-2.5 py-0.5 rounded-full border border-emerald-400/20 flex items-center gap-1">
                        <MapPin className="w-3 h-3" />
                        <span>{profile.distanceMeters}m</span>
                      </span>
                    </div>
                    <p className="text-xs text-neutral-400 truncate">{profile.subtitle || profile.handle}</p>
                  </div>
                </div>

                {/* Quoted note preview */}
                <p className="text-xs text-neutral-200 italic line-clamp-2 bg-black/40 p-2.5 rounded-xl border border-white/10">
                  &ldquo;{profile.quotePrompt}&rdquo;
                </p>

                {profile.socialLinks && Object.entries(profile.socialLinks).length > 0 && (
                  <div className="flex flex-wrap gap-2">
                    {Object.entries(profile.socialLinks).map(([network, url]) => (
                      <a
                        key={network}
                        href={url}
                        target="_blank"
                        rel="noreferrer"
                        className="text-[11px] text-emerald-300 border border-emerald-400/20 bg-emerald-400/10 px-2.5 py-1 rounded-full"
                      >
                        {network}
                      </a>
                    ))}
                  </div>
                )}

                {/* Action Buttons */}
                <div className="flex items-center gap-2 pt-1">
                  <button
                    onClick={() => {
                      onOpenDetails(profile);
                      onClose();
                    }}
                    className="flex-1 px-3.5 py-2.5 rounded-xl bg-white hover:bg-neutral-200 text-black text-xs font-extrabold flex items-center justify-center gap-1.5 transition-all shadow-md active:scale-95"
                  >
                    <span>View Profile</span>
                    <ArrowUpRight className="w-3.5 h-3.5" />
                  </button>

                  <button
                    onClick={() => onRemoveMatch(profile.id)}
                    className="p-2.5 rounded-xl bg-rose-500/10 hover:bg-rose-500/20 text-rose-400 border border-rose-500/20 transition-all active:scale-95"
                    title="Remove Profile"
                  >
                    <Trash2 className="w-4 h-4" />
                  </button>
                </div>
              </div>
            ))
          )}
        </div>

        {/* Footer */}
        <div className="pt-3 border-t border-white/10 text-center shrink-0">
          <p className="text-[11px] text-neutral-500 font-mono">
            k. • Proximity Connections (30m Radius)
          </p>
        </div>
      </div>
    </div>
  );
};

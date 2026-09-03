import React, { useRef, useEffect } from 'react';
import type { UserProfile } from '../types';
import { X } from 'lucide-react';

interface ProfileDetailsModalProps {
  profile: UserProfile | null;
  isOpen: boolean;
  onClose: () => void;
}

export const ProfileDetailsModal: React.FC<ProfileDetailsModalProps> = ({
  profile,
  isOpen,
  onClose,
}) => {
  const overlayRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    if (isOpen) document.addEventListener('keydown', handler);
    return () => document.removeEventListener('keydown', handler);
  }, [isOpen, onClose]);

  if (!isOpen || !profile) return null;

  return (
    <div
      ref={overlayRef}
      className="fixed inset-0 z-50 flex items-end justify-center bg-black/70 backdrop-blur-sm select-none"
      onClick={(e) => { if (e.target === overlayRef.current) onClose(); }}
    >
      <div className="w-full max-w-md bg-[#080808] rounded-t-[28px] overflow-hidden"
        style={{ maxHeight: '90vh' }}
      >
        {/* Drag handle */}
        <div className="flex justify-center pt-3 pb-1">
          <div className="w-9 h-1 rounded-full bg-white/15" />
        </div>

        {/* Header */}
        <div className="px-5 pt-3 pb-4 flex items-center justify-between border-b border-white/[0.06]">
          <div className="flex items-center gap-2">
            <img
              src={profile.avatar}
              alt={profile.name}
              className="w-8 h-8 rounded-full object-cover opacity-90"
            />
            <span className="text-sm font-semibold text-white">{profile.name}</span>
          </div>
          <button
            onClick={onClose}
            className="p-1 text-white/30 hover:text-white/70 transition-colors"
          >
            <X className="w-4 h-4" strokeWidth={1.8} />
          </button>
        </div>

        <div className="overflow-y-auto" style={{ maxHeight: 'calc(90vh - 120px)' }}>
          {/* Hero Photo */}
          <div className="relative w-full" style={{ height: '280px' }}>
            <img
              src={profile.avatar}
              alt={profile.name}
              className="w-full h-full object-cover"
            />
            <div className="absolute inset-0 bg-gradient-to-t from-[#080808] via-transparent to-transparent" />
          </div>

          {/* Details */}
          <div className="px-5 pt-5 pb-8 space-y-5">
            {/* Name + Profession */}
            <div>
              <h2 className="text-2xl font-bold text-white tracking-tight">{profile.name}</h2>
              {profile.profession && (
                <p className="text-sm font-semibold text-white/70 mt-1">{profile.profession}</p>
              )}
            </div>

            {/* Looking For */}
            {profile.lookingFor && (
              <div className="space-y-1.5">
                <p className="text-[11px] font-medium text-white/40 uppercase tracking-widest">
                  Looking for
                </p>
                <p className="text-base font-medium text-white/90 leading-snug">
                  {profile.lookingFor}
                </p>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
};

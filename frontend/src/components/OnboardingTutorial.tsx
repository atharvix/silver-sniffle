import React, { useState } from 'react';
import type { UserProfile } from '../types';
import { CardDeck } from './CardDeck';
import { ProfileCard } from './ProfileCard';

interface OnboardingTutorialProps {
  isOpen: boolean;
  step: 'swipe' | 'settings';
  userProfile?: UserProfile | null;
  onAdvance: () => void;
  onClose: () => void;
  onOpenSettings?: () => void;
  onOpenMenu?: () => void;
}

// Curated demo profiles with authentic high-resolution imagery and distance data
const MOCK_PROFILES: UserProfile[] = [
  {
    id: 'tutorial-aiden',
    email: '',
    name: 'Aiden Cross',
    avatar: 'https://images.unsplash.com/photo-1534528741775-53994a69daeb?auto=format&fit=crop&w=800&q=80',
    bio: 'Architectural Designer · Exploring generative spatial tools for architecture studios',
    profession: 'Architectural Designer',
    lookingFor: 'Design Collaborations',
    distanceMeters: 8,
    online: true,
  },
  {
    id: 'tutorial-priya',
    email: '',
    name: 'Priya Nair',
    avatar: 'https://images.unsplash.com/photo-1524504388940-b1c1722653e1?auto=format&fit=crop&w=800&q=80',
    bio: 'Community Ecologist · Building rooftop farms & biodiversity corridors across the city',
    profession: 'Community Ecologist',
    lookingFor: 'Local Projects',
    distanceMeters: 16,
    online: true,
  },
  {
    id: 'tutorial-owen',
    email: '',
    name: 'Owen Baptiste',
    avatar: 'https://images.unsplash.com/photo-1507003211169-0a1dd7228f2d?auto=format&fit=crop&w=800&q=80',
    bio: 'Independent Audio Producer · Recording stories of neighborhood founders and craftsmen',
    profession: 'Audio Producer',
    lookingFor: 'Craft Stories',
    distanceMeters: 24,
    online: true,
  },
];

export const OnboardingTutorial: React.FC<OnboardingTutorialProps> = ({
  isOpen,
  step,
  userProfile,
  onAdvance,
  onClose,
  onOpenSettings,
  onOpenMenu,
}) => {
  const [swipedCards, setSwipedCards] = useState(0);

  const handleCardSwipe = () => {
    setSwipedCards((prev) => {
      const next = prev + 1;
      if (next >= MOCK_PROFILES.length) {
        setTimeout(() => {
          onAdvance();
        }, 400);
      }
      return next;
    });
  };

  if (!isOpen) return null;

  const currentUserCard: UserProfile = {
    id: userProfile?.id || 'current_user',
    email: userProfile?.email || '',
    name: userProfile?.name || 'You',
    avatar: userProfile?.avatar || '',
    bio: userProfile?.bio || 'Tap Settings to customize your presence and what you are looking for',
    profession: userProfile?.profession || 'Kinjo Member',
    lookingFor: userProfile?.lookingFor || '',
    distanceMeters: 0,
    online: true,
  };

  return (
    <div
      className="fixed inset-0 z-[100] flex flex-col select-none overflow-x-hidden w-full max-w-full transition-colors duration-300"
      style={{ background: 'var(--bg)', color: 'var(--fg)' }}
    >
      {/* Top Header matching app-header exactly */}
      <header className="app-header relative z-30 w-full max-w-md mx-auto px-6 flex items-center justify-between select-none">
        {/* Wordmark */}
        <span className="wordmark">
          <svg width="18" height="18" viewBox="0 0 100 100" aria-hidden="true">
            <path fill="currentColor" d="M28 0H63.2V45.3H27.8V100A28 28 0 0 1 0 72V28A28 28 0 0 1 28 0Z"/>
            <path fill="currentColor" d="M71.6 0H72A28 28 0 0 1 100 28V72A28 28 0 0 1 72 100H36.2V53.7H71.6Z"/>
          </svg>
          <span>KINJO</span>
        </span>

        {/* Story Progress Segment Track */}
        {step === 'swipe' ? (
          <div className="flex items-center gap-1.5 py-1">
            {MOCK_PROFILES.map((_, i) => (
              <span
                key={i}
                className="h-1 rounded-full transition-all duration-300"
                style={{
                  width: swipedCards === i ? 24 : 10,
                  background: i <= swipedCards ? 'var(--fg)' : 'var(--hairline)',
                  opacity: i <= swipedCards ? 1 : 0.35,
                }}
              />
            ))}
          </div>
        ) : (
          <span className="text-[11px] font-medium tracking-widest uppercase text-zinc-400">
            Card Setup
          </span>
        )}

        {/* Right Navigation Target */}
        {step === 'swipe' ? (
          <button
            type="button"
            onClick={onClose}
            className="text-xs uppercase tracking-wider transition-opacity hover:opacity-75 cursor-pointer py-1 text-zinc-400"
          >
            Skip
          </button>
        ) : (
          <button
            type="button"
            id="header-profile-menu-button"
            onClick={() => {
              if (onOpenMenu) onOpenMenu();
              else if (onOpenSettings) onOpenSettings();
              else onClose();
            }}
            className="p-1 -mr-1 hover:opacity-75 transition-opacity cursor-pointer flex items-center justify-center text-white"
            aria-label="Open menu"
          >
            <svg width="20" height="13" viewBox="0 0 20 13" fill="none" xmlns="http://www.w3.org/2000/svg" aria-hidden="true">
              <line x1="0" y1="1" x2="20" y2="1" stroke="currentColor" strokeWidth="1.6" />
              <line x1="6" y1="6.5" x2="20" y2="6.5" stroke="currentColor" strokeWidth="1.6" />
              <line x1="0" y1="12" x2="20" y2="12" stroke="currentColor" strokeWidth="1.6" />
            </svg>
          </button>
        )}
      </header>

      {/* Discovery Stage: Interactive Card Stack with Inline Typography Imagery */}
      {step === 'swipe' && (
        <>
          <main className="flex-1 flex flex-col w-full max-w-md mx-auto relative overflow-hidden">
            <CardDeck profiles={MOCK_PROFILES} onSwipe={handleCardSwipe} />
          </main>

          <footer className="w-full max-w-md mx-auto px-6 pb-6 pt-2 flex items-center justify-between select-none shrink-0">
            <span className="text-[11px] tracking-widest uppercase font-medium text-zinc-400">
              Swipe card left or right
            </span>
            <button
              type="button"
              onClick={onAdvance}
              className="text-xs uppercase tracking-wider font-semibold hover:opacity-80 transition-opacity cursor-pointer text-white"
            >
              {swipedCards > 0 ? 'Next: Settings →' : 'Next →'}
            </button>
          </footer>
        </>
      )}

      {/* Presence Stage: Gapless Bento Grid Architecture */}
      {step === 'settings' && (
        <>
          <main className="flex-1 flex flex-col w-full max-w-md mx-auto relative overflow-hidden px-6 pt-2">
            <div className="w-full text-left shrink-0 mb-4">
              <div className="flex items-center gap-2 mb-1.5">
                <span className="w-1.5 h-1.5 bg-zinc-400 shrink-0" />
                <span className="text-[11px] tracking-widest text-zinc-400 uppercase font-medium">
                  YOUR PRESENCE
                </span>
              </div>
              <h1 className="mt-1" style={{ fontSize: 32, fontWeight: 500, lineHeight: 1.12, letterSpacing: '-.035em' }}>
                <span className="block text-zinc-400">This is your card.</span>
                <span className="block text-white">
                  Visible
                  <span
                    className="inline-block w-11 h-5 rounded-full align-middle mx-1.5 overflow-hidden border border-white/20 shadow-inner align-baseline relative top-[-1px]"
                    style={{
                      backgroundImage: currentUserCard.avatar
                        ? `url(${currentUserCard.avatar})`
                        : 'url(https://images.unsplash.com/photo-1534528741775-53994a69daeb?auto=format&fit=crop&w=200&q=80)',
                      backgroundSize: 'cover',
                      backgroundPosition: 'center',
                    }}
                  />
                  within 30m.
                </span>
              </h1>
            </div>

            {/* Gapless Bento Grid */}
            <div className="w-full flex-1 flex flex-col justify-center grid grid-cols-2 grid-flow-dense gap-3 my-auto shrink-0 select-none">
              {/* Bento Cell 1: Full-Bleed User Card Panel */}
              <div
                className="col-span-2 relative rounded-[24px] overflow-hidden shadow-2xl transition-transform duration-500 ease-out"
                style={{
                  height: 'min(38vh, 290px)',
                  boxShadow: 'var(--deck-shadow)',
                  border: '1px solid var(--hairline)',
                }}
              >
                <ProfileCard profile={currentUserCard} />
              </div>

              {/* Bento Cell 2: Proximity Perimeter Indicator */}
              <div
                className="col-span-1 p-3.5 rounded-[18px] flex flex-col justify-between"
                style={{
                  background: 'var(--surface)',
                  border: '1px solid var(--hairline)',
                }}
              >
                <span className="text-[10px] tracking-widest uppercase font-medium text-zinc-400">
                  Radar Radius
                </span>
                <div className="mt-2">
                  <span className="text-xl font-semibold tracking-tight text-white">30 Meters</span>
                  <p className="text-[11px] text-zinc-400 leading-tight mt-0.5">Physical proximity only</p>
                </div>
              </div>

              {/* Bento Cell 3: Appearance & Control Indicator */}
              <div
                className="col-span-1 p-3.5 rounded-[18px] flex flex-col justify-between"
                style={{
                  background: 'var(--surface)',
                  border: '1px solid var(--hairline)',
                }}
              >
                <span className="text-[10px] tracking-widest uppercase font-medium text-zinc-400">
                  Preferences
                </span>
                <div className="mt-2">
                  <span className="text-xl font-semibold tracking-tight text-white">Menu Drawer</span>
                  <p className="text-[11px] text-zinc-400 leading-tight mt-0.5">Themes, bio, settings</p>
                </div>
              </div>
            </div>
          </main>

          {/* Action Trigger Block with High-Contrast Legibility */}
          <footer className="w-full max-w-md mx-auto px-6 pb-6 pt-3 flex flex-col gap-2.5 select-none shrink-0">
            <button
              type="button"
              onClick={() => {
                if (onOpenSettings) onOpenSettings();
                else onClose();
              }}
              className="w-full py-3.5 px-5 font-semibold text-xs uppercase tracking-wider transition-opacity hover:opacity-90 active:scale-[0.99] cursor-pointer text-center"
              style={{
                background: '#ffffff',
                color: '#000000',
                borderRadius: 8,
              }}
            >
              Open Profile & Settings
            </button>
            <button
              type="button"
              onClick={onClose}
              className="w-full py-2 text-xs font-medium uppercase tracking-wider hover:opacity-80 transition-opacity cursor-pointer text-center text-zinc-400"
            >
              Start Exploring Kinjo
            </button>
          </footer>
        </>
      )}
    </div>
  );
};



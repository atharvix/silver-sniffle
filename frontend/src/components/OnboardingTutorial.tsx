import React, { useState, useEffect } from 'react';
import { Lightbulb } from 'lucide-react';
import type { UserProfile } from '../types';
import { CardDeck } from './CardDeck';

interface OnboardingTutorialProps {
  isOpen: boolean;
  step: 'swipe' | 'settings';
  /** The mock card was swiped — move on to the "find settings" step. */
  onAdvance: () => void;
  onClose: () => void;
}

// Fixed, self-contained demo cards so the "swipe to explore" step always has
// something to interact with, even for a brand-new user with nobody nearby yet.
const MOCK_PROFILES: UserProfile[] = [
  {
    id: 'tutorial-aiden',
    email: '',
    name: 'Aiden Cross',
    avatar: 'https://images.unsplash.com/photo-1519085360753-af0119f7cbe7?auto=format&fit=crop&w=800&q=80',
    bio: 'Exploring generative design tools for architecture studios',
    profession: '',
    lookingFor: '',
  },
  {
    id: 'tutorial-priya',
    email: '',
    name: 'Priya Nair',
    avatar: 'https://images.unsplash.com/photo-1524504388940-b1c1722653e1?auto=format&fit=crop&w=800&q=80',
    bio: 'Building community gardens across the city',
    profession: '',
    lookingFor: '',
  },
  {
    id: 'tutorial-owen',
    email: '',
    name: 'Owen Baptiste',
    avatar: 'https://images.unsplash.com/photo-1500648767791-00dcc994a43e?auto=format&fit=crop&w=800&q=80',
    bio: 'Recording a podcast about small-town founders',
    profession: '',
    lookingFor: '',
  },
];

export const OnboardingTutorial: React.FC<OnboardingTutorialProps> = ({
  isOpen,
  step,
  onAdvance,
  onClose,
}) => {
  const [menuRect, setMenuRect] = useState<{ top: number; right: number; width: number; height: number } | null>(null);
  const [settingsRect, setSettingsRect] = useState<{ top: number; left: number; width: number; height: number } | null>(null);

  const [swipedCards, setSwipedCards] = useState(0);

  const handleCardSwipe = () => {
    setSwipedCards((prev) => {
      const next = prev + 1;
      if (next >= MOCK_PROFILES.length) {
        // Allow the final card to complete its exit animation before transitioning
        setTimeout(() => {
          onAdvance();
        }, 450);
      }
      return next;
    });
  };

  useEffect(() => {
    if (!isOpen) return;

    const updateRects = () => {
      const menuBtn = document.getElementById('header-profile-menu-button');
      if (menuBtn) {
        const r = menuBtn.getBoundingClientRect();
        setMenuRect({
          top: r.top,
          right: window.innerWidth - r.right,
          width: r.width,
          height: r.height,
        });
      }

      const settingsBtn = document.getElementById('sidebar-settings-button');
      if (settingsBtn) {
        const r = settingsBtn.getBoundingClientRect();
        setSettingsRect({
          top: r.top,
          left: r.left,
          width: r.width,
          height: r.height,
        });
      } else {
        setSettingsRect(null);
      }
    };

    updateRects();
    const interval = setInterval(updateRects, 300);
    window.addEventListener('resize', updateRects);
    return () => {
      clearInterval(interval);
      window.removeEventListener('resize', updateRects);
    };
  }, [isOpen, step]);

  if (!isOpen) return null;

  // ─── STEP 1: a full-screen guided demo deck — always has cards to swipe ───
  if (step === 'swipe') {
    return (
      <div className="fixed inset-0 z-[100] flex flex-col select-none" style={{ background: 'var(--bg)' }}>
        <div className="flex-1 min-h-0 flex flex-col w-full max-w-md mx-auto relative overflow-hidden">
          <CardDeck profiles={MOCK_PROFILES} onSwipe={handleCardSwipe} />
        </div>

        {/* Floating instruction tooltip */}
        <div className="fixed bottom-8 left-0 right-0 z-[100] flex items-center justify-center pointer-events-auto px-6">
          <div className="flex items-center gap-2 text-xs select-none">
            <span className="font-semibold" style={{ color: 'var(--fg)' }}>1.</span>
            <Lightbulb className="w-3.5 h-3.5 text-amber-400 shrink-0" />
            <span style={{ color: 'var(--fg)' }}>
              {swipedCards === 0 ? 'Swipe cards left or right to explore' : 'Keep swiping or continue'}
            </span>
            <span className="opacity-40" style={{ color: 'var(--muted)' }}>·</span>
            <button
              type="button"
              onClick={onAdvance}
              className="uppercase tracking-wider transition-opacity cursor-pointer hover:opacity-70"
              style={{ color: 'var(--muted)' }}
            >
              {swipedCards > 0 ? 'Continue' : 'Skip tutorial'}
            </button>
          </div>
        </div>
      </div>
    );
  }

  // ─── STEP 2: point at the real Settings entry point ───────────────────────
  return (
    <div className="fixed inset-0 z-[100] pointer-events-none select-none">
      {step === 'settings' && (
        <>
          {/* Highlight on Settings button in drawer, or fallback to header menu icon */}
          {settingsRect ? (
            <div
              style={{
                position: 'absolute',
                top: `${settingsRect.top - 2}px`,
                left: `${settingsRect.left - 2}px`,
                width: `${settingsRect.width + 4}px`,
                height: `${settingsRect.height + 4}px`,
                border: '1.5px solid var(--fg)',
                borderRadius: 8,
              }}
              className="animate-pulse pointer-events-none z-[100]"
            />
          ) : menuRect ? (
            <div
              style={{
                position: 'absolute',
                top: `${menuRect.top - 4}px`,
                right: `${menuRect.right - 4}px`,
                width: `${menuRect.width + 8}px`,
                height: `${menuRect.height + 8}px`,
                border: '1.5px solid var(--fg)',
                borderRadius: 8,
              }}
              className="animate-pulse pointer-events-none z-[100]"
            />
          ) : null}

          {/* Floating instruction tooltip */}
          <div className="fixed bottom-8 left-0 right-0 z-[100] flex items-center justify-center pointer-events-auto px-6">
            <div className="flex items-center gap-2 text-xs select-none">
              <span className="font-semibold" style={{ color: 'var(--fg)' }}>2.</span>
              <Lightbulb className="w-3.5 h-3.5 text-amber-400 shrink-0" />
              <span style={{ color: 'var(--fg)' }}>
                {settingsRect ? 'Tap Settings to manage card & preferences' : 'Tap Menu to open Settings'}
              </span>
              <span className="opacity-40" style={{ color: 'var(--muted)' }}>·</span>
              <button
                type="button"
                onClick={onClose}
                className="uppercase tracking-wider transition-opacity cursor-pointer hover:opacity-70"
                style={{ color: 'var(--muted)' }}
              >
                Skip tutorial
              </button>
            </div>
          </div>
        </>
      )}
    </div>
  );
};

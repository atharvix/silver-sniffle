import React, { useState, useRef, useEffect } from 'react';
import type { UserProfile, SwipeDirection } from '../types';
import gsap from 'gsap';
import { ProfileCard } from './ProfileCard';
import { RotateCw, ArrowDown } from 'lucide-react';

interface CardDeckProps {
  profiles: UserProfile[];
  onSwipe: (direction: SwipeDirection, profile: UserProfile) => void;
  onOpenDetails: (profile: UserProfile) => void;
  onRefresh?: () => void;
}

// Back card stack visual config
const BEHIND: { tx: number; ty: number; rot: number; scale: number }[] = [
  { tx: 12, ty: 8, rot: 3.5, scale: 0.965 },
  { tx: 22, ty: 16, rot: 6.8, scale: 0.93 },
];

export const CardDeck: React.FC<CardDeckProps> = ({
  profiles,
  onSwipe,
  onOpenDetails,
  onRefresh,
}) => {
  const [deck, setDeck] = useState<UserProfile[]>(() => [...profiles]);
  const [isAnimating, setIsAnimating] = useState(false);
  const [isDragging, setIsDragging] = useState(false);
  const [pullDistance, setPullDistance] = useState(0);
  const [isRefreshing, setIsRefreshing] = useState(false);

  const topRef = useRef<HTMLDivElement>(null);
  const dragStart = useRef<{ x: number; y: number; t: number }>({ x: 0, y: 0, t: 0 });

  useEffect(() => {
    setDeck((prevDeck) => {
      if (prevDeck.length === 0) return [...profiles];
      const existingIds = new Set(prevDeck.map((p) => p.id));
      const newProfiles = profiles.filter((p) => !existingIds.has(p.id));
      if (newProfiles.length === 0) return prevDeck;
      return [...prevDeck, ...newProfiles];
    });
  }, [profiles]);

  // Handle Pull-to-Refresh & Card Shuffle
  const handleTriggerRefresh = () => {
    setIsRefreshing(true);
    if (onRefresh) onRefresh();

    // Shuffle deck array smoothly
    setDeck((prev) => {
      const shuffled = [...prev].sort(() => Math.random() - 0.5);
      return shuffled.length > 0 ? shuffled : [...profiles];
    });

    setTimeout(() => {
      setIsRefreshing(false);
      setPullDistance(0);
    }, 600);
  };

  // ─── Empty State Screen ──────────────────────────────────────────────────────
  if (deck.length === 0) {
    return (
      <div className="flex-1 flex flex-col items-center justify-center text-center px-6 py-12 select-none max-w-sm mx-auto">
        <div className="w-16 h-16 rounded-full bg-white/5 border border-white/10 flex items-center justify-center mb-6 shadow-inner">
          <span className="text-2xl font-black text-white/30">k.</span>
        </div>

        <h2 className="text-xl font-bold text-white tracking-tight leading-snug">
          Who’s around?
        </h2>

        <p className="text-sm font-medium text-white/70 mt-2">
          No one to discover just yet.
        </p>

        <p className="text-xs text-white/40 mt-1.5 leading-relaxed">
          Keep exploring — people will appear as you get closer.
        </p>
      </div>
    );
  }

  // ─── Send top card to back (Matches Swipe Direction: Right -> Right; Left -> Left) ───
  const sendToBottom = (direction: SwipeDirection) => {
    const top = deck[0];
    if (!top || !topRef.current) return;

    setIsAnimating(true);

    const vw = window.innerWidth;
    const flyX = direction === 'right' ? vw * 1.3 : -vw * 1.3;
    const flyRot = direction === 'right' ? 22 : -22;

    // Phase 1: Fly current top card off-screen
    gsap.to(topRef.current, {
      x: flyX,
      y: -40,
      rotation: flyRot,
      opacity: 0,
      duration: 0.26,
      ease: 'power2.in',
      onComplete: () => {
        // Re-insert swiped card into a random order near the bottom so it doesn't repeat back-to-back
        setDeck((prev) => {
          if (prev.length <= 1) return prev;
          const [first, ...rest] = prev;
          // Insert first into random index in bottom half of deck
          const insertIdx = Math.floor(rest.length / 2) + Math.floor(Math.random() * (rest.length / 2 + 1));
          const updated = [...rest];
          updated.splice(insertIdx, 0, first);
          return updated;
        });
        setIsAnimating(false);
      },
    });

    // Phase 2: Create a ghost card entering from SAME side direction (Right or Left)
    const stage = topRef.current.parentElement;
    if (stage) {
      const clone = topRef.current.cloneNode(true) as HTMLElement;
      clone.style.position = 'absolute';
      clone.style.inset = '0';
      clone.style.zIndex = '5'; // behind stack
      clone.style.pointerEvents = 'none';
      stage.appendChild(clone);

      const { tx, ty, rot, scale } = BEHIND[1];
      const startX = direction === 'right' ? vw * 0.4 : -vw * 0.4;
      const startRot = direction === 'right' ? 6 : -6;

      gsap.fromTo(
        clone,
        { x: startX, y: 20, rotation: startRot, scale: 0.85, opacity: 0 },
        {
          keyframes: [
            { x: startX * 0.4, y: 15, rotation: startRot * 0.5, scale: 0.9, opacity: 0.7, duration: 0.16, ease: 'power1.out' },
            { x: tx, y: ty, rotation: rot, scale: scale, opacity: 0.8, duration: 0.22, ease: 'power2.out' },
            { opacity: 0, duration: 0.1, ease: 'none' },
          ],
          onComplete: () => clone.remove(),
        }
      );
    }

    onSwipe(direction, top);
  };

  // ─── Pointer gesture handlers (Supports Swipe + Pull to Refresh) ─────────────
  const onDown = (e: React.PointerEvent) => {
    if (isAnimating) return;
    dragStart.current = { x: e.clientX, y: e.clientY, t: Date.now() };
    setIsDragging(true);
    setPullDistance(0);
    topRef.current?.setPointerCapture(e.pointerId);
  };

  const onMove = (e: React.PointerEvent) => {
    if (!isDragging || isAnimating || !topRef.current) return;
    const dx = e.clientX - dragStart.current.x;
    const dy = e.clientY - dragStart.current.y;
    const rot = dx * 0.05;

    // Track pull down distance if drag is predominantly downward
    if (dy > 15 && Math.abs(dx) < Math.abs(dy) * 1.5) {
      setPullDistance(Math.min(dy, 100));
    } else {
      setPullDistance(0);
    }

    topRef.current.style.transform = `translate3d(${dx}px,${dy * 0.35}px,0) rotate(${rot}deg)`;
    topRef.current.style.transition = 'none';
  };

  const onUp = (e: React.PointerEvent) => {
    if (!isDragging) return;
    setIsDragging(false);

    const dx = e.clientX - dragStart.current.x;
    const dy = e.clientY - dragStart.current.y;
    const elapsed = Date.now() - dragStart.current.t;
    const isTap = Math.abs(dx) < 8 && Math.abs(dy) < 8 && elapsed < 260;

    if (isTap) {
      if (topRef.current) {
        topRef.current.style.transform = '';
        topRef.current.style.transition = '';
      }
      onOpenDetails(deck[0]);
      setPullDistance(0);
      return;
    }

    // Check Pull-to-Refresh threshold (Pulled down > 65px)
    if (dy > 65 && Math.abs(dx) < 60) {
      handleTriggerRefresh();
      gsap.to(topRef.current!, {
        x: 0,
        y: 0,
        rotation: 0,
        duration: 0.3,
        ease: 'power2.out',
        clearProps: 'transform',
      });
      return;
    }

    setPullDistance(0);

    const triggered = elapsed < 320 ? Math.abs(dx) > 30 : Math.abs(dx) > 75;
    if (triggered) {
      sendToBottom(dx > 0 ? 'right' : 'left');
    } else {
      gsap.to(topRef.current!, {
        x: 0,
        y: 0,
        rotation: 0,
        duration: 0.35,
        ease: 'back.out(1.7)',
        clearProps: 'transform',
      });
    }
  };

  return (
    <div className="relative flex flex-col items-center justify-between w-full h-full select-none max-w-md mx-auto px-4 py-2">

      {/* Pull-to-Refresh Indicator Ring */}
      {(pullDistance > 15 || isRefreshing) && (
        <div className="absolute top-2 left-1/2 -translate-x-1/2 z-40 flex items-center gap-2 px-4 py-1.5 rounded-full bg-white/10 backdrop-blur-md border border-white/20 text-xs font-semibold text-white animate-in fade-in">
          <RotateCw className={`w-3.5 h-3.5 ${(pullDistance > 65 || isRefreshing) ? 'animate-spin text-emerald-400' : ''}`} />
          <span>{isRefreshing ? 'Refreshing radar…' : pullDistance > 65 ? 'Release to refresh radar' : 'Pull down to refresh'}</span>
        </div>
      )}

      {/* Header Text Above Cards — NO REFRESH BUTTON, Large spacing so it's far above the card stack */}
      <div className="w-full text-center space-y-1 mt-4 mb-16 sm:mb-20 pt-2 pb-2">
        <h2 className="text-base sm:text-lg font-bold text-white tracking-tight leading-snug">
          These are the people within your 30 meters.
        </h2>
        <p className="text-xs font-semibold text-white/60 tracking-wide">
          Discover nearby profiles around you
        </p>
      </div>

      {/* Card Stack Stage */}
      <div
        className="relative flex-1 flex items-center justify-center w-full"
        style={{ width: 'min(82vw,310px)', height: 'min(54vh,450px)', minHeight: 370 }}
      >
        {/* Back Cards (deck[2], deck[1]) */}
        {BEHIND.map((cfg, i) => {
          const profile = deck[i + 1];
          if (!profile) return null;
          return (
            <div
              key={profile.id}
              style={{
                position: 'absolute',
                inset: 0,
                zIndex: 10 + i,
                transform: `translate3d(${cfg.tx}px,${cfg.ty}px,0) rotate(${cfg.rot}deg) scale(${cfg.scale})`,
                transformOrigin: 'bottom left',
                pointerEvents: 'none',
                transition: 'transform 320ms cubic-bezier(0.34,1.2,0.64,1)',
              }}
            >
              <ProfileCard profile={profile} isBackCard />
            </div>
          );
        })}

        {/* Top Active Card */}
        <div
          key={deck[0].id}
          ref={topRef}
          onPointerDown={onDown}
          onPointerMove={onMove}
          onPointerUp={onUp}
          style={{
            position: 'absolute',
            inset: 0,
            zIndex: 40,
            cursor: isDragging ? 'grabbing' : 'grab',
            touchAction: 'none',
            willChange: 'transform',
          }}
        >
          <ProfileCard profile={deck[0]} />
        </div>
      </div>

      {/* Bottom Hint */}
      <div className="mt-6 flex items-center gap-1 text-[11px] text-white/30 tracking-wide font-medium">
        <ArrowDown className="w-3 h-3 text-white/20 animate-bounce" />
        <span>swipe cards or pull down to refresh</span>
      </div>
    </div>
  );
};

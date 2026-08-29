import React, { useState, useRef, useEffect } from 'react';
import type { UserProfile, SwipeDirection } from '../types';
import gsap from 'gsap';
import { ProfileCard } from './ProfileCard';

interface CardDeckProps {
  profiles: UserProfile[];
  onSwipe: (direction: SwipeDirection, profile: UserProfile) => void;
  onOpenDetails: (profile: UserProfile) => void;
}

// Back card stack visual config (index 0 = position behind top card)
const BEHIND: { tx: number; ty: number; rot: number; scale: number }[] = [
  { tx: 14, ty: 10, rot:  4.0, scale: 0.965 },
  { tx: 26, ty: 18, rot:  7.6, scale: 0.930 },
];

export const CardDeck: React.FC<CardDeckProps> = ({ profiles, onSwipe, onOpenDetails }) => {
  const [deck, setDeck]           = useState<UserProfile[]>([...profiles]);
  const [isAnimating, setIsAnimating] = useState(false);
  const [isDragging, setIsDragging]   = useState(false);

  const topRef      = useRef<HTMLDivElement>(null);
  const dragStart   = useRef<{ x: number; y: number; t: number }>({ x: 0, y: 0, t: 0 });

  useEffect(() => { setDeck([...profiles]); }, [profiles]);

  if (deck.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center gap-4 text-center py-16 px-8">
        <div className="text-5xl font-bold text-white/10 tracking-tight">k.</div>
        <p className="text-sm text-white/30 leading-relaxed max-w-xs">
          No profiles within 30m. Move around to discover people.
        </p>
      </div>
    );
  }

  // ─── send top card to bottom of deck ────────────────────────────────────────
  const sendToBottom = (direction: SwipeDirection) => {
    const top = deck[0];
    if (!top || !topRef.current) return;

    setIsAnimating(true);

    const vw = window.innerWidth;
    const flyX   = direction === 'right' ? vw * 1.35 : -vw * 1.35;
    const flyRot  = direction === 'right' ? 24 : -24;

    // --- Phase 1: fly card off screen (fast) ---
    gsap.to(topRef.current, {
      x: flyX,
      y: -60,
      rotation: flyRot,
      opacity: 0,
      duration: 0.28,
      ease: 'power2.in',
      onComplete: () => {
        // Move card to bottom of deck state.
        // React will give the NEW deck[0] a fresh DOM node (key changes),
        // so there's zero transform contamination.
        setDeck(prev => {
          const [first, ...rest] = prev;
          return [...rest, first];
        });
        setIsAnimating(false);
      },
    });

    // --- Phase 2 (concurrent): animate bottom ghost arc ---
    // A ghost clone arcs from top-card position DOWN behind the stack,
    // giving the physical "placed under the deck" feel.
    const stage = topRef.current.parentElement;
    if (stage) {
      const clone = topRef.current.cloneNode(true) as HTMLElement;
      clone.style.position   = 'absolute';
      clone.style.inset      = '0';
      clone.style.zIndex     = '5';           // behind all cards
      clone.style.pointerEvents = 'none';
      clone.style.opacity    = '1';
      stage.appendChild(clone);

      // Start at current position, arc down-and-inward to the "3rd card" position
      const { tx, ty, rot, scale } = BEHIND[1];
      gsap.fromTo(clone,
        { x: 0, y: 0, rotation: 0, scale: 1, opacity: 0.85 },
        {
          keyframes: [
            // first move off to the side quickly (following the real card slightly)
            { x: flyX * 0.3, y: 40, rotation: flyRot * 0.5, scale: 0.88, opacity: 0.6, duration: 0.18, ease: 'power1.in' },
            // then curve DOWN and land at the back-of-stack position
            { x: tx, y: ty, rotation: rot, scale, opacity: 0.75, duration: 0.30, ease: 'power2.out' },
            // settle / fade slightly
            { opacity: 0.0, duration: 0.12, ease: 'none' },
          ],
          onComplete: () => clone.remove(),
        }
      );
    }

    onSwipe(direction, top);
  };

  // ─── pointer handlers ────────────────────────────────────────────────────────
  const onDown = (e: React.PointerEvent) => {
    if (isAnimating) return;
    dragStart.current = { x: e.clientX, y: e.clientY, t: Date.now() };
    setIsDragging(true);
    topRef.current?.setPointerCapture(e.pointerId);
  };

  const onMove = (e: React.PointerEvent) => {
    if (!isDragging || isAnimating || !topRef.current) return;
    const dx = e.clientX - dragStart.current.x;
    const dy = e.clientY - dragStart.current.y;
    const rot = dx * 0.05;
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
      if (topRef.current) { topRef.current.style.transform = ''; topRef.current.style.transition = ''; }
      onOpenDetails(deck[0]);
      return;
    }

    const triggered = elapsed < 320 ? Math.abs(dx) > 30 : Math.abs(dx) > 75;
    if (triggered) {
      sendToBottom(dx > 0 ? 'right' : 'left');
    } else {
      // snap back
      gsap.to(topRef.current!, {
        x: 0, y: 0, rotation: 0,
        duration: 0.38, ease: 'back.out(1.7)',
        clearProps: 'transform',
      });
    }
  };

  // ─── render ──────────────────────────────────────────────────────────────────
  return (
    <div className="relative flex flex-col items-center justify-center w-full select-none">
      <div
        className="relative"
        style={{ width: 'min(78vw,300px)', height: 'min(54vh,440px)', minHeight: 360 }}
      >
        {/* Back cards — deck[2], deck[1] rendered bottom→up */}
        {BEHIND.map((cfg, i) => {
          const profile = deck[i + 1];
          if (!profile) return null;
          return (
            <div
              key={profile.id}
              style={{
                position: 'absolute', inset: 0,
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

        {/* Top active card — key = id so React creates a FRESH DOM node on swap */}
        <div
          key={deck[0].id}
          ref={topRef}
          onPointerDown={onDown}
          onPointerMove={onMove}
          onPointerUp={onUp}
          style={{
            position: 'absolute', inset: 0,
            zIndex: 40,
            cursor: isDragging ? 'grabbing' : 'grab',
            touchAction: 'none',
            willChange: 'transform',
          }}
        >
          <ProfileCard profile={deck[0]} />
        </div>
      </div>

      <p className="mt-6 text-[11px] text-white/18 tracking-wide">
        swipe or tap to explore
      </p>
    </div>
  );
};

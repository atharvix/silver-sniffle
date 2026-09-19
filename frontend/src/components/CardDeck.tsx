import React, { useState, useRef, useEffect } from 'react';
import type { UserProfile, SwipeDirection } from '../types';
import { ProfileCard } from './ProfileCard';
import { resolvePhotoUrl } from '../utils/api';
import { Users } from 'lucide-react';

interface CardDeckProps {
  profiles: UserProfile[];
  isLoading?: boolean;
  onSwipe: (direction: SwipeDirection, profile: UserProfile) => void;
  onOpenDetails: (profile: UserProfile) => void;
  onRefresh?: () => void;
  onLoadDemoCards?: () => void;
}

// Behind stack positions
const BEHIND: { tx: number; ty: number; rot: number; scale: number }[] = [
  { tx: 10, ty: 8, rot: 3, scale: 0.96 },
  { tx: 18, ty: 16, rot: 6, scale: 0.92 },
];

export const CardDeck: React.FC<CardDeckProps> = ({
  profiles,
  isLoading = false,
  onSwipe,
  onOpenDetails,
  onRefresh: _onRefresh,
  onLoadDemoCards,
}) => {
  const [deck, setDeck] = useState<UserProfile[]>(() => [...profiles]);
  const [dragOffset, setDragOffset] = useState<{ x: number; y: number }>({ x: 0, y: 0 });
  const [isDragging, setIsDragging] = useState(false);
  const [isSwipingOut, setIsSwipingOut] = useState(false);
  const [swipeDirection, setSwipeDirection] = useState<SwipeDirection | null>(null);
  const [pullDistance, setPullDistance] = useState(0);
  const [isLocalRefreshing, setIsLocalRefreshing] = useState(false);

  const dragStart = useRef<{ x: number; y: number; t: number }>({ x: 0, y: 0, t: 0 });
  const isPullingRef = useRef(false);

  // Sync incoming profiles from props
  useEffect(() => {
    if (profiles.length === 0) {
      setDeck([]);
      return;
    }
    setDeck((prevDeck) => {
      if (prevDeck.length === 0) return [...profiles];

      const incomingMap = new Map(profiles.map((p) => [p.id, p]));
      const updatedDeck = prevDeck
        .filter((p) => incomingMap.has(p.id))
        .map((p) => ({ ...p, ...incomingMap.get(p.id)! }));

      const existingIds = new Set(prevDeck.map((p) => p.id));
      const brandNewProfiles = profiles.filter((p) => !existingIds.has(p.id));

      if (updatedDeck.length === 0) return [...profiles];
      return [...updatedDeck, ...brandNewProfiles];
    });
  }, [profiles]);

  // Preload upcoming avatars
  const topCardId = deck[0]?.id;
  useEffect(() => {
    if (!topCardId) return;
    deck.slice(0, 3).forEach((p) => {
      const url = resolvePhotoUrl(p.avatar);
      if (url && url.startsWith('http')) {
        const img = new Image();
        img.src = url;
      }
    });
  }, [topCardId, deck]);

  // ─── Gesture Handlers for Cards & Pull-Down ──────────────────────────────
  const handlePointerDown = (e: React.PointerEvent) => {
    if (isSwipingOut || isLocalRefreshing) return;
    dragStart.current = { x: e.clientX, y: e.clientY, t: Date.now() };
    setIsDragging(true);
    isPullingRef.current = false;
    try {
      e.currentTarget.setPointerCapture(e.pointerId);
    } catch {}
  };

  const handlePointerMove = (e: React.PointerEvent) => {
    if (!isDragging || isSwipingOut || isLocalRefreshing) return;
    const dx = e.clientX - dragStart.current.x;
    const dy = e.clientY - dragStart.current.y;

    if (dy > 12 && dy > Math.abs(dx) * 1.1) {
      isPullingRef.current = true;
      const damped = Math.min(dy * 0.45, 90);
      setPullDistance(damped);
      setDragOffset({ x: dx * 0.1, y: damped });
    } else if (!isPullingRef.current && deck.length > 0) {
      setPullDistance(0);
      setDragOffset({ x: dx, y: dy * 0.2 });
    }
  };

  const executeSwipe = (direction: SwipeDirection) => {
    if (isSwipingOut || deck.length === 0) return;
    const topCard = deck[0];
    setIsSwipingOut(true);
    setSwipeDirection(direction);

    // After CSS transition finishes (240ms), rotate deck state immediately
    setTimeout(() => {
      onSwipe(direction, topCard);
      setDeck((prev) => {
        if (prev.length <= 1) return prev;
        const [first, ...rest] = prev;
        return [...rest, first];
      });
      setDragOffset({ x: 0, y: 0 });
      setIsSwipingOut(false);
      setSwipeDirection(null);
    }, 240);
  };

  const handlePointerUp = (e: React.PointerEvent) => {
    if (!isDragging) return;
    setIsDragging(false);
    try {
      if (e.currentTarget.hasPointerCapture(e.pointerId)) {
        e.currentTarget.releasePointerCapture(e.pointerId);
      }
    } catch {}

    const dx = e.clientX - dragStart.current.x;
    const dy = e.clientY - dragStart.current.y;
    const elapsed = Date.now() - dragStart.current.t;

    // Case 1: Pull-down gesture released -> trigger blur loader refresh
    if (isPullingRef.current) {
      isPullingRef.current = false;
      const shouldRefresh = pullDistance > 35 || dy > 45;
      setPullDistance(0);
      setDragOffset({ x: 0, y: 0 });
      if (shouldRefresh) {
        setIsLocalRefreshing(true);
        if (_onRefresh) _onRefresh();
        setTimeout(() => setIsLocalRefreshing(false), 900);
      }
      return;
    }

    // Case 2: Tap on card to open details
    const isTap = Math.abs(dx) < 8 && Math.abs(dy) < 8 && elapsed < 260;
    if (isTap && deck.length > 0) {
      onOpenDetails(deck[0]);
      setDragOffset({ x: 0, y: 0 });
      return;
    }

    // Case 3: Horizontal card swipe
    const velocity = Math.abs(dx) / Math.max(elapsed, 1);
    const isHorizontal = Math.abs(dx) > Math.abs(dy) * 0.7;
    const shouldSwipe = isHorizontal && (Math.abs(dx) > 45 || velocity > 0.2);

    if (shouldSwipe && deck.length > 0) {
      executeSwipe(dx > 0 ? 'right' : 'left');
    } else {
      setDragOffset({ x: 0, y: 0 });
    }
  };

  const handlePointerCancel = (e: React.PointerEvent) => {
    setIsDragging(false);
    isPullingRef.current = false;
    try {
      if (e.currentTarget.hasPointerCapture(e.pointerId)) {
        e.currentTarget.releasePointerCapture(e.pointerId);
      }
    } catch {}
    setPullDistance(0);
    setDragOffset({ x: 0, y: 0 });
  };

  // ─── Empty State Screen ──────────────────────────────────────────
  if (deck.length === 0) {
    return (
      <div
        onPointerDown={handlePointerDown}
        onPointerMove={handlePointerMove}
        onPointerUp={handlePointerUp}
        onPointerCancel={handlePointerCancel}
        className="flex-1 flex flex-col items-center justify-center text-center px-6 py-12 select-none max-w-sm mx-auto h-full relative touch-none"
        style={{
          transform: pullDistance > 0 ? `translate3d(0, ${pullDistance * 0.6}px, 0)` : undefined,
          transition: isDragging ? 'none' : 'transform 260ms cubic-bezier(0.25, 1, 0.5, 1)',
        }}
      >
        <div className="w-20 h-20 rounded-[24px] bg-black/50 backdrop-blur-xl border border-white/15 flex items-center justify-center mb-6 shadow-2xl">
          <Users className="w-9 h-9 text-white/90" strokeWidth={1.8} />
        </div>

        <h2 className="text-xl font-bold text-white tracking-tight leading-snug">
          Who’s within 30m?
        </h2>

        <p className="text-sm font-medium text-white/70 mt-2">
          No profiles discovered within 30 meters right now.
        </p>

        <p className="text-xs text-white/40 mt-1.5 leading-relaxed mb-6">
          Walk around to discover people nearby!
        </p>

        {onLoadDemoCards && (
          <button
            onClick={onLoadDemoCards}
            className="light-theme-cta flex items-center gap-2 px-5 py-2.5 rounded-full bg-white/15 hover:bg-white/25 active:scale-95 border border-white/20 text-xs font-bold text-white transition-all shadow-lg"
          >
            <span>⚡ Load 10 Demo Cards</span>
          </button>
        )}

        {/* Full-Screen Blur Overlay with Centered Loader */}
        {(isLoading || isLocalRefreshing) && (
          <div className="card-deck-refresh-overlay fixed inset-0 z-50 bg-black/40 backdrop-blur-md flex flex-col items-center justify-center pointer-events-auto animate-in fade-in duration-200">
            <div className="relative flex items-center justify-center">
              <div className="w-16 h-16 rounded-full bg-white/5 animate-ping absolute pointer-events-none" />
              <div className="refresh-spinner w-12 h-12 rounded-full border-2 border-white/20 border-t-white animate-spin shadow-2xl" />
            </div>
            <p className="text-xs font-semibold text-white/80 tracking-widest uppercase mt-4 animate-pulse">
              Discovering nearby…
            </p>
          </div>
        )}
      </div>
    );
  }

  // Active top card transform computation
  const windowWidth = typeof window !== 'undefined' ? window.innerWidth : 360;
  let topTransform = `translate3d(${dragOffset.x}px, ${dragOffset.y}px, 0) rotate(${dragOffset.x * 0.05}deg)`;

  if (isSwipingOut && swipeDirection) {
    const exitX = swipeDirection === 'right' ? windowWidth * 1.2 : -windowWidth * 1.2;
    const exitRot = swipeDirection === 'right' ? 25 : -25;
    topTransform = `translate3d(${exitX}px, ${dragOffset.y}px, 0) rotate(${exitRot}deg)`;
  }

  return (
    <div
      onPointerDown={handlePointerDown}
      onPointerMove={handlePointerMove}
      onPointerUp={handlePointerUp}
      onPointerCancel={handlePointerCancel}
      className="relative flex flex-col items-center justify-center w-full h-full select-none max-w-md mx-auto px-4 py-2 flex-1 my-auto touch-none"
      style={{
        transform: pullDistance > 0 ? `translate3d(0, ${pullDistance * 0.6}px, 0)` : undefined,
        transition: isDragging ? 'none' : 'transform 260ms cubic-bezier(0.25, 1, 0.5, 1)',
      }}
    >
      {/* Full-Screen Blur Overlay with Centered Loader */}
      {(isLoading || isLocalRefreshing) && (
        <div className="card-deck-refresh-overlay fixed inset-0 z-50 bg-black/40 backdrop-blur-md flex flex-col items-center justify-center pointer-events-auto animate-in fade-in duration-200">
          <div className="relative flex items-center justify-center">
            <div className="w-16 h-16 rounded-full bg-white/5 animate-ping absolute pointer-events-none" />
            <div className="refresh-spinner w-12 h-12 rounded-full border-2 border-white/20 border-t-white animate-spin shadow-2xl" />
          </div>
          <p className="text-xs font-semibold text-white/80 tracking-widest uppercase mt-4 animate-pulse">
            Discovering nearby…
          </p>
        </div>
      )}
      {/* Header Text Above Cards */}
      <div className="w-full text-center space-y-1 mb-3 shrink-0">
        <h2 className="text-lg sm:text-xl font-bold text-white tracking-tight leading-snug">
          People within 30 meters
        </h2>
        <p className="text-xs font-semibold text-white/60 tracking-wide">
          Swipe left or right to explore
        </p>
      </div>

      {/* Card Stack Stage */}
      <div className="flex-1 flex items-center justify-center w-full my-auto shrink-0">
        <div
          className="relative flex items-center justify-center"
          style={{ width: 'min(92vw, 360px)', height: 'min(70vh, 550px)', minHeight: 440 }}
        >
          {/* Back Cards Stack */}
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
                  transition: isDragging ? 'none' : 'transform 240ms ease-out',
                }}
              >
                <ProfileCard profile={profile} isBackCard />
              </div>
            );
          })}

          {/* Top Active Card */}
          <div
            key={deck[0].id}
            style={{
              position: 'absolute',
              inset: 0,
              zIndex: 40,
              cursor: isDragging ? 'grabbing' : 'grab',
              willChange: 'transform',
              transform: topTransform,
              opacity: isSwipingOut ? 0.6 : 1,
              transition: isDragging ? 'none' : 'transform 240ms cubic-bezier(0.25, 1, 0.5, 1), opacity 240ms ease-out',
            }}
          >
            <ProfileCard profile={deck[0]} />
          </div>
        </div>
      </div>
    </div>
  );
};

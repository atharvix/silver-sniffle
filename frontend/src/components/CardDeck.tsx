import React, { useState, useRef, useEffect } from 'react';
import type { UserProfile, SwipeDirection } from '../types';
import { ProfileCard } from './ProfileCard';
import { SkeletonCard } from './SkeletonCard';
import { resolvePhotoUrl } from '../utils/api';
import { RotateCw, Users, Radio } from 'lucide-react';

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
  onRefresh,
  onLoadDemoCards,
}) => {
  const [deck, setDeck] = useState<UserProfile[]>(() => [...profiles]);
  const [dragOffset, setDragOffset] = useState<{ x: number; y: number }>({ x: 0, y: 0 });
  const [isDragging, setIsDragging] = useState(false);
  const [isSwipingOut, setIsSwipingOut] = useState(false);
  const [swipeDirection, setSwipeDirection] = useState<SwipeDirection | null>(null);
  const [pullDistance, setPullDistance] = useState(0);
  const [isRefreshing, setIsRefreshing] = useState(false);

  const dragStart = useRef<{ x: number; y: number; t: number }>({ x: 0, y: 0, t: 0 });

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
  useEffect(() => {
    if (deck.length === 0) return;
    deck.slice(0, 3).forEach((p) => {
      const url = resolvePhotoUrl(p.avatar);
      if (url && url.startsWith('http')) {
        const img = new Image();
        img.src = url;
      }
    });
  }, [deck[0]?.id]);

  const handleTriggerRefresh = () => {
    setIsRefreshing(true);
    if (onRefresh) onRefresh();
    setTimeout(() => {
      setIsRefreshing(false);
      setPullDistance(0);
      setDragOffset({ x: 0, y: 0 });
    }, 600);
  };

  // ─── Gesture Handlers ───────────────────────────────────────────────
  const handlePointerDown = (e: React.PointerEvent) => {
    if (isSwipingOut || deck.length === 0) return;
    dragStart.current = { x: e.clientX, y: e.clientY, t: Date.now() };
    setIsDragging(true);
    try {
      e.currentTarget.setPointerCapture(e.pointerId);
    } catch {}
  };

  const handlePointerMove = (e: React.PointerEvent) => {
    if (!isDragging || isSwipingOut) return;
    const dx = e.clientX - dragStart.current.x;
    const dy = e.clientY - dragStart.current.y;

    if (dy > 10 && Math.abs(dx) < Math.abs(dy) * 1.5) {
      setPullDistance(Math.min(dy, 110));
      setDragOffset({ x: dx * 0.3, y: dy * 0.25 });
    } else {
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
    const isTap = Math.abs(dx) < 8 && Math.abs(dy) < 8 && elapsed < 260;

    if (isTap && deck.length > 0) {
      onOpenDetails(deck[0]);
      setPullDistance(0);
      setDragOffset({ x: 0, y: 0 });
      return;
    }

    if (dy > 55 && Math.abs(dx) < 70) {
      handleTriggerRefresh();
      return;
    }

    setPullDistance(0);

    const velocity = Math.abs(dx) / Math.max(elapsed, 1);
    const isHorizontal = Math.abs(dx) > Math.abs(dy) * 0.7;
    const shouldSwipe = isHorizontal && (Math.abs(dx) > 45 || velocity > 0.2);

    if (shouldSwipe) {
      executeSwipe(dx > 0 ? 'right' : 'left');
    } else {
      setDragOffset({ x: 0, y: 0 });
    }
  };

  const handlePointerCancel = (e: React.PointerEvent) => {
    setIsDragging(false);
    try {
      if (e.currentTarget.hasPointerCapture(e.pointerId)) {
        e.currentTarget.releasePointerCapture(e.pointerId);
      }
    } catch {}
    setDragOffset({ x: 0, y: 0 });
  };

  // ─── Loading Skeleton Screen ──────────────────────────────────────────────────
  if (isLoading) {
    return (
      <div className="flex-1 flex flex-col items-center justify-center w-full h-full select-none max-w-md mx-auto px-4 py-2">
        <div className="w-full text-center space-y-1 mb-3 shrink-0">
          <h2 className="text-lg sm:text-xl font-bold text-white tracking-tight leading-snug flex items-center justify-center gap-2">
            <Radio className="w-5 h-5 text-emerald-400 animate-spin" />
            Discovering nearby cards…
          </h2>
          <p className="text-xs font-semibold text-white/60 tracking-wide">
            Locating nearby profiles
          </p>
        </div>
        <div className="flex-1 flex items-center justify-center w-full my-auto shrink-0">
          <SkeletonCard />
        </div>
      </div>
    );
  }

  // ─── Empty State Screen ──────────────────────────────────────────────────────
  if (deck.length === 0) {
    return (
      <div className="flex-1 flex flex-col items-center justify-center text-center px-6 py-12 select-none max-w-sm mx-auto h-full relative">
        {(pullDistance > 15 || isRefreshing) && (
          <div className="absolute top-2 left-1/2 -translate-x-1/2 z-40 flex items-center gap-2 px-4 py-1.5 rounded-full bg-white/10 backdrop-blur-md border border-white/20 text-xs font-semibold text-white animate-in fade-in">
            <RotateCw className={`w-3.5 h-3.5 ${pullDistance > 55 || isRefreshing ? 'animate-spin text-emerald-400' : ''}`} />
            <span>{isRefreshing ? 'Refreshing nearby cards…' : pullDistance > 55 ? 'Release to refresh' : 'Pull down to refresh'}</span>
          </div>
        )}

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
          Walk around to discover people physically nearby in your 30m radius!
        </p>

        {onLoadDemoCards && (
          <button
            onClick={onLoadDemoCards}
            className="flex items-center gap-2 px-5 py-2.5 rounded-full bg-white/15 hover:bg-white/25 active:scale-95 border border-white/20 text-xs font-bold text-white transition-all shadow-lg"
          >
            <span>⚡ Load 10 Demo Cards (UI Redesign)</span>
          </button>
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
    <div className="relative flex flex-col items-center justify-center w-full h-full select-none max-w-md mx-auto px-4 py-2 flex-1 my-auto">
      {/* Pull-to-Refresh Indicator Ring */}
      {(pullDistance > 15 || isRefreshing) && (
        <div className="absolute top-2 left-1/2 -translate-x-1/2 z-50 flex items-center gap-2 px-4 py-1.5 rounded-full bg-white/10 backdrop-blur-md border border-white/20 text-xs font-semibold text-white animate-in fade-in shadow-xl">
          <RotateCw className={`w-3.5 h-3.5 ${pullDistance > 55 || isRefreshing ? 'animate-spin text-emerald-400' : ''}`} />
          <span>{isRefreshing ? 'Refreshing discovery…' : pullDistance > 55 ? 'Release to refresh' : 'Pull down to refresh'}</span>
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
            onPointerDown={handlePointerDown}
            onPointerMove={handlePointerMove}
            onPointerUp={handlePointerUp}
            onPointerCancel={handlePointerCancel}
            style={{
              position: 'absolute',
              inset: 0,
              zIndex: 40,
              cursor: isDragging ? 'grabbing' : 'grab',
              touchAction: 'none',
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

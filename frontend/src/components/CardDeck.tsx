import React, { useState, useRef } from 'react';
import type { UserProfile, SwipeDirection } from '../types';
import { ProfileCard } from './ProfileCard';
import { RotateCcw } from 'lucide-react';
import gsap from 'gsap';

interface CardDeckProps {
  profiles: UserProfile[];
  onSwipe: (direction: SwipeDirection, profile: UserProfile) => void;
  onUndo: () => void;
  onOpenDetails: (profile: UserProfile) => void;
  canUndo: boolean;
}

export const CardDeck: React.FC<CardDeckProps> = ({
  profiles,
  onSwipe,
  onUndo,
  onOpenDetails,
  canUndo,
}) => {
  const [currentIndex, setCurrentIndex] = useState(0);
  const [dragOffset, setDragOffset] = useState({ x: 0, y: 0 });
  const [isDragging, setIsDragging] = useState(false);
  const topCardRef = useRef<HTMLDivElement>(null);
  const dragStartRef = useRef<{ x: number; y: number }>({ x: 0, y: 0 });

  const currentProfile = profiles[currentIndex];
  // Slice up to 5 visible cards for multi-card peeking stack effect
  const visibleProfiles = profiles.slice(currentIndex, currentIndex + 5);

  const handlePointerDown = (e: React.PointerEvent) => {
    dragStartRef.current = { x: e.clientX, y: e.clientY };
    setIsDragging(true);
  };

  const handlePointerMove = (e: React.PointerEvent) => {
    if (!isDragging) return;
    const deltaX = e.clientX - dragStartRef.current.x;
    const deltaY = e.clientY - dragStartRef.current.y;
    setDragOffset({ x: deltaX, y: deltaY });

    if (topCardRef.current) {
      const rotation = deltaX * 0.07;
      topCardRef.current.style.transform = `translate3d(${deltaX}px, ${deltaY}px, 0px) rotate(${rotation}deg)`;
    }
  };

  const handlePointerUp = () => {
    if (!isDragging) return;
    setIsDragging(false);

    const threshold = 110;
    if (dragOffset.x > threshold) {
      triggerSwipe('right');
    } else if (dragOffset.x < -threshold) {
      triggerSwipe('left');
    } else {
      if (topCardRef.current) {
        gsap.to(topCardRef.current, {
          x: 0,
          y: 0,
          rotation: 0,
          duration: 0.35,
          ease: 'back.out(1.4)',
        });
      }
      setDragOffset({ x: 0, y: 0 });
    }
  };

  const triggerSwipe = (direction: SwipeDirection) => {
    if (!currentProfile || !topCardRef.current) return;

    const flyX = direction === 'right' ? window.innerWidth * 1.1 : -window.innerWidth * 1.1;
    const flyRotation = direction === 'right' ? 30 : -30;

    gsap.to(topCardRef.current, {
      x: flyX,
      y: dragOffset.y * 1.4,
      rotation: flyRotation,
      opacity: 0,
      duration: 0.4,
      ease: 'power2.in',
      onComplete: () => {
        onSwipe(direction, currentProfile);
        setCurrentIndex((prev) => prev + 1);
        setDragOffset({ x: 0, y: 0 });
        if (topCardRef.current) {
          gsap.set(topCardRef.current, { x: 0, y: 0, rotation: 0, opacity: 1 });
        }
      },
    });
  };

  if (!currentProfile) {
    return (
      <div className="relative w-full max-w-sm h-[460px] md:h-[480px] -translate-y-6 md:-translate-y-8 mx-auto bg-[#18181b] border border-white/10 rounded-[36px] p-8 flex flex-col items-center justify-center text-center gap-4 card-shadow">
        <div className="w-14 h-14 rounded-full bg-white/10 text-white flex items-center justify-center text-xl font-bold">
          k.
        </div>
        <h3 className="text-xl font-bold text-white">All Cards Reviewed</h3>
        <p className="text-xs text-neutral-400 max-w-xs">
          You have seen all nearby cards in your area.
        </p>
        <button
          onClick={onUndo}
          disabled={!canUndo}
          className="mt-2 flex items-center gap-2 px-5 py-2.5 rounded-full bg-white/10 hover:bg-white/20 text-white text-xs font-semibold border border-white/15 disabled:opacity-40 transition-all"
        >
          <RotateCcw className="w-4 h-4" />
          <span>Rewind Stack</span>
        </button>
      </div>
    );
  }

  const rightOpacity = Math.min(1, Math.max(0, dragOffset.x / 90));
  const leftOpacity = Math.min(1, Math.max(0, -dragOffset.x / 90));

  return (
    <div className="relative w-full max-w-[290px] sm:max-w-[310px] md:max-w-[330px] h-[460px] md:h-[480px] -translate-y-6 md:-translate-y-8 mx-auto -translate-x-6 sm:-translate-x-8 md:-translate-x-10 flex items-center justify-center">
      {/* Render Back Cards in Stack Peeking Out (up to 5 cards deep with photos visible) */}
      {visibleProfiles.slice(1).map((profile, idx) => {
        const stackDepth = idx + 1; // 1 to 4
        const rotation = stackDepth * 6.5; // 6.5deg, 13deg, 19.5deg, 26deg
        const translateX = stackDepth * 30; // 30px, 60px, 90px, 120px
        const translateY = stackDepth * 1.5;
        const scale = 1 - stackDepth * 0.01;
        const zIndex = 40 - stackDepth * 5;

        return (
          <div
            key={profile.id}
            style={{
              transform: `rotate(${rotation}deg) translate3d(${translateX}px, ${translateY}px, 0px) scale(${scale})`,
              zIndex,
            }}
            className="absolute inset-0 transition-all duration-300 pointer-events-none"
          >
            <ProfileCard profile={profile} isBackCard />
          </div>
        );
      })}

      {/* Top Active Interactive Card */}
      <div
        ref={topCardRef}
        onPointerDown={handlePointerDown}
        onPointerMove={handlePointerMove}
        onPointerUp={handlePointerUp}
        onClick={() => {
          if (Math.abs(dragOffset.x) < 5 && Math.abs(dragOffset.y) < 5) {
            onOpenDetails(currentProfile);
          }
        }}
        className="relative z-50 w-full h-full touch-none cursor-grab active:cursor-grabbing"
      >
        <ProfileCard profile={currentProfile} />

        {/* Swipe Right Overlay */}
        <div
          style={{ opacity: rightOpacity }}
          className="absolute top-6 left-6 z-20 border-4 border-emerald-500 text-emerald-600 font-black text-xl px-4 py-1 rounded-2xl -rotate-12 bg-white/90 backdrop-blur-md pointer-events-none transition-opacity duration-75"
        >
          CONNECT 💚
        </div>

        {/* Swipe Left Overlay */}
        <div
          style={{ opacity: leftOpacity }}
          className="absolute top-6 right-6 z-20 border-4 border-rose-500 text-rose-600 font-black text-xl px-4 py-1 rounded-2xl rotate-12 bg-white/90 backdrop-blur-md pointer-events-none transition-opacity duration-75"
        >
          PASS ❌
        </div>
      </div>
    </div>
  );
};

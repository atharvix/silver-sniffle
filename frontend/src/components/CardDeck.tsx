import React, { useState, useRef } from 'react';
import type { UserProfile } from '../types';
import type { SwipeDirection } from '../types';
import gsap from 'gsap';
import { ProfileCard } from './ProfileCard';

interface CardDeckProps {
  profiles: UserProfile[];
  onSwipe: (direction: SwipeDirection, profile: UserProfile) => void;
  onOpenDetails: (profile: UserProfile) => void;
}

export const CardDeck: React.FC<CardDeckProps> = ({
  profiles,
  onSwipe,
  onOpenDetails,
}) => {
  const [currentIndex, setCurrentIndex] = useState(0);
  const [isDragging, setIsDragging] = useState(false);
  const topCardRef = useRef<HTMLDivElement>(null);
  const dragStartRef = useRef<{ x: number; y: number; time: number }>({ x: 0, y: 0, time: 0 });

  if (!profiles || profiles.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center gap-4 text-center py-16 px-8">
        <div className="text-5xl font-bold text-white/10 tracking-tight">k.</div>
        <p className="text-sm text-white/30 font-normal leading-relaxed max-w-xs">
          No active profiles within 30 meters. Move around to discover people nearby.
        </p>
      </div>
    );
  }

  const activeIndex = currentIndex % profiles.length;
  const currentProfile = profiles[activeIndex];

  // Show up to 3 cards in stack
  const stackProfiles = [
    profiles[activeIndex],
    profiles[(activeIndex + 1) % profiles.length],
    profiles[(activeIndex + 2) % profiles.length],
  ];

  const handlePointerDown = (e: React.PointerEvent) => {
    dragStartRef.current = { x: e.clientX, y: e.clientY, time: Date.now() };
    setIsDragging(true);
    if (topCardRef.current) {
      topCardRef.current.setPointerCapture(e.pointerId);
    }
  };

  const handlePointerMove = (e: React.PointerEvent) => {
    if (!isDragging) return;
    const deltaX = e.clientX - dragStartRef.current.x;
    const deltaY = e.clientY - dragStartRef.current.y;

    if (topCardRef.current) {
      const rotation = deltaX * 0.05;
      topCardRef.current.style.transform = `translate3d(${deltaX}px, ${deltaY * 0.4}px, 0) rotate(${rotation}deg)`;
      topCardRef.current.style.transition = 'none';
    }
  };

  const handlePointerUp = (e: React.PointerEvent) => {
    if (!isDragging) return;
    setIsDragging(false);

    const deltaX = e.clientX - dragStartRef.current.x;
    const deltaY = e.clientY - dragStartRef.current.y;
    const elapsed = Date.now() - dragStartRef.current.time;
    const isQuickSwipe = elapsed < 300 && Math.abs(deltaX) > 40;
    const isSlowSwipe = Math.abs(deltaX) > 90;
    const isTap = Math.abs(deltaX) < 8 && Math.abs(deltaY) < 8 && elapsed < 300;

    if (isTap) {
      // Reset and open details
      if (topCardRef.current) {
        topCardRef.current.style.transform = '';
        topCardRef.current.style.transition = '';
      }
      onOpenDetails(currentProfile);
      return;
    }

    if (isQuickSwipe || isSlowSwipe) {
      const direction: SwipeDirection = deltaX > 0 ? 'right' : 'left';
      triggerSwipe(direction, deltaY);
    } else {
      // Snap back
      if (topCardRef.current) {
        gsap.to(topCardRef.current, {
          x: 0,
          y: 0,
          rotation: 0,
          duration: 0.4,
          ease: 'back.out(1.6)',
          clearProps: 'transform',
        });
      }
    }
  };

  const triggerSwipe = (direction: SwipeDirection, yOffset = 0) => {
    if (!currentProfile || !topCardRef.current) return;

    const flyX = direction === 'right' ? window.innerWidth * 1.2 : -window.innerWidth * 1.2;
    const flyRotation = direction === 'right' ? 20 : -20;

    gsap.to(topCardRef.current, {
      x: flyX,
      y: yOffset * 0.5 - 40,
      rotation: flyRotation,
      opacity: 0,
      duration: 0.32,
      ease: 'power2.in',
      onComplete: () => {
        // Both directions just cycle the card to the bottom of the deck
        onSwipe(direction, currentProfile);
        setCurrentIndex((prev) => prev + 1);

        if (topCardRef.current) {
          gsap.set(topCardRef.current, {
            x: 0, y: 0, rotation: 0, opacity: 1, clearProps: 'transform,opacity',
          });
          topCardRef.current.style.transform = '';
          topCardRef.current.style.transition = '';
        }
      },
    });
  };

  return (
    <div className="relative flex flex-col items-center justify-center w-full select-none">
      {/* Card Stage */}
      <div
        className="relative"
        style={{
          width: 'min(78vw, 300px)',
          height: 'min(54vh, 440px)',
          minHeight: '360px',
        }}
      >
        {/* Back cards in stack (right-shifted, slightly rotated) */}
        {stackProfiles.slice(1).map((profile, idx) => {
          const stackIndex = idx + 1;
          const tx = stackIndex * 18;
          const ty = stackIndex * 6;
          const rot = stackIndex * 4.2;
          const scale = 1 - stackIndex * 0.03;
          const z = 30 - stackIndex * 10;
          return (
            <div
              key={`stack-${profile.id}-${idx}`}
              style={{
                position: 'absolute',
                inset: 0,
                transform: `translate3d(${tx}px, ${ty}px, 0) rotate(${rot}deg) scale(${scale})`,
                zIndex: z,
                transformOrigin: 'bottom left',
                pointerEvents: 'none',
              }}
            >
              <ProfileCard profile={profile} isBackCard />
            </div>
          );
        })}

        {/* Top Active Card */}
        <div
          ref={topCardRef}
          onPointerDown={handlePointerDown}
          onPointerMove={handlePointerMove}
          onPointerUp={handlePointerUp}
          style={{
            position: 'absolute',
            inset: 0,
            zIndex: 40,
            cursor: isDragging ? 'grabbing' : 'grab',
            touchAction: 'none',
            willChange: 'transform',
          }}
        >
          <ProfileCard profile={currentProfile} />
        </div>
      </div>

      {/* Subtle swipe hint */}
      <p className="mt-6 text-[11px] text-white/20 font-normal tracking-wide">
        swipe or tap to explore
      </p>
    </div>
  );
};

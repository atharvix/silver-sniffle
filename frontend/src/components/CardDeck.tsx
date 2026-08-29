import React, { useState, useRef } from 'react';
import type { UserProfile, SwipeDirection } from '../types';
import { ProfileCard } from './ProfileCard';
import gsap from 'gsap';

interface CardDeckProps {
  profiles: UserProfile[];
  onSwipe: (direction: SwipeDirection, profile: UserProfile) => void;
  onUndo?: () => void;
  onOpenDetails: (profile: UserProfile) => void;
  canUndo?: boolean;
}

export const CardDeck: React.FC<CardDeckProps> = ({
  profiles,
  onSwipe,
  onOpenDetails,
}) => {
  const [currentIndex, setCurrentIndex] = useState(0);
  const [dragOffset, setDragOffset] = useState({ x: 0, y: 0 });
  const [isDragging, setIsDragging] = useState(false);
  const topCardRef = useRef<HTMLDivElement>(null);
  const dragStartRef = useRef<{ x: number; y: number }>({ x: 0, y: 0 });

  if (!profiles || profiles.length === 0) {
    return (
      <div className="relative w-full max-w-[300px] sm:max-w-sm h-[440px] sm:h-[480px] mx-auto glass-panel rounded-[36px] p-6 sm:p-8 flex flex-col items-center justify-center text-center gap-4 shadow-2xl">
        <div className="w-14 h-14 rounded-full bg-white/10 text-white border border-white/20 flex items-center justify-center text-2xl font-black font-sans shadow-lg">
          k<span className="text-white/40">.</span>
        </div>
        <h3 className="text-lg sm:text-xl font-bold text-white">No Nearby Profiles</h3>
        <p className="text-xs text-neutral-400 max-w-xs leading-relaxed">
          There are currently no active profiles within 30 meters. Move around to discover people nearby.
        </p>
      </div>
    );
  }

  // Safe index handling (Infinite Loop Cycling)
  const activeIndex = currentIndex % profiles.length;
  const currentProfile = profiles[activeIndex];

  // Top 3 profiles for right-tilted stack effect
  const peekStackProfiles = [
    profiles[activeIndex],
    profiles[(activeIndex + 1) % profiles.length],
    profiles[(activeIndex + 2) % profiles.length],
  ];

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
      const rotation = deltaX * 0.06;
      topCardRef.current.style.transform = `translate3d(${deltaX}px, ${deltaY}px, 0px) rotate(${rotation}deg)`;
    }
  };

  const handlePointerUp = () => {
    if (!isDragging) return;
    setIsDragging(false);

    const threshold = 80;
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
    const flyRotation = direction === 'right' ? 25 : -25;

    gsap.to(topCardRef.current, {
      x: flyX,
      y: dragOffset.y * 1.2,
      rotation: flyRotation,
      opacity: 0,
      duration: 0.35,
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

  return (
    <div className="card-stage relative w-[min(80vw,310px)] h-[min(58vh,460px)] min-h-[380px] mx-auto flex items-center justify-center">
      {/* Background cards shifted to the right peeking top-right */}
      {peekStackProfiles.slice(1).map((profile, idx) => {
        const stackIndex = idx + 1;
        const translateX = stackIndex * 20;
        const translateY = stackIndex * 4;
        const rotation = stackIndex * 3.8;
        const scale = 1 - stackIndex * 0.025;
        const zIndex = 30 - stackIndex * 5;

        return (
          <div
            key={`${profile.id}_stack_${idx}`}
            style={{
              transform: `translate3d(${translateX}px, ${translateY}px, 0px) rotate(${rotation}deg) scale(${scale})`,
              zIndex,
            }}
            className="absolute inset-0 transition-all duration-300 pointer-events-none origin-bottom-left"
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
        onClick={() => {
          if (Math.abs(dragOffset.x) < 6 && Math.abs(dragOffset.y) < 6) {
            onOpenDetails(currentProfile);
          }
        }}
        className="relative z-40 w-full h-full touch-none cursor-grab active:cursor-grabbing"
      >
        <ProfileCard profile={currentProfile} />
      </div>
    </div>
  );
};

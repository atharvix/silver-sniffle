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
      <div className="relative w-full max-w-[290px] sm:max-w-sm h-[430px] sm:h-[470px] mx-auto bg-[#0d0f16] border border-white/10 rounded-[32px] p-6 sm:p-8 flex flex-col items-center justify-center text-center gap-4 shadow-2xl">
        <div className="w-14 h-14 rounded-full bg-sky-500/10 text-sky-400 border border-sky-500/20 flex items-center justify-center text-xl font-bold font-sans">
          kinjo<span className="text-sky-400">.</span>
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

    const threshold = 90;
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

  const rightOpacity = Math.min(1, Math.max(0, dragOffset.x / 80));
  const leftOpacity = Math.min(1, Math.max(0, -dragOffset.x / 80));

  return (
    <div className="relative w-full max-w-[270px] xs:max-w-[290px] sm:max-w-[310px] md:max-w-[330px] h-[430px] sm:h-[460px] md:h-[490px] mx-auto -translate-x-3 sm:-translate-x-4 flex items-center justify-center">
      {/* Background cards shifted to the right peeking top-right */}
      {peekStackProfiles.slice(1).map((profile, idx) => {
        const stackIndex = idx + 1; // 1 or 2
        const translateX = stackIndex * 18; // 18px, 36px shifted right (perfect mobile fit!)
        const translateY = stackIndex * 3;  // 3px, 6px slightly down
        const rotation = stackIndex * 3.5;  // 3.5deg, 7deg right tilt
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

        {/* Swipe Right Overlay */}
        <div
          style={{ opacity: rightOpacity }}
          className="absolute top-6 left-6 z-50 border-4 border-emerald-500 text-emerald-400 font-black text-base sm:text-lg px-3.5 py-1 rounded-2xl -rotate-12 bg-black/80 backdrop-blur-md pointer-events-none transition-opacity duration-75 shadow-2xl"
        >
          CONNECT 💚
        </div>

        {/* Swipe Left Overlay */}
        <div
          style={{ opacity: leftOpacity }}
          className="absolute top-6 right-6 z-50 border-4 border-rose-500 text-rose-400 font-black text-base sm:text-lg px-3.5 py-1 rounded-2xl rotate-12 bg-black/80 backdrop-blur-md pointer-events-none transition-opacity duration-75 shadow-2xl"
        >
          PASS ❌
        </div>
      </div>
    </div>
  );
};

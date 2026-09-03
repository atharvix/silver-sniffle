import React, { useState } from 'react';
import { ArrowRight, Check } from 'lucide-react';

interface OnboardingTutorialProps {
  isOpen: boolean;
  onClose: () => void;
}

export const OnboardingTutorial: React.FC<OnboardingTutorialProps> = ({
  isOpen,
  onClose,
}) => {
  const [step, setStep] = useState<0 | 1>(0);

  if (!isOpen) return null;

  const handleNext = () => {
    if (step === 0) {
      setStep(1);
    } else {
      onClose();
    }
  };

  return (
    <div className="fixed inset-0 z-50 pointer-events-auto select-none bg-black/60 backdrop-blur-sm animate-in fade-in duration-300">
      
      {/* ─── STEP 0: CARD DECK SWIPE & PULL REFRESH TOUR ─────────────────── */}
      {step === 0 && (
        <div className="relative w-full h-full flex flex-col justify-center items-center px-6">
          
          {/* Target Highlight Box around Card Deck Area */}
          <div className="absolute inset-x-6 top-1/2 -translate-y-1/2 max-w-sm mx-auto h-[440px] rounded-[36px] border-2 border-dashed border-white/40 animate-pulse pointer-events-none" />

          {/* Animated Dotted Arc Arrow pointing down to card deck */}
          <svg className="absolute w-full h-44 top-[22%] pointer-events-none max-w-md mx-auto" viewBox="0 0 300 160">
            <defs>
              <linearGradient id="arrowGrad" x1="0%" y1="0%" x2="100%" y2="100%">
                <stop offset="0%" stopColor="#ffffff" stopOpacity="0.8" />
                <stop offset="100%" stopColor="#ffffff" stopOpacity="0.2" />
              </linearGradient>
            </defs>
            <path
              d="M 150 10 Q 220 80 150 140"
              fill="none"
              stroke="url(#arrowGrad)"
              strokeWidth="2.5"
              strokeDasharray="6,6"
              className="animate-[dash_1.5s_linear_infinite]"
            />
            <polygon points="150,140 160,130 145,130" fill="#ffffff" opacity="0.8" />
          </svg>

          {/* Floating Dark Glass Step Card */}
          <div className="relative z-10 max-w-sm w-full bg-[#121212]/95 border border-white/15 rounded-3xl p-6 shadow-2xl backdrop-blur-xl text-white space-y-4 text-center mt-32">
            <div className="inline-flex items-center gap-2 px-3 py-1 rounded-full bg-white/10 border border-white/15 text-[11px] font-semibold text-white/80 uppercase tracking-widest">
              <span>Step 1 of 2</span>
            </div>

            <h3 className="text-lg font-bold tracking-tight text-white leading-snug">
              Discover & Refresh Deck
            </h3>

            <p className="text-xs text-white/70 leading-relaxed font-normal">
              Swipe cards left or right to explore nearby people. Pull down on the card deck anytime to refresh your 30-meter radar.
            </p>

            <button
              onClick={handleNext}
              className="w-full flex items-center justify-center gap-2 py-3 px-4 rounded-2xl bg-white hover:bg-neutral-200 text-black font-bold text-xs transition-all active:scale-[0.98] shadow-lg mt-2"
            >
              <span>Next Feature</span>
              <ArrowRight className="w-4 h-4" strokeWidth={2.5} />
            </button>
          </div>
        </div>
      )}

      {/* ─── STEP 1: HEADER PROFILE & SETTINGS TOUR ────────────────────────── */}
      {step === 1 && (
        <div className="relative w-full h-full flex flex-col justify-start items-center px-6 pt-16">
          
          {/* Target Highlight Ring around Header Profile Photo Circle */}
          <div className="absolute top-3.5 right-4 w-11 h-11 rounded-full border-2 border-dashed border-white/80 animate-ping pointer-events-none" />
          <div className="absolute top-3.5 right-4 w-11 h-11 rounded-full border-2 border-white pointer-events-none shadow-[0_0_20px_rgba(255,255,255,0.4)]" />

          {/* Curved Dotted Arrow pointing to Top Right Profile Circle */}
          <svg className="absolute w-full h-48 top-12 pointer-events-none max-w-md mx-auto" viewBox="0 0 320 180">
            <path
              d="M 160 150 Q 250 80 290 25"
              fill="none"
              stroke="#ffffff"
              strokeWidth="2.5"
              strokeDasharray="6,6"
              className="animate-[dash_1.5s_linear_infinite]"
              opacity="0.8"
            />
            <polygon points="290,25 280,32 284,20" fill="#ffffff" opacity="0.9" />
          </svg>

          {/* Floating Dark Glass Step Card */}
          <div className="relative z-10 max-w-sm w-full bg-[#121212]/95 border border-white/15 rounded-3xl p-6 shadow-2xl backdrop-blur-xl text-white space-y-4 text-center mt-36">
            <div className="inline-flex items-center gap-2 px-3 py-1 rounded-full bg-white/10 border border-white/15 text-[11px] font-semibold text-white/80 uppercase tracking-widest">
              <span>Step 2 of 2</span>
            </div>

            <h3 className="text-lg font-bold tracking-tight text-white leading-snug">
              Account & Settings
            </h3>

            <p className="text-xs text-white/70 leading-relaxed font-normal">
              Tap your profile circle in the top-right corner to view your account details and manage settings.
            </p>

            <button
              onClick={handleNext}
              className="w-full flex items-center justify-center gap-2 py-3 px-4 rounded-2xl bg-white hover:bg-neutral-200 text-black font-bold text-xs transition-all active:scale-[0.98] shadow-lg mt-2"
            >
              <span>Get Started</span>
              <Check className="w-4 h-4" strokeWidth={2.5} />
            </button>
          </div>
        </div>
      )}

      {/* SVG Animation Keyframes */}
      <style>{`
        @keyframes dash {
          to {
            stroke-dashoffset: -24;
          }
        }
      `}</style>
    </div>
  );
};

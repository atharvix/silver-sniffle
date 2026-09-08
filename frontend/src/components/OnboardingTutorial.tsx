import React, { useState } from 'react';
import { ArrowRight, Check, ArrowDown, MapPin, RefreshCw } from 'lucide-react';

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
    <div className="fixed inset-0 z-50 pointer-events-auto select-none bg-black/85 animate-in fade-in duration-300 overflow-y-auto">

      {/* ─── STEP 0: MOCK CARDS, SWIPE & PULL SCREEN REFRESH TOUR ─────────────────── */}
      {step === 0 && (
        <div className="relative w-full h-full min-h-screen flex flex-col justify-between items-center px-6 py-10 max-w-md mx-auto">

          {/* Top Screen Pull-to-Refresh Visual Guide */}
          <div className="w-full flex flex-col items-center pt-2 space-y-1.5 animate-bounce">
            <div className="flex items-center gap-2 px-3.5 py-1.5 rounded-full bg-white/10 border border-white/20 text-xs font-semibold text-white">
              <RefreshCw className="w-3.5 h-3.5 text-emerald-400 animate-spin" />
              <span>Pull screen down to refresh nearby cards</span>
            </div>
            <ArrowDown className="w-5 h-5 text-white/70" strokeWidth={2.5} />
          </div>

          {/* Center Mock Card Stack with Interactive Swipe Indicators */}
          <div className="relative w-[260px] h-[340px] my-auto">
            {/* Back Mock Card */}
            <div className="absolute inset-0 rounded-[24px] bg-[#1a1a1a] border border-white/10 translate-x-3 translate-y-3 rotate-3 opacity-60 pointer-events-none" />

            {/* Top Mock Card */}
            <div className="relative w-full h-full rounded-[24px] overflow-hidden bg-[#111111] border border-white/20 shadow-2xl flex flex-col justify-between p-4">
              <img
                src="https://images.unsplash.com/photo-1534528741775-53994a69daeb?auto=format&fit=crop&q=80&w=600"
                alt="Mock User"
                className="absolute inset-0 w-full h-full object-cover opacity-80"
              />
              <div className="absolute inset-0 bg-gradient-to-t from-black/90 via-black/30 to-transparent" />

              {/* Distance Badge */}
              <div className="relative z-10 self-end px-2.5 py-1 rounded-full bg-black/60 backdrop-blur-md border border-white/20 text-[10px] font-bold text-white flex items-center gap-1">
                <MapPin className="w-3 h-3 text-emerald-400" />
                <span>12m</span>
              </div>



              {/* Mock Bio Info */}
              <div className="relative z-10 space-y-0.5 text-left">
                <h4 className="text-base font-bold text-white tracking-tight">Sarah Chen</h4>
                <p className="text-[11px] font-semibold text-white/80">Product Designer</p>
                <p className="text-[10px] text-white/60 line-clamp-1">Looking for tech co-founders nearby</p>
              </div>
            </div>
          </div>

          {/* Bottom Floating Step Card */}
          <div className="relative z-10 w-full bg-[#121212] border border-white/15 rounded-3xl p-5 shadow-2xl text-white space-y-3 text-center">
            <div className="inline-flex items-center gap-2 px-3 py-1 rounded-full bg-white/10 border border-white/15 text-[10px] font-bold text-white/80 uppercase tracking-widest">
              <span>Step 1 of 2</span>
            </div>

            <h3 className="text-base font-bold tracking-tight text-white leading-snug">
              Swipe Cards & Refresh Cards
            </h3>

            <p className="text-xs text-white/70 leading-relaxed font-normal">
              Swipe cards left or right to explore people around you. Pull the screen down anytime to refresh nearby discovery.
            </p>

            <button
              onClick={handleNext}
              className="w-full flex items-center justify-center gap-2 py-3 px-4 rounded-2xl bg-white hover:bg-neutral-200 text-black font-bold text-xs transition-all active:scale-[0.98] shadow-lg"
            >
              <span>Next Feature</span>
              <ArrowRight className="w-4 h-4" strokeWidth={2.5} />
            </button>
          </div>
        </div>
      )}

      {/* ─── STEP 1: HEADER PROFILE & SETTINGS TOUR ────────────────────────── */}
      {step === 1 && (
        <div className="relative w-full h-full min-h-screen flex flex-col justify-start items-center px-6 pt-16 max-w-md mx-auto">

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
          <div className="relative z-10 max-w-sm w-full bg-[#121212] border border-white/15 rounded-3xl p-6 shadow-2xl text-white space-y-4 text-center mt-36">
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

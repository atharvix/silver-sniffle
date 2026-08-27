import React, { useState } from 'react';
import { X, Navigation, Layers, Menu, ShieldCheck, Mail, ArrowRight, Check, KeyRound } from 'lucide-react';

interface OnboardingModalProps {
  isOpen: boolean;
  onClose: () => void;
  onComplete: (userEmail?: string) => void;
}

export const OnboardingModal: React.FC<OnboardingModalProps> = ({
  isOpen,
  onClose,
  onComplete,
}) => {
  const [step, setStep] = useState<'auth' | 'tour'>('auth');
  const [tourStep, setTourStep] = useState(0);

  // Auth Form State
  const [authMode, setAuthMode] = useState<'signin' | 'signup'>('signup');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [forgotSent, setForgotSent] = useState(false);

  if (!isOpen) return null;

  const handleAuthSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!email || !password) return;
    setStep('tour');
  };

  const handleGoogleAuth = () => {
    setEmail('alex.rivera@gmail.com');
    setStep('tour');
  };

  const handleForgotPassword = () => {
    if (!email) {
      alert('Please enter your email address above to reset password.');
      return;
    }
    setForgotSent(true);
    setTimeout(() => setForgotSent(false), 4000);
  };

  const TOUR_STEPS = [
    {
      title: '30-Meter Radius Limit',
      icon: <Navigation className="w-8 h-8 text-sky-400" />,
      description:
        'kinjo works exclusively within a strict 30-meter radius around your real-time device GPS location. Discover real people right where you are.',
      badge: '30m Device GPS',
    },
    {
      title: 'Right-Tilted Card Stack',
      icon: <Layers className="w-8 h-8 text-sky-400" />,
      description:
        'Swipe left to pass or right to connect. The top 3 profiles cascade to the right so you can preview who is available nearby.',
      badge: 'Gesture Cards',
    },
    {
      title: 'Hamburg Connected Menu',
      icon: <Menu className="w-8 h-8 text-sky-400" />,
      description:
        'Every profile you swipe right on is saved to your Hamburg Drawer. Open it anytime to view full profiles and details.',
      badge: 'Further Interaction',
    },
    {
      title: 'Liveness & Real Face Verification',
      icon: <ShieldCheck className="w-8 h-8 text-emerald-400" />,
      description:
        'Verify your profile with live camera liveness detection or upload your face photo. Ensures 100% authentic human connections.',
      badge: 'Liveness AI',
    },
  ];

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/90 backdrop-blur-2xl overflow-y-auto select-none">
      <div className="relative w-full max-w-md bg-[#090b10]/95 border border-white/15 rounded-[32px] p-6 md:p-8 shadow-2xl space-y-6 my-auto">
        {/* Header */}
        <div className="flex items-center justify-between pb-2 border-b border-white/10">
          <div className="flex items-center gap-2">
            <h1 className="text-2xl font-bold tracking-tight text-white font-sans">
              kinjo<span className="text-sky-400">.</span>
            </h1>
            <span className="text-[11px] font-semibold text-neutral-400 bg-white/5 border border-white/10 px-2.5 py-0.5 rounded-full">
              {step === 'auth' ? 'Authentication' : `Guide ${tourStep + 1}/4`}
            </span>
          </div>

          <button
            onClick={onClose}
            className="w-8 h-8 rounded-full bg-white/5 hover:bg-white/10 text-neutral-400 hover:text-white flex items-center justify-center transition-colors border border-white/10"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        {/* STEP 1: AUTHENTICATION */}
        {step === 'auth' ? (
          <div className="space-y-5">
            <div>
              <h2 className="text-lg font-bold text-white tracking-tight">Welcome to kinjo</h2>
              <p className="text-xs text-neutral-400 mt-0.5">
                Sign in or create your profile to start connecting within 30m
              </p>
            </div>

            {/* Password reset notification */}
            {forgotSent && (
              <div className="p-3 rounded-xl bg-emerald-500/10 border border-emerald-500/30 text-emerald-300 text-xs font-medium flex items-center gap-2">
                <Check className="w-4 h-4" />
                <span>Password reset link sent to {email}!</span>
              </div>
            )}

            {/* Google Auth Button */}
            <button
              type="button"
              onClick={handleGoogleAuth}
              className="w-full py-3 px-4 rounded-xl bg-white hover:bg-neutral-100 text-black font-bold text-xs flex items-center justify-center gap-3 shadow-lg transition-all active:scale-95"
            >
              <svg className="w-4 h-4" viewBox="0 0 24 24">
                <path
                  fill="#4285F4"
                  d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z"
                />
                <path
                  fill="#34A853"
                  d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z"
                />
                <path
                  fill="#FBBC05"
                  d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.06H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.94l2.85-2.22.81-.63z"
                />
                <path
                  fill="#EA4335"
                  d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.06l3.66 2.84c.87-2.6 3.3-4.52 6.16-4.52z"
                />
              </svg>
              <span>Continue with Google</span>
            </button>

            <div className="relative flex items-center justify-center my-2">
              <div className="border-t border-white/10 w-full" />
              <span className="bg-[#090b10] px-3 text-[11px] font-semibold text-neutral-500 uppercase font-mono">
                or email
              </span>
            </div>

            {/* Email Form */}
            <form onSubmit={handleAuthSubmit} className="space-y-3.5">
              <div>
                <label className="text-xs font-semibold text-neutral-400 block mb-1">Email Address</label>
                <div className="relative">
                  <Mail className="w-4 h-4 absolute left-3.5 top-3 text-neutral-500" />
                  <input
                    type="email"
                    required
                    value={email}
                    onChange={(e) => setEmail(e.target.value)}
                    placeholder="alex@example.com"
                    className="w-full bg-white/[0.04] border border-white/12 rounded-xl pl-10 pr-3.5 py-2.5 text-xs text-white focus:border-sky-400/50 focus:outline-none"
                  />
                </div>
              </div>

              <div>
                <div className="flex items-center justify-between mb-1">
                  <label className="text-xs font-semibold text-neutral-400">Password</label>
                  <button
                    type="button"
                    onClick={handleForgotPassword}
                    className="text-[11px] text-sky-400 hover:underline flex items-center gap-1"
                  >
                    <KeyRound className="w-3 h-3" />
                    <span>Forgot?</span>
                  </button>
                </div>
                <input
                  type="password"
                  required
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  placeholder="••••••••"
                  className="w-full bg-white/[0.04] border border-white/12 rounded-xl px-3.5 py-2.5 text-xs text-white focus:border-sky-400/50 focus:outline-none"
                />
              </div>

              <button
                type="submit"
                className="w-full py-3 px-4 rounded-xl bg-sky-400 hover:bg-sky-300 text-black font-extrabold text-xs flex items-center justify-center gap-2 shadow-lg transition-all active:scale-95 mt-2"
              >
                <span>{authMode === 'signup' ? 'Create Account & Continue' : 'Sign In'}</span>
                <ArrowRight className="w-4 h-4" />
              </button>
            </form>

            <div className="text-center pt-2">
              <button
                type="button"
                onClick={() => setAuthMode(authMode === 'signup' ? 'signin' : 'signup')}
                className="text-xs text-neutral-400 hover:text-white transition-colors"
              >
                {authMode === 'signup'
                  ? 'Already have an account? Sign In'
                  : "Don't have an account? Create Account"}
              </button>
            </div>
          </div>
        ) : (
          /* STEP 2: INTERACTIVE FEATURE TOUR GUIDE */
          <div className="space-y-6 py-2">
            <div className="flex flex-col items-center text-center space-y-3">
              <div className="w-16 h-16 rounded-2xl bg-sky-400/10 border border-sky-400/20 flex items-center justify-center shadow-lg">
                {TOUR_STEPS[tourStep].icon}
              </div>

              <span className="px-3 py-1 rounded-full bg-sky-400/10 text-sky-300 font-bold text-[10px] uppercase font-mono">
                {TOUR_STEPS[tourStep].badge}
              </span>

              <h2 className="text-xl font-bold text-white tracking-tight">
                {TOUR_STEPS[tourStep].title}
              </h2>

              <p className="text-xs text-neutral-300 max-w-xs leading-relaxed">
                {TOUR_STEPS[tourStep].description}
              </p>
            </div>

            {/* Pagination Indicators */}
            <div className="flex items-center justify-center gap-2 pt-2">
              {TOUR_STEPS.map((_, i) => (
                <span
                  key={i}
                  className={`h-2 rounded-full transition-all ${
                    i === tourStep ? 'w-6 bg-sky-400' : 'w-2 bg-white/20'
                  }`}
                />
              ))}
            </div>

            {/* Actions */}
            <div className="flex items-center justify-between pt-2">
              <button
                type="button"
                onClick={() => {
                  onComplete(email);
                  onClose();
                }}
                className="text-xs text-neutral-400 hover:text-white font-medium"
              >
                Skip Guide
              </button>

              <button
                type="button"
                onClick={() => {
                  if (tourStep < TOUR_STEPS.length - 1) {
                    setTourStep((prev) => prev + 1);
                  } else {
                    onComplete(email);
                    onClose();
                  }
                }}
                className="px-5 py-2.5 rounded-xl bg-sky-400 hover:bg-sky-300 text-black font-extrabold text-xs flex items-center gap-2 shadow-lg transition-all active:scale-95"
              >
                <span>{tourStep < TOUR_STEPS.length - 1 ? 'Next' : 'Get Started'}</span>
                {tourStep < TOUR_STEPS.length - 1 ? (
                  <ArrowRight className="w-4 h-4" />
                ) : (
                  <Check className="w-4 h-4" />
                )}
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
};

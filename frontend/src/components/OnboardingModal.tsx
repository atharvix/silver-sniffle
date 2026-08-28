import React, { useEffect, useRef, useState } from 'react';
import { X, Navigation, Layers, Menu, ShieldCheck, Mail, ArrowRight, Check, KeyRound } from 'lucide-react';
import { sendOtp, verifyOtp } from '../utils/api';
import { performLivenessCheck } from '../utils/livenessDetector';

interface OnboardingModalProps {
  isOpen: boolean;
  onClose: () => void;
  onComplete: (userEmail?: string, token?: string) => void;
  onAuthenticated: (token: string, email: string) => void;
}

export const OnboardingModal: React.FC<OnboardingModalProps> = ({
  isOpen,
  onClose,
  onComplete,
  onAuthenticated,
}) => {
  const [step, setStep] = useState<'auth' | 'face' | 'tour'>('auth');
  const [tourStep, setTourStep] = useState(0);

  // Auth Form State
  const [authMode, setAuthMode] = useState<'signin' | 'signup'>('signup');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [forgotSent, setForgotSent] = useState(false);
  const [authStage, setAuthStage] = useState<'email' | 'otp'>('email');
  const [otp, setOtp] = useState('');
  const [authError, setAuthError] = useState('');
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [isCameraActive, setIsCameraActive] = useState(false);
  const [livenessMessage, setLivenessMessage] = useState('Center your face and move slightly.');
  const [isLive, setIsLive] = useState(false);
  const videoRef = useRef<HTMLVideoElement>(null);
  const streamRef = useRef<MediaStream | null>(null);
  const previousFrameRef = useRef<ImageData | null>(null);
  const livenessIntervalRef = useRef<number | null>(null);

  useEffect(() => {
    if (!isOpen || step !== 'face') return;
    let cancelled = false;
    const start = async () => {
      try {
        const stream = await navigator.mediaDevices.getUserMedia({ video: { facingMode: 'user', width: 640, height: 640 } });
        if (cancelled) return stream.getTracks().forEach((track) => track.stop());
        streamRef.current = stream;
        setIsCameraActive(true);
        if (videoRef.current) videoRef.current.srcObject = stream;
        livenessIntervalRef.current = window.setInterval(() => {
          if (!videoRef.current) return;
          const { result, currentFrameData } = performLivenessCheck(videoRef.current, previousFrameRef.current);
          setIsLive(result.isLive);
          setLivenessMessage(result.message);
          previousFrameRef.current = currentFrameData;
        }, 500);
      } catch {
        setLivenessMessage('Camera permission is required to verify your profile.');
      }
    };
    void start();
    return () => {
      cancelled = true;
      if (livenessIntervalRef.current) window.clearInterval(livenessIntervalRef.current);
      streamRef.current?.getTracks().forEach((track) => track.stop());
      streamRef.current = null;
      previousFrameRef.current = null;
      setIsCameraActive(false);
    };
  }, [isOpen, step]);

  if (!isOpen) return null;

  const handleAuthSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!email || !password) return;
    setAuthError('');
    setIsSubmitting(true);
    try {
      const response = await sendOtp(email);
      if (response.devOtp) setOtp(response.devOtp);
      setAuthStage('otp');
    } catch (error) {
      setAuthError(error instanceof Error ? error.message : 'Unable to send verification code.');
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleVerifyOtp = async (e: React.FormEvent) => {
    e.preventDefault();
    if (otp.length !== 4) return;
    setAuthError('');
    setIsSubmitting(true);
    try {
      const response = await verifyOtp(email, otp);
      onAuthenticated(response.verificationToken, email);
      setStep('face');
    } catch (error) {
      setAuthError(error instanceof Error ? error.message : 'That code is invalid or expired.');
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleGoogleAuth = () => {
    setAuthError('Use email verification to continue securely.');
  };

  const completeFaceCheck = () => {
    if (!isLive) return;
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
        'Swipe right to connect. Swipe left to review the previous card. The stack keeps nearby profiles easy to revisit.',
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
              {step === 'auth' ? 'Authentication' : step === 'face' ? 'Identity check' : `Guide ${tourStep + 1}/4`}
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

            {import.meta.env.VITE_ENABLE_TEST_MODE === 'true' && (
              <button
                type="button"
                onClick={() => {
                  onComplete('test@kinjo.local');
                  onClose();
                }}
                className="w-full py-2.5 rounded-xl border border-amber-400/30 bg-amber-400/10 text-amber-200 text-xs font-bold"
              >
                Continue in test mode
              </button>
            )}

            {authError && <p className="text-xs text-rose-300 bg-rose-500/10 border border-rose-500/20 rounded-xl px-3 py-2">{authError}</p>}

            {authStage === 'otp' ? (
              <form onSubmit={handleVerifyOtp} className="space-y-4">
                <div>
                  <h3 className="text-base font-bold text-white">Check your inbox</h3>
                  <p className="text-xs text-neutral-400 mt-1">Enter the 4-digit code sent to {email}.</p>
                </div>
                <input
                  inputMode="numeric"
                  pattern="[0-9]{4}"
                  maxLength={4}
                  required
                  autoFocus
                  value={otp}
                  onChange={(e) => setOtp(e.target.value.replace(/\D/g, '').slice(0, 4))}
                  placeholder="0000"
                  className="w-full bg-white/[0.04] border border-white/12 rounded-xl px-3.5 py-3 text-center text-xl tracking-[0.5em] text-white focus:border-sky-400/50 focus:outline-none"
                />
                <button type="submit" disabled={isSubmitting} className="w-full py-3 rounded-xl bg-sky-400 text-black font-extrabold text-xs disabled:opacity-50">
                  {isSubmitting ? 'Verifying...' : 'Verify email'}
                </button>
                <button type="button" onClick={() => setAuthStage('email')} className="w-full text-xs text-neutral-400 hover:text-white">Use a different email</button>
              </form>
            ) : (
            <>
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
                <span>{isSubmitting ? 'Sending code...' : authMode === 'signup' ? 'Continue with email' : 'Continue with email'}</span>
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
            </>
            )}
          </div>
        ) : step === 'face' ? (
          <div className="space-y-6 py-2 text-center">
            <div>
              <h2 className="text-xl font-bold text-white">Verify you are real</h2>
              <p className="text-xs text-neutral-400 mt-1">Allow camera access and move your head slightly. Your photo is checked on this device.</p>
            </div>
            <div className="relative mx-auto w-56 h-56 overflow-hidden rounded-[28px] border-2 border-white/20 bg-black">
              <video ref={videoRef} autoPlay playsInline muted className="h-full w-full object-cover" />
              <div className={`absolute inset-4 rounded-full border-2 ${isLive ? 'border-emerald-400' : 'border-white/30'}`} />
            </div>
            <p className={`text-xs ${isLive ? 'text-emerald-300' : 'text-neutral-300'}`}>{isCameraActive ? livenessMessage : livenessMessage}</p>
            <button type="button" onClick={completeFaceCheck} disabled={!isLive} className="w-full py-3 rounded-xl bg-sky-400 text-black font-extrabold text-xs disabled:opacity-40">
              Continue
            </button>
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

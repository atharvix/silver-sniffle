import React, { useState, useRef } from 'react';
import { ArrowRight, Camera, Upload } from 'lucide-react';
import { sendOtp, verifyOtp } from '../utils/api';

interface OnboardingModalProps {
  isOpen: boolean;
  onClose: () => void;
  onComplete: (userEmail?: string, token?: string, isGuest?: boolean) => void;
  onAuthenticated: (token: string, email: string) => void;
  onProfileSetupComplete?: (profileData: { name: string; avatar: string; profession: string; lookingFor: string }) => void;
  initialStep?: AuthStep;
  initialProfile?: { name: string; avatar: string; profession: string; lookingFor: string };
}

type AuthStep = 'email' | 'password' | 'otp' | 'profile_setup';

export const OnboardingModal: React.FC<OnboardingModalProps> = ({
  isOpen,
  onClose,
  onComplete,
  onAuthenticated,
  onProfileSetupComplete,
  initialStep = 'email',
  initialProfile,
}) => {
  const [step, setStep] = useState<AuthStep>(initialStep);
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [otp, setOtp] = useState('');
  const [authError, setAuthError] = useState('');
  const [isSubmitting, setIsSubmitting] = useState(false);

  // Profile setup state (pre-populated when editing profile)
  const [name, setName] = useState(initialProfile?.name || '');
  const [avatar, setAvatar] = useState(initialProfile?.avatar || '');
  const [profession, setProfession] = useState(initialProfile?.profession || '');
  const [lookingFor, setLookingFor] = useState(initialProfile?.lookingFor || '');
  const fileInputRef = useRef<HTMLInputElement>(null);

  if (!isOpen) return null;

  const handleEmailSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!email.trim()) return;
    setAuthError('');
    setStep('password');
  };

  const handlePasswordSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!password) return;
    setAuthError('');
    setIsSubmitting(true);
    try {
      const response = await sendOtp(email);
      if (response.devOtp) setOtp(response.devOtp);
      setStep('otp');
    } catch (error) {
      setAuthError(error instanceof Error ? error.message : 'Could not send verification code.');
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleOtpSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (otp.length !== 4) return;
    setAuthError('');
    setIsSubmitting(true);
    try {
      const response = await verifyOtp(email, otp);
      onAuthenticated(response.verificationToken, email);
      // Take user to First-Time Profile Setup step
      setStep('profile_setup');
    } catch (error) {
      setAuthError(error instanceof Error ? error.message : 'Invalid or expired code.');
    } finally {
      setIsSubmitting(false);
    }
  };

  const handlePhotoUpload = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    const reader = new FileReader();
    reader.onload = (ev) => {
      setAvatar(ev.target?.result as string);
    };
    reader.readAsDataURL(file);
  };

  const handleFinalProfileSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (onProfileSetupComplete) {
      onProfileSetupComplete({
        name: name.trim() || 'Kinjo User',
        avatar,
        profession: profession.trim(),
        lookingFor: lookingFor.trim(),
      });
    }
    localStorage.setItem('kinjo_onboarded', 'true');
    onComplete(email, undefined, false);
    onClose();
  };

  const handleGoogleAuth = () => {
    setStep('profile_setup');
  };

  const handleSkipTest = () => {
    onComplete(undefined, undefined, true);
    onClose();
  };

  return (
    <div className="fixed inset-0 z-50 bg-[#060606] flex flex-col justify-between select-none p-6 sm:p-8 animate-in fade-in duration-200 overflow-y-auto">
      
      {/* ─── TOP HEADER ────────────────────────────────────────────────────────── */}
      <div className="w-full flex items-center justify-between max-w-md mx-auto pt-4">
        {/* Top-Left Logo */}
        <span className="text-3xl font-bold tracking-tight text-white leading-none">
          k<span className="text-white/30">.</span>
        </span>

        {/* Skip for testing */}
        {step !== 'profile_setup' && (
          <button
            onClick={handleSkipTest}
            className="text-xs font-semibold text-white/40 hover:text-white transition-colors"
          >
            Skip for testing →
          </button>
        )}
      </div>

      {/* ─── MIDDLE MAIN CONTENT ────────────────────────────────────────────────── */}
      <div className="w-full max-w-md mx-auto my-auto space-y-6 py-4">

        {/* STEP 1: EMAIL & GOOGLE OAUTH */}
        {step === 'email' && (
          <div className="space-y-6">
            {/* Google OAuth Button */}
            <button
              type="button"
              onClick={handleGoogleAuth}
              className="w-full flex items-center justify-center gap-3 py-3.5 px-4 rounded-2xl bg-white/10 hover:bg-white/15 border border-white/15 text-white font-medium text-sm transition-all active:scale-[0.98] shadow-lg"
            >
              <svg className="w-5 h-5" viewBox="0 0 24 24">
                <path
                  fill="#EA4335"
                  d="M12 5c1.6 0 3 .6 4.1 1.6l3.1-3.1C17.3 1.7 14.8 1 12 1 7.5 1 3.7 3.6 1.9 7.3l3.7 2.9C6.5 7.3 9 5 12 5z"
                />
                <path
                  fill="#4285F4"
                  d="M23.5 12.3c0-.8-.1-1.7-.2-2.3H12v4.6h6.5c-.3 1.5-1.1 2.8-2.4 3.7l3.7 2.9c2.2-2 3.7-5 3.7-8.9z"
                />
                <path
                  fill="#FBBC05"
                  d="M5.6 14.8c-.2-.7-.4-1.5-.4-2.3s.2-1.6.4-2.3L1.9 7.3C.7 9.7 0 12.3 0 15s.7 5.3 1.9 7.7l3.7-2.9c-.4-.7-.6-1.5-.6-2.3z"
                />
                <path
                  fill="#34A853"
                  d="M12 23c3.2 0 6-1.1 8-3l-3.7-2.9c-1.1.7-2.5 1.2-4.3 1.2-3 0-5.5-2.3-6.4-5.2L1.9 16C3.7 19.7 7.5 23 12 23z"
                />
              </svg>
              <span>Continue with Google</span>
            </button>

            {/* Divider */}
            <div className="relative flex items-center justify-center">
              <div className="w-full border-t border-white/10" />
              <span className="absolute px-3 bg-[#060606] text-xs font-semibold text-white/30 uppercase tracking-widest">
                or
              </span>
            </div>

            {/* Generic Email Input */}
            <form onSubmit={handleEmailSubmit} className="space-y-4">
              <div className="space-y-1.5">
                <label className="text-[11px] font-semibold text-white/40 uppercase tracking-wider block">
                  Email
                </label>
                <input
                  type="email"
                  required
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                  placeholder="Enter email address"
                  className="w-full px-4 py-3.5 rounded-2xl bg-white/[0.06] border border-white/10 text-white placeholder:text-white/20 text-sm outline-none focus:border-white/30 transition-colors font-medium"
                />
              </div>

              <button
                type="submit"
                className="w-full flex items-center justify-center gap-2 py-4 px-4 rounded-2xl bg-white hover:bg-neutral-200 text-black font-bold text-sm transition-all active:scale-[0.98] shadow-lg mt-4"
              >
                <span>Continue</span>
                <ArrowRight className="w-4 h-4" strokeWidth={2.5} />
              </button>
            </form>
          </div>
        )}

        {/* STEP 2: PASSWORD */}
        {step === 'password' && (
          <div className="space-y-6">
            <div>
              <h2 className="text-xl font-bold text-white tracking-tight leading-snug">
                Enter Password
              </h2>
              <p className="text-xs text-white/50 mt-1 font-normal">
                Signing in as <span className="text-white font-medium">{email}</span>
              </p>
            </div>

            <form onSubmit={handlePasswordSubmit} className="space-y-4">
              <div className="space-y-1.5">
                <label className="text-[11px] font-semibold text-white/40 uppercase tracking-wider block">
                  Password
                </label>
                <input
                  type="password"
                  required
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  placeholder="••••••••"
                  className="w-full px-4 py-3.5 rounded-2xl bg-white/[0.06] border border-white/10 text-white placeholder:text-white/20 text-sm outline-none focus:border-white/30 transition-colors font-medium"
                />
              </div>

              {authError && <p className="text-xs text-red-400 font-medium">{authError}</p>}

              <button
                type="submit"
                disabled={isSubmitting}
                className="w-full flex items-center justify-center gap-2 py-4 px-4 rounded-2xl bg-white hover:bg-neutral-200 text-black font-bold text-sm transition-all active:scale-[0.98] shadow-lg mt-4 disabled:opacity-50"
              >
                <span>{isSubmitting ? 'Sending Code…' : 'Send Code'}</span>
                <ArrowRight className="w-4 h-4" strokeWidth={2.5} />
              </button>
            </form>
          </div>
        )}

        {/* STEP 3: OTP */}
        {step === 'otp' && (
          <div className="space-y-6">
            <div>
              <h2 className="text-xl font-bold text-white tracking-tight leading-snug">
                Verify Email
              </h2>
              <p className="text-xs text-white/50 mt-1 font-normal">
                Enter code sent to <span className="text-white font-medium">{email}</span>
              </p>
            </div>

            <form onSubmit={handleOtpSubmit} className="space-y-4">
              <div className="space-y-1.5">
                <label className="text-[11px] font-semibold text-white/40 uppercase tracking-wider block">
                  Verification Code
                </label>
                <input
                  type="text"
                  maxLength={4}
                  required
                  value={otp}
                  onChange={(e) => setOtp(e.target.value.replace(/\D/g, ''))}
                  placeholder="0000"
                  className="w-full px-4 py-3.5 rounded-2xl bg-white/[0.06] border border-white/10 text-white placeholder:text-white/20 text-center tracking-[0.5em] text-lg outline-none focus:border-white/30 transition-colors font-mono"
                />
              </div>

              {authError && <p className="text-xs text-red-400 font-medium">{authError}</p>}

              <button
                type="submit"
                disabled={isSubmitting}
                className="w-full flex items-center justify-center gap-2 py-4 px-4 rounded-2xl bg-white hover:bg-neutral-200 text-black font-bold text-sm transition-all active:scale-[0.98] shadow-lg mt-4 disabled:opacity-50"
              >
                <span>{isSubmitting ? 'Verifying…' : 'Verify & Continue'}</span>
                <ArrowRight className="w-4 h-4" strokeWidth={2.5} />
              </button>
            </form>
          </div>
        )}

        {/* STEP 4: FIRST-TIME PROFILE SETUP */}
        {step === 'profile_setup' && (
          <div className="space-y-5">
            <div>
              <h2 className="text-xl font-bold text-white tracking-tight leading-snug">
                Create Your Profile
              </h2>
              <p className="text-xs text-white/50 mt-1 font-normal">
                Set up your profile details for nearby discovery
              </p>
            </div>

            <form onSubmit={handleFinalProfileSubmit} className="space-y-4">
              {/* Photo Upload Circle */}
              <div className="flex flex-col items-center justify-center p-4 rounded-2xl bg-white/[0.04] border border-white/10 text-center space-y-2">
                <div
                  className="relative w-20 h-20 rounded-full overflow-hidden bg-white/10 border-2 border-white/20 cursor-pointer shadow-xl flex items-center justify-center group"
                  onClick={() => fileInputRef.current?.click()}
                >
                  {avatar ? (
                    <img src={avatar} alt="Avatar" className="w-full h-full object-cover" />
                  ) : (
                    <Camera className="w-7 h-7 text-white/40" />
                  )}
                  <div className="absolute inset-0 bg-black/40 flex flex-col items-center justify-center opacity-0 group-hover:opacity-100 transition-opacity">
                    <Upload className="w-4 h-4 text-white" />
                  </div>
                </div>
                <p className="text-[11px] text-white/40">Tap photo circle to upload picture</p>
                <input ref={fileInputRef} type="file" accept="image/*" onChange={handlePhotoUpload} className="hidden" />
              </div>

              {/* Full Name */}
              <div className="space-y-1.5">
                <label className="text-[11px] font-semibold text-white/40 uppercase tracking-wider block">
                  Full Name
                </label>
                <input
                  type="text"
                  required
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="Your full name"
                  className="w-full px-4 py-3 rounded-2xl bg-white/[0.06] border border-white/10 text-white placeholder:text-white/20 text-sm outline-none focus:border-white/30 transition-colors font-medium"
                />
              </div>

              {/* Question 1: What you do */}
              <div className="space-y-1.5">
                <label className="text-[11px] font-semibold text-white/40 uppercase tracking-wider block">
                  What you do
                </label>
                <input
                  type="text"
                  required
                  value={profession}
                  onChange={(e) => setProfession(e.target.value)}
                  placeholder="e.g. Founder, Tech Startup"
                  className="w-full px-4 py-3 rounded-2xl bg-white/[0.06] border border-white/10 text-white placeholder:text-white/20 text-sm outline-none focus:border-white/30 transition-colors font-medium"
                />
              </div>

              {/* Question 2: What are you looking for */}
              <div className="space-y-1.5">
                <label className="text-[11px] font-semibold text-white/40 uppercase tracking-wider block">
                  What are you looking for
                </label>
                <textarea
                  rows={2}
                  required
                  value={lookingFor}
                  onChange={(e) => setLookingFor(e.target.value)}
                  placeholder="e.g. Looking to connect with local builders…"
                  className="w-full px-4 py-3 rounded-2xl bg-white/[0.06] border border-white/10 text-white placeholder:text-white/20 text-sm outline-none focus:border-white/30 transition-colors font-medium resize-none leading-relaxed"
                />
              </div>

              <button
                type="submit"
                className="w-full flex items-center justify-center gap-2 py-4 px-4 rounded-2xl bg-white hover:bg-neutral-200 text-black font-bold text-sm transition-all active:scale-[0.98] shadow-lg mt-4"
              >
                <span>Save & Enter App</span>
                <ArrowRight className="w-4 h-4" strokeWidth={2.5} />
              </button>
            </form>
          </div>
        )}
      </div>

      {/* ─── BOTTOM TERMS AGREEMENT TEXT ───────────────────────────────────────── */}
      <div className="w-full max-w-md mx-auto text-center pb-4">
        <p className="text-[11px] text-white/40 leading-relaxed font-normal">
          By continuing, you agree to our Terms & Conditions and Privacy Policy.
        </p>
      </div>
    </div>
  );
};

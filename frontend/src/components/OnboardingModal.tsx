import React, { useState } from 'react';
import { ArrowRight, Mail, Eye, EyeOff } from 'lucide-react';
import { sendOtp, verifyOtp } from '../utils/api';

interface OnboardingModalProps {
  isOpen: boolean;
  onClose: () => void;
  onComplete: (userEmail?: string, token?: string) => void;
  onAuthenticated: (token: string, email: string) => void;
}

type AuthStep = 'email' | 'password' | 'otp';

export const OnboardingModal: React.FC<OnboardingModalProps> = ({
  isOpen,
  onClose,
  onComplete,
  onAuthenticated,
}) => {
  const [step, setStep] = useState<AuthStep>('email');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [showPassword, setShowPassword] = useState(false);
  const [otp, setOtp] = useState('');
  const [authError, setAuthError] = useState('');
  const [isSubmitting, setIsSubmitting] = useState(false);

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
      onComplete(email, response.verificationToken);
      onClose();
    } catch (error) {
      setAuthError(error instanceof Error ? error.message : 'Invalid or expired code.');
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 bg-black flex flex-col select-none">
      {/* Top: Logo */}
      <div className="px-7 pt-16 pb-4">
        <span className="text-[28px] font-bold tracking-tight text-white leading-none">
          k<span className="text-white/25">.</span>
        </span>
      </div>

      {/* Content */}
      <div className="flex-1 px-7 flex flex-col justify-center">

        {/* Test mode bypass */}
        {import.meta.env.VITE_ENABLE_TEST_MODE === 'true' && (
          <button
            type="button"
            onClick={() => { onComplete('test@kinjo.local'); onClose(); }}
            className="mb-8 text-xs text-white/30 hover:text-white/60 underline text-left transition-colors"
          >
            Continue in test mode →
          </button>
        )}

        {/* Step: Email */}
        {step === 'email' && (
          <form onSubmit={handleEmailSubmit} className="space-y-10">
            <div className="space-y-1">
              <h1 className="text-3xl font-semibold text-white tracking-tight leading-tight">
                What's your<br />email?
              </h1>
              <p className="text-sm text-white/35 font-normal mt-2">
                getting into the world starts here.
              </p>
            </div>

            <div className="space-y-2">
              <input
                type="email"
                required
                autoFocus
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder="your@email.com"
                className="auth-input"
              />
              {authError && <p className="text-xs text-red-400/80">{authError}</p>}
            </div>

            <button
              type="submit"
              className="flex items-center gap-2 text-white font-medium text-sm group"
            >
              <span>Continue</span>
              <ArrowRight className="w-4 h-4 group-hover:translate-x-1 transition-transform" strokeWidth={2} />
            </button>
          </form>
        )}

        {/* Step: Password */}
        {step === 'password' && (
          <form onSubmit={handlePasswordSubmit} className="space-y-10">
            <div className="space-y-1">
              <h1 className="text-3xl font-semibold text-white tracking-tight leading-tight">
                Your password
              </h1>
              <p className="text-sm text-white/35 font-normal mt-2">{email}</p>
            </div>

            <div className="space-y-2">
              <div className="relative">
                <input
                  type={showPassword ? 'text' : 'password'}
                  required
                  autoFocus
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  placeholder="••••••••"
                  className="auth-input pr-10"
                />
                <button
                  type="button"
                  onClick={() => setShowPassword(!showPassword)}
                  className="absolute right-0 top-3 text-white/30 hover:text-white/60 transition-colors"
                >
                  {showPassword
                    ? <EyeOff className="w-4 h-4" strokeWidth={1.8} />
                    : <Eye className="w-4 h-4" strokeWidth={1.8} />
                  }
                </button>
              </div>
              {authError && <p className="text-xs text-red-400/80">{authError}</p>}
            </div>

            <div className="flex items-center justify-between">
              <button
                type="button"
                onClick={() => setStep('email')}
                className="text-sm text-white/30 hover:text-white/60 transition-colors"
              >
                ← Back
              </button>
              <button
                type="submit"
                disabled={isSubmitting}
                className="flex items-center gap-2 text-white font-medium text-sm group disabled:opacity-40"
              >
                <span>{isSubmitting ? 'Sending code…' : 'Continue'}</span>
                <ArrowRight className="w-4 h-4 group-hover:translate-x-1 transition-transform" strokeWidth={2} />
              </button>
            </div>
          </form>
        )}

        {/* Step: OTP */}
        {step === 'otp' && (
          <form onSubmit={handleOtpSubmit} className="space-y-10">
            <div className="space-y-1">
              <h1 className="text-3xl font-semibold text-white tracking-tight leading-tight">
                Check your<br />inbox
              </h1>
              <p className="text-sm text-white/35 font-normal mt-2">
                4-digit code sent to {email}
              </p>
            </div>

            <div className="space-y-2">
              <input
                inputMode="numeric"
                pattern="[0-9]{4}"
                maxLength={4}
                required
                autoFocus
                value={otp}
                onChange={(e) => setOtp(e.target.value.replace(/\D/g, '').slice(0, 4))}
                placeholder="0 0 0 0"
                className="auth-input text-center text-2xl tracking-[0.6em] font-medium"
              />
              {authError && <p className="text-xs text-red-400/80">{authError}</p>}
            </div>

            <div className="flex items-center justify-between">
              <button
                type="button"
                onClick={() => setStep('password')}
                className="text-sm text-white/30 hover:text-white/60 transition-colors"
              >
                ← Back
              </button>
              <button
                type="submit"
                disabled={isSubmitting || otp.length !== 4}
                className="flex items-center gap-2 text-white font-medium text-sm group disabled:opacity-40"
              >
                <span>{isSubmitting ? 'Verifying…' : 'Verify'}</span>
                <ArrowRight className="w-4 h-4 group-hover:translate-x-1 transition-transform" strokeWidth={2} />
              </button>
            </div>

            <button
              type="button"
              onClick={() => setStep('email')}
              className="flex items-center gap-1.5 text-xs text-white/25 hover:text-white/50 transition-colors"
            >
              <Mail className="w-3.5 h-3.5" strokeWidth={1.8} />
              <span>Use a different email</span>
            </button>
          </form>
        )}
      </div>

      {/* Bottom tagline */}
      <div className="px-7 pb-10">
        <p className="text-[11px] text-white/18 font-normal tracking-widest lowercase">
          meet nearby · make it matter
        </p>
      </div>
    </div>
  );
};

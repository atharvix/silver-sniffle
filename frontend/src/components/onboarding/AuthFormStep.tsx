import React from 'react';
import { ArrowRight, Check, X } from 'lucide-react';

interface AuthFormStepProps {
  authMode: 'sign_up' | 'sign_in';
  setAuthMode: React.Dispatch<React.SetStateAction<'sign_up' | 'sign_in'>>;
  email: string;
  setEmail: (email: string) => void;
  password: string;
  setPassword: (password: string) => void;
  authError: string;
  isSubmitting: boolean;
  onSubmit: (e: React.FormEvent) => void;
  onGoogleAuth: () => void;
}

export const AuthFormStep: React.FC<AuthFormStepProps> = ({
  authMode,
  setAuthMode,
  email,
  setEmail,
  password,
  setPassword,
  authError,
  isSubmitting,
  onSubmit,
  onGoogleAuth,
}) => {
  const isLengthValid = password.length >= 8;
  const hasNumber = /\d/.test(password);
  const hasLetter = /[a-zA-Z]/.test(password);
  const isPasswordValid = isLengthValid && hasNumber && hasLetter;

  return (
    <div className="space-y-6">
      {/* Title Header */}
      <div>
        <h2 className="text-2xl font-extrabold text-white tracking-tight leading-snug">
          {authMode === 'sign_up' ? 'Create your account' : 'Welcome back'}
        </h2>
        <p className="text-xs text-white/50 mt-1 font-normal">
          {authMode === 'sign_up'
            ? 'Connect with people physically within 30m of you'
            : 'Sign in to discover nearby creators and innovators'}
        </p>
      </div>

      {/* Top Google Sign-In Button */}
      <button
        type="button"
        onClick={onGoogleAuth}
        className="w-full flex items-center justify-center gap-3 py-3.5 px-4 rounded-2xl bg-white/10 hover:bg-white/15 border border-white/15 text-white font-medium text-sm transition-all active:scale-[0.98] shadow-lg"
      >
        <svg className="w-5 h-5" viewBox="0 0 24 24">
          <path fill="#EA4335" d="M12 5c1.6 0 3 .6 4.1 1.6l3.1-3.1C17.3 1.7 14.8 1 12 1 7.5 1 3.7 3.6 1.9 7.3l3.7 2.9C6.5 7.3 9 5 12 5z" />
          <path fill="#4285F4" d="M23.5 12.3c0-.8-.1-1.7-.2-2.3H12v4.6h6.5c-.3 1.5-1.1 2.8-2.4 3.7l3.7 2.9c2.2-2 3.7-5 3.7-8.9z" />
          <path fill="#FBBC05" d="M5.6 14.8c-.2-.7-.4-1.5-.4-2.3s.2-1.6.4-2.3L1.9 7.3C.7 9.7 0 12.3 0 15s.7 5.3 1.9 7.7l3.7-2.9c-.4-.7-.6-1.5-.6-2.3z" />
          <path fill="#34A853" d="M12 23c3.2 0 6-1.1 8-3l-3.7-2.9c-1.1.7-2.5 1.2-4.3 1.2-3 0-5.5-2.3-6.4-5.2L1.9 16C3.7 19.7 7.5 23 12 23z" />
        </svg>
        <span>Continue with Google</span>
      </button>

      <div className="relative flex items-center justify-center">
        <div className="w-full border-t border-white/10" />
        <span className="absolute px-3 bg-[#0A0A0C] text-xs font-semibold text-white/30 uppercase tracking-widest">
          or
        </span>
      </div>

      {/* Auth Form (Email + Password together) */}
      <form onSubmit={onSubmit} className="space-y-4">
        {/* Email Field */}
        <div className="space-y-1.5">
          <label className="text-[11px] font-semibold text-white/40 uppercase tracking-wider block">
            Email Address
          </label>
          <input
            type="email"
            required
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder="Enter your email address"
            className="w-full px-4 py-3.5 rounded-2xl bg-white/[0.06] border border-white/10 text-white placeholder:text-white/20 text-sm outline-none focus:border-white/30 transition-colors font-medium"
          />
        </div>

        {/* Password Field */}
        <div className="space-y-1.5">
          <label className="text-[11px] font-semibold text-white/40 uppercase tracking-wider block">
            Password
          </label>
          <input
            type="password"
            required
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            placeholder="Enter your password"
            className="w-full px-4 py-3.5 rounded-2xl bg-white/[0.06] border border-white/10 text-white placeholder:text-white/20 text-sm outline-none focus:border-white/30 transition-colors font-medium"
          />
        </div>

        {/* Password Requirements Checklist (for Sign Up) */}
        {authMode === 'sign_up' && (
          <div className="py-2 px-1 space-y-2 text-xs">
            <div className="flex items-center gap-2.5">
              {isLengthValid ? (
                <Check className="w-3.5 h-3.5 text-emerald-400 shrink-0" strokeWidth={3} />
              ) : (
                <X className="w-3.5 h-3.5 text-white/20 shrink-0" strokeWidth={2} />
              )}
              <span className={isLengthValid ? 'text-white font-medium' : 'text-white/40'}>
                At least 8 characters
              </span>
            </div>

            <div className="flex items-center gap-2.5">
              {hasNumber ? (
                <Check className="w-3.5 h-3.5 text-emerald-400 shrink-0" strokeWidth={3} />
              ) : (
                <X className="w-3.5 h-3.5 text-white/20 shrink-0" strokeWidth={2} />
              )}
              <span className={hasNumber ? 'text-white font-medium' : 'text-white/40'}>
                Contains a number (0-9)
              </span>
            </div>

            <div className="flex items-center gap-2.5">
              {hasLetter ? (
                <Check className="w-3.5 h-3.5 text-emerald-400 shrink-0" strokeWidth={3} />
              ) : (
                <X className="w-3.5 h-3.5 text-white/20 shrink-0" strokeWidth={2} />
              )}
              <span className={hasLetter ? 'text-white font-medium' : 'text-white/40'}>
                Contains letters (a-z, A-Z)
              </span>
            </div>
          </div>
        )}

        {authError && <p className="text-xs text-red-400 font-medium">{authError}</p>}

        <button
          type="submit"
          disabled={isSubmitting || (authMode === 'sign_up' && !isPasswordValid)}
          className="w-full flex items-center justify-center gap-2 py-4 px-4 rounded-2xl bg-white hover:bg-neutral-200 text-black font-bold text-sm transition-all active:scale-[0.98] shadow-lg mt-4 disabled:opacity-50"
        >
          <span>
            {isSubmitting
              ? 'Please wait…'
              : authMode === 'sign_up'
              ? 'Create Account'
              : 'Sign In'}
          </span>
          <ArrowRight className="w-4 h-4" strokeWidth={2.5} />
        </button>
      </form>

      {/* Switch Mode Link */}
      <button
        type="button"
        onClick={() => {
          setAuthMode((mode) => (mode === 'sign_in' ? 'sign_up' : 'sign_in'));
        }}
        className="w-full text-xs text-white/50 hover:text-white transition-colors text-center block pt-2"
      >
        {authMode === 'sign_in'
          ? "Don't have an account? Create one"
          : 'Already have an account? Log in'}
      </button>
    </div>
  );
};

import React from 'react';
import { ArrowRight, Check, X } from 'lucide-react';

interface PasswordStepProps {
  email: string;
  password: string;
  setPassword: (password: string) => void;
  authMode: 'sign_in' | 'sign_up';
  authError: string;
  isSubmitting: boolean;
  onSubmit: (e: React.FormEvent) => void;
}

export const PasswordStep: React.FC<PasswordStepProps> = ({
  email,
  password,
  setPassword,
  authMode,
  authError,
  isSubmitting,
  onSubmit,
}) => {
  const isLengthValid = password.length >= 8;
  const hasNumber = /\d/.test(password);
  const hasLetter = /[a-zA-Z]/.test(password);
  const isPasswordValid = isLengthValid && hasNumber && hasLetter;

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-xl font-bold text-white tracking-tight leading-snug">
          {authMode === 'sign_up' ? 'Create Password' : 'Enter Password'}
        </h2>
        <p className="text-xs text-white/50 mt-1 font-normal">
          Signing in as <span className="text-white font-medium">{email}</span>
        </p>
      </div>

      <form onSubmit={onSubmit} className="space-y-4">
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

        {/* Standardized Password Practice Live Checklist */}
        {authMode === 'sign_up' && (
          <div className="py-2 px-1 space-y-2.5 text-xs">
            <p className="text-[11px] font-semibold text-white/40 uppercase tracking-wider mb-2">
              Password Security Requirements
            </p>

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
          className="w-full flex items-center justify-center gap-2 py-4 px-4 rounded-2xl bg-white hover:bg-neutral-200 text-black font-bold text-sm transition-all active:scale-[0.98] shadow-lg disabled:opacity-50 mt-4"
        >
          <span>{isSubmitting ? 'Please wait…' : authMode === 'sign_up' ? 'Create Account' : 'Sign In'}</span>
          <ArrowRight className="w-4 h-4" strokeWidth={2.5} />
        </button>
      </form>
    </div>
  );
};

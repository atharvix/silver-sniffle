import React from 'react';
import { ArrowRight, Check, X, Loader2 } from 'lucide-react';

interface PasswordStepProps {
  email: string;
  password: string;
  setPassword: (password: string) => void;
  authMode: 'sign_in' | 'sign_up';
  setAuthMode?: React.Dispatch<React.SetStateAction<'sign_in' | 'sign_up'>>;
  authError: string;
  setAuthError?: (err: string) => void;
  isSubmitting: boolean;
  onSubmit: (e: React.FormEvent) => void;
}

export const PasswordStep: React.FC<PasswordStepProps> = ({
  email,
  password,
  setPassword,
  authMode,
  setAuthMode,
  authError,
  setAuthError,
  isSubmitting,
  onSubmit,
}) => {
  const isLengthValid = password.length >= 8;
  const hasNumber = /\d/.test(password);
  const hasLetter = /[a-zA-Z]/.test(password);
  const isPasswordValid = isLengthValid && hasNumber && hasLetter;

  return (
    <div className="space-y-6 text-left">
      <div className="space-y-1.5">
        <h2 className="text-[34px] font-normal leading-[1.04]" style={{ letterSpacing: '-.045em' }}>
          {authMode === 'sign_up' ? 'Set your password' : 'Enter your password'}
        </h2>
        <p className="text-sm leading-relaxed" style={{ color: 'var(--muted)' }}>
          {authMode === 'sign_up' ? 'Securing account for ' : 'Signing into '}
          <span style={{ color: 'var(--fg)', fontWeight: 500 }}>{email}</span>
        </p>
      </div>

      <form onSubmit={onSubmit} className="space-y-4" noValidate>
        <div className="field">
          <input
            id="authPassword"
            type="password"
            required
            autoComplete={authMode === 'sign_up' ? 'new-password' : 'current-password'}
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            placeholder=" "
          />
          <label htmlFor="authPassword">Password</label>
        </div>

        {/* Password Requirements Checklist (Sign-up only) */}
        {authMode === 'sign_up' && (
          <div className="p-4 space-y-2.5 text-sm" style={{ background: 'var(--surface)', borderRadius: 0 }}>
            <p className="text-[11px] tracking-wider uppercase mb-1" style={{ color: 'var(--faint)' }}>
              Password requirements
            </p>

            <div className="flex items-center gap-2.5">
              {isLengthValid ? (
                <Check className="w-3.5 h-3.5 shrink-0" strokeWidth={2.5} />
              ) : (
                <X className="w-3.5 h-3.5 shrink-0" style={{ color: 'var(--faint)' }} strokeWidth={2} />
              )}
              <span style={{ color: isLengthValid ? 'var(--fg)' : 'var(--faint)', fontWeight: isLengthValid ? 500 : 400 }}>
                Minimum 8 characters
              </span>
            </div>

            <div className="flex items-center gap-2.5">
              {hasNumber ? (
                <Check className="w-3.5 h-3.5 shrink-0" strokeWidth={2.5} />
              ) : (
                <X className="w-3.5 h-3.5 shrink-0" style={{ color: 'var(--faint)' }} strokeWidth={2} />
              )}
              <span style={{ color: hasNumber ? 'var(--fg)' : 'var(--faint)', fontWeight: hasNumber ? 500 : 400 }}>
                At least one number (0–9)
              </span>
            </div>

            <div className="flex items-center gap-2.5">
              {hasLetter ? (
                <Check className="w-3.5 h-3.5 shrink-0" strokeWidth={2.5} />
              ) : (
                <X className="w-3.5 h-3.5 shrink-0" style={{ color: 'var(--faint)' }} strokeWidth={2} />
              )}
              <span style={{ color: hasLetter ? 'var(--fg)' : 'var(--faint)', fontWeight: hasLetter ? 500 : 400 }}>
                At least one letter (A–Z)
              </span>
            </div>
          </div>
        )}

        {authError && (
          <p className="text-sm font-normal leading-relaxed text-left" style={{ color: 'var(--danger)' }}>
            {authError}
          </p>
        )}

        <button
          type="submit"
          disabled={isSubmitting || (authMode === 'sign_up' && !isPasswordValid) || !password}
          className="btn"
        >
          {isSubmitting ? (
            <>
              <Loader2 className="w-4 h-4 animate-spin" />
              <span>Authenticating…</span>
            </>
          ) : (
            <>
              <span>{authMode === 'sign_up' ? 'Create account' : 'Sign in'}</span>
              <ArrowRight className="stroke" strokeWidth={2} />
            </>
          )}
        </button>
      </form>

      {setAuthMode && (
        <button
          type="button"
          onClick={() => {
            setAuthMode((mode) => (mode === 'sign_in' ? 'sign_up' : 'sign_in'));
            if (setAuthError) setAuthError('');
          }}
          className="w-full text-sm transition-colors text-center pt-1"
          style={{ color: 'var(--muted)' }}
        >
          {authMode === 'sign_in' ? (
            <span>
              Don't have an account?{' '}
              <strong className="font-bold underline" style={{ color: 'var(--fg)' }}>
                Sign up
              </strong>
            </span>
          ) : (
            <span>
              Already have an account?{' '}
              <strong className="font-bold underline" style={{ color: 'var(--fg)' }}>
                Sign in
              </strong>
            </span>
          )}
        </button>
      )}
    </div>
  );
};

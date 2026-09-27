import React from 'react';
import { ArrowRight, Loader2 } from 'lucide-react';

interface EmailStepProps {
  email: string;
  setEmail: (email: string) => void;
  authMode: 'sign_in' | 'sign_up';
  setAuthMode: React.Dispatch<React.SetStateAction<'sign_in' | 'sign_up'>>;
  authError: string;
  setAuthError: (error: string) => void;
  onSubmit: (e: React.FormEvent) => void;
  onGoogleAuth: () => void;
  isChecking?: boolean;
}

export const EmailStep: React.FC<EmailStepProps> = ({
  email,
  setEmail,
  authMode,
  setAuthMode,
  authError,
  setAuthError,
  onSubmit,
  onGoogleAuth,
  isChecking = false,
}) => {
  return (
    <div className="space-y-6">
      {/* Editorial Headline matching reference typography */}
      <div className="text-left pt-1 pb-1">
        <h1
          className="text-[38px] sm:text-[44px] font-normal leading-[1.08]"
          style={{ color: 'var(--fg)', letterSpacing: '-0.035em' }}
        >
          Build networks.<br />
          Within 30 meters.
        </h1>
      </div>

      {/* Sign In vs Create Account Toggle */}
      <div className="seg" style={{ gridTemplateColumns: '1fr 1fr', height: 40 }}>
        <button
          type="button"
          role="radio"
          aria-checked={authMode === 'sign_in'}
          className="font-bold"
          onClick={() => {
            setAuthMode('sign_in');
            setAuthError('');
          }}
        >
          Sign in
        </button>
        <button
          type="button"
          role="radio"
          aria-checked={authMode === 'sign_up'}
          className="font-bold"
          onClick={() => {
            setAuthMode('sign_up');
            setAuthError('');
          }}
        >
          Create account
        </button>
      </div>

      {/* Google Sign In */}
      <button type="button" onClick={onGoogleAuth} className="btn btn--line">
        <svg className="w-4 h-4" viewBox="0 0 24 24">
          <path fill="#EA4335" d="M12 5c1.6 0 3 .6 4.1 1.6l3.1-3.1C17.3 1.7 14.8 1 12 1 7.5 1 3.7 3.6 1.9 7.3l3.7 2.9C6.5 7.3 9 5 12 5z" />
          <path fill="#4285F4" d="M23.5 12.3c0-.8-.1-1.7-.2-2.3H12v4.6h6.5c-.3 1.5-1.1 2.8-2.4 3.7l3.7 2.9c2.2-2 3.7-5 3.7-8.9z" />
          <path fill="#FBBC05" d="M5.6 14.8c-.2-.7-.4-1.5-.4-2.3s.2-1.6.4-2.3L1.9 7.3C.7 9.7 0 12.3 0 15s.7 5.3 1.9 7.7l3.7-2.9c-.4-.7-.6-1.5-.6-2.3z" />
          <path fill="#34A853" d="M12 23c3.2 0 6-1.1 8-3l-3.7-2.9c-1.1.7-2.5 1.2-4.3 1.2-3 0-5.5-2.3-6.4-5.2L1.9 16C3.7 19.7 7.5 23 12 23z" />
        </svg>
        <span>Continue with Google</span>
      </button>

      <div className="relative flex items-center justify-center">
        <div className="w-full" style={{ borderTop: '1px solid var(--soft)' }} />
        <span className="absolute px-3 text-[11px] uppercase tracking-widest" style={{ background: 'var(--bg)', color: 'var(--faint)' }}>
          or continue with email
        </span>
      </div>

      <form onSubmit={onSubmit} className="space-y-4" noValidate>
        <div className="field">
          <input
            id="authEmail"
            type="email"
            required
            autoCapitalize="none"
            autoCorrect="off"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder=" "
          />
          <label htmlFor="authEmail">Email address</label>
        </div>

        {authError && (
          <p className="text-sm font-normal leading-relaxed text-left" style={{ color: 'var(--danger)' }}>
            {authError}
          </p>
        )}

        <button
          type="submit"
          disabled={isChecking || !email.trim()}
          className="btn"
        >
          {isChecking ? (
            <>
              <Loader2 className="w-4 h-4 animate-spin" />
              <span>Verifying…</span>
            </>
          ) : (
            <>
              <span>Continue</span>
              <ArrowRight className="stroke" strokeWidth={2} />
            </>
          )}
        </button>
      </form>
    </div>
  );
};

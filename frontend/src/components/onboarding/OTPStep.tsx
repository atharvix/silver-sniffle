import React from 'react';
import { ArrowRight, Loader2 } from 'lucide-react';

interface OTPStepProps {
  email: string;
  otp: string;
  setOtp: (otp: string) => void;
  authError: string;
  isSubmitting: boolean;
  onSubmit: (e: React.FormEvent) => void;
  onResend: () => void;
  isResending: boolean;
  /** Set only in development, when the backend has no mail provider to send through. */
  devOtp?: string;
}

export const OTPStep: React.FC<OTPStepProps> = ({
  email,
  otp,
  setOtp,
  authError,
  isSubmitting,
  onSubmit,
  onResend,
  isResending,
  devOtp,
}) => {
  return (
    <div className="space-y-6 text-left">
      <div className="space-y-1.5">
        <h2 className="text-[34px] font-normal leading-[1.04]" style={{ letterSpacing: '-.045em' }}>
          Verify your email
        </h2>
        <p className="text-sm leading-relaxed" style={{ color: 'var(--muted)' }}>
          Enter the 4-digit code sent to <span style={{ color: 'var(--fg)', fontWeight: 500 }}>{email}</span>
        </p>
        {devOtp && (
          <p className="text-xs leading-relaxed" style={{ color: 'var(--muted)' }}>
            Dev mode — no mail provider is configured, so your code is{' '}
            <strong style={{ color: 'var(--fg)', letterSpacing: '0.2em' }}>{devOtp}</strong>
          </p>
        )}
      </div>

      <form onSubmit={onSubmit} className="space-y-4" noValidate>
        <input
          type="text"
          inputMode="numeric"
          pattern="[0-9]*"
          maxLength={4}
          required
          autoFocus
          value={otp}
          onChange={(e) => setOtp(e.target.value.replace(/\D/g, ''))}
          placeholder="····"
          aria-label="4-digit code"
          className={`otp-input${authError ? ' invalid' : ''}`}
          style={{ letterSpacing: '0.6em' }}
        />

        {authError && (
          <p className="text-sm font-normal leading-relaxed text-left" style={{ color: 'var(--danger)' }}>
            {authError}
          </p>
        )}

        <button
          type="submit"
          disabled={isSubmitting || otp.length < 4}
          className="btn"
        >
          {isSubmitting ? (
            <>
              <Loader2 className="w-4 h-4 animate-spin" />
              <span>Verifying code…</span>
            </>
          ) : (
            <>
              <span>Verify &amp; continue</span>
              <ArrowRight className="stroke" strokeWidth={2} />
            </>
          )}
        </button>
      </form>

      <button
        type="button"
        onClick={onResend}
        disabled={isResending || isSubmitting}
        className="w-full text-sm transition-colors text-center disabled:opacity-40 pt-1"
        style={{ color: 'var(--muted)' }}
      >
        {isResending ? (
          'Sending new code…'
        ) : (
          <span>
            Didn't receive the code?{' '}
            <strong className="font-bold underline" style={{ color: 'var(--fg)' }}>
              Resend
            </strong>
          </span>
        )}
      </button>
    </div>
  );
};

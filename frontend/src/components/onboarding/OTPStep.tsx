import React from 'react';
import { ArrowRight } from 'lucide-react';

interface OTPStepProps {
  email: string;
  otp: string;
  setOtp: (otp: string) => void;
  authError: string;
  isSubmitting: boolean;
  onSubmit: (e: React.FormEvent) => void;
}

export const OTPStep: React.FC<OTPStepProps> = ({
  email,
  otp,
  setOtp,
  authError,
  isSubmitting,
  onSubmit,
}) => {
  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-xl font-bold text-white tracking-tight leading-snug">
          Verify Email
        </h2>
        <p className="text-xs text-white/50 mt-1 font-normal">
          Enter code sent to <span className="text-white font-medium">{email}</span>
        </p>
      </div>

      <form onSubmit={onSubmit} className="space-y-4">
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
  );
};

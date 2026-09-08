import React from 'react';
import { ArrowRight } from 'lucide-react';

interface EmailStepProps {
  email: string;
  setEmail: (email: string) => void;
  authMode: 'sign_in' | 'sign_up';
  setAuthMode: React.Dispatch<React.SetStateAction<'sign_in' | 'sign_up'>>;
  authError: string;
  setAuthError: (error: string) => void;
  onSubmit: (e: React.FormEvent) => void;
  onGoogleAuth: () => void;
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
}) => {
  return (
    <div className="space-y-6">
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
        <span className="absolute px-3 bg-[#060606] text-xs font-semibold text-white/30 uppercase tracking-widest">
          or
        </span>
      </div>

      <form onSubmit={onSubmit} className="space-y-4">
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

        {authError && <p className="text-xs text-red-400 font-medium">{authError}</p>}

        <button
          type="submit"
          className="w-full flex items-center justify-center gap-2 py-4 px-4 rounded-2xl bg-white hover:bg-neutral-200 text-black font-bold text-sm transition-all active:scale-[0.98] shadow-lg mt-4"
        >
          <span>Continue</span>
          <ArrowRight className="w-4 h-4" strokeWidth={2.5} />
        </button>
      </form>

      <button
        type="button"
        onClick={() => {
          setAuthMode((mode) => (mode === 'sign_in' ? 'sign_up' : 'sign_in'));
          setAuthError('');
        }}
        className="w-full text-xs text-white/50 hover:text-white transition-colors"
      >
        {authMode === 'sign_in' ? 'Need an account? Sign up' : 'Already have an account? Sign in'}
      </button>
    </div>
  );
};

import { useRef, useState, useEffect, useCallback, KeyboardEvent, ClipboardEvent } from 'react';

interface Props {
  onClose: () => void;
}

type Step = 'email' | 'otp' | 'success';

export default function EmailVerificationModal({ onClose }: Props) {
  const [step, setStep] = useState<Step>('email');
  const [email, setEmail] = useState('');
  const [otp, setOtp] = useState<string[]>(['', '', '', '']);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [devOtp, setDevOtp] = useState<string | null>(null);
  const [countdown, setCountdown] = useState(0);

  const emailRef = useRef<HTMLInputElement>(null);
  const otpRefs = [
    useRef<HTMLInputElement>(null),
    useRef<HTMLInputElement>(null),
    useRef<HTMLInputElement>(null),
    useRef<HTMLInputElement>(null),
  ];

  useEffect(() => { setTimeout(() => emailRef.current?.focus(), 80); }, []);
  useEffect(() => {
    if (step === 'otp') setTimeout(() => otpRefs[0].current?.focus(), 80);
  }, [step]);
  useEffect(() => {
    if (countdown <= 0) return;
    const t = setTimeout(() => setCountdown(c => c - 1), 1000);
    return () => clearTimeout(t);
  }, [countdown]);
  useEffect(() => {
    const handler = (e: globalThis.KeyboardEvent) => { if (e.key === 'Escape') onClose(); };
    document.addEventListener('keydown', handler);
    return () => document.removeEventListener('keydown', handler);
  }, [onClose]);

  const emailRe = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

  // Verify accepts an explicit code so it can be called right after a state build
  const submitVerify = useCallback(async (code: string) => {
    if (code.length < 4) { setError('Enter all 4 digits'); return; }
    setLoading(true);
    setError('');
    try {
      const res = await fetch('/api/auth/verify-otp', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email, otp: code }),
      });
      const data = await res.json();
      if (!res.ok) {
        setError(data.error || 'Verification failed');
        setOtp(['', '', '', '']);
        setTimeout(() => otpRefs[0].current?.focus(), 50);
        return;
      }
      setStep('success');
    } catch {
      setError('Network error. Please try again.');
    } finally {
      setLoading(false);
    }
  }, [email]);

  async function handleSendOtp(e?: React.FormEvent) {
    e?.preventDefault();
    setError('');
    if (!emailRe.test(email)) {
      setError('Enter a valid email address');
      return;
    }
    setLoading(true);
    try {
      const res = await fetch('/api/auth/send-otp', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email }),
      });
      const data = await res.json();
      if (!res.ok) { setError(data.error || 'Failed to send OTP'); return; }
      setDevOtp(data.devOtp ?? null);
      setOtp(['', '', '', '']);
      setStep('otp');
      setCountdown(30);
    } catch {
      setError('Network error. Please try again.');
    } finally {
      setLoading(false);
    }
  }

  function handleVerifySubmit(e?: React.FormEvent) {
    e?.preventDefault();
    submitVerify(otp.join(''));
  }

  function handleOtpChange(index: number, value: string) {
    const digit = value.replace(/\D/g, '').slice(-1);
    const next = [...otp];
    next[index] = digit;
    setOtp(next);
    setError('');
    if (digit && index < 3) {
      otpRefs[index + 1].current?.focus();
    }
    // Auto-submit using the freshly built array, not stale state
    if (digit && index === 3) {
      const code = next.join('');
      if (code.length === 4) submitVerify(code);
    }
  }

  function handleOtpKeyDown(index: number, e: KeyboardEvent<HTMLInputElement>) {
    if (e.key === 'Backspace') {
      if (otp[index]) {
        const next = [...otp]; next[index] = ''; setOtp(next);
      } else if (index > 0) {
        otpRefs[index - 1].current?.focus();
        const next = [...otp]; next[index - 1] = ''; setOtp(next);
      }
    }
  }

  function handleOtpPaste(e: ClipboardEvent<HTMLInputElement>) {
    e.preventDefault();
    const pasted = e.clipboardData.getData('text').replace(/\D/g, '').slice(0, 4);
    if (!pasted) return;
    const next: string[] = ['', '', '', ''];
    for (let i = 0; i < pasted.length; i++) next[i] = pasted[i];
    setOtp(next);
    const focusIdx = Math.min(pasted.length, 3);
    otpRefs[focusIdx].current?.focus();
    // Auto-submit using the freshly built array
    if (pasted.length === 4) submitVerify(next.join(''));
  }

  // Truncate long email for display
  const displayEmail = email.length > 28 ? email.slice(0, 25) + '…' : email;

  return (
    <div style={styles.overlay} onClick={onClose}>
      <div style={styles.modal} onClick={e => e.stopPropagation()}>
        <button style={styles.closeBtn} onClick={onClose} aria-label="Close">
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round">
            <line x1="18" y1="6" x2="6" y2="18" /><line x1="6" y1="6" x2="18" y2="18" />
          </svg>
        </button>

        {step === 'email' && (
          <form onSubmit={handleSendOtp} style={styles.form}>
            <div style={styles.iconWrap}>
              <svg width="26" height="26" viewBox="0 0 24 24" fill="none" stroke="#fff" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <rect x="2" y="4" width="20" height="16" rx="2" />
                <path d="m22 7-8.97 5.7a1.94 1.94 0 0 1-2.06 0L2 7" />
              </svg>
            </div>
            <h2 style={styles.title}>Enter your email</h2>
            <p style={styles.subtitle}>We'll send a 4-digit code to verify your address.</p>

            <input
              ref={emailRef}
              type="email"
              inputMode="email"
              placeholder="you@example.com"
              autoComplete="email"
              value={email}
              onChange={e => { setEmail(e.target.value.trim()); setError(''); }}
              style={styles.emailInput}
            />

            {error && <p style={styles.error}>{error}</p>}

            <button
              type="submit"
              disabled={loading || !emailRe.test(email)}
              style={{ ...styles.primaryBtn, opacity: (loading || !emailRe.test(email)) ? 0.5 : 1 }}
            >
              {loading ? 'Sending…' : 'Send Code'}
            </button>
          </form>
        )}

        {step === 'otp' && (
          <form onSubmit={handleVerifySubmit} style={styles.form}>
            <div style={{ ...styles.iconWrap, background: '#30d158' }}>
              <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="#fff" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
                <rect x="5" y="11" width="14" height="10" rx="2" />
                <path d="M8 11V7a4 4 0 1 1 8 0v4" />
              </svg>
            </div>
            <h2 style={styles.title}>Enter the code</h2>
            <p style={styles.subtitle}>
              Sent to <strong style={{ color: '#fff' }}>{displayEmail}</strong>
              {devOtp && (
                <span style={styles.devBadge}>&nbsp;· Demo code: <strong>{devOtp}</strong></span>
              )}
            </p>

            <div style={styles.otpRow}>
              {otp.map((digit, i) => (
                <input
                  key={i}
                  ref={otpRefs[i]}
                  type="tel"
                  inputMode="numeric"
                  maxLength={1}
                  value={digit}
                  onChange={e => handleOtpChange(i, e.target.value)}
                  onKeyDown={e => handleOtpKeyDown(i, e)}
                  onPaste={i === 0 ? handleOtpPaste : undefined}
                  style={{
                    ...styles.otpBox,
                    borderColor: digit ? 'rgba(255,255,255,0.6)' : 'rgba(255,255,255,0.2)',
                    background: digit ? 'rgba(255,255,255,0.12)' : 'rgba(255,255,255,0.06)',
                  }}
                />
              ))}
            </div>

            {error && <p style={styles.error}>{error}</p>}

            <button
              type="submit"
              disabled={loading || otp.join('').length < 4}
              style={{ ...styles.primaryBtn, opacity: (loading || otp.join('').length < 4) ? 0.5 : 1 }}
            >
              {loading ? 'Verifying…' : 'Verify'}
            </button>

            <div style={styles.resendRow}>
              {countdown > 0 ? (
                <span style={styles.resendNote}>Resend in {countdown}s</span>
              ) : (
                <button type="button" style={styles.resendBtn}
                  onClick={() => { setOtp(['', '', '', '']); setError(''); handleSendOtp(); }}>
                  Resend code
                </button>
              )}
              <button type="button" style={styles.changeBtn}
                onClick={() => { setStep('email'); setOtp(['', '', '', '']); setError(''); }}>
                Change email
              </button>
            </div>
          </form>
        )}

        {step === 'success' && (
          <div style={{ ...styles.form, alignItems: 'center', textAlign: 'center' }}>
            <div style={{ ...styles.iconWrap, background: '#30d158', width: 64, height: 64 }}>
              <svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="#fff" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
                <polyline points="20 6 9 17 4 12" />
              </svg>
            </div>
            <h2 style={{ ...styles.title, marginTop: 8 }}>You're verified!</h2>
            <p style={{ ...styles.subtitle, textAlign: 'center' }}>
              Welcome to Series. Find your people<br />on iMessage.
            </p>
            <button style={styles.primaryBtn} onClick={onClose}>Get Started</button>
          </div>
        )}
      </div>
    </div>
  );
}

const styles: Record<string, React.CSSProperties> = {
  overlay: {
    position: 'fixed',
    inset: 0,
    background: 'rgba(0,0,0,0.72)',
    backdropFilter: 'blur(6px)',
    WebkitBackdropFilter: 'blur(6px)',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    zIndex: 1000,
  },
  modal: {
    position: 'relative',
    background: 'linear-gradient(145deg, #2a1206 0%, #1a0a06 60%, #110806 100%)',
    border: '1px solid rgba(255,255,255,0.12)',
    borderRadius: 20,
    padding: '44px 40px 40px',
    width: '100%',
    maxWidth: 420,
    boxShadow: '0 32px 80px rgba(0,0,0,0.7)',
  },
  closeBtn: {
    position: 'absolute',
    top: 16,
    right: 16,
    background: 'rgba(255,255,255,0.08)',
    border: '1px solid rgba(255,255,255,0.12)',
    borderRadius: 8,
    color: 'rgba(255,255,255,0.6)',
    cursor: 'pointer',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    width: 32,
    height: 32,
    padding: 0,
  },
  form: {
    display: 'flex',
    flexDirection: 'column',
    gap: 16,
  },
  iconWrap: {
    width: 52,
    height: 52,
    borderRadius: 14,
    background: 'rgba(255,255,255,0.1)',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    marginBottom: 4,
  },
  title: {
    fontSize: 24,
    fontWeight: 800,
    color: '#fff',
    letterSpacing: '-0.5px',
    margin: 0,
  },
  subtitle: {
    fontSize: 14,
    color: 'rgba(255,255,255,0.55)',
    lineHeight: 1.5,
    margin: 0,
  },
  emailInput: {
    width: '100%',
    padding: '14px 16px',
    fontSize: 16,
    fontWeight: 500,
    color: '#fff',
    background: 'rgba(255,255,255,0.07)',
    border: '1px solid rgba(255,255,255,0.18)',
    borderRadius: 12,
    outline: 'none',
    fontFamily: 'inherit',
    boxSizing: 'border-box',
    letterSpacing: '0.01em',
  },
  otpRow: {
    display: 'flex',
    gap: 12,
    justifyContent: 'center',
    marginTop: 8,
    marginBottom: 4,
  },
  otpBox: {
    width: 62,
    height: 68,
    borderRadius: 14,
    border: '1.5px solid rgba(255,255,255,0.2)',
    fontSize: 28,
    fontWeight: 700,
    color: '#fff',
    textAlign: 'center',
    background: 'rgba(255,255,255,0.06)',
    outline: 'none',
    fontFamily: 'inherit',
    transition: 'border-color 0.15s, background 0.15s',
  },
  error: {
    fontSize: 13,
    color: '#ff6b6b',
    margin: 0,
    lineHeight: 1.4,
  },
  primaryBtn: {
    padding: '14px 24px',
    background: '#fff',
    border: 'none',
    borderRadius: 12,
    fontSize: 16,
    fontWeight: 700,
    color: '#111',
    cursor: 'pointer',
    fontFamily: 'inherit',
    transition: 'opacity 0.15s',
    marginTop: 4,
  },
  resendRow: {
    display: 'flex',
    justifyContent: 'space-between',
    alignItems: 'center',
    marginTop: -4,
  },
  resendNote: {
    fontSize: 13,
    color: 'rgba(255,255,255,0.4)',
  },
  resendBtn: {
    fontSize: 13,
    color: 'rgba(255,255,255,0.65)',
    background: 'none',
    border: 'none',
    cursor: 'pointer',
    fontFamily: 'inherit',
    padding: 0,
    textDecoration: 'underline',
    textUnderlineOffset: 3,
  },
  changeBtn: {
    fontSize: 13,
    color: 'rgba(255,255,255,0.45)',
    background: 'none',
    border: 'none',
    cursor: 'pointer',
    fontFamily: 'inherit',
    padding: 0,
  },
  devBadge: {
    color: '#f0a040',
    fontStyle: 'normal',
  },
};

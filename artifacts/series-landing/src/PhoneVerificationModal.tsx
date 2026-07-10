import { useRef, useState, useEffect, useCallback, KeyboardEvent, ClipboardEvent } from 'react';

interface Props {
  onClose: () => void;
}

type Step = 'phone' | 'otp' | 'success';

export default function PhoneVerificationModal({ onClose }: Props) {
  const [step, setStep] = useState<Step>('phone');
  const [phone, setPhone] = useState('');
  const [otp, setOtp] = useState<string[]>(['', '', '', '']);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [devOtp, setDevOtp] = useState<string | null>(null);
  const [countdown, setCountdown] = useState(0);

  const phoneRef = useRef<HTMLInputElement>(null);
  const otpRefs = [
    useRef<HTMLInputElement>(null),
    useRef<HTMLInputElement>(null),
    useRef<HTMLInputElement>(null),
    useRef<HTMLInputElement>(null),
  ];

  useEffect(() => { setTimeout(() => phoneRef.current?.focus(), 80); }, []);
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

  const indianPhoneRe = /^[6-9]\d{9}$/;

  // Verify accepts an explicit code so it can be called right after a state build
  const submitVerify = useCallback(async (code: string) => {
    if (code.length < 4) { setError('Enter all 4 digits'); return; }
    setLoading(true);
    setError('');
    try {
      const res = await fetch('/api/auth/verify-otp', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ phone, otp: code }),
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
  }, [phone]);

  async function handleSendOtp(e?: React.FormEvent) {
    e?.preventDefault();
    setError('');
    if (!indianPhoneRe.test(phone)) {
      setError('Enter a valid 10-digit number starting with 6–9');
      return;
    }
    setLoading(true);
    try {
      const res = await fetch('/api/auth/send-otp', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ phone }),
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

  return (
    <div style={styles.overlay} onClick={onClose}>
      <div style={styles.modal} onClick={e => e.stopPropagation()}>
        <button style={styles.closeBtn} onClick={onClose} aria-label="Close">
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round">
            <line x1="18" y1="6" x2="6" y2="18" /><line x1="6" y1="6" x2="18" y2="18" />
          </svg>
        </button>

        {step === 'phone' && (
          <form onSubmit={handleSendOtp} style={styles.form}>
            <div style={styles.iconWrap}>
              <svg width="28" height="28" viewBox="0 0 24 24" fill="#fff">
                <path d="M12 2C6.48 2 2 6.48 2 12c0 1.85.5 3.58 1.37 5.06L2 22l4.94-1.37C8.42 21.5 10.15 22 12 22c5.52 0 10-4.48 10-10S17.52 2 12 2z"/>
              </svg>
            </div>
            <h2 style={styles.title}>Enter your number</h2>
            <p style={styles.subtitle}>We'll send a 4-digit code to verify your Indian mobile number.</p>

            <div style={styles.phoneRow}>
              <div style={styles.prefix}>+91</div>
              <input
                ref={phoneRef}
                type="tel"
                inputMode="numeric"
                placeholder="98765 43210"
                maxLength={10}
                value={phone}
                onChange={e => { setPhone(e.target.value.replace(/\D/g, '').slice(0, 10)); setError(''); }}
                style={styles.phoneInput}
              />
            </div>

            {error && <p style={styles.error}>{error}</p>}

            <button
              type="submit"
              disabled={loading || phone.length !== 10}
              style={{ ...styles.primaryBtn, opacity: (loading || phone.length !== 10) ? 0.5 : 1 }}
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
              Sent to <strong style={{ color: '#fff' }}>+91 {phone}</strong>
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
                onClick={() => { setStep('phone'); setOtp(['', '', '', '']); setError(''); }}>
                Change number
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
  phoneRow: {
    display: 'flex',
    alignItems: 'center',
    background: 'rgba(255,255,255,0.07)',
    border: '1px solid rgba(255,255,255,0.18)',
    borderRadius: 12,
    overflow: 'hidden',
    marginTop: 4,
  },
  prefix: {
    padding: '14px 14px 14px 16px',
    fontSize: 16,
    fontWeight: 600,
    color: 'rgba(255,255,255,0.8)',
    borderRight: '1px solid rgba(255,255,255,0.12)',
    background: 'rgba(255,255,255,0.05)',
    flexShrink: 0,
    userSelect: 'none',
  },
  phoneInput: {
    flex: 1,
    padding: '14px 16px',
    fontSize: 17,
    fontWeight: 500,
    color: '#fff',
    background: 'transparent',
    border: 'none',
    outline: 'none',
    letterSpacing: '0.02em',
    fontFamily: 'inherit',
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

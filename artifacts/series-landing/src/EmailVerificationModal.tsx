import { useRef, useState, useEffect, useCallback, KeyboardEvent, ClipboardEvent, ChangeEvent } from 'react';

interface Props {
  onClose: () => void;
}

type Step = 'email' | 'otp' | 'profile' | 'success';

export default function EmailVerificationModal({ onClose }: Props) {
  const [step, setStep] = useState<Step>('email');
  const [email, setEmail] = useState('');
  const [otp, setOtp] = useState<string[]>(['', '', '', '']);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [devOtp, setDevOtp] = useState<string | null>(null);
  const [countdown, setCountdown] = useState(0);

  // Profile fields
  const [photoUrl, setPhotoUrl] = useState<string | null>(null);
  const [name, setName] = useState('');
  const [about, setAbout] = useState('');
  const [photoDragging, setPhotoDragging] = useState(false);
  const [photoError, setPhotoError] = useState('');

  const emailRef  = useRef<HTMLInputElement>(null);
  const nameRef   = useRef<HTMLInputElement>(null);
  const fileRef   = useRef<HTMLInputElement>(null);
  const otpRefs   = [
    useRef<HTMLInputElement>(null),
    useRef<HTMLInputElement>(null),
    useRef<HTMLInputElement>(null),
    useRef<HTMLInputElement>(null),
  ];

  useEffect(() => { setTimeout(() => emailRef.current?.focus(), 80); }, []);
  useEffect(() => {
    if (step === 'otp')     setTimeout(() => otpRefs[0].current?.focus(), 80);
    if (step === 'profile') setTimeout(() => nameRef.current?.focus(), 80);
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

  // ── OTP helpers ─────────────────────────────────────────────────────────────

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
      setStep('profile');
    } catch {
      setError('Network error. Please try again.');
    } finally {
      setLoading(false);
    }
  }, [email]);

  async function handleSendOtp(e?: React.FormEvent) {
    e?.preventDefault();
    setError('');
    if (!emailRe.test(email)) { setError('Enter a valid email address'); return; }
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
    const next  = [...otp];
    next[index] = digit;
    setOtp(next);
    setError('');
    if (digit && index < 3) otpRefs[index + 1].current?.focus();
    if (digit && index === 3) { const code = next.join(''); if (code.length === 4) submitVerify(code); }
  }

  function handleOtpKeyDown(index: number, e: KeyboardEvent<HTMLInputElement>) {
    if (e.key === 'Backspace') {
      if (otp[index]) { const n = [...otp]; n[index] = ''; setOtp(n); }
      else if (index > 0) { otpRefs[index - 1].current?.focus(); const n = [...otp]; n[index - 1] = ''; setOtp(n); }
    }
  }

  function handleOtpPaste(e: ClipboardEvent<HTMLInputElement>) {
    e.preventDefault();
    const pasted = e.clipboardData.getData('text').replace(/\D/g, '').slice(0, 4);
    if (!pasted) return;
    const next: string[] = ['', '', '', ''];
    for (let i = 0; i < pasted.length; i++) next[i] = pasted[i];
    setOtp(next);
    otpRefs[Math.min(pasted.length, 3)].current?.focus();
    if (pasted.length === 4) submitVerify(next.join(''));
  }

  // ── Profile helpers ──────────────────────────────────────────────────────────

  const MAX_PHOTO_BYTES = 8 * 1024 * 1024; // 8 MB

  function applyPhoto(file: File) {
    setPhotoError('');
    if (!file.type.startsWith('image/')) {
      setPhotoError('Please upload an image file (JPG, PNG, GIF, etc.).');
      return;
    }
    if (file.size > MAX_PHOTO_BYTES) {
      setPhotoError('Image must be under 8 MB.');
      return;
    }
    const reader = new FileReader();
    reader.onload  = e => setPhotoUrl(e.target?.result as string);
    reader.onerror = () => setPhotoError('Could not read the file. Please try another.');
    reader.onabort = () => setPhotoError('Upload was cancelled.');
    reader.readAsDataURL(file);
  }

  function handleFileChange(e: ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    if (file) applyPhoto(file);
    e.target.value = '';
  }

  function handlePhotoDrop(e: React.DragEvent) {
    e.preventDefault();
    setPhotoDragging(false);
    const file = e.dataTransfer.files?.[0];
    if (file) applyPhoto(file);
  }

  async function handleProfileSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!name.trim()) return;
    setLoading(true);
    setError('');
    try {
      await fetch('/api/auth/send-welcome', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email, name: name.trim(), about: about.trim() }),
      });
      // Non-fatal — proceed to success regardless of email delivery outcome
    } catch {
      // Silently ignore network errors; the user's profile is complete
    } finally {
      setLoading(false);
    }
    setStep('success');
  }

  const displayEmail  = email.length > 28 ? email.slice(0, 25) + '…' : email;
  const nameValid     = name.trim().length > 0;
  const ABOUT_MAX     = 160;

  // ── Render ───────────────────────────────────────────────────────────────────

  return (
    <div style={styles.overlay} onClick={onClose}>
      <div style={styles.modal} onClick={e => e.stopPropagation()}>
        <button style={styles.closeBtn} onClick={onClose} aria-label="Close">
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round">
            <line x1="18" y1="6" x2="6" y2="18" /><line x1="6" y1="6" x2="18" y2="18" />
          </svg>
        </button>

        {/* ── Step 1: Email ── */}
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

        {/* ── Step 2: OTP ── */}
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
              {devOtp && <span style={styles.devBadge}>&nbsp;· Demo code: <strong>{devOtp}</strong></span>}
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
                    background:  digit ? 'rgba(255,255,255,0.12)' : 'rgba(255,255,255,0.06)',
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
              {countdown > 0
                ? <span style={styles.resendNote}>Resend in {countdown}s</span>
                : <button type="button" style={styles.resendBtn}
                    onClick={() => { setOtp(['', '', '', '']); setError(''); handleSendOtp(); }}>
                    Resend code
                  </button>
              }
              <button type="button" style={styles.changeBtn}
                onClick={() => { setStep('email'); setOtp(['', '', '', '']); setError(''); }}>
                Change email
              </button>
            </div>
          </form>
        )}

        {/* ── Step 3: Profile ── */}
        {step === 'profile' && (
          <form onSubmit={handleProfileSubmit} style={styles.form}>
            <div style={styles.profileHeader}>
              <div style={styles.stepBadge}>
                <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="#30d158" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
                  <polyline points="20 6 9 17 4 12" />
                </svg>
                <span style={{ color: '#30d158', fontSize: 12, fontWeight: 600 }}>Verified</span>
              </div>
              <h2 style={styles.title}>Set up your profile</h2>
              <p style={styles.subtitle}>This is how you'll appear to others on Series.</p>
            </div>

            {/* Photo picker — native label drives the hidden file input for keyboard/screen-reader support */}
            <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 8, marginTop: 4 }}>
              <label
                htmlFor="photo-upload"
                aria-label="Upload profile photo"
                style={{
                  ...styles.photoRing,
                  borderColor: photoDragging ? 'rgba(255,255,255,0.6)' : 'rgba(255,255,255,0.18)',
                  background:  photoDragging ? 'rgba(255,255,255,0.08)' : 'rgba(255,255,255,0.04)',
                  cursor: 'pointer',
                }}
                onDragOver={e => { e.preventDefault(); setPhotoDragging(true); }}
                onDragLeave={() => setPhotoDragging(false)}
                onDrop={handlePhotoDrop}
              >
                <input
                  ref={fileRef}
                  id="photo-upload"
                  type="file"
                  accept="image/*"
                  style={{ display: 'none' }}
                  onChange={handleFileChange}
                />
                {photoUrl ? (
                  <img src={photoUrl} alt="Profile preview" style={styles.photoImg} />
                ) : (
                  <div style={styles.photoPlaceholder}>
                    <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="rgba(255,255,255,0.4)" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
                      <circle cx="12" cy="8" r="4" />
                      <path d="M4 20c0-4 3.6-7 8-7s8 3 8 7" />
                    </svg>
                  </div>
                )}
                <div style={styles.photoBadge}>
                  <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="#111" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
                    <path d="M23 19a2 2 0 0 1-2 2H3a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h4l2-3h6l2 3h4a2 2 0 0 1 2 2z" />
                    <circle cx="12" cy="13" r="4" />
                  </svg>
                </div>
              </label>
              {photoError
                ? <p style={{ ...styles.error, textAlign: 'center', marginTop: 0 }}>{photoError}</p>
                : <p style={{ ...styles.subtitle, textAlign: 'center', fontSize: 12, margin: 0 }}>
                    {photoUrl
                      ? <button type="button" style={styles.changeBtn} onClick={() => fileRef.current?.click()}>Change photo</button>
                      : 'Click or drag to upload a photo'}
                  </p>
              }
            </div>

            {/* Name */}
            <div style={styles.fieldGroup}>
              <label style={styles.label} htmlFor="profile-name">Name <span style={{ color: '#ff6b6b' }}>*</span></label>
              <input
                ref={nameRef}
                id="profile-name"
                type="text"
                placeholder="Your full name"
                maxLength={60}
                value={name}
                onChange={e => setName(e.target.value)}
                style={styles.emailInput}
                autoComplete="name"
              />
            </div>

            {/* About */}
            <div style={styles.fieldGroup}>
              <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline' }}>
                <label style={styles.label} htmlFor="profile-about">About</label>
                <span style={{ fontSize: 11, color: about.length >= ABOUT_MAX ? '#ff6b6b' : 'rgba(255,255,255,0.3)' }}>
                  {about.length}/{ABOUT_MAX}
                </span>
              </div>
              <textarea
                id="profile-about"
                placeholder="A short bio — interests, what you're looking for…"
                maxLength={ABOUT_MAX}
                rows={3}
                value={about}
                onChange={e => setAbout(e.target.value)}
                style={styles.textarea}
              />
            </div>

            <button
              type="submit"
              disabled={!nameValid}
              style={{ ...styles.primaryBtn, opacity: nameValid ? 1 : 0.45, marginTop: 4 }}
            >
              Continue
            </button>
          </form>
        )}

        {/* ── Step 4: Success ── */}
        {step === 'success' && (
          <div style={{ ...styles.form, alignItems: 'center', textAlign: 'center' }}>
            {photoUrl ? (
              <img src={photoUrl} alt={name} style={styles.successPhoto} />
            ) : (
              <div style={{ ...styles.iconWrap, background: '#30d158', width: 64, height: 64 }}>
                <svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="#fff" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
                  <polyline points="20 6 9 17 4 12" />
                </svg>
              </div>
            )}
            <h2 style={{ ...styles.title, marginTop: 8 }}>Welcome, {name.trim().split(/\s+/)[0]}!</h2>
            <p style={{ ...styles.subtitle, textAlign: 'center' }}>
              Your profile is all set.<br />Find your people on iMessage.
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
    padding: '13px 16px',
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
  // Profile step
  profileHeader: {
    display: 'flex',
    flexDirection: 'column',
    gap: 6,
  },
  stepBadge: {
    display: 'inline-flex',
    alignItems: 'center',
    gap: 5,
    background: 'rgba(48,209,88,0.12)',
    border: '1px solid rgba(48,209,88,0.25)',
    borderRadius: 20,
    padding: '3px 10px 3px 7px',
    width: 'fit-content',
    marginBottom: 2,
  },
  photoRing: {
    position: 'relative',
    width: 88,
    height: 88,
    borderRadius: '50%',
    border: '2px dashed rgba(255,255,255,0.18)',
    cursor: 'pointer',
    overflow: 'visible',
    transition: 'border-color 0.15s, background 0.15s',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
  },
  photoPlaceholder: {
    width: 84,
    height: 84,
    borderRadius: '50%',
    background: 'rgba(255,255,255,0.06)',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
  },
  photoImg: {
    width: 84,
    height: 84,
    borderRadius: '50%',
    objectFit: 'cover',
    display: 'block',
  },
  photoBadge: {
    position: 'absolute',
    bottom: 2,
    right: 2,
    width: 26,
    height: 26,
    borderRadius: '50%',
    background: '#fff',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    boxShadow: '0 2px 6px rgba(0,0,0,0.4)',
    pointerEvents: 'none',
  },
  fieldGroup: {
    display: 'flex',
    flexDirection: 'column',
    gap: 7,
  },
  label: {
    fontSize: 13,
    fontWeight: 600,
    color: 'rgba(255,255,255,0.65)',
    letterSpacing: '0.01em',
  },
  textarea: {
    width: '100%',
    padding: '13px 16px',
    fontSize: 15,
    fontWeight: 400,
    color: '#fff',
    background: 'rgba(255,255,255,0.07)',
    border: '1px solid rgba(255,255,255,0.18)',
    borderRadius: 12,
    outline: 'none',
    fontFamily: 'inherit',
    boxSizing: 'border-box',
    resize: 'none',
    lineHeight: 1.5,
  },
  successPhoto: {
    width: 72,
    height: 72,
    borderRadius: '50%',
    objectFit: 'cover',
    border: '2px solid rgba(48,209,88,0.6)',
  },
};

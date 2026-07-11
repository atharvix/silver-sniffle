import { useRef, useState, useEffect, useCallback, KeyboardEvent, ClipboardEvent, ChangeEvent } from 'react';
import { useUpsertProfile, useUpdateLocation } from '@workspace/api-client-react';

interface Props {
  onClose: () => void;
  onDiscovery: () => void;
  initialStep?: Step;
}

type Step = 'email' | 'otp' | 'profile' | 'location';

// ── localStorage helpers ──────────────────────────────────────────────────────

const LS_TOKEN       = 'series_token';
const LS_EMAIL       = 'series_email';
const LS_HAS_PROFILE = 'series_has_profile';

// Clears only the (expired) verification token. Email + profile-completion
// flag are preserved so a returning user can silently re-verify instead of
// redoing the whole email → OTP → profile flow.
function clearExpiredToken() {
  localStorage.removeItem(LS_TOKEN);
}

// ── Component ─────────────────────────────────────────────────────────────────

export default function EmailVerificationModal({ onClose, onDiscovery, initialStep = 'email' }: Props) {
  const [step, setStep] = useState<Step>(initialStep);
  const [email, setEmail] = useState(() => localStorage.getItem(LS_EMAIL) ?? '');
  const [otp, setOtp]     = useState<string[]>(['', '', '', '']);
  const [loading, setLoading] = useState(false);
  const [error, setError]     = useState('');
  const [devOtp, setDevOtp]   = useState<string | null>(null);
  const [countdown, setCountdown] = useState(0);
  // Set while re-verifying an expired session (silent re-auth): tracks which
  // step to resume once the fresh OTP is verified — 'location' for a returning
  // user whose profile is already saved, 'profile' when the expired token hit
  // mid profile-save so the entered name/about/photo should be resubmitted.
  const [reverifyTarget, setReverifyTarget] = useState<'profile' | 'location' | null>(null);
  const isReverify = reverifyTarget !== null;

  // Profile fields
  const [photoUrl, setPhotoUrl]         = useState<string | null>(null);
  const [name, setName]                 = useState('');
  const [about, setAbout]               = useState('');
  const [photoDragging, setPhotoDragging] = useState(false);
  const [photoError, setPhotoError]     = useState('');

  // Location fields
  // 'checking' = probing existing browser permission before showing any UI —
  // lets a returning user with permission already granted glide straight
  // through to the dashboard with no extra click.
  const [locState, setLocState] = useState<'checking' | 'idle' | 'requesting' | 'sending' | 'denied' | 'error'>('checking');
  const [locError, setLocError] = useState('');

  const emailRef = useRef<HTMLInputElement>(null);
  const nameRef  = useRef<HTMLInputElement>(null);
  const fileRef  = useRef<HTMLInputElement>(null);
  const otpRefs  = [
    useRef<HTMLInputElement>(null),
    useRef<HTMLInputElement>(null),
    useRef<HTMLInputElement>(null),
    useRef<HTMLInputElement>(null),
  ];

  const upsertProfile  = useUpsertProfile();
  const updateLocation = useUpdateLocation();

  // Focus management
  useEffect(() => { if (step === 'email') setTimeout(() => emailRef.current?.focus(), 80); }, []);
  useEffect(() => {
    if (step === 'otp')     setTimeout(() => otpRefs[0].current?.focus(), 80);
    if (step === 'profile') setTimeout(() => nameRef.current?.focus(), 80);
  }, [step]);

  // Countdown
  useEffect(() => {
    if (countdown <= 0) return;
    const t = setTimeout(() => setCountdown(c => c - 1), 1000);
    return () => clearTimeout(t);
  }, [countdown]);

  // Escape to close
  useEffect(() => {
    const handler = (e: globalThis.KeyboardEvent) => { if (e.key === 'Escape') onClose(); };
    document.addEventListener('keydown', handler);
    return () => document.removeEventListener('keydown', handler);
  }, [onClose]);

  const emailRe = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

  // ── OTP helpers ──────────────────────────────────────────────────────────────

  const submitVerify = useCallback(async (code: string) => {
    if (code.length < 4) { setError('Enter all 4 digits'); return; }
    setLoading(true);
    setError('');
    try {
      const res  = await fetch('/api/auth/verify-otp', {
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
      // Persist token and email for future sessions
      localStorage.setItem(LS_TOKEN, data.verificationToken);
      localStorage.setItem(LS_EMAIL, email);
      if (reverifyTarget === 'location') {
        // Returning user re-verifying an expired session — their profile is
        // already saved, so skip straight to location instead of re-asking.
        setReverifyTarget(null);
        setLocState('checking');
        setLocError('');
        setStep('location');
      } else if (reverifyTarget === 'profile') {
        // Expired token hit mid profile-save — resume on the profile step
        // with whatever the user had already entered (name/about/photo are
        // untouched local state, so nothing is lost).
        setReverifyTarget(null);
        setStep('profile');
      } else {
        setStep('profile');
      }
    } catch {
      setError('Network error. Please try again.');
    } finally {
      setLoading(false);
    }
  }, [email, reverifyTarget]);

  async function handleSendOtp(e?: React.FormEvent) {
    e?.preventDefault();
    setError('');
    if (!emailRe.test(email)) { setError('Enter a valid email address'); return; }
    setLoading(true);
    try {
      const res  = await fetch('/api/auth/send-otp', {
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

  // Silent re-verification: the stored token expired (401 from the server).
  // Rather than clearing everything and forcing email → OTP → profile again,
  // we already know the (verified-before) email, so just fire off a fresh
  // OTP and land on the OTP step. `target` is the step to resume once the
  // fresh code is verified — 'location' when the token expired while sharing
  // location (profile already saved), 'profile' when it expired mid
  // profile-save (so the profile step is shown again to resubmit).
  async function startReverify(target: 'profile' | 'location') {
    setReverifyTarget(target);
    setError('');
    setLoading(true);
    try {
      const res  = await fetch('/api/auth/send-otp', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email }),
      });
      const data = await res.json();
      if (!res.ok) {
        const msg = data.error || 'Could not resend a code. Please try again.';
        if (target === 'location') { setLocState('error'); setLocError(msg); }
        else setError(msg);
        setReverifyTarget(null);
        return;
      }
      setDevOtp(data.devOtp ?? null);
      setOtp(['', '', '', '']);
      setStep('otp');
      setCountdown(30);
    } catch {
      const msg = 'Network error. Please try again.';
      if (target === 'location') { setLocState('error'); setLocError(msg); }
      else setError(msg);
      setReverifyTarget(null);
    } finally {
      setLoading(false);
    }
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

  // ── Profile helpers ───────────────────────────────────────────────────────────

  const MAX_PHOTO_BYTES = 8 * 1024 * 1024;

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
      await upsertProfile.mutateAsync({
        data: {
          name: name.trim(),
          about: about.trim() || undefined,
          photo: photoUrl ?? undefined,
        },
      });
      localStorage.setItem(LS_HAS_PROFILE, 'true');

      // Send welcome email (non-fatal — never blocks the profile flow, but
      // failures are logged so a broken email pipeline doesn't go unnoticed).
      fetch('/api/auth/send-welcome', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email, name: name.trim(), about: about.trim() }),
      })
        .then(async res => {
          const data = await res.json().catch(() => ({}));
          // Server returns 2xx even when the email itself couldn't be sent
          // (so the profile flow is never blocked), so check the payload too.
          if (!res.ok || data?.error || /could not be sent/i.test(data?.message ?? '')) {
            console.error('[welcome-email] send-welcome failed', { status: res.status, ...data });
          }
        })
        .catch(err => {
          console.error('[welcome-email] send-welcome request errored', err);
        });

      setStep('location');
    } catch (err: unknown) {
      const isAuthError = (err as { status?: number })?.status === 401;
      if (isAuthError) {
        // Token expired mid profile-save — keep the email and entered
        // profile fields, and silently re-verify instead of clearing the
        // whole session and forcing email → OTP → profile again.
        clearExpiredToken();
        startReverify('profile');
      } else {
        setError('Could not save profile. Please try again.');
      }
    } finally {
      setLoading(false);
    }
  }

  // ── Location helpers ──────────────────────────────────────────────────────────

  // On arriving at the location step, silently probe whether the browser
  // already has geolocation permission (e.g. a returning user who granted it
  // last visit). If so, skip the "Allow Location Access" prompt entirely and
  // go straight through to the dashboard. Only fall back to showing the
  // button when permission genuinely needs to be requested or the Permissions
  // API isn't supported.
  useEffect(() => {
    if (step !== 'location' || locState !== 'checking') return;
    if (!navigator.permissions?.query) { setLocState('idle'); return; }

    let cancelled = false;
    navigator.permissions
      .query({ name: 'geolocation' as PermissionName })
      .then(status => {
        if (cancelled) return;
        if (status.state === 'granted') requestLocation();
        else setLocState('idle');
      })
      .catch(() => { if (!cancelled) setLocState('idle'); });

    return () => { cancelled = true; };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [step, locState]);

  async function requestLocation() {
    if (!navigator.geolocation) {
      setLocState('error');
      setLocError('Your browser doesn\'t support location. Try a modern browser.');
      return;
    }

    setLocState('requesting');
    setLocError('');

    navigator.geolocation.getCurrentPosition(
      async position => {
        setLocState('sending');
        try {
          await updateLocation.mutateAsync({
            data: {
              latitude:  position.coords.latitude,
              longitude: position.coords.longitude,
            },
          });
          onDiscovery();
        } catch (err: unknown) {
          const isAuthError = (err as { status?: number })?.status === 401;
          if (isAuthError) {
            // Token expired — keep the email/profile we already have and
            // silently re-verify instead of dumping the user back to step 1.
            clearExpiredToken();
            setLocState('idle');
            startReverify('location');
          } else {
            setLocState('error');
            setLocError('Could not share location. Please try again.');
          }
        }
      },
      geolocationError => {
        if (geolocationError.code === geolocationError.PERMISSION_DENIED) {
          setLocState('denied');
          setLocError('Location access was denied. Series uses your location only to show people within 30 metres — it\'s never stored beyond your session.');
        } else if (geolocationError.code === geolocationError.POSITION_UNAVAILABLE) {
          setLocState('error');
          setLocError('Location unavailable. Make sure location is enabled on your device.');
        } else {
          setLocState('error');
          setLocError('Location request timed out. Please try again.');
        }
      },
      { timeout: 15_000, maximumAge: 60_000 }
    );
  }

  // ── Derived ───────────────────────────────────────────────────────────────────

  const displayEmail = email.length > 28 ? email.slice(0, 25) + '…' : email;
  const nameValid    = name.trim().length > 0;
  const ABOUT_MAX    = 160;

  // ── Render ────────────────────────────────────────────────────────────────────

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
            <h2 style={styles.title}>{isReverify ? 'Re-verify your email' : 'Enter the code'}</h2>
            <p style={styles.subtitle}>
              {reverifyTarget === 'location'
                ? <>Your session expired. We sent a fresh code to <strong style={{ color: '#fff' }}>{displayEmail}</strong> — your profile is already saved.</>
                : reverifyTarget === 'profile'
                ? <>Your session expired. We sent a fresh code to <strong style={{ color: '#fff' }}>{displayEmail}</strong> — verify to finish setting up your profile.</>
                : <>Sent to <strong style={{ color: '#fff' }}>{displayEmail}</strong></>}
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
                onClick={() => { setReverifyTarget(null); setStep('email'); setOtp(['', '', '', '']); setError(''); }}>
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

            {/* Photo picker */}
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

            {error && <p style={styles.error}>{error}</p>}

            <button
              type="submit"
              disabled={loading || !nameValid}
              style={{ ...styles.primaryBtn, opacity: (loading || !nameValid) ? 0.45 : 1, marginTop: 4 }}
            >
              {loading ? 'Saving…' : 'Continue'}
            </button>
          </form>
        )}

        {/* ── Step 4: Location ── */}
        {step === 'location' && locState === 'checking' && (
          // Probing existing browser permission — kept minimal so a returning
          // user with permission already granted never sees a flash of the
          // "Allow Location Access" prompt before gliding into the dashboard.
          <div style={{ ...styles.form, alignItems: 'center', padding: '24px 0' }}>
            <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="rgba(255,255,255,0.5)" strokeWidth="2.5" strokeLinecap="round" style={{ animation: 'spin 1s linear infinite' }}>
              <path d="M21 12a9 9 0 1 1-6.219-8.56" />
            </svg>
            <p style={{ ...styles.subtitle, textAlign: 'center' }}>Setting things up…</p>
          </div>
        )}

        {step === 'location' && locState !== 'checking' && (
          <div style={styles.form}>
            <div style={{ ...styles.iconWrap, background: 'rgba(48,209,88,0.15)', border: '1px solid rgba(48,209,88,0.3)' }}>
              <svg width="26" height="26" viewBox="0 0 24 24" fill="none" stroke="#30d158" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <path d="M21 10c0 7-9 13-9 13s-9-6-9-13a9 9 0 0 1 18 0z" />
                <circle cx="12" cy="10" r="3" />
              </svg>
            </div>
            <h2 style={styles.title}>Find people nearby</h2>
            <p style={styles.subtitle}>
              Series uses your location <em>once</em> to show you people within 30 metres. Your exact position is never stored or shared.
            </p>

            {/* Privacy points */}
            <div style={locInfoStyles.bullets}>
              {[
                'Only used to match you with nearby people',
                'Never stored beyond this session',
                'Never shared with anyone',
              ].map(txt => (
                <div key={txt} style={locInfoStyles.bullet}>
                  <div style={locInfoStyles.bulletDot} />
                  <span style={{ fontSize: 13, color: 'rgba(255,255,255,0.6)', lineHeight: 1.4 }}>{txt}</span>
                </div>
              ))}
            </div>

            {/* Denied state */}
            {locState === 'denied' && (
              <div style={locInfoStyles.deniedBox}>
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="#ff9f0a" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" style={{ flexShrink: 0, marginTop: 2 }}>
                  <circle cx="12" cy="12" r="10" />
                  <line x1="12" y1="8" x2="12" y2="12" />
                  <line x1="12" y1="16" x2="12.01" y2="16" />
                </svg>
                <span style={{ fontSize: 13, color: 'rgba(255,255,255,0.75)', lineHeight: 1.5 }}>{locError}</span>
              </div>
            )}

            {/* Error state */}
            {locState === 'error' && (
              <p style={styles.error}>{locError}</p>
            )}

            <button
              onClick={requestLocation}
              disabled={locState === 'requesting' || locState === 'sending'}
              style={{
                ...styles.primaryBtn,
                opacity: (locState === 'requesting' || locState === 'sending') ? 0.6 : 1,
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                gap: 8,
              }}
            >
              {locState === 'requesting' && (
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" style={{ animation: 'spin 1s linear infinite' }}>
                  <path d="M21 12a9 9 0 1 1-6.219-8.56" />
                </svg>
              )}
              {locState === 'sending' && (
                <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" style={{ animation: 'spin 1s linear infinite' }}>
                  <path d="M21 12a9 9 0 1 1-6.219-8.56" />
                </svg>
              )}
              {locState === 'requesting' ? 'Waiting for permission…' :
               locState === 'sending'    ? 'Finding people…' :
               locState === 'denied'     ? 'Try again' :
               locState === 'error'      ? 'Retry' :
               'Allow Location Access'}
            </button>

            {(locState === 'denied' || locState === 'error') && (
              <p style={{ ...styles.subtitle, textAlign: 'center', fontSize: 12 }}>
                To allow access, check your browser's address bar or site settings.
              </p>
            )}
          </div>
        )}
      </div>

      <style>{`
        @keyframes spin {
          from { transform: rotate(0deg); }
          to   { transform: rotate(360deg); }
        }
      `}</style>
    </div>
  );
}

// ── Styles ────────────────────────────────────────────────────────────────────

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
    maxHeight: '90vh',
    overflowY: 'auto',
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
    background: 'rgba(255,255,255,0.06)',
    border: '1px solid rgba(255,255,255,0.15)',
    borderRadius: 10,
    padding: '12px 14px',
    fontSize: 16,
    color: '#fff',
    outline: 'none',
    width: '100%',
    fontFamily: 'inherit',
  },
  primaryBtn: {
    background: '#fff',
    color: '#111',
    border: 'none',
    borderRadius: 10,
    padding: '13px 0',
    fontSize: 15,
    fontWeight: 700,
    cursor: 'pointer',
    width: '100%',
    fontFamily: 'inherit',
    letterSpacing: '-0.2px',
  },
  error: {
    color: '#ff6b6b',
    fontSize: 13,
    margin: 0,
  },
  otpRow: {
    display: 'flex',
    gap: 10,
    justifyContent: 'center',
    marginTop: 4,
    marginBottom: 4,
  },
  otpBox: {
    width: 60,
    height: 64,
    borderRadius: 12,
    border: '1.5px solid rgba(255,255,255,0.2)',
    background: 'rgba(255,255,255,0.06)',
    color: '#fff',
    fontSize: 28,
    fontWeight: 700,
    textAlign: 'center',
    outline: 'none',
    fontFamily: 'inherit',
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
    background: 'none',
    border: 'none',
    color: 'rgba(255,255,255,0.6)',
    fontSize: 13,
    cursor: 'pointer',
    padding: 0,
    fontFamily: 'inherit',
  },
  changeBtn: {
    background: 'none',
    border: 'none',
    color: 'rgba(255,255,255,0.5)',
    fontSize: 13,
    cursor: 'pointer',
    padding: 0,
    fontFamily: 'inherit',
    textDecoration: 'underline',
  },
  devBadge: {
    fontSize: 12,
    color: '#30d158',
  },
  profileHeader: {
    display: 'flex',
    flexDirection: 'column',
    gap: 6,
  },
  stepBadge: {
    display: 'flex',
    alignItems: 'center',
    gap: 4,
  },
  photoRing: {
    width: 84,
    height: 84,
    borderRadius: '50%',
    border: '2px dashed rgba(255,255,255,0.18)',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    position: 'relative',
    overflow: 'visible',
  },
  photoImg: {
    width: 80,
    height: 80,
    borderRadius: '50%',
    objectFit: 'cover',
  },
  photoPlaceholder: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    width: '100%',
    height: '100%',
  },
  photoBadge: {
    position: 'absolute',
    bottom: 0,
    right: 0,
    width: 24,
    height: 24,
    borderRadius: '50%',
    background: '#fff',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    border: '2px solid #1a0a06',
  },
  fieldGroup: {
    display: 'flex',
    flexDirection: 'column',
    gap: 6,
  },
  label: {
    fontSize: 13,
    fontWeight: 600,
    color: 'rgba(255,255,255,0.7)',
    letterSpacing: '0.01em',
  },
  textarea: {
    background: 'rgba(255,255,255,0.06)',
    border: '1px solid rgba(255,255,255,0.15)',
    borderRadius: 10,
    padding: '10px 14px',
    fontSize: 14,
    color: '#fff',
    outline: 'none',
    width: '100%',
    resize: 'vertical',
    fontFamily: 'inherit',
    lineHeight: 1.5,
  },
  successPhoto: {
    width: 72,
    height: 72,
    borderRadius: '50%',
    objectFit: 'cover',
    border: '3px solid #30d158',
  },
};

const locInfoStyles: Record<string, React.CSSProperties> = {
  bullets: {
    display: 'flex',
    flexDirection: 'column',
    gap: 8,
    background: 'rgba(255,255,255,0.04)',
    border: '1px solid rgba(255,255,255,0.08)',
    borderRadius: 12,
    padding: '12px 14px',
  },
  bullet: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: 10,
  },
  bulletDot: {
    width: 6,
    height: 6,
    borderRadius: '50%',
    background: '#30d158',
    marginTop: 5,
    flexShrink: 0,
  },
  deniedBox: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: 10,
    background: 'rgba(255,159,10,0.08)',
    border: '1px solid rgba(255,159,10,0.25)',
    borderRadius: 10,
    padding: '12px 14px',
  },
};

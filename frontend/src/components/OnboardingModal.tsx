import React, { useState, useRef, useEffect, useCallback } from 'react';
import { Capacitor } from '@capacitor/core';
import { App as CapApp } from '@capacitor/app';
import { Geolocation } from '@capacitor/geolocation';
import { Camera } from '@capacitor/camera';
import { GoogleAuth } from '@shardev/capacitor-google-auth';
import { signIn, signUp, sendOtp, verifyOtp, saveProfile, googleSignIn, compressImage, resolvePhotoUrl, verifyFaceScan, requestFaceChallenge, checkEmail } from '../utils/api';
import { useToast } from './Toast';
import { captureFaceSnapshot, verifyUploadedPhotoMatch, LiveHumanTracker, extractFacialFeatures } from '../utils/faceDetector';
import { openAppSettings } from '../utils/nativeUi';

import { EmailStep } from './onboarding/EmailStep';
import { PasswordStep } from './onboarding/PasswordStep';
import { OTPStep } from './onboarding/OTPStep';
import { FaceVerificationStep } from './onboarding/FaceVerificationStep';
import { ProfileSetupStep } from './onboarding/ProfileSetupStep';
import { LocationPermissionStep } from './onboarding/LocationPermissionStep';

interface OnboardingModalProps {
  isOpen: boolean;
  onClose: () => void;
  onComplete: (userEmail?: string, token?: string) => void;
  onAuthenticated: (token: string, email: string, fallbackPhoto?: string, skipAutoClose?: boolean) => Promise<boolean> | boolean | void;
  onProfileSetupComplete?: (profileData: { name: string; avatar: string; email?: string; bio?: string; profession: string; lookingFor: string }) => void;
  initialStep?: AuthStep;
  initialProfile?: { name: string; avatar: string; bio?: string; profession?: string; lookingFor?: string };
}

export type AuthStep = 'email' | 'password' | 'create_password' | 'otp' | 'face_verification' | 'profile_setup' | 'location_permission';

/**
 * Reads the email claim out of a Google ID token, for display only. The token is
 * NOT trusted here — the backend independently verifies its signature, audience
 * and expiry before issuing a session. Name and photo are deliberately never
 * read from it: those must come from the user, not from their Google account.
 */
function decodeGoogleIdToken(idToken: string): { email?: string } | null {
  try {
    const payload = idToken.split('.')[1];
    if (!payload) return null;
    const normalized = payload.replace(/-/g, '+').replace(/_/g, '/');
    const json = decodeURIComponent(
      atob(normalized)
        .split('')
        .map((c) => '%' + ('00' + c.charCodeAt(0).toString(16)).slice(-2))
        .join('')
    );
    return JSON.parse(json);
  } catch {
    return null;
  }
}

export const OnboardingModal: React.FC<OnboardingModalProps> = ({
  isOpen,
  onClose,
  onComplete,
  onAuthenticated,
  onProfileSetupComplete,
  initialStep = 'email',
  initialProfile,
}) => {
  const [step, setStep] = useState<AuthStep>(initialStep);
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [authMode, setAuthMode] = useState<'sign_in' | 'sign_up'>('sign_in');
  const [otp, setOtp] = useState('');
  const [authError, setAuthError] = useState('');
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [isCheckingEmail, setIsCheckingEmail] = useState(false);
  const [isResendingOtp, setIsResendingOtp] = useState(false);
  // Development-only: the backend echoes the OTP when no mail provider exists.
  const [devOtp, setDevOtp] = useState('');
  const toast = useToast();

  // ─── Step History Stack for Back Navigation ─────────────────────────────────
  const [stepHistory, setStepHistory] = useState<AuthStep[]>([]);
  const [authTokenRef, setAuthTokenRef] = useState('');

  const isEditMode = initialStep === 'profile_setup';
  const activeSessionToken =
    authTokenRef || localStorage.getItem('kinjo_auth_token') || '';

  const goToStep = useCallback((nextStep: AuthStep) => {
    setStepHistory((prev) => [...prev, step]);
    setAuthError('');
    setStep(nextStep);
  }, [step]);

  const goBack = useCallback(() => {
    if (stepHistory.length === 0) {
      if (isEditMode) {
        onClose();
        return;
      }
      if (Capacitor.isNativePlatform()) {
        CapApp.minimizeApp();
      }
      return;
    }
    const prev = [...stepHistory];
    const previousStep = prev.pop()!;
    setStepHistory(prev);
    setAuthError('');
    setStep(previousStep);
  }, [stepHistory, isEditMode, onClose]);

  // Face Verification State
  const videoRef = useRef<HTMLVideoElement>(null);
  const [cameraActive, setCameraActive] = useState(false);
  const [scanStatus, setScanStatus] = useState('Preparing camera...');
  const [faceProgress, setFaceProgress] = useState(0);
  // Bumping this tears down and restarts the camera effect, which is how the
  // "Retry" action recovers after the user grants camera access in Settings.
  const [faceScanAttempt, setFaceScanAttempt] = useState(0);
  const [locationError, setLocationError] = useState('');
  const [isRequestingLocation, setIsRequestingLocation] = useState(false);
  const presenceFramesRef = useRef(0);
  const verifiedRef = useRef(false);
  // Single-use nonce the server issued for this scan; fetched when the step opens.
  const faceChallengeRef = useRef('');



  // Profile setup state
  // Always start empty. Edit mode repopulates these from the stored profile in
  // the effect below; onboarding never does, because a prefilled field is what
  // made an account-derived name look like the user's own choice.
  const [name, setName] = useState('');
  const [avatar, setAvatar] = useState('');
  const [bio, setBio] = useState('');
  const fileInputRef = useRef<HTMLInputElement>(null);

  const prevIsOpenRef = useRef(false);

  // Sync initial setup only when modal transitions from closed to open
  useEffect(() => {
    if (isOpen && !prevIsOpenRef.current) {
      setStep(initialStep);
      setStepHistory([]);
      setAuthError('');
      if (initialStep === 'email') {
        setEmail('');
        setPassword('');
        setOtp('');
        setAuthTokenRef('');
        setName('');
        setAvatar('');
        setBio('');
        setLocationError('');
      }
    }
    prevIsOpenRef.current = isOpen;
  }, [isOpen, initialStep]);

  // Prefilling from a stored profile is only correct when the user is explicitly
  // editing that profile. Onboarding is untouched by it.
  useEffect(() => {
    if (!isEditMode || !initialProfile) return;
    if (initialProfile.name) setName(initialProfile.name);
    if (initialProfile.avatar) setAvatar(initialProfile.avatar);
    if (initialProfile.bio) setBio(initialProfile.bio);
  }, [initialProfile, isEditMode]);

  // Register native & browser back button listeners
  useEffect(() => {
    if (!isOpen) return;

    let appBackHandle: any = null;
    if (Capacitor.isNativePlatform()) {
      void CapApp.addListener('backButton', () => {
        if (stepHistory.length > 0 || isEditMode) {
          goBack();
        } else {
          CapApp.minimizeApp();
        }
      }).then((handle) => {
        appBackHandle = handle;
      });
    }

    const handlePopState = () => {
      if (stepHistory.length > 0 || isEditMode) {
        goBack();
      }
    };
    window.addEventListener('popstate', handlePopState);

    return () => {
      if (appBackHandle) void appBackHandle.remove();
      window.removeEventListener('popstate', handlePopState);
    };
  }, [isOpen, stepHistory.length, goBack, isEditMode]);

  // Photo Upload Handler
  const handlePhotoUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;

    if (file.size > 15 * 1024 * 1024) {
      setAuthError('Image size exceeds 15MB limit. Please choose a smaller photo.');
      return;
    }

    try {
      setAuthError('');
      const compressedDataUrl = await compressImage(file);

      // Verify photo match with live face scan (at least 50% biometric match)
      let storedFeatures = localStorage.getItem('kinjo_face_features');
      const storedPhoto = localStorage.getItem('kinjo_face_photo');
      if (!storedFeatures && storedPhoto) {
        const img = new Image();
        img.src = storedPhoto;
        await new Promise((res) => { img.onload = res; img.onerror = res; });
        const feat = extractFacialFeatures(img);
        if (feat) {
          storedFeatures = JSON.stringify(feat);
          localStorage.setItem('kinjo_face_features', storedFeatures);
        }
      }

      if (storedFeatures) {
        try {
          const refFeatures = JSON.parse(storedFeatures);
          const match = await verifyUploadedPhotoMatch(compressedDataUrl, refFeatures, 0.50);
          if (!match.isMatch) {
            setAuthError(match.message);
            toast.error(match.message);
            return;
          }
          toast.success(match.message);
        } catch {}
      }

      setAvatar(compressedDataUrl);
      setAuthError('');
    } catch (err: any) {
      const msg = err?.message || 'Failed to process image. Please upload a clear photo.';
      setAuthError(msg);
      toast.error(msg);
    }
  };

  // Auth Submit Handlers
  const handlePasswordSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setAuthError('');
    setIsSubmitting(true);

    try {
      if (authMode === 'sign_up') {
        try {
          const signUpRes = await signUp(email, password);
          if (signUpRes?.devOtp) setDevOtp(signUpRes.devOtp);

          goToStep('otp');
        } catch (err: any) {
          const msg = err?.message || '';
          if (err?.status === 409 || msg.toLowerCase().includes('already exists')) {
            setAuthMode('sign_in');
            setAuthError('An account with this email already exists. Please enter your password to sign in.');
            return;
          }
          throw err;
        }
      } else {
        const resp = await signIn(email, password);
        const token = resp.verificationToken;
        setAuthTokenRef(token);
        const hasExisting = await onAuthenticated(token, email, undefined, false);
        if (hasExisting) {
          onComplete(email, token);
          onClose();
        } else {
          goToStep('face_verification');
        }
      }
    } catch (err: any) {
      const msg = err?.message || 'Authentication failed. Please check your credentials.';
      setAuthError(msg);
      toast.error(msg);
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleOtpSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setAuthError('');
    setIsSubmitting(true);

    try {
      const resp = await verifyOtp(email, otp);
      const token = resp.verificationToken;
      setAuthTokenRef(token);
      await onAuthenticated(token, email, undefined, true);

      goToStep('face_verification');
    } catch (err: any) {
      const msg = err?.message || 'Invalid verification code. Please check the code sent to your email.';
      setAuthError(msg);
      toast.error(msg);
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleResendOtp = async () => {
    setAuthError('');
    setIsResendingOtp(true);
    try {
      const res = await sendOtp(email);
      if (res?.devOtp) setDevOtp(res.devOtp);
      toast.success('A new verification code has been sent ✓');
    } catch (err: any) {
      const msg = err?.message || 'Could not resend the code. Please try again.';
      setAuthError(msg);
      toast.error(msg);
    } finally {
      setIsResendingOtp(false);
    }
  };

  const handleGoogleAuth = async () => {
    setAuthError('');
    setIsSubmitting(true);
    const googleClientId = import.meta.env.VITE_GOOGLE_CLIENT_ID || '469545347988-vsu4c3rvqh6tcelvm8c1sce13ea5dopc.apps.googleusercontent.com';

    try {
      // 1. Native Android / iOS via Capacitor Plugin
      if (Capacitor.isNativePlatform()) {
        await GoogleAuth.initialize({
          clientId: googleClientId,
          serverClientId: googleClientId,
          scopes: ['profile', 'email'],
        }).catch(() => {});

        await GoogleAuth.logout().catch(() => {});

        const loginRes = await GoogleAuth.login();
        // The backend verifies an ID token — an access token is not accepted.
        const idToken = loginRes.idToken || loginRes.account?.idToken;
        const googleEmail = loginRes.account?.email || '';

        if (!idToken) {
          setAuthError('Google sign-in did not return an ID token. Please try again or use email verification.');
          return;
        }

        const resp = await googleSignIn(idToken);
        const token = resp.verificationToken;
        setAuthTokenRef(token);

        setEmail(googleEmail);

        // Name and photo are never imported from the Google account — the user
        // types their own name and picks their own photo during onboarding.
        const hasExisting = await onAuthenticated(token, googleEmail, undefined, false);
        if (hasExisting) {            onComplete(googleEmail, token);
          onClose();
          return;
        }
        goToStep('face_verification');
        return;
      }

      // 2. Web Browser via Google Identity Services (One Tap).
      // The backend verifies a Google *ID token* (id_token), so the oauth2
      // token client (which yields an access token) must not be used here.
      const gsi = (window as any).google?.accounts?.id;
      if (gsi) {
        const idToken = await new Promise<string | null>((resolve) => {
          let settled = false;
          const settle = (value: string | null) => {
            if (!settled) {
              settled = true;
              resolve(value);
            }
          };

          gsi.initialize({
            client_id: googleClientId,
            callback: (response: any) => settle(response?.credential || null),
          });

          gsi.prompt((notification: any) => {
            // One Tap could not be shown or the user dismissed it
            if (notification?.isNotDisplayed?.() || notification?.isSkippedMoment?.()) {
              settle(null);
            }
          });
        });

        if (!idToken) {
          setAuthError('Google sign-in was dismissed. Please try again or use email verification.');
          return;
        }

        const claims = decodeGoogleIdToken(idToken);
        const googleEmail = claims?.email || '';
        setEmail(googleEmail);

        const resp = await googleSignIn(idToken);
        const token = resp.verificationToken;
        setAuthTokenRef(token);

        // Name and photo are never imported from the Google account.
        const hasExisting = await onAuthenticated(
          token,
          googleEmail || resp.email || '',
          undefined,
          false
        );
        if (hasExisting) {
          onComplete(googleEmail || resp.email, token);
          onClose();
          return;
        }
        goToStep('face_verification');
        return;
      }

      setAuthError('Google sign-in is unavailable in this browser. Please use email verification.');
    } catch (err: any) {
      const rawMsg = err?.message || err?.error || String(err);
      console.warn('Google sign-in error:', err);
      if (rawMsg.includes('10:') || rawMsg === '10' || rawMsg.includes('DEVELOPER_ERROR')) {
        setAuthError(
          'Google Sign-In Error 10 (DEVELOPER_ERROR): Please register the SHA-1 fingerprint of the signing keystore in your Google Cloud Console OAuth Client for package com.kinjo.app.'
        );
      } else if (!rawMsg.toLowerCase().includes('cancel') && !rawMsg.toLowerCase().includes('closed')) {
        setAuthError(rawMsg || 'Google sign in failed. Please try email verification.');
      }
    } finally {
      setIsSubmitting(false);
    }
  };

  // Face Verification Camera Effect (Live Facial Motion & Liveness Scan)
  useEffect(() => {
    if (step !== 'face_verification') {
      setCameraActive(false);
      return;
    }

    let stream: MediaStream | null = null;
    let animationFrameId: number;
    let cancelled = false;
    const canvas = document.createElement('canvas');
    canvas.width = 160;
    canvas.height = 120;

    presenceFramesRef.current = 0;
    verifiedRef.current = false;
    faceChallengeRef.current = '';
    setFaceProgress(0);

    // Ask the server for a one-shot nonce up front. It is only consumed when the
    // scan is submitted, so the capture cannot be replayed in a later session.
    const requestChallenge = async () => {
      const activeToken = authTokenRef || localStorage.getItem('kinjo_auth_token') || '';
      if (!activeToken) return;
      try {
        const res = await requestFaceChallenge(activeToken);
        if (!cancelled) faceChallengeRef.current = res?.challenge || '';
      } catch {
        if (!cancelled) {
          setScanStatus('Could not reach the server. Check your connection and try again.');
        }
      }
    };
    void requestChallenge();

    const tracker = new LiveHumanTracker();
    tracker.reset();

    const startFaceScan = async () => {
      try {
        setScanStatus('Initializing secure camera…');

        // getUserMedia inside the WebView still needs the runtime CAMERA
        // permission granted first, otherwise the request is rejected outright
        // and no dialog is ever shown. Ask for it explicitly so the prompt is
        // guaranteed to happen at this step.
        if (Capacitor.isNativePlatform()) {
          const cam = await Camera.requestPermissions({ permissions: ['camera'] });
          if (cam.camera !== 'granted') {
            setScanStatus('Camera permission denied. Allow Camera in Settings, then tap Retry.');
            return;
          }
        }

        stream = await navigator.mediaDevices.getUserMedia({
          video: { facingMode: 'user', width: { ideal: 640 }, height: { ideal: 480 } },
        });

        if (videoRef.current) {
          videoRef.current.srcObject = stream;
          await videoRef.current.play();
          setCameraActive(true);
        }

        setScanStatus('Center your face inside the oval');

        const detectFrame = () => {
          if (!videoRef.current || verifiedRef.current) return;

          if (videoRef.current.readyState === 4) {
            const status = tracker.processFrame(canvas, videoRef.current);
            setFaceProgress(status.progress);
            setScanStatus(status.message);

            if (status.isHuman && status.progress >= 100) {
              verifiedRef.current = true;
              setScanStatus('Face Verified ✓ Real Human Confirmed');

              // Biometrically capture live face snapshot
              const snapshot = captureFaceSnapshot(videoRef.current);
              if (snapshot) {
                // Live face scan is stored exclusively for biometric verification, not public profile photo
                if (snapshot.features) {
                  localStorage.setItem('kinjo_face_features', JSON.stringify(snapshot.features));
                }
                localStorage.setItem('kinjo_face_photo', snapshot.dataUrl);

                // Record verification server-side (required gate for
                // saving the profile; cannot be bypassed client-side).
                const activeToken =
                  authTokenRef || localStorage.getItem('kinjo_auth_token') || '';
                if (activeToken && faceChallengeRef.current) {
                  const toastId = toast.loading('Confirming human face verification…');
                  verifyFaceScan(activeToken, snapshot.dataUrl, faceChallengeRef.current)
                    .then(() => {
                      toast.update(toastId, 'Face verified ✓', 'success', 2200);
                      setTimeout(() => goToStep('profile_setup'), 350);
                    })
                    .catch((err: any) => {
                      toast.dismiss(toastId);
                      verifiedRef.current = false;
                      tracker.reset();
                      setFaceProgress(40);
                      // The nonce is spent or stale either way, so fetch a new one.
                      void requestChallenge();
                      setScanStatus(
                        err?.message || 'Could not confirm verification. Please scan again.'
                      );
                    });
                  animationFrameId = requestAnimationFrame(detectFrame);
                  return;
                }
              }

              // No snapshot, no session, or no challenge yet: never verify
              // client-side, just keep scanning until the server can accept it.
              verifiedRef.current = false;
              tracker.reset();
              setFaceProgress(40);
              setScanStatus(
                faceChallengeRef.current
                  ? 'Please hold still and center your face in the oval…'
                  : 'Preparing secure verification…'
              );
              animationFrameId = requestAnimationFrame(detectFrame);
              return;
            }
          }

          animationFrameId = requestAnimationFrame(detectFrame);
        };

        detectFrame();
      } catch (err) {
        setScanStatus('Camera permission denied. Please enable camera access in device settings to verify.');
      }
    };

    void startFaceScan();

    return () => {
      cancelled = true;
      if (animationFrameId) cancelAnimationFrame(animationFrameId);
      if (stream) stream.getTracks().forEach((t) => t.stop());
    };
  }, [step, goToStep, faceScanAttempt]);

  const handleFinalProfileSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim()) {
      setAuthError('Please enter your full name.');
      return;
    }
    if (!avatar) {
      setAuthError('Please upload a profile photo of yourself.');
      return;
    }
    if (!bio.trim()) {
      setAuthError('Please enter what you do & what you are looking for (up to 50 words).');
      return;
    }

    setAuthError('');
    setIsSubmitting(true);

    try {
      const activeToken = authTokenRef || localStorage.getItem('kinjo_auth_token') || '';
      if (!activeToken) {
        setAuthError('Session expired. Please sign in again.');
        setIsSubmitting(false);
        return;
      }

      const activeEmail = email || localStorage.getItem('kinjo_user_email') || '';

      const res = await saveProfile(
        {
          id: activeEmail,
          email: activeEmail,
          name: name.trim(),
          avatar,
          bio: bio.trim(),
          profession: bio.trim(),
          lookingFor: bio.trim(),
          distanceMeters: 0,
          locationName: 'Current Location',
          online: true,
        },
        activeToken
      );

      const finalPhoto = res?.photo_url ? resolvePhotoUrl(res.photo_url) : avatar;

      if (onProfileSetupComplete) {
        onProfileSetupComplete({
          name: name.trim(),
          avatar: finalPhoto,
          email: activeEmail,
          bio: bio.trim(),
          profession: bio.trim(),
          lookingFor: bio.trim(),
        });
      }
      if (isEditMode) {
        onComplete(activeEmail, activeToken);
        onClose();
      } else {
        goToStep('location_permission');
      }
    } catch (err: any) {
      const msg = err?.message || 'Failed to save profile. Please check your connection and try again.';
      setAuthError(msg);
      toast.error(msg);
      if (msg.toLowerCase().includes('face verification')) {
        setTimeout(() => goToStep('face_verification'), 800);
      }
    } finally {
      setIsSubmitting(false);
    }
  };

  // Requests the location permission from the OS and waits for the user's answer
  // before onboarding continues. The old version fired a fire-and-forget
  // getCurrentPosition and closed the modal immediately, so the permission dialog
  // never actually appeared at this step.
  const handleAllowLocation = async () => {
    if (!activeSessionToken) {
      setAuthError('Session expired. Please sign in again.');
      return;
    }

    setLocationError('');
    setIsRequestingLocation(true);

    let denied = false;
    try {
      if (Capacitor.isNativePlatform()) {
        const status = await Geolocation.requestPermissions({ permissions: ['location'] });
        denied = status.location !== 'granted' && status.coarseLocation !== 'granted';
      } else if (navigator?.geolocation) {
        denied = await new Promise<boolean>((resolve) => {
          navigator.geolocation.getCurrentPosition(
            () => resolve(false),
            () => resolve(true),
            { timeout: 10000 }
          );
        });
      }
    } catch {
      denied = true;
    } finally {
      setIsRequestingLocation(false);
    }

    if (denied) {
      // Never continue silently: without location Kinjo cannot show anyone, and
      // once the OS has a "deny" on record it stops showing the dialog at all,
      // so the user needs to be told where the switch actually is.
      setLocationError(
        'Location is off. Kinjo only works with it on — allow Location in Settings, then tap Allow location again.'
      );
      return;
    }

    onComplete(email.trim() || localStorage.getItem('kinjo_user_email') || '', activeSessionToken);
    onClose();
  };

  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-[100] flex flex-col justify-between p-6 sm:p-10 overflow-y-auto min-h-screen select-none" style={{ background: 'var(--bg)', color: 'var(--fg)' }}>
      {/* Top Header Row with Logo */}
      <div className="flex items-center justify-between w-full max-w-md mx-auto pt-[max(28px,calc(env(safe-area-inset-top)+16px))]">
        <span className="wordmark">
          <svg width="18" height="18" viewBox="0 0 100 100" aria-hidden="true">
            <path fill="currentColor" d="M28 0H63.2V45.3H27.8V100A28 28 0 0 1 0 72V28A28 28 0 0 1 28 0Z"/>
            <path fill="currentColor" d="M71.6 0H72A28 28 0 0 1 100 28V72A28 28 0 0 1 72 100H36.2V53.7H71.6Z"/>
          </svg>
          <span>KINJO</span>
        </span>
        <div className="flex items-center gap-2">
          {step === 'face_verification' && (
            <span className="text-[11px] tracking-widest text-zinc-400 uppercase mr-1 font-medium">
              STEP 1 OF 2
            </span>
          )}
          {(step === 'profile_setup' || step === 'location_permission') && !isEditMode && (
            <span className="text-[11px] tracking-widest text-zinc-400 uppercase mr-1 font-medium">
              STEP 2 OF 2
            </span>
          )}
          {stepHistory.length > 0 && (
            <button
              type="button"
              onClick={goBack}
              className="text-xs font-medium hover:opacity-75 transition-opacity cursor-pointer px-1 py-1"
              style={{ background: 'transparent', border: 0, color: 'var(--fg)' }}
            >
              <span>← Back</span>
            </button>
          )}
          {isEditMode && (
            <button
              type="button"
              onClick={onClose}
              className="text-xs font-medium hover:opacity-75 transition-opacity cursor-pointer px-1 py-1"
              style={{ background: 'transparent', border: 0, color: 'var(--fg)' }}
            >
              <span>← Back</span>
            </button>
          )}
        </div>
      </div>

      {/* Middle Section: Auth Step Forms */}
      <div className="my-auto w-full max-w-md mx-auto py-8">
        {step === 'email' && (
          <EmailStep
            email={email}
            setEmail={setEmail}
            authMode={authMode}
            setAuthMode={setAuthMode}
            authError={authError}
            setAuthError={setAuthError}
            isChecking={isCheckingEmail}
            onSubmit={async (e) => {
              e.preventDefault();
              const trimmed = email.trim().toLowerCase();
              if (!trimmed || !trimmed.includes('@') || !trimmed.includes('.')) {
                setAuthError('Please enter a valid email address (e.g. name@example.com).');
                return;
              }
              setAuthError('');
              setIsCheckingEmail(true);
              try {
                const info = await checkEmail(trimmed);
                if (info && info.exists && info.hasPassword === false) {
                  // Google-only account: there is no password to enter here.
                  setAuthError(
                    'This account was created with Google. Please choose “Continue with Google” instead.'
                  );
                  setIsCheckingEmail(false);
                  return;
                } else if (info && info.exists) {
                  setAuthMode('sign_in');
                } else if (authMode === 'sign_in' && (!info || !info.exists)) {
                  setAuthError('No account found with this email. Please check your spelling or choose Create Account.');
                  setIsCheckingEmail(false);
                  return;
                } else {
                  setAuthMode('sign_up');
                }
                goToStep('password');
              } catch {
                goToStep('password');
              } finally {
                setIsCheckingEmail(false);
              }
            }}
            onGoogleAuth={handleGoogleAuth}
          />
        )}

        {(step === 'password' || step === 'create_password') && (
          <PasswordStep
            email={email}
            password={password}
            setPassword={setPassword}
            authMode={authMode}
            setAuthMode={setAuthMode}
            authError={authError}
            setAuthError={setAuthError}
            isSubmitting={isSubmitting}
            onSubmit={handlePasswordSubmit}
          />
        )}

        {step === 'otp' && (            <OTPStep
            email={email}
            otp={otp}
            setOtp={setOtp}
            authError={authError}
            isSubmitting={isSubmitting}
            onSubmit={handleOtpSubmit}
            onResend={handleResendOtp}
            isResending={isResendingOtp}
            devOtp={devOtp}
          />
        )}

        {step === 'face_verification' && (
          <FaceVerificationStep
            videoRef={videoRef}
            cameraActive={cameraActive}
            scanStatus={scanStatus}
            faceProgress={faceProgress}
            onOpenSettings={() => void openAppSettings()}
            onRetry={() => setFaceScanAttempt((n) => n + 1)}
          />
        )}

        {step === 'profile_setup' && (
          <ProfileSetupStep
            name={name}
            setName={setName}
            avatar={avatar}
            setAvatar={setAvatar}
            bio={bio}
            setBio={setBio}
            authError={authError}
            isSubmitting={isSubmitting}
            fileInputRef={fileInputRef}
            handlePhotoUpload={handlePhotoUpload}
            onSubmit={handleFinalProfileSubmit}
            isEditMode={isEditMode}
          />
        )}

        {step === 'location_permission' && (
          <LocationPermissionStep
            onAllow={handleAllowLocation}
            isRequesting={isRequestingLocation}
            error={locationError}
            onOpenSettings={() => void openAppSettings()}
          />
        )}
      </div>

      {/* Bottom Section: Terms & Privacy Agreement Text */}
      <div className="w-full max-w-md mx-auto text-center pb-2">
        <p className="text-[12px] leading-relaxed font-normal" style={{ color: 'var(--faint)' }}>
          By continuing you agree to Kinjo's<br />
          <span className="underline cursor-pointer" style={{ color: 'var(--muted)' }}>
            Terms of Use
          </span>{' '}
          and{' '}
          <span className="underline cursor-pointer" style={{ color: 'var(--muted)' }}>
            Privacy Policy
          </span>.
        </p>
      </div>
    </div>
  );
};

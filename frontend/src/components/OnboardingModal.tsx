import React, { useState, useRef, useEffect, useCallback } from 'react';
import { ArrowLeft } from 'lucide-react';
import { Capacitor } from '@capacitor/core';
import { App as CapApp } from '@capacitor/app';
import { GoogleAuth } from '@shardev/capacitor-google-auth';
import { signIn, signUp, verifyOtp, saveProfile, googleSignIn, compressImage, resolvePhotoUrl } from '../utils/api';

import { EmailStep } from './onboarding/EmailStep';
import { PasswordStep } from './onboarding/PasswordStep';
import { OTPStep } from './onboarding/OTPStep';
import { FaceVerificationStep } from './onboarding/FaceVerificationStep';
import { ProfileSetupStep } from './onboarding/ProfileSetupStep';

interface OnboardingModalProps {
  isOpen: boolean;
  onClose: () => void;
  onComplete: (userEmail?: string, token?: string, isGuest?: boolean) => void;
  onAuthenticated: (token: string, email: string, fallbackPhoto?: string, skipAutoClose?: boolean) => Promise<boolean> | boolean | void;
  onProfileSetupComplete?: (profileData: { name: string; avatar: string; bio?: string; profession: string; lookingFor: string }) => void;
  initialStep?: AuthStep;
  initialProfile?: { name: string; avatar: string; bio?: string; profession?: string; lookingFor?: string };
}

export type AuthStep = 'email' | 'create_password' | 'otp' | 'face_verification' | 'profile_setup';

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
  const [authMode, setAuthMode] = useState<'sign_in' | 'sign_up'>('sign_up');
  const [otp, setOtp] = useState('');
  const [authError, setAuthError] = useState('');
  const [isSubmitting, setIsSubmitting] = useState(false);

  // ─── Step History Stack for Back Navigation ─────────────────────────────────
  const [stepHistory, setStepHistory] = useState<AuthStep[]>([]);
  const [authTokenRef, setAuthTokenRef] = useState('');

  const goToStep = useCallback((nextStep: AuthStep) => {
    setStepHistory((prev) => [...prev, step]);
    setAuthError('');
    setStep(nextStep);
  }, [step]);

  const goBack = useCallback(() => {
    if (stepHistory.length === 0) {
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
  }, [stepHistory]);

  // Face Verification State
  const videoRef = useRef<HTMLVideoElement>(null);
  const [cameraActive, setCameraActive] = useState(false);
  const [scanStatus, setScanStatus] = useState('Preparing camera...');
  const [faceProgress, setFaceProgress] = useState(0);
  const presenceFramesRef = useRef(0);
  const verifiedRef = useRef(false);

  // Profile setup state
  const [name, setName] = useState(initialProfile?.name || '');
  const [avatar, setAvatar] = useState(initialProfile?.avatar || '');
  const [bio, setBio] = useState(initialProfile?.bio || initialProfile?.profession || '');
  const fileInputRef = useRef<HTMLInputElement>(null);

  // Sync initial setup
  useEffect(() => {
    setStep(initialStep);
  }, [initialStep]);

  useEffect(() => {
    if (initialProfile) {
      if (initialProfile.name) setName(initialProfile.name);
      if (initialProfile.avatar) setAvatar(initialProfile.avatar);
      if (initialProfile.bio) setBio(initialProfile.bio);
    }
  }, [initialProfile]);

  // Register native & browser back button listeners
  useEffect(() => {
    if (!isOpen) return;

    let appBackHandle: any = null;
    if (Capacitor.isNativePlatform()) {
      void CapApp.addListener('backButton', () => {
        if (stepHistory.length > 0) {
          goBack();
        } else {
          CapApp.minimizeApp();
        }
      }).then((handle) => {
        appBackHandle = handle;
      });
    }

    const handlePopState = () => {
      if (stepHistory.length > 0) {
        goBack();
      }
    };
    window.addEventListener('popstate', handlePopState);

    return () => {
      if (appBackHandle) void appBackHandle.remove();
      window.removeEventListener('popstate', handlePopState);
    };
  }, [isOpen, stepHistory.length, goBack]);

  // Photo Upload Handler
  const handlePhotoUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;

    if (file.size > 10 * 1024 * 1024) {
      setAuthError('Image size exceeds 10MB limit.');
      return;
    }

    try {
      setAuthError('');
      const compressedDataUrl = await compressImage(file);
      setAvatar(compressedDataUrl);
    } catch (err: any) {
      setAuthError(err?.message || 'Failed to compress image.');
    }
  };

  // Auth Submit Handlers

  const handlePasswordSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setAuthError('');
    setIsSubmitting(true);

    try {
      if (authMode === 'sign_up') {
        await signUp(email, password);
        goToStep('otp');
      } else {
        const resp = await signIn(email, password);
        const token = resp.verificationToken;
        setAuthTokenRef(token);
        const hasExisting = await onAuthenticated(token, email, undefined, false);
        if (hasExisting) {
          onComplete(email, token, false);
          onClose();
        } else {
          goToStep('face_verification');
        }
      }
    } catch (err: any) {
      setAuthError(err?.message || 'Authentication failed. Please check your credentials.');
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
      setAuthError(err?.message || 'Invalid verification code.');
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleGoogleAuth = async () => {
    setAuthError('');
    setIsSubmitting(true);
    const googleClientId = import.meta.env.VITE_GOOGLE_CLIENT_ID || '599627705479-os5q2be0jnrjcbftfkatv75nd5idmhsk.apps.googleusercontent.com';

    try {
      // 1. Native Android / iOS via Capacitor Plugin
      if (Capacitor.isNativePlatform()) {
        await GoogleAuth.initialize({
          clientId: googleClientId,
          serverClientId: googleClientId,
          scopes: ['profile', 'email'],
        }).catch(() => {});

        // Force clear cached session so the Google Account Chooser popup sheet displays EVERY TIME
        await GoogleAuth.logout().catch(() => {});

        const loginRes = await GoogleAuth.login();
        const idToken = loginRes.idToken || loginRes.account?.idToken || loginRes.accessToken;
        const googleEmail = loginRes.account?.email || '';
        const googleName = loginRes.account?.name || loginRes.account?.givenName || '';
        const googlePhoto = loginRes.account?.photoUrl || '';

        if (idToken) {
          const resp = await googleSignIn(idToken);
          const token = resp.verificationToken;
          setAuthTokenRef(token);

          setEmail(googleEmail);
          if (googleName) setName(googleName);
          if (googlePhoto) setAvatar(googlePhoto);

          const hasExisting = await onAuthenticated(token, googleEmail, googlePhoto, authMode === 'sign_up');
          if (hasExisting && authMode === 'sign_in') {
            onComplete(googleEmail, token, false);
            onClose();
          } else {
            goToStep('face_verification');
          }
          return;
        }
      }

      // 2. Web Browser Popup via Google Identity Services SDK
      if ((window as any).google?.accounts?.oauth2) {
        const client = (window as any).google.accounts.oauth2.initTokenClient({
          client_id: googleClientId,
          scope: 'email profile',
          prompt: 'select_account',
          callback: async (tokenResponse: any) => {
            if (tokenResponse?.access_token) {
              try {
                const userInfoRes = await fetch('https://www.googleapis.com/oauth2/v3/userinfo', {
                  headers: { Authorization: `Bearer ${tokenResponse.access_token}` },
                });
                const userInfo = await userInfoRes.json();
                const userEmail = userInfo.email || '';
                const userName = userInfo.name || '';
                const userPhoto = userInfo.picture || '';

                setEmail(userEmail);
                if (userName) setName(userName);
                if (userPhoto) setAvatar(userPhoto);

                const resp = await googleSignIn(tokenResponse.access_token);
                const token = resp.verificationToken;
                setAuthTokenRef(token);

                const hasExisting = await onAuthenticated(token, userEmail, userPhoto, authMode === 'sign_up');
                if (hasExisting && authMode === 'sign_in') {
                  onComplete(userEmail, token, false);
                  onClose();
                } else {
                  goToStep('face_verification');
                }
              } catch (err: any) {
                setAuthError(err?.message || 'Google sign in failed.');
              } finally {
                setIsSubmitting(false);
              }
            } else {
              setIsSubmitting(false);
            }
          },
          error_callback: () => {
            setIsSubmitting(false);
          },
        });
        client.requestAccessToken();
        return;
      }

      // 3. Fallback Centered Popup Window
      const backendBase = (import.meta.env.VITE_API_URL || 'http://localhost:8080/api').replace(/\/api\/?$/, '');
      const width = 500;
      const height = 600;
      const left = window.screenX + (window.outerWidth - width) / 2;
      const top = window.screenY + (window.outerHeight - height) / 2;
      const popup = window.open(
        `${backendBase}/api/auth/google/login`,
        'google_oauth_popup',
        `width=${width},height=${height},left=${left},top=${top},status=0,toolbar=0`
      );
      if (!popup) {
        window.location.href = `${backendBase}/api/auth/google/login`;
      }
    } catch (err: any) {
      const errMsg = err?.message || err?.error || String(err);
      if (!errMsg.toLowerCase().includes('cancel') && !errMsg.toLowerCase().includes('closed')) {
        setAuthError(errMsg || 'Google sign in failed. Please try email verification.');
      }
    } finally {
      setIsSubmitting(false);
    }
  };

  // Face Verification Camera Effect (Real 2-Phase Facial Motion & Liveness Scan)
  useEffect(() => {
    if (step !== 'face_verification') {
      setCameraActive(false);
      return;
    }

    let stream: MediaStream | null = null;
    let animationFrameId: number;
    const canvas = document.createElement('canvas');
    canvas.width = 160;
    canvas.height = 120;
    const ctx = canvas.getContext('2d', { willReadFrequently: true });

    presenceFramesRef.current = 0;
    verifiedRef.current = false;
    setFaceProgress(0);

    const startFaceScan = async () => {
      try {
        setScanStatus('Initializing camera...');
        stream = await navigator.mediaDevices.getUserMedia({
          video: { facingMode: 'user', width: { ideal: 640 }, height: { ideal: 480 } },
        });

        if (videoRef.current) {
          videoRef.current.srcObject = stream;
          await videoRef.current.play();
          setCameraActive(true);
        }

        setScanStatus('Center your face inside oval');

        let prevData: Uint8ClampedArray | null = null;
        let cumulativeMotion = 0;

        const detectFrame = () => {
          if (!videoRef.current || verifiedRef.current) return;

          if (ctx && videoRef.current.readyState === 4) {
            ctx.drawImage(videoRef.current, 0, 0, 160, 120);
            const imgData = ctx.getImageData(35, 15, 90, 90);
            const data = imgData.data;

            let skinPixelCount = 0;
            let currentFrameMotion = 0;
            const totalPixels = data.length / 4;

            for (let i = 0; i < data.length; i += 4) {
              const r = data[i];
              const g = data[i + 1];
              const b = data[i + 2];

              if (r > 40 && g > 20 && b > 20 && Math.max(r, g, b) - Math.min(r, g, b) > 15 && Math.abs(r - g) > 10 && r > g && r > b) {
                skinPixelCount++;
              }

              if (prevData) {
                currentFrameMotion += Math.abs(r - prevData[i]) + Math.abs(g - prevData[i + 1]) + Math.abs(b - prevData[i + 2]);
              }
            }

            prevData = new Uint8ClampedArray(data);
            const skinRatio = skinPixelCount / totalPixels;

            if (skinRatio > 0.20) {
              // Phase 1: Face Centered (up to 50%)
              if (presenceFramesRef.current < 10) {
                presenceFramesRef.current += 1;
                const progress = Math.min(50, Math.floor((presenceFramesRef.current / 10) * 50));
                setFaceProgress(progress);
                setScanStatus('Face detected. Blink or tilt head slightly...');
              } else {
                // Phase 2: Motion Liveness Verification (50% to 100%)
                if (currentFrameMotion > 12000) {
                  cumulativeMotion += currentFrameMotion;
                  presenceFramesRef.current += 1;
                  const progress = Math.min(100, 50 + Math.floor((presenceFramesRef.current - 10) / 10 * 50));
                  setFaceProgress(progress);
                  setScanStatus('Verifying live movement...');

                  if (progress >= 100 && cumulativeMotion > 100000) {
                    verifiedRef.current = true;
                    setScanStatus('Face Verified!');
                    setTimeout(() => {
                      goToStep('profile_setup');
                    }, 300);
                    return;
                  }
                } else {
                  setScanStatus('Blink eyes or turn head slightly...');
                }
              }
            } else {
              setScanStatus('Center your face inside oval');
            }
          }

          animationFrameId = requestAnimationFrame(detectFrame);
        };

        detectFrame();
      } catch (err: any) {
        setScanStatus('Camera access required for face verification.');
      }
    };

    startFaceScan();

    return () => {
      if (animationFrameId) cancelAnimationFrame(animationFrameId);
      if (stream) stream.getTracks().forEach((t) => t.stop());
    };
  }, [step, goToStep]);

  const handleFinalProfileSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim()) {
      setAuthError('Name is required.');
      return;
    }
    if (!bio.trim()) {
      setAuthError('Please enter what you do & what you are looking for.');
      return;
    }

    setAuthError('');
    setIsSubmitting(true);

    try {
      const activeToken = authTokenRef || localStorage.getItem('kinjo_auth_token') || '';
      if (!activeToken) {
        setAuthError('Session expired. Please start registration again.');
        setIsSubmitting(false);
        return;
      }

      const res = await saveProfile(
        {
          id: email,
          email,
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
        onProfileSetupComplete({ name: name.trim(), avatar: finalPhoto, bio: bio.trim(), profession: bio.trim(), lookingFor: bio.trim() });
      }
      onComplete(email, activeToken, false);
      onClose();
    } catch (err: any) {
      setAuthError(err?.message || 'Failed to save profile. Please try again.');
    } finally {
      setIsSubmitting(false);
    }
  };

  if (!isOpen) return null;

  // Back Button Subcomponent
  const BackButton = () => (
    <button
      type="button"
      onClick={goBack}
      className="flex items-center gap-1.5 px-3.5 py-1.5 rounded-full bg-white/10 hover:bg-white/20 text-white/80 hover:text-white text-xs font-semibold transition-all active:scale-95 border border-white/10 shadow-sm"
    >
      <ArrowLeft className="w-3.5 h-3.5" />
      <span>Back</span>
    </button>
  );

  return (
    <div className="fixed inset-0 z-50 bg-[#060608] text-white flex flex-col justify-between p-6 sm:p-10 overflow-y-auto min-h-screen select-none">
      {/* Top Header Row with Logo */}
      <div className="flex items-center justify-between w-full max-w-md mx-auto pt-2">
        <span className="text-3xl font-extrabold tracking-tight text-white font-sans leading-none">
          k<span className="text-white/30">.</span>
        </span>
        {stepHistory.length > 0 && <BackButton />}
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
            onSubmit={(e) => {
              e.preventDefault();
              if (!email || !email.includes('@')) {
                setAuthError('Please enter a valid email address.');
                return;
              }
              goToStep('create_password');
            }}
            onGoogleAuth={handleGoogleAuth}
          />
        )}

        {step === 'create_password' && (
          <PasswordStep
            email={email}
            password={password}
            setPassword={setPassword}
            authMode={authMode}
            authError={authError}
            isSubmitting={isSubmitting}
            onSubmit={handlePasswordSubmit}
          />
        )}

        {step === 'otp' && (
          <OTPStep
            email={email}
            otp={otp}
            setOtp={setOtp}
            authError={authError}
            isSubmitting={isSubmitting}
            onSubmit={handleOtpSubmit}
          />
        )}

        {step === 'face_verification' && (
          <FaceVerificationStep
            videoRef={videoRef}
            cameraActive={cameraActive}
            scanStatus={scanStatus}
            faceProgress={faceProgress}
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
          />
        )}
      </div>

      {/* Bottom Section: Terms & Privacy Agreement Text */}
      <div className="w-full max-w-md mx-auto text-center pb-2">
        <p className="text-xs text-white/35 leading-relaxed font-normal">
          By continuing, you agree to Kinjo's{' '}
          <span className="text-white/60 font-medium underline underline-offset-2 cursor-pointer">
            Terms of Service
          </span>{' '}
          and{' '}
          <span className="text-white/60 font-medium underline underline-offset-2 cursor-pointer">
            Privacy Policy
          </span>.
        </p>
      </div>
    </div>
  );
};

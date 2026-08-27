import React, { useState, useRef, useEffect } from 'react';
import type { UserProfile } from '../types';
import {
  Camera,
  Upload,
  ShieldCheck,
  RefreshCw,
  LogOut,
  Trash2,
  Activity,
  ChevronRight,
  Check,
  Eye,
  X
} from 'lucide-react';
import { ProfileCard } from './ProfileCard';
import { detectAndVerifyFace, type FaceVerificationResult } from '../utils/faceDetector';
import { performLivenessCheck, type LivenessCheckResult } from '../utils/livenessDetector';

interface ProfileViewProps {
  userProfile: UserProfile;
  onSave: (updatedProfile: UserProfile) => void;
  onLogout?: () => void;
  onDeactivateAccount?: () => void;
  onDeleteAccount?: () => void;
}

export const ProfileView: React.FC<ProfileViewProps> = ({
  userProfile,
  onSave,
  onLogout,
  onDeactivateAccount,
  onDeleteAccount,
}) => {
  const [formData, setFormData] = useState<UserProfile>({ ...userProfile });
  const [editingField, setEditingField] = useState<string | null>(null);
  const [showPreview, setShowPreview] = useState(false);

  // Security Account States
  const [isDeactivated, setIsDeactivated] = useState(false);
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [resetMessage, setResetMessage] = useState<string | null>(null);

  // Photo & Face Verification state
  const [photoUrl, setPhotoUrl] = useState(userProfile.avatar);
  const [isScanningFace, setIsScanningFace] = useState(false);

  // Live Camera & Liveness Detection states
  const [isCameraActive, setIsCameraActive] = useState(false);
  const [livenessResult, setLivenessResult] = useState<LivenessCheckResult | null>(null);
  const videoRef = useRef<HTMLVideoElement>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const previousFrameRef = useRef<ImageData | null>(null);
  const livenessIntervalRef = useRef<number | null>(null);

  useEffect(() => {
    setFormData({ ...userProfile });
    setPhotoUrl(userProfile.avatar);
  }, [userProfile]);

  const runFaceDetectionOnImage = (src: string) => {
    setIsScanningFace(true);

    const img = new Image();
    img.crossOrigin = 'anonymous';
    img.src = src;

    img.onload = async () => {
      const res: FaceVerificationResult = await detectAndVerifyFace(img);
      setIsScanningFace(false);
      const updated = { ...formData, avatar: src, faceVerified: res.isRealFace };
      setFormData(updated);
      onSave(updated);
    };

    img.onerror = () => {
      setIsScanningFace(false);
    };
  };

  const handleFileUpload = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;

    const reader = new FileReader();
    reader.onload = (event) => {
      const result = event.target?.result as string;
      setPhotoUrl(result);
      runFaceDetectionOnImage(result);
    };
    reader.readAsDataURL(file);
  };

  const startCamera = async () => {
    setIsCameraActive(true);
    setLivenessResult(null);
    try {
      const stream = await navigator.mediaDevices.getUserMedia({
        video: { facingMode: 'user', width: { ideal: 640 }, height: { ideal: 640 } },
      });
      if (videoRef.current) {
        videoRef.current.srcObject = stream;
      }

      livenessIntervalRef.current = window.setInterval(() => {
        if (videoRef.current) {
          const { result, currentFrameData } = performLivenessCheck(
            videoRef.current,
            previousFrameRef.current
          );
          setLivenessResult(result);
          if (currentFrameData) {
            previousFrameRef.current = currentFrameData;
          }
        }
      }, 500);
    } catch (err) {
      alert('Camera access denied or unavailable.');
      setIsCameraActive(false);
    }
  };

  const stopCamera = () => {
    if (livenessIntervalRef.current) {
      clearInterval(livenessIntervalRef.current);
      livenessIntervalRef.current = null;
    }
    if (videoRef.current && videoRef.current.srcObject) {
      const stream = videoRef.current.srcObject as MediaStream;
      stream.getTracks().forEach((track) => track.stop());
      videoRef.current.srcObject = null;
    }
    setIsCameraActive(false);
  };

  const capturePhoto = () => {
    if (!videoRef.current) return;
    const canvas = document.createElement('canvas');
    canvas.width = videoRef.current.videoWidth || 400;
    canvas.height = videoRef.current.videoHeight || 400;
    const ctx = canvas.getContext('2d');
    if (ctx) {
      ctx.drawImage(videoRef.current, 0, 0, canvas.width, canvas.height);
      const dataUrl = canvas.toDataURL('image/jpeg');
      setPhotoUrl(dataUrl);
      stopCamera();
      runFaceDetectionOnImage(dataUrl);
    }
  };

  const saveFieldValue = (field: keyof UserProfile, val: any) => {
    const updated = { ...formData, [field]: val };
    setFormData(updated);
    onSave(updated);
    setEditingField(null);
  };

  const handleForgotPassword = () => {
    setResetMessage('Password reset link sent to your email.');
    setTimeout(() => setResetMessage(null), 4000);
  };

  // FULL SCREEN PREVIEW MODE
  if (showPreview) {
    return (
      <div className="fixed inset-0 z-50 flex flex-col items-center justify-between p-4 bg-black/95 backdrop-blur-2xl select-none">
        {/* Top Bar with Close Button */}
        <div className="w-full max-w-sm flex items-center justify-between pt-2 pb-2">
          <span className="text-xs font-mono uppercase tracking-wider text-neutral-400">Card Preview</span>
          <button
            type="button"
            onClick={() => setShowPreview(false)}
            className="px-3.5 py-1.5 rounded-full bg-white text-black font-extrabold text-xs flex items-center gap-1.5 shadow-lg active:scale-95 transition-all"
          >
            <X className="w-3.5 h-3.5" />
            <span>Close Preview</span>
          </button>
        </div>

        {/* Centered Full Screen Card */}
        <div className="w-full max-w-[280px] xs:max-w-[300px] sm:max-w-[320px] h-[440px] sm:h-[480px] my-auto">
          <ProfileCard profile={formData} />
        </div>

        {/* Bottom hint */}
        <p className="text-[11px] text-neutral-500 font-mono pb-2">
          This is how nearby users see your card within 30m
        </p>
      </div>
    );
  }

  return (
    <div className="w-full max-w-md mx-auto space-y-5 pb-28 px-4 select-none">
      {/* 1. APPLE PROFILE HERO */}
      <div className="flex flex-col items-center text-center space-y-3 pt-1">
        <div className="relative">
          {isCameraActive ? (
            <div className="relative w-32 h-32 rounded-full overflow-hidden border-2 border-white/30 shadow-2xl bg-black flex items-center justify-center">
              <video ref={videoRef} autoPlay playsInline className="w-full h-full object-cover" />
              <div className="absolute inset-x-0 bottom-2 flex items-center justify-center gap-1">
                <button
                  type="button"
                  onClick={capturePhoto}
                  disabled={!livenessResult?.isLive}
                  className="px-2.5 py-1 rounded-full bg-white text-black text-[10px] font-extrabold shadow-md"
                >
                  Snap
                </button>
                <button
                  type="button"
                  onClick={stopCamera}
                  className="px-2 py-1 rounded-full bg-black/80 text-white text-[10px]"
                >
                  Cancel
                </button>
              </div>
            </div>
          ) : (
            <div className="relative group cursor-pointer" onClick={() => fileInputRef.current?.click()}>
              <div className="w-24 h-24 sm:w-26 sm:h-26 rounded-full overflow-hidden border border-white/20 shadow-2xl bg-neutral-900 mx-auto">
                <img src={photoUrl} alt={formData.name} className="w-full h-full object-cover" />
                {isScanningFace && (
                  <div className="absolute inset-0 bg-black/75 backdrop-blur-xs flex items-center justify-center">
                    <RefreshCw className="w-6 h-6 text-white animate-spin" />
                  </div>
                )}
              </div>

              <div className="absolute bottom-0 right-0 p-1.5 rounded-full bg-white text-black shadow-lg">
                <Camera className="w-3 h-3" />
              </div>
            </div>
          )}

          <input
            ref={fileInputRef}
            type="file"
            accept="image/*"
            onChange={handleFileUpload}
            className="hidden"
          />
        </div>

        <div>
          <h2 className="text-xl sm:text-2xl font-extrabold text-white tracking-tight flex items-center justify-center gap-1.5">
            <span>{formData.name}</span>
            <ShieldCheck className="w-4 h-4 text-emerald-400 shrink-0" />
          </h2>
          <p className="text-[11px] text-neutral-400 font-mono mt-0.5">{formData.handle}</p>
        </div>

        {/* Compact Single-Row Action Pills (Reduced Font & Padding) */}
        <div className="flex items-center justify-center gap-1.5 w-full">
          <button
            type="button"
            onClick={() => fileInputRef.current?.click()}
            className="px-2.5 py-1.5 rounded-full bg-white/10 hover:bg-white/15 text-[11px] font-medium text-neutral-200 flex items-center gap-1 transition-all whitespace-nowrap"
          >
            <Upload className="w-3 h-3 text-white shrink-0" />
            <span>Upload Photo</span>
          </button>

          <button
            type="button"
            onClick={startCamera}
            className="px-2.5 py-1.5 rounded-full bg-white/10 hover:bg-white/15 text-[11px] font-medium text-neutral-200 flex items-center gap-1 transition-all whitespace-nowrap"
          >
            <Activity className="w-3 h-3 text-white shrink-0" />
            <span>Live Camera</span>
          </button>

          <button
            type="button"
            onClick={() => setShowPreview(true)}
            className="px-3 py-1.5 rounded-full bg-white text-black hover:bg-neutral-200 text-[11px] font-extrabold flex items-center gap-1 transition-all shadow-md active:scale-95 whitespace-nowrap"
          >
            <Eye className="w-3 h-3 shrink-0" />
            <span>Preview Card</span>
          </button>
        </div>
      </div>

      {resetMessage && (
        <div className="p-3 rounded-xl bg-emerald-500/10 border border-emerald-500/30 text-emerald-300 text-xs font-medium flex items-center justify-center gap-2">
          <Check className="w-4 h-4" />
          <span>{resetMessage}</span>
        </div>
      )}

      {/* 2. MINIMALIST APPLE HIG GROUPED LIST */}
      <div className="space-y-4">
        {/* GROUP 1: PERSONAL INFORMATION */}
        <div className="space-y-1">
          <span className="text-[11px] font-mono uppercase tracking-wider text-neutral-500 px-3 block">
            Personal Information
          </span>

          <div className="bg-[#121319]/90 border border-white/10 rounded-2xl overflow-hidden divide-y divide-white/[0.08]">
            {/* Name Row */}
            <div className="p-3.5 flex items-center justify-between">
              <span className="text-xs font-semibold text-neutral-300">Name</span>
              {editingField === 'name' ? (
                <div className="flex items-center gap-2">
                  <input
                    type="text"
                    defaultValue={formData.name}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') saveFieldValue('name', (e.target as HTMLInputElement).value);
                    }}
                    className="bg-white/10 border border-white/20 rounded-lg px-2.5 py-1 text-xs text-white focus:outline-none"
                    autoFocus
                  />
                  <button
                    type="button"
                    onClick={(e) => {
                      const input = (e.currentTarget.previousElementSibling as HTMLInputElement);
                      saveFieldValue('name', input.value);
                    }}
                    className="p-1 rounded-full bg-white text-black"
                  >
                    <Check className="w-3 h-3" />
                  </button>
                </div>
              ) : (
                <button
                  type="button"
                  onClick={() => setEditingField('name')}
                  className="text-xs font-medium text-white flex items-center gap-1 hover:underline"
                >
                  <span>{formData.name}</span>
                  <ChevronRight className="w-3.5 h-3.5 text-neutral-500" />
                </button>
              )}
            </div>

            {/* Handle Row */}
            <div className="p-3.5 flex items-center justify-between">
              <span className="text-xs font-semibold text-neutral-300">Handle</span>
              {editingField === 'handle' ? (
                <div className="flex items-center gap-2">
                  <input
                    type="text"
                    defaultValue={formData.handle}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') saveFieldValue('handle', (e.target as HTMLInputElement).value);
                    }}
                    className="bg-white/10 border border-white/20 rounded-lg px-2.5 py-1 text-xs text-white focus:outline-none"
                    autoFocus
                  />
                  <button
                    type="button"
                    onClick={(e) => {
                      const input = (e.currentTarget.previousElementSibling as HTMLInputElement);
                      saveFieldValue('handle', input.value);
                    }}
                    className="p-1 rounded-full bg-white text-black"
                  >
                    <Check className="w-3 h-3" />
                  </button>
                </div>
              ) : (
                <button
                  type="button"
                  onClick={() => setEditingField('handle')}
                  className="text-xs font-medium text-white flex items-center gap-1 hover:underline"
                >
                  <span>{formData.handle}</span>
                  <ChevronRight className="w-3.5 h-3.5 text-neutral-500" />
                </button>
              )}
            </div>

            {/* Status Quote Row */}
            <div className="p-3.5 flex items-center justify-between">
              <span className="text-xs font-semibold text-neutral-300">Status Note</span>
              {editingField === 'quotePrompt' ? (
                <div className="flex items-center gap-2">
                  <input
                    type="text"
                    defaultValue={formData.quotePrompt}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') saveFieldValue('quotePrompt', (e.target as HTMLInputElement).value);
                    }}
                    className="bg-white/10 border border-white/20 rounded-lg px-2.5 py-1 text-xs text-white focus:outline-none"
                    autoFocus
                  />
                  <button
                    type="button"
                    onClick={(e) => {
                      const input = (e.currentTarget.previousElementSibling as HTMLInputElement);
                      saveFieldValue('quotePrompt', input.value);
                    }}
                    className="p-1 rounded-full bg-white text-black"
                  >
                    <Check className="w-3.5 h-3.5" />
                  </button>
                </div>
              ) : (
                <button
                  type="button"
                  onClick={() => setEditingField('quotePrompt')}
                  className="text-xs font-medium text-white max-w-[200px] truncate flex items-center gap-1 hover:underline"
                >
                  <span className="truncate">&ldquo;{formData.quotePrompt}&rdquo;</span>
                  <ChevronRight className="w-3.5 h-3.5 text-neutral-500 shrink-0" />
                </button>
              )}
            </div>

            {/* Bio Row */}
            <div className="p-3.5 flex items-center justify-between">
              <span className="text-xs font-semibold text-neutral-300">Bio</span>
              <button
                type="button"
                onClick={() => setEditingField(editingField === 'bio' ? null : 'bio')}
                className="text-xs font-medium text-white max-w-[200px] truncate flex items-center gap-1 hover:underline"
              >
                <span className="truncate">{formData.bio}</span>
                <ChevronRight className="w-3.5 h-3.5 text-neutral-500 shrink-0" />
              </button>
            </div>
          </div>
        </div>

        {/* Bio Edit Expandable Form */}
        {editingField === 'bio' && (
          <div className="p-4 bg-[#121319] border border-white/10 rounded-2xl space-y-3">
            <textarea
              rows={3}
              defaultValue={formData.bio}
              className="w-full bg-white/5 border border-white/10 rounded-xl p-3 text-xs text-white focus:outline-none"
            />
            <div className="flex justify-end gap-2">
              <button
                type="button"
                onClick={() => setEditingField(null)}
                className="px-3 py-1.5 rounded-lg bg-white/10 text-xs text-white"
              >
                Cancel
              </button>
              <button
                type="button"
                onClick={(e) => {
                  const txt = (e.currentTarget.parentElement?.previousElementSibling as HTMLTextAreaElement).value;
                  saveFieldValue('bio', txt);
                }}
                className="px-4 py-1.5 rounded-lg bg-white text-black text-xs font-bold"
              >
                Save
              </button>
            </div>
          </div>
        )}

        {/* GROUP 2: SECURITY & PREFERENCES */}
        <div className="space-y-1 pt-2">
          <span className="text-[11px] font-mono uppercase tracking-wider text-neutral-500 px-3 block">
            Security & Account
          </span>

          <div className="bg-[#121319]/90 border border-white/10 rounded-2xl overflow-hidden divide-y divide-white/[0.08]">
            {/* Liveness Status */}
            <div className="p-3.5 flex items-center justify-between">
              <span className="text-xs font-semibold text-neutral-300">Face Verification</span>
              <span className="text-xs font-bold text-emerald-400 flex items-center gap-1">
                <ShieldCheck className="w-3.5 h-3.5" />
                <span>Verified ✓</span>
              </span>
            </div>

            {/* Forgot Password */}
            <button
              type="button"
              onClick={handleForgotPassword}
              className="w-full p-3.5 flex items-center justify-between text-left hover:bg-white/[0.03] transition-colors"
            >
              <span className="text-xs font-semibold text-neutral-300">Forgot Password</span>
              <span className="text-xs text-neutral-400">Reset Link</span>
            </button>

            {/* Deactivate Account */}
            <button
              type="button"
              onClick={() => {
                setIsDeactivated(!isDeactivated);
                if (onDeactivateAccount) onDeactivateAccount();
              }}
              className="w-full p-3.5 flex items-center justify-between text-left hover:bg-white/[0.03] transition-colors"
            >
              <span className="text-xs font-semibold text-neutral-300">Deactivate Account</span>
              <span className="text-xs font-bold text-neutral-400">
                {isDeactivated ? 'Deactivated' : 'Active'}
              </span>
            </button>

            {/* Log Out */}
            <button
              type="button"
              onClick={() => {
                if (onLogout) onLogout();
              }}
              className="w-full p-3.5 flex items-center justify-between text-left hover:bg-white/[0.03] transition-colors"
            >
              <span className="text-xs font-semibold text-neutral-300">Log Out</span>
              <LogOut className="w-3.5 h-3.5 text-neutral-400" />
            </button>
          </div>
        </div>

        {/* Delete Account */}
        <div className="pt-2">
          {showDeleteConfirm ? (
            <div className="p-4 rounded-2xl bg-rose-500/10 border border-rose-500/30 space-y-3">
              <p className="text-xs font-bold text-rose-400">Permanently delete account?</p>
              <p className="text-xs text-neutral-300">
                This will wipe all your profile data and card history.
              </p>
              <div className="flex items-center gap-2 pt-1">
                <button
                  type="button"
                  onClick={() => {
                    if (onDeleteAccount) onDeleteAccount();
                  }}
                  className="flex-1 py-2.5 rounded-xl bg-rose-500 hover:bg-rose-600 text-white font-bold text-xs"
                >
                  Delete Permanently
                </button>
                <button
                  type="button"
                  onClick={() => setShowDeleteConfirm(false)}
                  className="px-4 py-2.5 rounded-xl bg-white/10 text-white text-xs"
                >
                  Cancel
                </button>
              </div>
            </div>
          ) : (
            <button
              type="button"
              onClick={() => setShowDeleteConfirm(true)}
              className="w-full p-3.5 rounded-2xl bg-rose-500/10 hover:bg-rose-500/20 border border-rose-500/20 text-rose-400 text-xs font-semibold flex items-center justify-between transition-all active:scale-95"
            >
              <div className="flex items-center gap-2.5">
                <Trash2 className="w-4 h-4 text-rose-400" />
                <span>Delete Account</span>
              </div>
              <span className="text-xs font-bold">Delete</span>
            </button>
          )}
        </div>
      </div>
    </div>
  );
};

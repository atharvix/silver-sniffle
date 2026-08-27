import React, { useState, useRef, useEffect } from 'react';
import type { UserProfile } from '../types';
import {
  X,
  Check,
  Eye,
  Camera,
  Upload,
  ShieldCheck,
  RefreshCw,
  LogOut,
  KeyRound,
  UserX,
  Trash2,
  AlertCircle,
  Sliders,
  Lock,
  Activity
} from 'lucide-react';
import { ProfileCard } from './ProfileCard';
import { detectAndVerifyFace, type FaceVerificationResult } from '../utils/faceDetector';
import { performLivenessCheck, type LivenessCheckResult } from '../utils/livenessDetector';

interface ProfileEditorModalProps {
  userProfile: UserProfile;
  isOpen: boolean;
  onClose: () => void;
  onSave: (updatedProfile: UserProfile) => void;
  onLogout?: () => void;
  onDeactivateAccount?: () => void;
  onDeleteAccount?: () => void;
}

export const ProfileEditorModal: React.FC<ProfileEditorModalProps> = ({
  userProfile,
  isOpen,
  onClose,
  onSave,
  onLogout,
  onDeactivateAccount,
  onDeleteAccount,
}) => {
  const [activeTab, setActiveTab] = useState<'profile' | 'account'>('profile');
  const [formData, setFormData] = useState<UserProfile>({ ...userProfile });
  const [tagInput, setTagInput] = useState(userProfile.tags.join(', '));
  const [showPreview, setShowPreview] = useState(false);

  // Security Account States
  const [isDeactivated, setIsDeactivated] = useState(false);
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [resetMessage, setResetMessage] = useState<string | null>(null);

  // Photo & Face Verification state
  const [photoUrl, setPhotoUrl] = useState(userProfile.avatar);
  const [isScanningFace, setIsScanningFace] = useState(false);
  const [faceResult, setFaceResult] = useState<FaceVerificationResult | null>({
    isRealFace: userProfile.faceVerified ?? true,
    confidence: 0.95,
    faceCount: 1,
    message: 'Face verified ✓',
  });

  // Live Camera & Liveness Detection states
  const [isCameraActive, setIsCameraActive] = useState(false);
  const [livenessResult, setLivenessResult] = useState<LivenessCheckResult | null>(null);
  const videoRef = useRef<HTMLVideoElement>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const previousFrameRef = useRef<ImageData | null>(null);
  const livenessIntervalRef = useRef<number | null>(null);

  useEffect(() => {
    if (isOpen) {
      setFormData({ ...userProfile });
      setPhotoUrl(userProfile.avatar);
      setTagInput(userProfile.tags.join(', '));
    }
  }, [isOpen, userProfile]);

  if (!isOpen) return null;

  const runFaceDetectionOnImage = (src: string) => {
    setIsScanningFace(true);
    setFaceResult(null);

    const img = new Image();
    img.crossOrigin = 'anonymous';
    img.src = src;

    img.onload = async () => {
      const res = await detectAndVerifyFace(img);
      setIsScanningFace(false);
      setFaceResult(res);
      setFormData((prev) => ({
        ...prev,
        avatar: src,
        faceVerified: res.isRealFace,
      }));
    };

    img.onerror = () => {
      setIsScanningFace(false);
      setFaceResult({
        isRealFace: false,
        confidence: 0,
        faceCount: 0,
        message: 'Could not load photo',
      });
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

  const handleTagsChange = (val: string) => {
    setTagInput(val);
    const parsed = val.split(',').map((t) => t.trim()).filter(Boolean);
    setFormData((prev) => ({ ...prev, tags: parsed }));
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!faceResult?.isRealFace) return;
    onSave(formData);
    onClose();
  };

  const handleForgotPassword = () => {
    setResetMessage('Password reset link sent to your email.');
    setTimeout(() => setResetMessage(null), 4000);
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-3 sm:p-4 bg-black/85 backdrop-blur-2xl overflow-y-auto select-none">
      {/* BESPOKE APPLE MINIMAL PROFILE SHEET */}
      <div className="relative w-full max-w-md max-h-[92vh] bg-[#0c0d12] rounded-[32px] border border-white/12 p-5 sm:p-6 overflow-y-auto my-auto shadow-2xl space-y-5">
        
        {/* TOP SEGMENTED PILL & CONTROLS */}
        <div className="flex items-center justify-between gap-2">
          {/* Segmented Control Pill */}
          <div className="flex items-center p-1 bg-white/[0.06] rounded-full border border-white/10 backdrop-blur-xl">
            <button
              type="button"
              onClick={() => setActiveTab('profile')}
              className={`px-3.5 sm:px-4 py-1.5 rounded-full text-xs font-semibold flex items-center gap-1.5 transition-all ${
                activeTab === 'profile'
                  ? 'bg-white text-black font-extrabold shadow-md'
                  : 'text-neutral-400 hover:text-white'
              }`}
            >
              <Sliders className="w-3.5 h-3.5" />
              <span>Profile</span>
            </button>
            <button
              type="button"
              onClick={() => setActiveTab('account')}
              className={`px-3.5 sm:px-4 py-1.5 rounded-full text-xs font-semibold flex items-center gap-1.5 transition-all ${
                activeTab === 'account'
                  ? 'bg-white text-black font-extrabold shadow-md'
                  : 'text-neutral-400 hover:text-white'
              }`}
            >
              <Lock className="w-3.5 h-3.5" />
              <span>Account</span>
            </button>
          </div>

          <div className="flex items-center gap-1.5">
            <button
              type="button"
              onClick={() => setShowPreview(!showPreview)}
              className="p-2 rounded-full bg-white/[0.06] hover:bg-white/12 text-neutral-300 border border-white/10 backdrop-blur-xl transition-colors"
              title="Toggle Card Preview"
            >
              <Eye className="w-4 h-4" />
            </button>

            <button
              onClick={() => {
                stopCamera();
                onClose();
              }}
              className="p-2 rounded-full bg-white/[0.06] hover:bg-white/12 text-neutral-400 hover:text-white border border-white/10 backdrop-blur-xl transition-colors"
            >
              <X className="w-4 h-4" />
            </button>
          </div>
        </div>

        {showPreview ? (
          <div className="w-full flex flex-col items-center justify-center py-4">
            <div className="w-full max-w-xs h-[460px]">
              <ProfileCard profile={formData} />
            </div>
          </div>
        ) : activeTab === 'profile' ? (
          /* TAB 1: BESPOKE MINIMAL PROFILE EDITOR */
          <form onSubmit={handleSubmit} className="space-y-4">
            {/* HERO PHOTO AVATAR */}
            <div className="flex flex-col items-center justify-center space-y-3 pt-1">
              {isCameraActive ? (
                <div className="relative w-44 h-44 sm:w-48 sm:h-48 rounded-full overflow-hidden border-2 border-white/40 shadow-2xl bg-black flex items-center justify-center">
                  <video ref={videoRef} autoPlay playsInline className="w-full h-full object-cover" />
                  <div className="absolute inset-x-0 bottom-3 flex items-center justify-center gap-2">
                    <button
                      type="button"
                      onClick={capturePhoto}
                      disabled={!livenessResult?.isLive}
                      className="px-3.5 py-1.5 rounded-full bg-white hover:bg-neutral-200 disabled:opacity-40 text-black text-xs font-extrabold shadow-lg transition-all"
                    >
                      Snap Photo
                    </button>
                    <button
                      type="button"
                      onClick={stopCamera}
                      className="px-3 py-1.5 rounded-full bg-black/80 text-white text-xs"
                    >
                      Cancel
                    </button>
                  </div>
                </div>
              ) : (
                <div className="relative group cursor-pointer" onClick={() => fileInputRef.current?.click()}>
                  <div className="w-24 h-24 sm:w-26 sm:h-26 rounded-full overflow-hidden border-2 border-white/20 group-hover:border-white transition-all shadow-2xl bg-neutral-900">
                    <img src={photoUrl} alt="Profile" className="w-full h-full object-cover" />
                    {isScanningFace && (
                      <div className="absolute inset-0 bg-black/75 backdrop-blur-xs flex items-center justify-center">
                        <RefreshCw className="w-5 h-5 text-white animate-spin" />
                      </div>
                    )}
                  </div>

                  <div className="absolute bottom-0 right-0 p-2 rounded-full bg-white text-black shadow-lg">
                    <Camera className="w-3.5 h-3.5" />
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

              {/* Photo Action Pills */}
              <div className="flex items-center gap-2">
                <button
                  type="button"
                  onClick={() => fileInputRef.current?.click()}
                  className="px-3.5 py-1.5 rounded-full bg-white/[0.06] hover:bg-white/12 border border-white/10 text-xs font-semibold text-neutral-200 flex items-center gap-1.5 transition-all backdrop-blur-xl"
                >
                  <Upload className="w-3.5 h-3.5 text-white" />
                  <span>Upload Photo</span>
                </button>

                <button
                  type="button"
                  onClick={startCamera}
                  className="px-3.5 py-1.5 rounded-full bg-white/[0.06] hover:bg-white/12 border border-white/10 text-xs font-semibold text-neutral-200 flex items-center gap-1.5 transition-all backdrop-blur-xl"
                >
                  <Activity className="w-3.5 h-3.5 text-white" />
                  <span>Live Camera</span>
                </button>
              </div>

              {/* Face Verification Badge */}
              {faceResult && (
                <div className="flex items-center gap-1.5 text-xs font-medium">
                  {faceResult.isRealFace ? (
                    <span className="text-emerald-400 flex items-center gap-1">
                      <ShieldCheck className="w-3.5 h-3.5" />
                      <span>Face Verified ✓</span>
                    </span>
                  ) : (
                    <span className="text-rose-400 flex items-center gap-1">
                      <AlertCircle className="w-3.5 h-3.5" />
                      <span>{faceResult.message}</span>
                    </span>
                  )}
                </div>
              )}
            </div>

            {/* FORM INPUTS */}
            <div className="space-y-3">
              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="text-[11px] font-mono uppercase tracking-wider text-neutral-400 block mb-1">
                    Name
                  </label>
                  <input
                    type="text"
                    required
                    value={formData.name}
                    onChange={(e) => setFormData((prev) => ({ ...prev, name: e.target.value }))}
                    className="w-full bg-[#161822] border border-white/10 rounded-2xl px-3.5 py-2.5 text-xs text-white focus:border-white/40 focus:outline-none transition-all"
                    placeholder="Alex Rivera"
                  />
                </div>

                <div>
                  <label className="text-[11px] font-mono uppercase tracking-wider text-neutral-400 block mb-1">
                    Handle
                  </label>
                  <input
                    type="text"
                    required
                    value={formData.handle}
                    onChange={(e) => setFormData((prev) => ({ ...prev, handle: e.target.value }))}
                    className="w-full bg-[#161822] border border-white/10 rounded-2xl px-3.5 py-2.5 text-xs text-white focus:border-white/40 focus:outline-none transition-all"
                    placeholder="@alex_dev"
                  />
                </div>
              </div>

              <div>
                <label className="text-[11px] font-mono uppercase tracking-wider text-neutral-400 block mb-1">
                  Status Note / Quote
                </label>
                <input
                  type="text"
                  required
                  value={formData.quotePrompt}
                  onChange={(e) => setFormData((prev) => ({ ...prev, quotePrompt: e.target.value }))}
                  className="w-full bg-[#161822] border border-white/10 rounded-2xl px-3.5 py-2.5 text-xs text-white font-medium focus:border-white/40 focus:outline-none transition-all"
                  placeholder="Building 3D web experiences with Three.js"
                />
              </div>

              <div className="grid grid-cols-2 gap-3">
                <div>
                  <label className="text-[11px] font-mono uppercase tracking-wider text-neutral-400 block mb-1">
                    Category
                  </label>
                  <select
                    value={formData.category}
                    onChange={(e) =>
                      setFormData((prev) => ({ ...prev, category: e.target.value as any }))
                    }
                    className="w-full bg-[#161822] border border-white/10 rounded-2xl px-3.5 py-2.5 text-xs text-white focus:border-white/40 focus:outline-none transition-all"
                  >
                    <option value="Tech">Tech</option>
                    <option value="Health">Health</option>
                    <option value="Design">Design</option>
                    <option value="Finance">Finance</option>
                    <option value="AI">AI</option>
                    <option value="Creative">Creative</option>
                  </select>
                </div>

                <div>
                  <label className="text-[11px] font-mono uppercase tracking-wider text-neutral-400 block mb-1">
                    Tags
                  </label>
                  <input
                    type="text"
                    value={tagInput}
                    onChange={(e) => handleTagsChange(e.target.value)}
                    className="w-full bg-[#161822] border border-white/10 rounded-2xl px-3.5 py-2.5 text-xs text-white focus:border-white/40 focus:outline-none transition-all"
                    placeholder="Web3D, React, UX"
                  />
                </div>
              </div>

              <div>
                <label className="text-[11px] font-mono uppercase tracking-wider text-neutral-400 block mb-1">
                  Bio
                </label>
                <textarea
                  required
                  rows={3}
                  value={formData.bio}
                  onChange={(e) => setFormData((prev) => ({ ...prev, bio: e.target.value }))}
                  className="w-full bg-[#161822] border border-white/10 rounded-2xl p-3 text-xs text-white focus:border-white/40 focus:outline-none leading-relaxed transition-all"
                  placeholder="Tell nearby people what you are working on within 30m..."
                />
              </div>
            </div>

            {/* SAVE BUTTON (PURE WHITE CTA) */}
            <div className="pt-2">
              <button
                type="submit"
                disabled={!faceResult?.isRealFace || isScanningFace}
                className="w-full py-3.5 rounded-2xl bg-white hover:bg-neutral-200 text-black font-extrabold text-xs flex items-center justify-center gap-2 shadow-xl disabled:opacity-40 disabled:cursor-not-allowed transition-all active:scale-95"
              >
                <Check className="w-4 h-4" />
                <span>Save Profile Changes</span>
              </button>
            </div>
          </form>
        ) : (
          /* TAB 2: ACCOUNT SECURITY */
          <div className="space-y-4 py-2">
            <div className="p-4 rounded-2xl bg-white/[0.04] border border-white/10 space-y-1">
              <span className="text-[10px] font-mono uppercase text-neutral-400">Account Session</span>
              <p className="text-sm font-bold text-white">{formData.handle}</p>
            </div>

            {resetMessage && (
              <div className="p-3 rounded-xl bg-emerald-500/10 border border-emerald-500/30 text-emerald-300 text-xs font-medium flex items-center gap-2">
                <Check className="w-4 h-4" />
                <span>{resetMessage}</span>
              </div>
            )}

            <div className="space-y-2">
              <button
                type="button"
                onClick={handleForgotPassword}
                className="w-full p-3.5 rounded-2xl bg-white/[0.04] hover:bg-white/[0.08] border border-white/10 text-white text-xs font-semibold flex items-center justify-between transition-all active:scale-95"
              >
                <div className="flex items-center gap-2.5">
                  <KeyRound className="w-4 h-4 text-white" />
                  <span>Forgot Password</span>
                </div>
                <span className="text-xs text-neutral-400">Reset</span>
              </button>

              <button
                type="button"
                onClick={() => {
                  setIsDeactivated(!isDeactivated);
                  if (onDeactivateAccount) onDeactivateAccount();
                }}
                className="w-full p-3.5 rounded-2xl bg-white/[0.04] hover:bg-white/[0.08] border border-white/10 text-white text-xs font-semibold flex items-center justify-between transition-all active:scale-95"
              >
                <div className="flex items-center gap-2.5">
                  <UserX className="w-4 h-4 text-white" />
                  <span>Deactivate Account</span>
                </div>
                <span className="text-xs font-bold text-neutral-400">
                  {isDeactivated ? 'Deactivated' : 'Active'}
                </span>
              </button>

              <button
                type="button"
                onClick={() => {
                  if (onLogout) onLogout();
                  onClose();
                }}
                className="w-full p-3.5 rounded-2xl bg-white/[0.04] hover:bg-white/[0.08] border border-white/10 text-white text-xs font-semibold flex items-center justify-between transition-all active:scale-95"
              >
                <div className="flex items-center gap-2.5">
                  <LogOut className="w-4 h-4 text-white" />
                  <span>Log Out</span>
                </div>
                <span className="text-xs text-neutral-400">Sign Out</span>
              </button>
            </div>

            {/* Danger Zone: Delete Account */}
            <div className="pt-4 border-t border-white/10">
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
                        onClose();
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
        )}
      </div>
    </div>
  );
};

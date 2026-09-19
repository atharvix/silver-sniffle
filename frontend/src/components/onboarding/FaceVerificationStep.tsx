import React from 'react';
import { Scan, ShieldCheck, Camera, Sparkles } from 'lucide-react';

interface FaceVerificationStepProps {
  videoRef: React.RefObject<HTMLVideoElement | null>;
  cameraActive: boolean;
  scanStatus: string;
  faceProgress: number;
}

export const FaceVerificationStep: React.FC<FaceVerificationStepProps> = ({
  videoRef,
  cameraActive,
  scanStatus,
  faceProgress,
}) => {
  const isDenied = scanStatus.toLowerCase().includes('denied') || scanStatus.toLowerCase().includes('required');
  const isComplete = faceProgress >= 100;

  return (
    <div className="space-y-6 text-center select-none">
      <div>
        <div className="flex items-center justify-center gap-2 text-white/60 text-[11px] font-semibold uppercase tracking-[0.2em] mb-2.5">
          <ShieldCheck className="w-4 h-4 text-emerald-400" />
          <span>Human Verification</span>
        </div>
        <h2 className="text-2xl font-extrabold text-white tracking-tight leading-snug">
          Real Human Verification
        </h2>
        <p className="text-xs text-white/60 mt-1.5 leading-relaxed max-w-xs mx-auto">
          Scan your face to prove you are an actual human before continuing.
        </p>
      </div>

      {/* Camera Frame & Scanning Oval */}
      <div className="relative w-64 h-80 mx-auto rounded-[130px] overflow-hidden bg-black/50 border-2 border-white/20 shadow-2xl flex items-center justify-center">
        <video
          ref={videoRef}
          autoPlay
          playsInline
          muted
          className="w-full h-full object-cover scale-x-[-1]"
        />

        {/* Animated Scanning Guideline */}
        {cameraActive && !isComplete && (
          <div className="absolute inset-0 pointer-events-none flex items-center justify-center">
            <div className="w-56 h-72 rounded-[115px] border-2 border-dashed border-white/30 animate-pulse" />
          </div>
        )}

        {isComplete && (
          <div className="absolute inset-0 bg-emerald-950/40 backdrop-blur-xs flex flex-col items-center justify-center p-4">
            <ShieldCheck className="w-12 h-12 text-emerald-400 mb-2 animate-bounce" />
            <p className="text-sm font-extrabold text-white">Face Verified!</p>
          </div>
        )}

        {!cameraActive && (
          <div className="absolute inset-0 flex flex-col items-center justify-center p-6 bg-black/85 backdrop-blur-xs text-center space-y-3">
            {isDenied ? (
              <>
                <Camera className="w-9 h-9 text-rose-400 mb-1" />
                <p className="text-xs text-rose-300 font-semibold leading-relaxed">
                  Camera permission is required to verify real human presence.
                </p>
                <p className="text-[11px] text-white/40">
                  Please enable camera access in your device/browser permissions.
                </p>
              </>
            ) : (
              <>
                <Scan className="w-10 h-10 text-white/70 animate-pulse mb-1" />
                <p className="text-xs text-white/80 font-medium">Starting secure camera…</p>
              </>
            )}
          </div>
        )}
      </div>

      {/* Progress & Live Guidance Text */}
      <div className="max-w-xs mx-auto space-y-3">
        <div className="h-1.5 rounded-full bg-white/10 overflow-hidden">
          <div
            className={`h-full rounded-full transition-[width] duration-300 ${
              isComplete ? 'bg-emerald-400' : 'bg-white'
            }`}
            style={{ width: `${faceProgress}%` }}
          />
        </div>

        <div className="min-h-8 flex items-center justify-center">
          <p
            className={`text-xs font-semibold tracking-wide ${
              isDenied ? 'text-rose-400' : isComplete ? 'text-emerald-400' : 'text-white/80'
            }`}
          >
            {scanStatus}
          </p>
        </div>

        {/* Informational Subtext */}
        <div className="flex items-center justify-center gap-1.5 text-[11px] text-white/40 font-medium">
          <Sparkles className="w-3.5 h-3.5 text-white/50 shrink-0" />
          <span>Kinjo uses live motion to prevent bots and fake accounts.</span>
        </div>
      </div>
    </div>
  );
};

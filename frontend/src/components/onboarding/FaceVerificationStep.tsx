import React from 'react';
import { Scan } from 'lucide-react';

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
  return (
    <div className="space-y-6 text-center">
      <div>
        <div className="flex items-center justify-center gap-2 text-white/50 text-[11px] font-semibold uppercase tracking-[0.2em] mb-3">
          <Scan className="w-3.5 h-3.5 text-white/70" />
          <span>Face Check</span>
        </div>
        <h2 className="text-2xl font-extrabold text-white tracking-tight leading-snug">
          Quick Face Verification
        </h2>
        <p className="text-xs text-white/60 mt-1.5 leading-relaxed">
          Center your face in the oval frame to confirm live presence.
        </p>
      </div>

      {/* Camera Frame & Scanning Oval */}
      <div className="relative w-64 h-80 mx-auto rounded-[130px] overflow-hidden bg-white/[0.03] border border-white/20 shadow-2xl flex items-center justify-center">
        <video
          ref={videoRef}
          autoPlay
          playsInline
          muted
          className="w-full h-full object-cover scale-x-[-1]"
        />

        {!cameraActive && (
          <div className="absolute inset-0 flex flex-col items-center justify-center p-4 bg-black/70 backdrop-blur-xs text-center">
            <Scan className="w-10 h-10 text-white/70 animate-pulse mb-2" />
            <p className="text-xs text-white/70 font-medium">Starting camera...</p>
          </div>
        )}
      </div>

      <div className="max-w-xs mx-auto space-y-3">
        <div className="h-1 rounded-full bg-white/10 overflow-hidden">
          <div
            className="h-full rounded-full bg-white transition-[width] duration-300"
            style={{ width: `${faceProgress}%` }}
          />
        </div>
        <p className="text-xs font-semibold text-white/70 tracking-wide min-h-4">
          {scanStatus}
        </p>
      </div>
    </div>
  );
};

import React from 'react';
import { Scan, ShieldCheck, Camera, RefreshCw } from 'lucide-react';

interface FaceVerificationStepProps {
  videoRef: React.RefObject<HTMLVideoElement | null>;
  cameraActive: boolean;
  scanStatus: string;
  faceProgress: number;
  onOpenSettings?: () => void;
  onRetry?: () => void;
}

export const FaceVerificationStep: React.FC<FaceVerificationStepProps> = ({
  videoRef,
  cameraActive,
  scanStatus,
  faceProgress,
  onOpenSettings,
  onRetry,
}) => {
  const isDenied = scanStatus.toLowerCase().includes('denied') || scanStatus.toLowerCase().includes('required');
  const isComplete = faceProgress >= 100;

  return (
    <div className="space-y-6 text-center select-none">
      <div className="space-y-1.5">
        <h2 className="text-[34px] font-normal leading-[1.04]" style={{ letterSpacing: '-.045em' }}>
          Face verification
        </h2>
      </div>

      {/* Camera Frame & Scanning Oval */}
      <div
        className="relative w-64 h-80 mx-auto overflow-hidden flex items-center justify-center shadow-2xl"
        style={{
          background: 'var(--surface)',
          border: '1.5px solid var(--hairline)',
          borderRadius: 130,
        }}
      >
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
            <div
              className="w-56 h-72 animate-pulse"
              style={{ border: '2px dashed var(--hairline)', borderRadius: 115 }}
            />
          </div>
        )}

        {isComplete && (
          <div className="absolute inset-0 flex flex-col items-center justify-center p-4 space-y-2" style={{ background: 'color-mix(in srgb, var(--bg) 88%, transparent)' }}>
            <div
              className="w-14 h-14 rounded-full flex items-center justify-center animate-bounce"
              style={{ background: 'rgba(52,211,153,0.15)', border: '1px solid rgba(52,211,153,0.35)' }}
            >
              <ShieldCheck className="w-8 h-8 text-emerald-400" />
            </div>
            <p className="text-sm font-semibold">Face verified</p>
            <p className="text-[11px]" style={{ color: 'var(--muted)' }}>Proceeding to your profile…</p>
          </div>
        )}

        {!cameraActive && (
          <div className="absolute inset-0 flex flex-col items-center justify-center p-6 text-center space-y-3" style={{ background: 'color-mix(in srgb, var(--bg) 95%, transparent)' }}>
            {isDenied ? (
              <>
                <Camera className="w-8 h-8 mb-1" style={{ color: 'var(--danger)' }} />
                <p className="text-sm font-medium leading-relaxed" style={{ color: 'var(--danger)' }}>
                  Camera permission required
                </p>
                <p className="text-[11px]" style={{ color: 'var(--faint)' }}>
                  Allow Camera in your device settings, then tap Retry.
                </p>
              </>
            ) : (
              <>
                <Scan className="w-8 h-8 animate-pulse mb-1" style={{ color: 'var(--muted)' }} />
                <p className="text-sm font-medium" style={{ color: 'var(--muted)' }}>Initializing camera…</p>
              </>
            )}
          </div>
        )}
      </div>

      {/* Progress & Live Guidance Text */}
      <div className="max-w-xs mx-auto space-y-3">
        <div className="h-1.5 rounded-full overflow-hidden" style={{ background: 'var(--soft)' }}>
          <div
            className="h-full rounded-full transition-[width] duration-300"
            style={{ width: `${faceProgress}%`, background: isComplete ? '#34d399' : 'var(--fg)' }}
          />
        </div>

        <div className="min-h-6 flex items-center justify-center">
          <p
            className="text-sm font-medium tracking-wide"
            style={{ color: isDenied ? 'var(--danger)' : isComplete ? '#34d399' : 'var(--muted)' }}
          >
            {scanStatus}
          </p>
        </div>

        {/*
          Recovery path. Once the OS has a denial on record it will not show the
          dialog again, so the only way forward is the app's settings screen.
        */}
        {isDenied && (
          <div className="flex flex-wrap gap-2 justify-center">
            <button
              type="button"
              className="btn btn--line btn--compact"
              onClick={onOpenSettings}
            >
              <span>Open settings</span>
            </button>
            <button
              type="button"
              className="btn btn--compact"
              onClick={onRetry}
            >
              <RefreshCw className="stroke" strokeWidth={2} />
              <span>Retry</span>
            </button>
          </div>
        )}
      </div>
    </div>
  );
};

import React from 'react';
import { ArrowRight, BellRing } from 'lucide-react';

interface LocationPermissionStepProps {
  onAllow: () => void;
}

export const LocationPermissionStep: React.FC<LocationPermissionStepProps> = ({
  onAllow,
}) => {
  return (
    <div className="space-y-8 text-left select-none max-w-md mx-auto">
      {/* Big 30m display and editorial headline matching Image 2 */}
      <div className="pt-2">
        <h1
          className="text-[76px] sm:text-[96px] font-bold leading-none tracking-tight"
          style={{ color: 'var(--fg)', letterSpacing: '-0.04em' }}
        >
          30<span className="text-[44px] sm:text-[54px] font-normal tracking-normal">m</span>
        </h1>

        <h2
          className="text-[34px] sm:text-[42px] font-normal leading-[1.08] mt-3"
          style={{ letterSpacing: '-0.035em', color: 'var(--fg)' }}
        >
          See who’s<br />around you.
        </h2>

        <p className="text-sm sm:text-base leading-relaxed mt-4 max-w-sm" style={{ color: 'var(--muted)' }}>
          Kinjo needs your location to show people within 30 metres, what they do and what they’re looking for.
        </p>
      </div>

      {/*
        Background location disclosure. Shown before the permission prompt
        because it is the honest description of what continuing does, and
        because being discoverable only while the app is open is the exact
        problem this feature exists to solve.
      */}
      <div
        className="p-4 space-y-2"
        style={{ background: 'var(--surface)', border: '1px solid var(--hairline)' }}
      >
        <p className="eyebrow flex items-center gap-2" style={{ color: 'var(--fg)' }}>
          <BellRing className="w-3.5 h-3.5 shrink-0" aria-hidden="true" />
          Keeps running in the background
        </p>
        <p className="text-[13px] leading-relaxed" style={{ color: 'var(--muted)' }}>
          So people nearby can still find you when your phone is in your pocket, Kinjo keeps your
          location updated while the app is closed. A small notification stays visible the whole
          time it is on, so you always know.
        </p>
        <p className="text-[13px] leading-relaxed" style={{ color: 'var(--faint)' }}>
          Change or turn this off any time in Settings. Kinjo never shares your exact location —
          only that you are within 30 metres.
        </p>
      </div>

      {/* Action Buttons */}
      <div className="space-y-3 pt-6">
        <button
          type="button"
          onClick={onAllow}
          className="btn"
        >
          <span>Allow location</span>
          <ArrowRight className="stroke" strokeWidth={2} />
        </button>
      </div>
    </div>
  );
};

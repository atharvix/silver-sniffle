import React from 'react';
import { Navigation, Menu, MapPin } from 'lucide-react';
import type { GPSState } from '../hooks/useGPSLocation';

interface HeaderProps {
  gps: GPSState;
  connectedCount: number;
  onOpenConnectedDrawer: () => void;
  onOpenLocationPicker: () => void;
}

export const Header: React.FC<HeaderProps> = ({
  gps,
  connectedCount,
  onOpenConnectedDrawer,
  onOpenLocationPicker,
}) => {
  return (
    <header className="app-header relative z-30 w-full max-w-md mx-auto px-5 pt-4 sm:pt-6 flex items-center justify-between gap-3 select-none">
      {/* Left: Minimalist Logo "k." */}
      <h1 className="text-2xl sm:text-3xl font-black tracking-tight text-white font-sans shrink-0">
        k<span className="text-white/40">.</span>
      </h1>

      {/* Center: Location Pill */}
      <button
        type="button"
        onClick={onOpenLocationPicker}
        className="glass-control flex items-center gap-1.5 px-3.5 py-1.5 rounded-full transition-all active:scale-95 group max-w-[200px] sm:max-w-xs shrink"
        title="Click to change location"
      >
        <span className="relative flex h-2 w-2 shrink-0">
          <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span>
          <span className="relative inline-flex rounded-full h-2 w-2 bg-emerald-500"></span>
        </span>
        <Navigation className="w-3 h-3 text-emerald-400 group-hover:text-white transition-colors shrink-0" />
        <span className="text-xs font-medium text-neutral-200 font-sans tracking-wide truncate">
          {gps.loading ? 'Locating...' : gps.formattedLocation}
        </span>
        <MapPin className="w-3 h-3 text-neutral-500 group-hover:text-white transition-colors shrink-0" />
      </button>

      {/* Right: Connected Profiles Drawer Button */}
      <button
        onClick={onOpenConnectedDrawer}
        className="glass-control relative p-2.5 rounded-full text-neutral-300 hover:text-white active:scale-95 transition-all shrink-0"
        title="Open Connected Profiles"
      >
        <Menu className="w-4 h-4 text-white" />
        {connectedCount > 0 && (
          <span className="absolute -top-1 -right-1 w-4.5 h-4.5 rounded-full bg-white text-black font-extrabold text-[10px] flex items-center justify-center border border-black shadow-md">
            {connectedCount}
          </span>
        )}
      </button>
    </header>
  );
};

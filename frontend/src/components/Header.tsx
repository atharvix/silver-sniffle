import React from 'react';
import type { FilterState } from '../types';
import { SlidersHorizontal } from 'lucide-react';


interface HeaderProps {
  filters: FilterState;
  onUpdateRadius: (radius: number) => void;
  onOpenFilterModal: () => void;
}

export const Header: React.FC<HeaderProps> = ({
  filters,
  onUpdateRadius,
  onOpenFilterModal,
}) => {
  const RADIUS_PRESETS = [
    { value: 10, label: '10m Event' },
    { value: 50, label: '50m Building' },
    { value: 100, label: '100m Block' },
    { value: 500, label: '500m Area' },
  ];

  return (
    <header className="relative z-30 w-full max-w-xl mx-auto px-4 pt-5 flex items-center justify-between gap-2">
      {/* Left: Branding */}
      <h1 className="text-3xl font-extrabold tracking-tighter text-white select-none">k.</h1>

      {/* Center: Radius Selector Presets */}
      <div className="flex items-center gap-1 bg-white/[0.05] border border-white/10 p-1 rounded-full backdrop-blur-md">
        {RADIUS_PRESETS.map((preset) => (
          <button
            key={preset.value}
            onClick={() => onUpdateRadius(preset.value)}
            className={`px-2.5 py-1 rounded-full text-[11px] font-semibold transition-all ${
              filters.maxRadiusMeters === preset.value
                ? 'bg-[#f2ece1] text-neutral-950 shadow-sm'
                : 'text-neutral-400 hover:text-white'
            }`}
          >
            {preset.label}
          </button>
        ))}
      </div>

      {/* Right: Search & Filter Trigger */}
      <button
        onClick={onOpenFilterModal}
        className={`p-2 rounded-full border transition-all active:scale-95 ${
          filters.searchQuery || filters.selectedCategory !== 'All'
            ? 'bg-white text-black border-white'
            : 'bg-white/[0.05] text-neutral-300 hover:text-white border-white/10'
        }`}
        title="Search Bio & Filter Options"
      >
        <SlidersHorizontal className="w-4 h-4" />
      </button>
    </header>
  );
};

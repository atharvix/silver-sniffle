import React from 'react';
import type { FilterState } from '../types';
import { X, Search, Check, RefreshCw } from 'lucide-react';

interface FilterModalProps {
  isOpen: boolean;
  onClose: () => void;
  filters: FilterState;
  onUpdateFilters: (newFilters: FilterState) => void;
  onResetFilters: () => void;
}

export const FilterModal: React.FC<FilterModalProps> = ({
  isOpen,
  onClose,
  filters,
  onUpdateFilters,
  onResetFilters,
}) => {
  if (!isOpen) return null;

  const CATEGORIES = ['All', 'Tech', 'Health', 'Design', 'Finance', 'AI', 'Creative'];

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/85 backdrop-blur-md overflow-y-auto">
      <div className="relative w-full max-w-md bg-[#121319] rounded-[32px] border border-white/10 p-6 space-y-6 my-auto shadow-2xl">
        {/* Header */}
        <div className="flex items-center justify-between pb-2 border-b border-white/10">
          <div>
            <h2 className="text-lg font-bold text-white tracking-tight">Search & Filter</h2>
            <p className="text-xs text-neutral-400">Discover people nearby within 30m</p>
          </div>

          <button
            onClick={onClose}
            className="w-8 h-8 rounded-full bg-white/5 hover:bg-white/10 text-neutral-400 hover:text-white flex items-center justify-center transition-colors"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        {/* Bio & Keyword Search Field */}
        <div>
          <label className="text-xs font-semibold text-neutral-400 block mb-1.5">
            Search Keywords
          </label>
          <div className="relative">
            <Search className="w-4 h-4 absolute left-3.5 top-3.5 text-neutral-500" />
            <input
              type="text"
              value={filters.searchQuery}
              onChange={(e) => onUpdateFilters({ ...filters, searchQuery: e.target.value })}
              placeholder="Search 'legal tech', 'co-founder', 'AI'..."
              className="w-full bg-white/[0.04] border border-white/10 rounded-xl pl-10 pr-3.5 py-2.5 text-xs text-white focus:border-white/30 focus:outline-none"
            />
          </div>
        </div>

        {/* Category Pills */}
        <div>
          <label className="text-xs font-semibold text-neutral-400 block mb-2">Category</label>
          <div className="flex flex-wrap gap-2">
            {CATEGORIES.map((cat) => (
              <button
                key={cat}
                type="button"
                onClick={() => onUpdateFilters({ ...filters, selectedCategory: cat })}
                className={`px-3 py-1.5 rounded-xl text-xs font-semibold transition-all ${
                  filters.selectedCategory === cat
                    ? 'bg-amber-400 text-black font-bold'
                    : 'bg-white/[0.04] text-neutral-300 hover:bg-white/[0.08] border border-white/10'
                }`}
              >
                {cat}
              </button>
            ))}
          </div>
        </div>

        {/* Footer Actions */}
        <div className="flex items-center justify-between pt-2 border-t border-white/10">
          <button
            onClick={onResetFilters}
            className="flex items-center gap-1.5 text-xs text-neutral-400 hover:text-white transition-colors"
          >
            <RefreshCw className="w-3.5 h-3.5" />
            <span>Reset</span>
          </button>

          <button
            onClick={onClose}
            className="flex items-center gap-1.5 bg-amber-400 hover:bg-amber-300 text-black text-xs font-bold py-2.5 px-5 rounded-xl shadow-lg transition-all"
          >
            <Check className="w-4 h-4" />
            <span>Apply</span>
          </button>
        </div>
      </div>
    </div>
  );
};

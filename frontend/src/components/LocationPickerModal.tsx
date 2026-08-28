import React, { useState } from 'react';
import { X, Search, Navigation, MapPin, Check, Sparkles } from 'lucide-react';
import { searchLocationByName, type GeoAddress } from '../utils/reverseGeocode';

interface LocationPickerModalProps {
  isOpen: boolean;
  onClose: () => void;
  currentLocation: string;
  onSelectLocation: (location: GeoAddress) => void;
  onUseAutoGPS: () => void;
}

export const LocationPickerModal: React.FC<LocationPickerModalProps> = ({
  isOpen,
  onClose,
  currentLocation,
  onSelectLocation,
  onUseAutoGPS,
}) => {
  const [searchQuery, setSearchQuery] = useState('');
  const [searchResults, setSearchResults] = useState<GeoAddress[]>([]);
  const [isSearching, setIsSearching] = useState(false);

  if (!isOpen) return null;

  const handleSearch = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!searchQuery.trim()) return;

    setIsSearching(true);
    const results = await searchLocationByName(searchQuery);
    setIsSearching(false);
    setSearchResults(results);
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/85 backdrop-blur-2xl overflow-y-auto select-none">
      <div className="relative w-full max-w-md bg-[#090b10]/95 rounded-[32px] border border-white/15 p-6 space-y-5 my-auto shadow-2xl">
        {/* Header */}
        <div className="flex items-center justify-between pb-3 border-b border-white/10">
          <div>
            <h2 className="text-lg font-bold text-white tracking-tight">Select Location</h2>
            <p className="text-xs text-neutral-400">Current: {currentLocation}</p>
          </div>

          <button
            onClick={onClose}
            className="w-8 h-8 rounded-full bg-white/5 hover:bg-white/10 text-neutral-400 hover:text-white flex items-center justify-center transition-colors border border-white/10"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        {/* Auto GPS Trigger Button */}
        <button
          type="button"
          onClick={() => {
            onUseAutoGPS();
            onClose();
          }}
          className="w-full p-3.5 rounded-2xl bg-sky-400/10 hover:bg-sky-400/20 border border-sky-400/30 text-sky-300 text-xs font-bold flex items-center justify-between transition-all active:scale-95 shadow-md backdrop-blur-xl"
        >
          <div className="flex items-center gap-2.5">
            <Navigation className="w-4 h-4 text-emerald-400" />
            <span>Use Auto GPS / IP Location</span>
          </div>
          <Sparkles className="w-4 h-4 text-sky-400" />
        </button>

        {/* Search Bar */}
        <form onSubmit={handleSearch} className="space-y-2">
          <label className="text-xs font-semibold text-neutral-400 block">
            Search Area or City Name
          </label>
          <div className="relative">
            <Search className="w-4 h-4 absolute left-3.5 top-3.5 text-neutral-500" />
            <input
              type="text"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              placeholder="Type 'Jaipur', 'Malviya Nagar', 'Delhi'..."
              className="w-full bg-white/[0.04] border border-white/12 rounded-xl pl-10 pr-3.5 py-2.5 text-xs text-white focus:border-sky-400/50 focus:outline-none"
            />
          </div>
          <button
            type="submit"
            disabled={isSearching || !searchQuery.trim()}
            className="w-full py-2.5 bg-sky-400 hover:bg-sky-300 text-black font-extrabold text-xs rounded-xl disabled:opacity-40 transition-all shadow-md active:scale-95"
          >
            {isSearching ? 'Searching Location...' : 'Search Location'}
          </button>
        </form>

        {/* Search Results */}
        {searchResults.length > 0 && (
          <div className="space-y-2 pt-1">
            <label className="text-xs font-semibold text-neutral-400 block">Search Results</label>
            <div className="space-y-1.5 max-h-48 overflow-y-auto pr-1">
              {searchResults.map((res, i) => (
                <button
                  key={i}
                  type="button"
                  onClick={() => {
                    onSelectLocation(res);
                    onClose();
                  }}
                  className="w-full p-2.5 rounded-xl bg-white/[0.04] hover:bg-sky-400/10 border border-white/10 text-left text-xs text-white flex items-center justify-between transition-all"
                >
                  <div className="flex items-center gap-2 truncate">
                    <MapPin className="w-3.5 h-3.5 text-sky-400 shrink-0" />
                    <span className="truncate">{res.formatted}</span>
                  </div>
                  <Check className="w-3.5 h-3.5 text-emerald-400 shrink-0 opacity-0 hover:opacity-100" />
                </button>
              ))}
            </div>
          </div>
        )}
      </div>
    </div>
  );
};

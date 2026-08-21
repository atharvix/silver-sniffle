import React, { useState } from 'react';
import type { UserProfile } from '../types';
import { X, Check, AlertCircle, Eye } from 'lucide-react';
import { ProfileCard } from './ProfileCard';

interface ProfileEditorModalProps {
  userProfile: UserProfile;
  isOpen: boolean;
  onClose: () => void;
  onSave: (updatedProfile: UserProfile) => void;
}

export const ProfileEditorModal: React.FC<ProfileEditorModalProps> = ({
  userProfile,
  isOpen,
  onClose,
  onSave,
}) => {
  const [formData, setFormData] = useState<UserProfile>({ ...userProfile });
  const [tagInput, setTagInput] = useState(userProfile.tags.join(', '));
  const [showPreview, setShowPreview] = useState(false);

  if (!isOpen) return null;

  // Bio Word Count Calculation
  const wordsArray = formData.bio.trim().split(/\s+/).filter(Boolean);
  const wordCount = wordsArray.length;
  const isTooShort = wordCount < 50;
  const isTooLong = wordCount > 100;
  const isValidWordCount = wordCount >= 50 && wordCount <= 100;

  const handleTagsChange = (val: string) => {
    setTagInput(val);
    const parsed = val.split(',').map((t) => t.trim()).filter(Boolean);
    setFormData((prev) => ({ ...prev, tags: parsed }));
  };

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!isValidWordCount) return;
    onSave(formData);
    onClose();
  };

  const AVATAR_PRESETS = [
    'https://images.unsplash.com/photo-1539571696357-5a69c17a67c6?auto=format&fit=crop&q=80&w=1000',
    'https://images.unsplash.com/photo-1534528741775-53994a69daeb?auto=format&fit=crop&q=80&w=1000',
    'https://images.unsplash.com/photo-1507003211169-0a1dd7228f2d?auto=format&fit=crop&q=80&w=1000',
    'https://images.unsplash.com/photo-1517841905240-472988babdf9?auto=format&fit=crop&q=80&w=1000',
  ];

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/80 backdrop-blur-md overflow-y-auto">
      <div className="relative w-full max-w-lg max-h-[85vh] bg-[#111217] rounded-[32px] border border-white/10 p-6 md:p-8 overflow-y-auto my-auto shadow-2xl space-y-6">
        {/* Header */}
        <div className="flex items-center justify-between pb-2 border-b border-white/10">
          <div>
            <h2 className="text-xl font-bold text-white tracking-tight">Your Profile & Settings</h2>
            <p className="text-xs text-neutral-400">Configure how nearby signals see your card</p>
          </div>

          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={() => setShowPreview(!showPreview)}
              className="text-xs text-neutral-300 hover:text-white bg-white/5 hover:bg-white/10 border border-white/10 px-3 py-1.5 rounded-full flex items-center gap-1.5 transition-colors"
            >
              <Eye className="w-3.5 h-3.5" />
              <span>{showPreview ? 'Edit Form' : 'Card Preview'}</span>
            </button>

            <button
              onClick={onClose}
              className="w-8 h-8 rounded-full bg-white/5 hover:bg-white/10 text-neutral-400 hover:text-white flex items-center justify-center transition-colors"
            >
              <X className="w-4 h-4" />
            </button>
          </div>
        </div>

        {/* Card Preview Toggle View */}
        {showPreview ? (
          <div className="w-full flex flex-col items-center justify-center py-4">
            <div className="w-full max-w-sm h-[480px]">
              <ProfileCard profile={formData} />
            </div>
          </div>
        ) : (
          /* Super Clean Uncluttered Form */
          <form onSubmit={handleSubmit} className="space-y-5">
            {/* Avatar Header & Preset Picker */}
            <div className="flex items-center gap-4 bg-white/[0.03] p-4 rounded-2xl border border-white/5">
              <img
                src={formData.avatar}
                alt="Avatar"
                className="w-14 h-14 rounded-full object-cover border border-white/20"
              />
              <div className="flex-1">
                <span className="text-xs font-semibold text-neutral-400 block mb-1.5">Choose Avatar</span>
                <div className="flex items-center gap-2">
                  {AVATAR_PRESETS.map((url, i) => (
                    <button
                      key={i}
                      type="button"
                      onClick={() => setFormData((prev) => ({ ...prev, avatar: url }))}
                      className={`w-7 h-7 rounded-full overflow-hidden border transition-all ${
                        formData.avatar === url ? 'border-emerald-400 scale-110' : 'border-white/20 opacity-60 hover:opacity-100'
                      }`}
                    >
                      <img src={url} alt="preset" className="w-full h-full object-cover" />
                    </button>
                  ))}
                </div>
              </div>
            </div>

            {/* Full Name & Handle */}
            <div className="grid grid-cols-2 gap-3">
              <div>
                <label className="text-xs font-semibold text-neutral-400 block mb-1">Full Name</label>
                <input
                  type="text"
                  required
                  value={formData.name}
                  onChange={(e) => setFormData((prev) => ({ ...prev, name: e.target.value }))}
                  className="w-full bg-white/[0.04] border border-white/10 rounded-xl px-3.5 py-2.5 text-xs text-white focus:border-white/30 focus:outline-none"
                  placeholder="Alex Rivera"
                />
              </div>

              <div>
                <label className="text-xs font-semibold text-neutral-400 block mb-1">Handle</label>
                <input
                  type="text"
                  required
                  value={formData.handle}
                  onChange={(e) => setFormData((prev) => ({ ...prev, handle: e.target.value }))}
                  className="w-full bg-white/[0.04] border border-white/10 rounded-xl px-3.5 py-2.5 text-xs text-white focus:border-white/30 focus:outline-none"
                  placeholder="@alex_dev"
                />
              </div>
            </div>

            {/* Quoted Prompt Headline */}
            <div>
              <label className="text-xs font-semibold text-neutral-400 block mb-1">Headline Prompt</label>
              <input
                type="text"
                required
                value={formData.quotePrompt}
                onChange={(e) => setFormData((prev) => ({ ...prev, quotePrompt: e.target.value }))}
                className="w-full bg-white/[0.04] border border-white/10 rounded-xl px-3.5 py-2.5 text-xs text-white font-medium focus:border-white/30 focus:outline-none"
                placeholder="Looking for a co-founder for my medical startup"
              />
            </div>

            {/* Category & Tags */}
            <div className="grid grid-cols-2 gap-3">
              <div>
                <label className="text-xs font-semibold text-neutral-400 block mb-1">Category</label>
                <select
                  value={formData.category}
                  onChange={(e) =>
                    setFormData((prev) => ({ ...prev, category: e.target.value as any }))
                  }
                  className="w-full bg-[#181920] border border-white/10 rounded-xl px-3.5 py-2.5 text-xs text-white focus:border-white/30 focus:outline-none"
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
                <label className="text-xs font-semibold text-neutral-400 block mb-1">Tags (comma separated)</label>
                <input
                  type="text"
                  value={tagInput}
                  onChange={(e) => handleTagsChange(e.target.value)}
                  className="w-full bg-white/[0.04] border border-white/10 rounded-xl px-3.5 py-2.5 text-xs text-white focus:border-white/30 focus:outline-none"
                  placeholder="Medical AI, Founder"
                />
              </div>
            </div>

            {/* Detailed Bio Description with Strict 50-100 Word Count Limit */}
            <div className="space-y-1.5">
              <div className="flex items-center justify-between">
                <label className="text-xs font-semibold text-neutral-400">Detailed Bio (Strict 50–100 Words)</label>
                <span
                  className={`text-xs font-semibold px-2.5 py-0.5 rounded-full ${
                    isValidWordCount
                      ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/20'
                      : isTooShort
                      ? 'bg-amber-500/10 text-amber-400 border border-amber-500/20'
                      : 'bg-rose-500/10 text-rose-400 border border-rose-500/20'
                  }`}
                >
                  {wordCount} / 50-100 words
                </span>
              </div>

              <textarea
                required
                rows={4}
                value={formData.bio}
                onChange={(e) => setFormData((prev) => ({ ...prev, bio: e.target.value }))}
                className={`w-full bg-white/[0.04] border rounded-xl p-3 text-xs text-white focus:outline-none leading-relaxed transition-colors ${
                  isValidWordCount
                    ? 'border-emerald-500/40 focus:border-emerald-400'
                    : isTooShort
                    ? 'border-amber-500/40 focus:border-amber-400'
                    : 'border-rose-500/40 focus:border-rose-400'
                }`}
                placeholder="Write a detailed bio about your background, current project, and what you are looking for in nearby connections (strictly 50 to 100 words)..."
              />

              {/* Status Message */}
              <div>
                {isTooShort && (
                  <p className="text-[11px] text-amber-400 flex items-center gap-1">
                    <AlertCircle className="w-3.5 h-3.5" />
                    <span>Too short! Please add {50 - wordCount} more words to reach 50 words minimum.</span>
                  </p>
                )}
                {isTooLong && (
                  <p className="text-[11px] text-rose-400 flex items-center gap-1">
                    <AlertCircle className="w-3.5 h-3.5" />
                    <span>Too long! Please trim {wordCount - 100} words to stay under 100 words maximum.</span>
                  </p>
                )}
                {isValidWordCount && (
                  <p className="text-[11px] text-emerald-400 flex items-center gap-1">
                    <Check className="w-3.5 h-3.5" />
                    <span>Bio length valid! Ready to publish.</span>
                  </p>
                )}
              </div>
            </div>

            {/* Save Button */}
            <div className="pt-2">
              <button
                type="submit"
                disabled={!isValidWordCount}
                className="w-full flex items-center justify-center gap-2 bg-[#f2ece1] hover:bg-white text-neutral-950 font-bold text-xs py-3.5 rounded-xl shadow-lg disabled:opacity-40 disabled:cursor-not-allowed transition-all"
              >
                <Check className="w-4 h-4" />
                <span>Save Profile</span>
              </button>

            </div>
          </form>
        )}
      </div>
    </div>
  );
};

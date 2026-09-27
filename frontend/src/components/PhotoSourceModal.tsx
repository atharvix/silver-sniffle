import React from 'react';
import { Camera, Image as ImageIcon, X } from 'lucide-react';
import { pickPhoto, type PhotoPickerSource } from '../utils/photoPicker';

interface PhotoSourceModalProps {
  isOpen: boolean;
  onClose: () => void;
  onPhotoSelected: (dataUrl: string) => void;
  title?: string;
}

export const PhotoSourceModal: React.FC<PhotoSourceModalProps> = ({
  isOpen,
  onClose,
  onPhotoSelected,
  title = 'Select Photo',
}) => {
  if (!isOpen) return null;

  const handleSelect = async (source: PhotoPickerSource) => {
    try {
      onClose();
      const dataUrl = await pickPhoto(source);
      if (dataUrl) {
        onPhotoSelected(dataUrl);
      }
    } catch (err: any) {
      if (err?.message !== 'cancelled') {
        console.warn('Failed to pick photo:', err);
      }
    }
  };

  return (
    <div className="fixed inset-0 z-[120] flex items-end sm:items-center justify-center p-4 bg-black/60 backdrop-blur-sm animate-in fade-in duration-150 select-none">
      <div
        className="w-full max-w-sm p-5 shadow-2xl space-y-4 animate-in slide-in-from-bottom-4 duration-200"
        style={{
          borderRadius: 0,
          background: 'var(--surface)',
          border: '1px solid var(--hairline)',
          color: 'var(--fg)',
        }}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-center justify-between pb-2" style={{ borderBottom: '1px solid var(--soft)' }}>
          <h3 className="text-sm font-semibold tracking-tight" style={{ color: 'var(--fg)' }}>{title}</h3>
          <button
            onClick={onClose}
            className="p-1 hover:opacity-75 transition-opacity cursor-pointer"
            style={{ color: 'var(--muted)' }}
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        <div className="space-y-2 pt-1">
          <button
            type="button"
            onClick={() => handleSelect('camera')}
            className="w-full flex items-center gap-3.5 px-4 py-3.5 text-left transition-all active:scale-[0.98] cursor-pointer hover:opacity-90"
            style={{
              borderRadius: 0,
              background: 'var(--surface-2)',
              border: '1px solid var(--hairline)',
              color: 'var(--fg)',
            }}
          >
            <div className="w-9 h-9 flex items-center justify-center shrink-0" style={{ background: 'var(--soft)', borderRadius: 0, color: 'var(--fg)' }}>
              <Camera className="w-4 h-4" />
            </div>
            <div>
              <p className="text-xs font-semibold" style={{ color: 'var(--fg)' }}>Take Live Photo</p>
              <p className="text-[11px]" style={{ color: 'var(--muted)' }}>Use front or rear camera</p>
            </div>
          </button>

          <button
            type="button"
            onClick={() => handleSelect('photos')}
            className="w-full flex items-center gap-3.5 px-4 py-3.5 text-left transition-all active:scale-[0.98] cursor-pointer hover:opacity-90"
            style={{
              borderRadius: 0,
              background: 'var(--surface-2)',
              border: '1px solid var(--hairline)',
              color: 'var(--fg)',
            }}
          >
            <div className="w-9 h-9 flex items-center justify-center shrink-0" style={{ background: 'var(--soft)', borderRadius: 0, color: 'var(--fg)' }}>
              <ImageIcon className="w-4 h-4" />
            </div>
            <div>
              <p className="text-xs font-semibold" style={{ color: 'var(--fg)' }}>Choose from Gallery</p>
              <p className="text-[11px]" style={{ color: 'var(--muted)' }}>Select from photo library</p>
            </div>
          </button>
        </div>

        <button
          type="button"
          onClick={onClose}
          className="w-full py-2.5 text-center text-xs font-medium transition-colors"
          style={{ color: 'var(--muted)' }}
        >
          Cancel
        </button>
      </div>
    </div>
  );
};

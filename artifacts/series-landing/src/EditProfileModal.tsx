import { useRef, useState, ChangeEvent } from 'react';
import { useUpsertProfile } from '@workspace/api-client-react';

interface Props {
  initialName: string;
  initialAbout: string;
  initialPhoto: string | null;
  onClose: () => void;
  onSaved: (profile: { name: string; about: string; photo: string | null }) => void;
}

const ABOUT_MAX = 160;
const MAX_PHOTO_BYTES = 8 * 1024 * 1024;

export default function EditProfileModal({ initialName, initialAbout, initialPhoto, onClose, onSaved }: Props) {
  const [name, setName] = useState(initialName);
  const initialParts = initialAbout.split('\n');
  const [whatYouDo, setWhatYouDo]           = useState(initialParts.length >= 2 ? initialParts[0] : '');
  const [whatLookingFor, setWhatLookingFor] = useState(initialParts.length >= 2 ? initialParts.slice(1).join('\n') : initialAbout);
  const [photoUrl, setPhotoUrl] = useState<string | null>(initialPhoto);
  const [photoDragging, setPhotoDragging] = useState(false);
  const [photoError, setPhotoError] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);

  const fileRef = useRef<HTMLInputElement>(null);
  const upsertProfile = useUpsertProfile();

  const nameValid = name.trim().length > 0;

  function applyPhoto(file: File) {
    setPhotoError('');
    if (!file.type.startsWith('image/')) {
      setPhotoError('Please upload an image file (JPG, PNG, GIF, etc.).');
      return;
    }
    if (file.size > MAX_PHOTO_BYTES) {
      setPhotoError('Image must be under 8 MB.');
      return;
    }
    const reader = new FileReader();
    reader.onload = e => setPhotoUrl(e.target?.result as string);
    reader.onerror = () => setPhotoError('Could not read the file. Please try another.');
    reader.readAsDataURL(file);
  }

  function handleFileChange(e: ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0];
    if (file) applyPhoto(file);
    e.target.value = '';
  }

  function handlePhotoDrop(e: React.DragEvent) {
    e.preventDefault();
    setPhotoDragging(false);
    const file = e.dataTransfer.files?.[0];
    if (file) applyPhoto(file);
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (!nameValid) return;
    setLoading(true);
    setError('');
    try {
      const combinedAbout = [whatYouDo.trim(), whatLookingFor.trim()].filter(Boolean).join('\n');
      await upsertProfile.mutateAsync({
        data: {
          name: name.trim(),
          about: combinedAbout || undefined,
          photo: photoUrl ?? undefined,
        },
      });
      onSaved({ name: name.trim(), about: combinedAbout, photo: photoUrl });
    } catch {
      setError('Could not save changes. Please try again.');
    } finally {
      setLoading(false);
    }
  }

  return (
    <div style={styles.overlay} onClick={onClose}>
      <div style={styles.modal} onClick={e => e.stopPropagation()}>
        <button style={styles.closeBtn} onClick={onClose} aria-label="Close">
          <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round">
            <line x1="18" y1="6" x2="6" y2="18" /><line x1="6" y1="6" x2="18" y2="18" />
          </svg>
        </button>

        <form onSubmit={handleSubmit} style={styles.form}>
          <h2 style={styles.title}>Edit profile</h2>
          <p style={styles.subtitle}>Update your photo, name, and about.</p>

          <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 8, marginTop: 4 }}>
            <label
              htmlFor="edit-photo-upload"
              aria-label="Upload profile photo"
              style={{
                ...styles.photoRing,
                borderColor: photoDragging ? 'rgba(255,255,255,0.6)' : 'rgba(255,255,255,0.18)',
                background:  photoDragging ? 'rgba(255,255,255,0.08)' : 'rgba(255,255,255,0.04)',
                cursor: 'pointer',
              }}
              onDragOver={e => { e.preventDefault(); setPhotoDragging(true); }}
              onDragLeave={() => setPhotoDragging(false)}
              onDrop={handlePhotoDrop}
            >
              <input
                ref={fileRef}
                id="edit-photo-upload"
                type="file"
                accept="image/*"
                style={{ display: 'none' }}
                onChange={handleFileChange}
              />
              {photoUrl ? (
                <img src={photoUrl} alt="Profile preview" style={styles.photoImg} />
              ) : (
                <div style={styles.photoPlaceholder}>
                  <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="rgba(255,255,255,0.4)" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
                    <circle cx="12" cy="8" r="4" />
                    <path d="M4 20c0-4 3.6-7 8-7s8 3 8 7" />
                  </svg>
                </div>
              )}
              <div style={styles.photoBadge}>
                <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="#111" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
                  <path d="M23 19a2 2 0 0 1-2 2H3a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h4l2-3h6l2 3h4a2 2 0 0 1 2 2z" />
                  <circle cx="12" cy="13" r="4" />
                </svg>
              </div>
            </label>
            {photoError
              ? <p style={{ ...styles.error, textAlign: 'center', marginTop: 0 }}>{photoError}</p>
              : <p style={{ ...styles.subtitle, textAlign: 'center', fontSize: 12, margin: 0 }}>
                  {photoUrl
                    ? <button type="button" style={styles.changeBtn} onClick={() => fileRef.current?.click()}>Change photo</button>
                    : 'Click or drag to upload a photo'}
                </p>
            }
          </div>

          <div style={styles.fieldGroup}>
            <label style={styles.label} htmlFor="edit-profile-name">Name</label>
            <input
              id="edit-profile-name"
              type="text"
              placeholder="Nischal Jain"
              maxLength={60}
              value={name}
              onChange={e => setName(e.target.value)}
              style={styles.emailInput}
              autoComplete="name"
            />
          </div>

          <div style={styles.fieldGroup}>
            <label style={styles.label} htmlFor="edit-what-you-do">What you do</label>
            <input
              id="edit-what-you-do"
              type="text"
              placeholder="Owner of a skincare brand"
              maxLength={100}
              value={whatYouDo}
              onChange={e => setWhatYouDo(e.target.value)}
              style={styles.emailInput}
            />
          </div>

          <div style={styles.fieldGroup}>
            <label style={styles.label} htmlFor="edit-looking-for">What you're looking for</label>
            <textarea
              id="edit-looking-for"
              placeholder="Looking to expand my network, meet interesting people, and build a great brand."
              maxLength={160}
              rows={3}
              value={whatLookingFor}
              onChange={e => setWhatLookingFor(e.target.value)}
              style={styles.textarea}
            />
          </div>

          {error && <p style={styles.error}>{error}</p>}

          <button
            type="submit"
            disabled={loading || !nameValid}
            style={{ ...styles.primaryBtn, opacity: (loading || !nameValid) ? 0.45 : 1, marginTop: 4 }}
          >
            {loading ? 'Saving…' : 'Save changes'}
          </button>
        </form>
      </div>
    </div>
  );
}

const styles: Record<string, React.CSSProperties> = {
  overlay: {
    position: 'fixed',
    inset: 0,
    background: 'rgba(0,0,0,0.72)',
    backdropFilter: 'blur(6px)',
    WebkitBackdropFilter: 'blur(6px)',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    zIndex: 1000,
    padding: 16,
  },
  modal: {
    position: 'relative',
    background: 'linear-gradient(145deg, #0c3a44 0%, #0a2e37 60%, #071f26 100%)',
    border: '1px solid rgba(255,255,255,0.12)',
    borderRadius: 20,
    padding: 'clamp(28px, 6vw, 44px) clamp(20px, 6vw, 40px) clamp(24px, 5vw, 40px)',
    width: '100%',
    maxWidth: 420,
    boxShadow: '0 32px 80px rgba(0,0,0,0.7)',
    maxHeight: '90vh',
    overflowY: 'auto',
  },
  closeBtn: {
    position: 'absolute',
    top: 16,
    right: 16,
    background: 'rgba(255,255,255,0.08)',
    border: '1px solid rgba(255,255,255,0.12)',
    borderRadius: 8,
    color: 'rgba(255,255,255,0.6)',
    cursor: 'pointer',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    width: 32,
    height: 32,
    padding: 0,
  },
  form: {
    display: 'flex',
    flexDirection: 'column',
    gap: 16,
  },
  title: {
    fontSize: 24,
    fontWeight: 800,
    color: '#fff',
    letterSpacing: '-0.5px',
    margin: 0,
  },
  subtitle: {
    fontSize: 14,
    color: 'rgba(255,255,255,0.55)',
    lineHeight: 1.5,
    margin: 0,
  },
  emailInput: {
    background: 'rgba(255,255,255,0.06)',
    border: '1px solid rgba(255,255,255,0.15)',
    borderRadius: 10,
    padding: '12px 14px',
    fontSize: 16,
    color: '#fff',
    outline: 'none',
    width: '100%',
    fontFamily: 'inherit',
  },
  primaryBtn: {
    background: '#fff',
    color: '#111',
    border: 'none',
    borderRadius: 10,
    padding: '13px 0',
    fontSize: 15,
    fontWeight: 700,
    cursor: 'pointer',
    width: '100%',
    fontFamily: 'inherit',
    letterSpacing: '-0.2px',
  },
  error: {
    color: '#ff6b6b',
    fontSize: 13,
    margin: 0,
  },
  changeBtn: {
    background: 'none',
    border: 'none',
    color: 'rgba(255,255,255,0.5)',
    fontSize: 13,
    cursor: 'pointer',
    padding: 0,
    fontFamily: 'inherit',
    textDecoration: 'underline',
  },
  photoRing: {
    width: 84,
    height: 84,
    borderRadius: '50%',
    border: '2px dashed rgba(255,255,255,0.18)',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    position: 'relative',
    overflow: 'visible',
  },
  photoImg: {
    width: 80,
    height: 80,
    borderRadius: '50%',
    objectFit: 'cover',
  },
  photoPlaceholder: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    width: '100%',
    height: '100%',
  },
  photoBadge: {
    position: 'absolute',
    bottom: 0,
    right: 0,
    width: 24,
    height: 24,
    borderRadius: '50%',
    background: '#fff',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    border: '2px solid #071f26',
  },
  fieldGroup: {
    display: 'flex',
    flexDirection: 'column',
    gap: 6,
  },
  label: {
    fontSize: 13,
    fontWeight: 600,
    color: 'rgba(255,255,255,0.7)',
    letterSpacing: '0.01em',
  },
  textarea: {
    background: 'rgba(255,255,255,0.06)',
    border: '1px solid rgba(255,255,255,0.15)',
    borderRadius: 10,
    padding: '10px 14px',
    fontSize: 14,
    color: '#fff',
    outline: 'none',
    width: '100%',
    resize: 'vertical',
    fontFamily: 'inherit',
    lineHeight: 1.5,
  },
};

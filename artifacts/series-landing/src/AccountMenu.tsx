import { useEffect, useRef, useState } from 'react';

interface Props {
  name: string;
  email: string;
  photo: string | null;
  onEditProfile: () => void;
  onSignOut: () => void;
}

export default function AccountMenu({ name, email, photo, onEditProfile, onSignOut }: Props) {
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    function handleClick(e: MouseEvent) {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) setOpen(false);
    }
    document.addEventListener('mousedown', handleClick);
    return () => document.removeEventListener('mousedown', handleClick);
  }, [open]);

  const initial = name.trim().charAt(0).toUpperCase() || '?';

  return (
    <div ref={rootRef} style={{ position: 'relative' }}>
      <button
        aria-label="Account menu"
        onClick={() => setOpen(o => !o)}
        style={styles.hamburgerBtn}
      >
        <span style={styles.bar} />
        <span style={styles.bar} />
        <span style={styles.bar} />
      </button>

      {open && (
        <div style={styles.dropdown}>
          <div style={styles.header}>
            {photo ? (
              <img src={photo} alt={name} style={styles.headerImg} />
            ) : (
              <span style={{ ...styles.avatarInitial, width: 40, height: 40, fontSize: 16 }}>{initial}</span>
            )}
            <div style={{ display: 'flex', flexDirection: 'column', minWidth: 0 }}>
              <span style={styles.name}>{name}</span>
              <span style={styles.email}>{email}</span>
            </div>
          </div>

          <div style={styles.divider} />

          <button
            style={styles.menuItem}
            onClick={() => { setOpen(false); onEditProfile(); }}
          >
            Edit profile
          </button>
          <button
            style={{ ...styles.menuItem, color: '#ff6b6b' }}
            onClick={() => { setOpen(false); onSignOut(); }}
          >
            Sign out
          </button>
        </div>
      )}
    </div>
  );
}

const styles: Record<string, React.CSSProperties> = {
  hamburgerBtn: {
    width: 40,
    height: 40,
    borderRadius: 10,
    border: '1px solid rgba(255,255,255,0.18)',
    background: 'rgba(255,255,255,0.08)',
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    gap: 5,
    cursor: 'pointer',
    padding: 0,
    flexShrink: 0,
  },
  bar: {
    display: 'block',
    width: 18,
    height: 2,
    borderRadius: 2,
    background: 'rgba(255,255,255,0.85)',
  },
  dropdown: {
    position: 'absolute',
    top: 'calc(100% + 10px)',
    right: 0,
    width: 'min(260px, 82vw)',
    background: 'linear-gradient(145deg, #1e1408 0%, #170f06 60%, #0e0b08 100%)',
    border: '1px solid rgba(255,255,255,0.14)',
    borderRadius: 14,
    boxShadow: '0 20px 48px rgba(0,0,0,0.6)',
    padding: 14,
    display: 'flex',
    flexDirection: 'column',
    gap: 10,
    zIndex: 50,
  },
  header: {
    display: 'flex',
    alignItems: 'center',
    gap: 10,
    padding: '2px 4px',
  },
  headerImg: {
    width: 40,
    height: 40,
    borderRadius: '50%',
    objectFit: 'cover',
    flexShrink: 0,
  },
  name: {
    fontSize: 14,
    fontWeight: 700,
    color: '#fff',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    textOverflow: 'ellipsis',
  },
  email: {
    fontSize: 12,
    color: 'rgba(255,255,255,0.5)',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    textOverflow: 'ellipsis',
  },
  divider: {
    height: 1,
    background: 'rgba(255,255,255,0.1)',
  },
  menuItem: {
    background: 'none',
    border: 'none',
    color: '#fff',
    fontSize: 14,
    fontWeight: 500,
    textAlign: 'left',
    padding: '9px 8px',
    borderRadius: 8,
    cursor: 'pointer',
    fontFamily: 'inherit',
  },
};

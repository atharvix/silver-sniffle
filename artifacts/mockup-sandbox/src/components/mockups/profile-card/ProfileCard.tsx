export function ProfileCard() {
  const TEAL_GRADIENT = 'linear-gradient(160deg, #5DCAA5 0%, #1D9E75 55%, #0F6E56 100%)';

  const card: React.CSSProperties = {
    position: 'relative',
    width: 340,
    height: 340,
    borderRadius: 20,
    overflow: 'hidden',
    border: '0.5px solid rgba(255,255,255,0.14)',
    backgroundImage: TEAL_GRADIENT,
    backgroundSize: 'cover',
    backgroundPosition: 'center',
    boxShadow: '0 20px 40px -12px rgba(0,0,0,0.5)',
    cursor: 'grab',
    fontFamily: "'Inter', -apple-system, BlinkMacSystemFont, sans-serif",
  };

  const peekFar: React.CSSProperties = {
    position: 'absolute',
    top: 16, left: 16, right: -16,
    height: 340,
    background: '#1c1c1c',
    border: '0.5px solid rgba(255,255,255,0.08)',
    borderRadius: 20,
  };

  const peekNear: React.CSSProperties = {
    position: 'absolute',
    top: 8, left: 8, right: -8,
    height: 340,
    background: '#262626',
    border: '0.5px solid rgba(255,255,255,0.09)',
    borderRadius: 20,
  };

  return (
    <div style={{
      minHeight: '100vh',
      background: 'linear-gradient(160deg, #1a0e06 0%, #0e0604 50%, #100806 100%)',
      display: 'flex',
      alignItems: 'center',
      justifyContent: 'center',
      padding: 40,
    }}>
      {/* Card stack */}
      <div style={{ position: 'relative', width: 340, height: 360 }}>
        {/* Peek cards behind */}
        <div style={peekFar} />
        <div style={peekNear} />

        {/* Front card */}
        <div style={card}>
          {/* No-photo initials */}
          <div style={{ position: 'absolute', inset: 0, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
            <span style={{ fontSize: 64, fontWeight: 800, color: 'rgba(255,255,255,0.55)', letterSpacing: '-1px' }}>AK</span>
          </div>

          {/* Shade */}
          <div style={{
            position: 'absolute', inset: 0,
            background: 'linear-gradient(to bottom, rgba(0,0,0,0.45) 0%, rgba(0,0,0,0.05) 26%, rgba(0,0,0,0.05) 48%, rgba(0,0,0,0.82) 100%)',
          }} />

          {/* Content */}
          <div style={{ position: 'relative', height: '100%', display: 'flex', flexDirection: 'column', justifyContent: 'space-between', padding: 12 }}>
            {/* Top row */}
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 6, background: 'rgba(0,0,0,0.32)', borderRadius: 999, padding: '6px 12px', minWidth: 0 }}>
                <span style={{ fontSize: 12, color: '#fff', fontWeight: 600, whiteSpace: 'nowrap' }}>Alex Kim nearby</span>
              </div>
              <div style={{ display: 'flex', alignItems: 'center', gap: 4, background: 'rgba(0,0,0,0.28)', borderRadius: 999, padding: '5px 10px', color: '#fff', fontSize: 11, fontWeight: 600, flexShrink: 0 }}>
                <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round">
                  <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8Z" /><circle cx="12" cy="12" r="3" />
                </svg>
                <span>3 m</span>
              </div>
            </div>

            {/* Bottom block */}
            <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
              <span style={{ display: 'inline-flex', alignItems: 'center', gap: 6, alignSelf: 'flex-start', background: 'rgba(0,0,0,0.32)', color: '#fff', fontSize: 11, fontWeight: 600, padding: '5px 10px', borderRadius: 999 }}>
                <span style={{ position: 'relative', display: 'inline-flex', width: 8, height: 8 }}>
                  <span style={{ position: 'absolute', inset: 0, borderRadius: '50%', background: '#fff', opacity: 0.6 }} />
                  <span style={{ position: 'absolute', inset: 0, borderRadius: '50%', background: '#fff' }} />
                </span>
                Live nearby
              </span>
              <p style={{ fontSize: 19, lineHeight: 1.25, color: '#fff', margin: 0, fontWeight: 800, letterSpacing: '-0.3px' }}>
                Coffee lover & indie hacker
              </p>
              <p style={{ fontSize: 13, lineHeight: 1.4, color: 'rgba(255,255,255,0.85)', margin: 0, fontWeight: 500 }}>
                &ldquo;What's your go-to order when you need to focus?&rdquo;
              </p>
              <div style={{ display: 'flex', alignItems: 'center', gap: 5, color: 'rgba(255,255,255,0.85)', fontSize: 11 }}>
                <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round">
                  <path d="M20 10c0 6-8 12-8 12s-8-6-8-12a8 8 0 0 1 16 0Z" /><circle cx="12" cy="10" r="3" />
                </svg>
                <span>3 m away · live now</span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

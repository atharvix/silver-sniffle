import { useGetNearbyProfiles } from '@workspace/api-client-react';
import type { NearbyProfileCard } from '@workspace/api-client-react';

interface Props {
  onBack: () => void;
}

export default function DiscoveryScreen({ onBack }: Props) {
  const { data, isLoading, error, refetch, isFetching } = useGetNearbyProfiles();

  const profiles = data?.profiles ?? [];

  return (
    <div style={screen.root}>
      {/* Header */}
      <header style={screen.header}>
        <button style={screen.backBtn} onClick={onBack} aria-label="Back">
          <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
            <path d="M19 12H5M12 5l-7 7 7 7" />
          </svg>
        </button>
        <div style={screen.headerCenter}>
          <span style={screen.logo}>s<span style={{ fontWeight: 200 }}>_</span></span>
          <h1 style={screen.heading}>People nearby</h1>
        </div>
        <button
          style={screen.refreshBtn}
          onClick={() => refetch()}
          disabled={isFetching}
          aria-label="Refresh"
          title="Refresh"
        >
          <svg
            width="18"
            height="18"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="2.5"
            strokeLinecap="round"
            strokeLinejoin="round"
            style={{ animation: isFetching ? 'spin 0.8s linear infinite' : 'none' }}
          >
            <path d="M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8" />
            <path d="M21 3v5h-5" />
            <path d="M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16" />
            <path d="M8 16H3v5" />
          </svg>
        </button>
      </header>

      {/* Body */}
      <main style={screen.main}>
        {/* Loading skeletons */}
        {isLoading && (
          <div style={screen.list}>
            {[1, 2, 3].map(i => (
              <div key={i} style={card.root}>
                <div style={{ ...card.avatar, background: 'rgba(255,255,255,0.06)', animation: 'pulse 1.4s ease-in-out infinite' }} />
                <div style={card.body}>
                  <div style={{ width: '55%', height: 16, borderRadius: 8, background: 'rgba(255,255,255,0.08)', marginBottom: 8, animation: 'pulse 1.4s ease-in-out infinite' }} />
                  <div style={{ width: '90%', height: 36, borderRadius: 8, background: 'rgba(255,255,255,0.05)', animation: 'pulse 1.4s ease-in-out 0.2s infinite' }} />
                </div>
              </div>
            ))}
          </div>
        )}

        {/* Error state */}
        {!isLoading && error && (
          <div style={screen.centerBox}>
            <div style={emptyIcon}>
              <svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="rgba(255,255,255,0.3)" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
                <circle cx="12" cy="12" r="10" />
                <line x1="12" y1="8" x2="12" y2="12" />
                <line x1="12" y1="16" x2="12.01" y2="16" />
              </svg>
            </div>
            <p style={screen.emptyTitle}>Couldn't load profiles</p>
            <p style={screen.emptySubtitle}>Check your connection and try again.</p>
            <button style={screen.retryBtn} onClick={() => refetch()}>Try again</button>
          </div>
        )}

        {/* Empty state */}
        {!isLoading && !error && profiles.length === 0 && (
          <div style={screen.centerBox}>
            <div style={emptyIcon}>
              <svg width="36" height="36" viewBox="0 0 24 24" fill="none" stroke="rgba(255,255,255,0.25)" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
                <path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2" />
                <circle cx="9" cy="7" r="4" />
                <path d="M23 21v-2a4 4 0 0 0-3-3.87" />
                <path d="M16 3.13a4 4 0 0 1 0 7.75" />
              </svg>
            </div>
            <p style={screen.emptyTitle}>No one nearby yet</p>
            <p style={screen.emptySubtitle}>
              Share this link with someone and meet up —<br />they'll appear here when they're within 30 m.
            </p>
            <button style={screen.retryBtn} onClick={() => refetch()}>Refresh</button>
          </div>
        )}

        {/* Profile cards */}
        {!isLoading && !error && profiles.length > 0 && (
          <>
            <p style={screen.countLine}>
              {profiles.length} {profiles.length === 1 ? 'person' : 'people'} within 30 m
            </p>
            <div style={screen.list}>
              {profiles.map((profile, i) => (
                <ProfileCard key={`${profile.name}-${i}`} profile={profile} />
              ))}
            </div>
          </>
        )}
      </main>

      <style>{`
        @keyframes spin {
          from { transform: rotate(0deg); }
          to   { transform: rotate(360deg); }
        }
        @keyframes pulse {
          0%, 100% { opacity: 1; }
          50%       { opacity: 0.4; }
        }
      `}</style>
    </div>
  );
}

// ── ProfileCard ───────────────────────────────────────────────────────────────

function ProfileCard({ profile }: { profile: NearbyProfileCard }) {
  const initials = profile.name
    .split(/\s+/)
    .slice(0, 2)
    .map(w => w[0]?.toUpperCase() ?? '')
    .join('');

  return (
    <div style={card.root}>
      {/* Avatar */}
      <div style={card.avatarWrap}>
        {profile.photo ? (
          <img src={profile.photo} alt={profile.name} style={card.avatar} />
        ) : (
          <div style={{ ...card.avatar, ...card.avatarFallback }}>
            <span style={card.initials}>{initials}</span>
          </div>
        )}
        {/* Online dot */}
        <div style={card.onlineDot} />
      </div>

      {/* Content */}
      <div style={card.body}>
        <div style={card.topRow}>
          <span style={card.name}>{profile.name}</span>
          <span style={card.distance}>{formatDistance(profile.distanceMeters)}</span>
        </div>

        {/* iMessage-style conversation starter bubble */}
        <div style={card.bubble}>
          <p style={card.bubbleText}>{profile.conversationStarter}</p>
          <div style={card.bubbleTail} />
        </div>
      </div>
    </div>
  );
}

function formatDistance(m: number): string {
  if (m < 1) return '< 1 m';
  return `${Math.round(m)} m`;
}

// ── Styles ────────────────────────────────────────────────────────────────────

const screen: Record<string, React.CSSProperties> = {
  root: {
    minHeight: '100vh',
    background: 'linear-gradient(160deg, #1a0e06 0%, #0e0604 50%, #100806 100%)',
    display: 'flex',
    flexDirection: 'column',
    fontFamily: "'Inter', -apple-system, BlinkMacSystemFont, sans-serif",
    color: '#fff',
  },
  header: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    padding: '20px 24px 16px',
    borderBottom: '1px solid rgba(255,255,255,0.06)',
    background: 'rgba(26,14,6,0.8)',
    backdropFilter: 'blur(12px)',
    WebkitBackdropFilter: 'blur(12px)',
    position: 'sticky',
    top: 0,
    zIndex: 10,
  },
  headerCenter: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    gap: 2,
  },
  logo: {
    fontSize: 18,
    fontWeight: 900,
    letterSpacing: '-0.8px',
    color: 'rgba(255,255,255,0.5)',
  },
  heading: {
    fontSize: 17,
    fontWeight: 700,
    color: '#fff',
    letterSpacing: '-0.3px',
    margin: 0,
  },
  backBtn: {
    background: 'rgba(255,255,255,0.08)',
    border: '1px solid rgba(255,255,255,0.1)',
    borderRadius: 10,
    color: 'rgba(255,255,255,0.7)',
    cursor: 'pointer',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    width: 40,
    height: 40,
    padding: 0,
    flexShrink: 0,
  },
  refreshBtn: {
    background: 'rgba(255,255,255,0.08)',
    border: '1px solid rgba(255,255,255,0.1)',
    borderRadius: 10,
    color: 'rgba(255,255,255,0.7)',
    cursor: 'pointer',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
    width: 40,
    height: 40,
    padding: 0,
    flexShrink: 0,
  },
  main: {
    flex: 1,
    padding: '24px 20px 40px',
    maxWidth: 600,
    width: '100%',
    margin: '0 auto',
  },
  list: {
    display: 'flex',
    flexDirection: 'column',
    gap: 12,
  },
  countLine: {
    fontSize: 13,
    color: 'rgba(255,255,255,0.4)',
    marginBottom: 16,
    letterSpacing: '0.02em',
  },
  centerBox: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    textAlign: 'center',
    paddingTop: 60,
    gap: 12,
  },
  emptyTitle: {
    fontSize: 20,
    fontWeight: 700,
    color: '#fff',
    letterSpacing: '-0.3px',
    margin: 0,
  },
  emptySubtitle: {
    fontSize: 14,
    color: 'rgba(255,255,255,0.45)',
    lineHeight: 1.6,
    margin: 0,
    maxWidth: 280,
  },
  retryBtn: {
    marginTop: 8,
    background: 'rgba(255,255,255,0.1)',
    border: '1px solid rgba(255,255,255,0.18)',
    borderRadius: 10,
    color: '#fff',
    fontSize: 14,
    fontWeight: 600,
    cursor: 'pointer',
    padding: '10px 24px',
    fontFamily: 'inherit',
  },
};

const emptyIcon: React.CSSProperties = {
  width: 72,
  height: 72,
  borderRadius: '50%',
  background: 'rgba(255,255,255,0.04)',
  border: '1px solid rgba(255,255,255,0.08)',
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center',
  marginBottom: 4,
};

const card: Record<string, React.CSSProperties> = {
  root: {
    display: 'flex',
    alignItems: 'flex-start',
    gap: 14,
    background: 'rgba(255,255,255,0.04)',
    border: '1px solid rgba(255,255,255,0.08)',
    borderRadius: 18,
    padding: '16px',
    transition: 'background 0.15s',
  },
  avatarWrap: {
    position: 'relative',
    flexShrink: 0,
  },
  avatar: {
    width: 52,
    height: 52,
    borderRadius: '50%',
    objectFit: 'cover',
  },
  avatarFallback: {
    background: 'linear-gradient(135deg, #3a1a08 0%, #2a1206 100%)',
    border: '1px solid rgba(255,255,255,0.1)',
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
  },
  initials: {
    fontSize: 18,
    fontWeight: 700,
    color: 'rgba(255,255,255,0.7)',
    letterSpacing: '-0.3px',
  },
  onlineDot: {
    position: 'absolute',
    bottom: 1,
    right: 1,
    width: 12,
    height: 12,
    borderRadius: '50%',
    background: '#30d158',
    border: '2px solid #1a0e06',
  },
  body: {
    flex: 1,
    minWidth: 0,
    display: 'flex',
    flexDirection: 'column',
    gap: 8,
  },
  topRow: {
    display: 'flex',
    alignItems: 'baseline',
    justifyContent: 'space-between',
    gap: 8,
  },
  name: {
    fontSize: 16,
    fontWeight: 700,
    color: '#fff',
    letterSpacing: '-0.2px',
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    textOverflow: 'ellipsis',
  },
  distance: {
    fontSize: 12,
    color: '#30d158',
    fontWeight: 600,
    flexShrink: 0,
  },
  bubble: {
    position: 'relative',
    background: '#30d158',
    borderRadius: '4px 18px 18px 18px',
    padding: '10px 14px',
    display: 'inline-block',
    maxWidth: '100%',
  },
  bubbleText: {
    fontSize: 14,
    color: '#fff',
    lineHeight: 1.45,
    margin: 0,
    fontWeight: 500,
  },
  bubbleTail: {
    position: 'absolute',
    top: 0,
    left: -6,
    width: 0,
    height: 0,
    borderStyle: 'solid',
    borderWidth: '0 0 10px 10px',
    borderColor: 'transparent transparent transparent #30d158',
    transform: 'scaleX(-1)',
  },
};

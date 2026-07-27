import { useEffect, useRef, useState } from 'react';
import { flushSync } from 'react-dom';
import { animate, motion, useMotionValue, useTransform, type PanInfo } from 'framer-motion';
import { useGetNearbyProfiles, useUpdateLocation, useSendHeartbeat, getGetNearbyProfilesQueryKey } from '@workspace/api-client-react';
import type { NearbyProfileCard } from '@workspace/api-client-react';

// A profile the user has encountered this session, plus whether the live
// feed still reports them as present. Once seen, a profile stays in this
// list (session-only) so a right-swipe can always bring it back — even
// after they've walked off or gone offline.
type SeenProfile = NearbyProfileCard & { isPresent: boolean; isDemo?: boolean };

// Always-visible demo cards — shown when no real profiles are nearby (or after
// swiping past real ones). They act as a permanent tail of the stack.
const DEMO_PROFILES: SeenProfile[] = [
  {
    name: 'Toni Smith',
    photo: '/demo-toni.png',
    headline: 'Creative tech founder',
    conversationStarter: "I'm a creative tech founder. Really into film, fashion, art & building community",
    distanceMeters: 15,
    isPresent: true,
    isDemo: true,
  },
  {
    name: 'Zahra',
    photo: '/demo-zahra.png',
    headline: '2× startup marketing head & founder',
    conversationStarter: "Hi! I have been a 2x startup head of marketing and 1x founder. Looking to connect with fellow founders who need help with marketing",
    distanceMeters: 22,
    isPresent: true,
    isDemo: true,
  },
  {
    name: 'Talin Bahrami',
    photo: '/demo-talin.png',
    headline: 'Product designer',
    conversationStarter: "Product designer looking to connect with other designers and founders.",
    distanceMeters: 28,
    isPresent: true,
    isDemo: true,
  },
];

interface Props {
  onBack: () => void;
}

// Presence tuning: must stay comfortably inside the server's PRESENCE_TTL_MS
// (20s) so a profile never flickers offline just from normal network jitter.
const HEARTBEAT_INTERVAL_MS = 6_000;
const NEARBY_POLL_INTERVAL_MS = 4_000;

// sendBeacon can't set an Authorization header, so the "go offline" ping
// carries the token in its JSON body instead.
function sendOfflineBeacon() {
  const token = localStorage.getItem('series_token');
  if (!token) return;
  const blob = new Blob([JSON.stringify({ token })], { type: 'application/json' });
  navigator.sendBeacon?.('/api/profiles/offline', blob);
}

export default function DiscoveryScreen({ onBack }: Props) {
  const { data, isLoading, error, refetch, isFetching } = useGetNearbyProfiles({
    query: {
      queryKey: getGetNearbyProfilesQueryKey(),
      // Poll frequently and never serve a cached/stale list — presence here
      // is meant to reflect who is actually nearby right now.
      refetchInterval: NEARBY_POLL_INTERVAL_MS,
      staleTime: 0,
      retry: false,
    },
  });

  const { mutate: updateLocation } = useUpdateLocation();
  const { mutate: sendHeartbeat } = useSendHeartbeat();

  const [seenProfiles, setSeenProfiles] = useState<SeenProfile[]>([]);
  const [activeIndex, setActiveIndex] = useState(0);

  // Real profiles first, demo profiles always appended at the end.
  // Deduplicate: if a real profile shares a name with a demo, skip the demo.
  const realNames = new Set(seenProfiles.map(p => p.name));
  const displayProfiles: SeenProfile[] = [
    ...seenProfiles,
    ...DEMO_PROFILES.filter(d => !realNames.has(d.name)),
  ];

  // Merge each live poll into the session history: update anyone we already
  // know about (and flag whether they're still actually nearby), append
  // anyone new to the end, but never drop someone just because they left —
  // that's exactly who a right-swipe should be able to find again.
  useEffect(() => {
    if (!data) return;
    setSeenProfiles(prev => {
      const updated = prev.map(p => {
        const live = data.profiles.find(np => np.name === p.name);
        return live ? { ...live, isPresent: true } : { ...p, isPresent: false };
      });
      const knownNames = new Set(prev.map(p => p.name));
      const additions = data.profiles
        .filter(p => !knownNames.has(p.name))
        .map(p => ({ ...p, isPresent: true }));
      return [...updated, ...additions];
    });
  }, [data]);


  // Keep a live presence loop running for as long as this screen is mounted
  // and visible: watch actual GPS movement (so walking out of the 30 m
  // radius removes you), and heartbeat on an interval (so closing the tab,
  // backgrounding it, or losing GPS still ages you out promptly).
  useEffect(() => {
    let watchId: number | null = null;
    let heartbeatTimer: ReturnType<typeof setInterval> | null = null;

    function start() {
      if (watchId == null && navigator.geolocation) {
        watchId = navigator.geolocation.watchPosition(
          (pos) => {
            updateLocation({
              data: {
                latitude: pos.coords.latitude,
                longitude: pos.coords.longitude,
              },
            });
          },
          () => {
            // Movement tracking failed (denied mid-session, GPS unavailable,
            // etc.) — fall back to plain heartbeats below so the profile
            // stays present at its last known position instead of vanishing.
          },
          { enableHighAccuracy: true, maximumAge: 5_000, timeout: 10_000 },
        );
      }
      if (heartbeatTimer == null) {
        heartbeatTimer = setInterval(() => sendHeartbeat(), HEARTBEAT_INTERVAL_MS);
      }
    }

    function stop() {
      if (watchId != null) {
        navigator.geolocation.clearWatch(watchId);
        watchId = null;
      }
      if (heartbeatTimer != null) {
        clearInterval(heartbeatTimer);
        heartbeatTimer = null;
      }
    }

    function handleVisibilityChange() {
      if (document.hidden) {
        // Backgrounded: stop pinging and disappear immediately rather than
        // lingering for the full TTL — a hidden tab isn't "here".
        stop();
        sendOfflineBeacon();
      } else {
        start();
        sendHeartbeat();
        refetch();
      }
    }

    start();
    document.addEventListener('visibilitychange', handleVisibilityChange);
    window.addEventListener('pagehide', sendOfflineBeacon);
    window.addEventListener('beforeunload', sendOfflineBeacon);

    return () => {
      stop();
      document.removeEventListener('visibilitychange', handleVisibilityChange);
      window.removeEventListener('pagehide', sendOfflineBeacon);
      window.removeEventListener('beforeunload', sendOfflineBeacon);
      // Leaving the discovery screen (e.g. navigating back) also ends presence.
      sendOfflineBeacon();
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // A stale/expired heartbeat surfaces as a 400 from the server — the presence
  // loop above will re-establish it within one heartbeat tick, so treat this
  // as a brief reconnect rather than a hard error.
  const isReconnecting = Boolean(error) && (error as { status?: number }).status === 400;
  void isReconnecting; // presence loop handles reconnect silently; no UI needed

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
          <span style={screen.logo}>Kinjo</span>
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
        {/* Card stack — always shown (demo cards fill when no real profiles are nearby) */}
        <div style={screen.cardSection}>
          <p style={screen.nearbyLabel}>
            These people are within 30 meters of you. Go say hello!
          </p>
          <ProfileCardStack
            profiles={displayProfiles}
            activeIndex={activeIndex}
            onNavigate={setActiveIndex}
          />
        </div>

        {/* Disclaimer — always visible */}
        <p style={screen.disclaimer}>
          Disclaimer: The above three profiles are for demonstration purposes only.
        </p>
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
        @keyframes pulseDot {
          0%   { transform: scale(1); opacity: 0.6; }
          70%  { transform: scale(2.2); opacity: 0; }
          100% { transform: scale(2.2); opacity: 0; }
        }
      `}</style>
    </div>
  );
}

// ── ProfileCardStack ─────────────────────────────────────────────────────────
// Fully imperative swipe animation — no AnimatePresence / variants — so drag
// and transition can never fight over the same motion value.

function ProfileCardStack({
  profiles,
  activeIndex,
  onNavigate,
}: {
  profiles: SeenProfile[];
  activeIndex: number;
  onNavigate: (index: number) => void;
}) {
  const current = profiles[activeIndex];
  const dragX   = useMotionValue(0);
  const scale    = useTransform(dragX, [-280, 0, 280], [0.82, 1, 0.82]);
  const busy     = useRef(false);

  async function swipeTo(dir: 1 | -1) {
    if (busy.current) return;
    const next = activeIndex + dir;
    if (next < 0 || next >= profiles.length) {
      // boundary — bounce back
      animate(dragX, 0, { type: 'spring', stiffness: 450, damping: 32 });
      return;
    }
    busy.current = true;
    // 1. fly out
    await animate(dragX, dir > 0 ? -320 : 320, { duration: 0.22, ease: [0.4, 0, 1, 1] });
    // 2. swap content (synchronous so there's no flash of old card at new position)
    flushSync(() => onNavigate(next));
    // 3. jump to enter side, then fly in
    dragX.set(dir > 0 ? 320 : -320);
    await animate(dragX, 0, { duration: 0.22, ease: [0, 0, 0.2, 1] });
    busy.current = false;
  }

  function handleDragEnd(_: unknown, info: PanInfo) {
    if      (info.offset.x < -80 || info.velocity.x < -500) swipeTo(1);
    else if (info.offset.x >  80 || info.velocity.x >  500) swipeTo(-1);
    else animate(dragX, 0, { type: 'spring', stiffness: 450, damping: 32 });
  }

  if (!current) return null;

  const initials = current.name
    .split(/\s+/).slice(0, 2)
    .map(w => w[0]?.toUpperCase() ?? '').join('');

  return (
    <div style={stack.wrap}>
      <div style={stack.deck}>
        <div style={stack.peekFar} />
        <div style={stack.peekNear} />

        <motion.div
          drag="x"
          dragMomentum={false}
          onDragEnd={handleDragEnd}
          style={{
            ...stack.card,
            x: dragX,
            scale,
            backgroundImage: current.photo ? `url(${current.photo})` : TEAL_GRADIENT,
            opacity: current.isPresent ? 1 : 0.55,
            cursor: 'grab',
            touchAction: 'pan-y',
          }}
          whileTap={{ cursor: 'grabbing' }}
        >
          {!current.photo && (
            <div style={stack.noPhotoFallback}>
              <span style={stack.noPhotoInitials}>{initials}</span>
            </div>
          )}
          <div style={stack.cardShade} />
          <div style={stack.cardContent}>
            {/* Top row — distance only, name moved to bottom */}
            <div style={stack.topRow}>
              <div style={{ flex: 1 }} />
              <div style={stack.metaPill}>
                <EyeIcon />
                <span>{formatDistance(current.distanceMeters)}</span>
              </div>
            </div>

            <div style={stack.bottomBlock}>
              <span style={stack.tagPill}>
                <PulseDotIcon />
                {current.isDemo ? 'Demo profile' : current.isPresent ? 'Live nearby' : 'No longer nearby'}
              </span>
              {/* Name is the hero line */}
              <p style={stack.headline}>{current.name}</p>
              <p style={stack.quote}>&ldquo;{current.conversationStarter}&rdquo;</p>
              {/* Distance row only for real profiles */}
              {!current.isDemo && (
                <div style={stack.metaRow}>
                  <PinIcon />
                  <span>
                    {formatDistance(current.distanceMeters)} away · {current.isPresent ? 'live now' : 'last seen nearby'}
                  </span>
                </div>
              )}
            </div>
          </div>
        </motion.div>
      </div>
    </div>
  );
}

function EyeIcon() {
  return (
    <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8Z" />
      <circle cx="12" cy="12" r="3" />
    </svg>
  );
}

function PinIcon() {
  return (
    <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round">
      <path d="M20 10c0 6-8 12-8 12s-8-6-8-12a8 8 0 0 1 16 0Z" />
      <circle cx="12" cy="10" r="3" />
    </svg>
  );
}

function PulseDotIcon() {
  return (
    <span style={{ position: 'relative', display: 'inline-flex', width: 8, height: 8 }}>
      <span style={{ position: 'absolute', inset: 0, borderRadius: '50%', background: '#fff', opacity: 0.6, animation: 'pulseDot 1.6s ease-in-out infinite' }} />
      <span style={{ position: 'absolute', inset: 0, borderRadius: '50%', background: '#fff' }} />
    </span>
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
    background: 'linear-gradient(160deg, #0c3a44 0%, #0a2e37 40%, #071f26 72%, #06232a 100%)',
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
    background: 'rgba(6,35,42,0.85)',
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
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    padding: '0 20px 32px',
    maxWidth: 600,
    width: '100%',
    margin: '0 auto',
  },
  cardSection: {
    flex: 1,
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    justifyContent: 'center',
    // nudge above vertical centre by consuming more space below
    paddingBottom: '14vh',
    width: '100%',
  },
  nearbyLabel: {
    fontSize: 14,
    fontWeight: 500,
    color: 'rgba(255,255,255,0.62)',
    textAlign: 'center',
    margin: '0 0 20px',
    lineHeight: 1.5,
    maxWidth: 300,
    letterSpacing: '0.01em',
  },
  disclaimer: {
    fontSize: 11,
    color: 'rgba(255,255,255,0.28)',
    textAlign: 'center',
    margin: '0',
    letterSpacing: '0.01em',
    lineHeight: 1.5,
    maxWidth: 320,
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

const TEAL_GRADIENT = 'linear-gradient(160deg, #5DCAA5 0%, #1D9E75 55%, #0F6E56 100%)';

const stack: Record<string, React.CSSProperties> = {
  wrap: {
    display: 'flex',
    flexDirection: 'column',
    alignItems: 'center',
    width: '100%',
    maxWidth: 340,
    margin: '0 auto',
  },
  deck: {
    position: 'relative',
    width: '100%',
    height: 360,
  },
  skeletonWrap: {
    position: 'relative',
    width: '100%',
    maxWidth: 340,
    height: 360,
    margin: '40px auto 0',
  },
  peekFar: {
    position: 'absolute',
    top: 16,
    left: 16,
    right: -16,
    height: 340,
    background: '#082530',
    border: '0.5px solid rgba(255,255,255,0.08)',
    borderRadius: 20,
  },
  peekNear: {
    position: 'absolute',
    top: 8,
    left: 8,
    right: -8,
    height: 340,
    background: '#0c3040',
    border: '0.5px solid rgba(255,255,255,0.09)',
    borderRadius: 20,
  },
  card: {
    position: 'absolute',
    inset: 0,
    width: '100%',
    height: 340,
    borderRadius: 20,
    overflow: 'hidden',
    border: '0.5px solid rgba(255,255,255,0.14)',
    backgroundSize: 'cover',
    backgroundPosition: 'center',
    backgroundRepeat: 'no-repeat',
    boxShadow: '0 20px 40px -12px rgba(0,0,0,0.5)',
  },
  noPhotoFallback: {
    position: 'absolute',
    inset: 0,
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'center',
  },
  noPhotoInitials: {
    fontSize: 64,
    fontWeight: 800,
    color: 'rgba(255,255,255,0.55)',
    letterSpacing: '-1px',
  },
  cardShade: {
    position: 'absolute',
    inset: 0,
    background: 'linear-gradient(to bottom, rgba(0,0,0,0.45) 0%, rgba(0,0,0,0.05) 26%, rgba(0,0,0,0.05) 48%, rgba(0,0,0,0.82) 100%)',
  },
  cardContent: {
    position: 'relative',
    height: '100%',
    display: 'flex',
    flexDirection: 'column',
    justifyContent: 'space-between',
    padding: 12,
  },
  topRow: {
    display: 'flex',
    alignItems: 'center',
    justifyContent: 'space-between',
    gap: 8,
  },
  namePill: {
    display: 'flex',
    alignItems: 'center',
    gap: 6,
    background: 'rgba(0,0,0,0.32)',
    borderRadius: 999,
    padding: '6px 12px',
    minWidth: 0,
  },
  namePillText: {
    fontSize: 12,
    color: '#fff',
    fontWeight: 600,
    whiteSpace: 'nowrap',
    overflow: 'hidden',
    textOverflow: 'ellipsis',
  },
  metaPill: {
    display: 'flex',
    alignItems: 'center',
    gap: 4,
    background: 'rgba(0,0,0,0.28)',
    borderRadius: 999,
    padding: '5px 10px',
    color: '#fff',
    fontSize: 11,
    fontWeight: 600,
    flexShrink: 0,
  },
  bottomBlock: {
    display: 'flex',
    flexDirection: 'column',
    gap: 8,
  },
  tagPill: {
    display: 'inline-flex',
    alignItems: 'center',
    gap: 6,
    alignSelf: 'flex-start',
    background: 'rgba(0,0,0,0.32)',
    color: '#fff',
    fontSize: 11,
    fontWeight: 600,
    padding: '5px 10px',
    borderRadius: 999,
  },
  headline: {
    fontSize: 19,
    lineHeight: 1.25,
    color: '#fff',
    margin: 0,
    fontWeight: 800,
    letterSpacing: '-0.3px',
  },
  quote: {
    fontSize: 13,
    lineHeight: 1.4,
    color: 'rgba(255,255,255,0.85)',
    margin: 0,
    fontWeight: 500,
  },
  metaRow: {
    display: 'flex',
    alignItems: 'center',
    gap: 5,
    color: 'rgba(255,255,255,0.85)',
    fontSize: 11,
  },
};

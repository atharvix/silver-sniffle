import { useEffect, useState } from 'react';
import { AnimatePresence, motion, type PanInfo } from 'framer-motion';
import { useGetNearbyProfiles, useUpdateLocation, useSendHeartbeat, getGetNearbyProfilesQueryKey } from '@workspace/api-client-react';
import type { NearbyProfileCard } from '@workspace/api-client-react';

// A profile the user has encountered this session, plus whether the live
// feed still reports them as present. Once seen, a profile stays in this
// list (session-only) so a right-swipe can always bring it back — even
// after they've walked off or gone offline.
type SeenProfile = NearbyProfileCard & { isPresent: boolean };

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
  const [direction, setDirection] = useState(1);

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

  function goNext() {
    setActiveIndex(i => {
      if (i >= seenProfiles.length - 1) return i;
      setDirection(1);
      return i + 1;
    });
  }

  function goPrev() {
    setActiveIndex(i => {
      if (i <= 0) return i;
      setDirection(-1);
      return i - 1;
    });
  }

  function handleDragEnd(_: unknown, info: PanInfo) {
    if (info.offset.x < -80 || info.velocity.x < -500) goNext();
    else if (info.offset.x > 80 || info.velocity.x > 500) goPrev();
  }

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
  const hasHistory = seenProfiles.length > 0;
  const current = seenProfiles[activeIndex];

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
        {/* Loading skeleton — only before we've ever seen anyone this session */}
        {!hasHistory && (isLoading || isReconnecting) && (
          <div style={stack.skeletonWrap}>
            <div style={{ ...stack.card, background: 'rgba(255,255,255,0.04)', animation: 'pulse 1.4s ease-in-out infinite' }} />
          </div>
        )}

        {/* Error state — excludes the brief stale-heartbeat reconnect window,
            which the presence loop resolves on its own within one tick */}
        {!hasHistory && !isLoading && !isReconnecting && error && (
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
        {!hasHistory && !isLoading && !isReconnecting && !error && (
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

        {/* Swipeable card stack */}
        {hasHistory && current && (
          <>
            <ProfileCardStack
              current={current}
              direction={direction}
              onDragEnd={handleDragEnd}
            />
            <p style={screen.positionLine}>
              {activeIndex + 1} of {seenProfiles.length}
              {!current.isPresent && <span style={screen.awayTag}> · no longer nearby</span>}
            </p>
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
// iMessage-share-card styling (teal gradient, pill badges, quoted starter)
// with a peeking stack behind to hint there's more, and drag-to-navigate:
// swipe left for the next nearby profile, swipe right to go back.

const cardVariants = {
  enter: (dir: number) => ({ x: dir > 0 ? 260 : -260, opacity: 0, scale: 0.94, rotate: dir > 0 ? 6 : -6 }),
  center: { x: 0, opacity: 1, scale: 1, rotate: 0 },
  exit: (dir: number) => ({ x: dir > 0 ? -260 : 260, opacity: 0, scale: 0.94, rotate: dir > 0 ? -6 : 6 }),
};

function ProfileCardStack({
  current,
  direction,
  onDragEnd,
}: {
  current: SeenProfile;
  direction: number;
  onDragEnd: (event: unknown, info: PanInfo) => void;
}) {
  const initials = current.name
    .split(/\s+/)
    .slice(0, 2)
    .map(w => w[0]?.toUpperCase() ?? '')
    .join('');

  return (
    <div style={stack.wrap}>
      <div style={stack.deck}>
        {/* Peek cards behind — always shown so the deck reads as a stack of
            three, matching the reference design, regardless of how many
            profiles are actually queued up next. */}
        <div style={stack.peekFar} />
        <div style={stack.peekNear} />

        <AnimatePresence initial={false} custom={direction} mode="popLayout">
          <motion.div
            key={current.name}
            custom={direction}
            variants={cardVariants}
            initial="enter"
            animate="center"
            exit="exit"
            transition={{ duration: 0.32, ease: [0.32, 0.72, 0, 1] }}
            drag="x"
            dragConstraints={{ left: 0, right: 0 }}
            dragElastic={0.65}
            onDragEnd={onDragEnd}
            style={{
              ...stack.card,
              backgroundImage: current.photo ? `url(${current.photo})` : TEAL_GRADIENT,
              opacity: current.isPresent ? 1 : 0.55,
              cursor: 'grab',
              touchAction: 'pan-y',
            }}
            whileTap={{ cursor: 'grabbing' }}
          >
            {/* Full-bleed photo — falls back to a gradient + initials when no photo exists */}
            {!current.photo && (
              <div style={stack.noPhotoFallback}>
                <span style={stack.noPhotoInitials}>{initials}</span>
              </div>
            )}

            {/* Darkens the top and bottom of the photo so pills/text stay readable */}
            <div style={stack.cardShade} />

            {/* Content sits above the photo + shade */}
            <div style={stack.cardContent}>
              {/* Top row */}
              <div style={stack.topRow}>
                <div style={stack.namePill}>
                  <span style={stack.namePillText}>{current.name} nearby</span>
                </div>
                <div style={stack.metaPill}>
                  <EyeIcon />
                  <span>{formatDistance(current.distanceMeters)}</span>
                </div>
              </div>

              {/* Bottom content */}
              <div style={stack.bottomBlock}>
                <span style={stack.tagPill}>
                  <PulseDotIcon />
                  {current.isPresent ? 'Live nearby' : 'No longer nearby'}
                </span>
                {/* AI-generated headline — the single most prominent line on the card */}
                <p style={stack.headline}>{current.headline}</p>
                <p style={stack.quote}>&ldquo;{current.conversationStarter}&rdquo;</p>
                <div style={stack.metaRow}>
                  <PinIcon />
                  <span>
                    {formatDistance(current.distanceMeters)} away · {current.isPresent ? 'live now' : 'last seen nearby'}
                  </span>
                </div>
              </div>
            </div>
          </motion.div>
        </AnimatePresence>
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
  countLine: {
    fontSize: 13,
    color: 'rgba(255,255,255,0.4)',
    marginBottom: 20,
    letterSpacing: '0.02em',
    textAlign: 'center',
  },
  positionLine: {
    fontSize: 13,
    color: 'rgba(255,255,255,0.4)',
    marginTop: 18,
    textAlign: 'center',
    letterSpacing: '0.02em',
  },
  awayTag: {
    color: '#ff9f5a',
    fontWeight: 600,
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
    maxWidth: 340,
    margin: '0 auto',
  },
  deck: {
    position: 'relative',
    width: '100%',
    height: 300,
  },
  skeletonWrap: {
    position: 'relative',
    width: '100%',
    maxWidth: 340,
    height: 300,
    margin: '40px auto 0',
  },
  peekFar: {
    position: 'absolute',
    top: 16,
    left: 16,
    right: -16,
    height: 280,
    background: '#1c1c1c',
    border: '0.5px solid rgba(255,255,255,0.08)',
    borderRadius: 20,
  },
  peekNear: {
    position: 'absolute',
    top: 8,
    left: 8,
    right: -8,
    height: 280,
    background: '#262626',
    border: '0.5px solid rgba(255,255,255,0.09)',
    borderRadius: 20,
  },
  card: {
    position: 'absolute',
    inset: 0,
    width: '100%',
    height: 280,
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

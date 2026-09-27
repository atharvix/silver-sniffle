import React, { useCallback, useEffect, useRef, useState } from 'react';
import type { UserProfile, SwipeDirection } from '../types';
import { ProfileCard } from './ProfileCard';
import { resolvePhotoUrl } from '../utils/api';
import { useMountedReveal } from '../hooks/useMountedReveal';

interface CardDeckProps {
  profiles: UserProfile[];
  isLoading?: boolean;
  onSwipe: (direction: SwipeDirection, profile: UserProfile) => void;
  onRefresh?: () => void;
}

/**
 * A fixed photo stack, in order:
 * drag left  -> the top card slides away and tucks behind; the next person is underneath.
 * drag right -> the previous person appears underneath; the dragged card tucks back in behind it.
 * First card: can't go right. Last card: can't go left (the card just stretches and springs back).
 */
type Pose = { x: number; y: number; r: number; s: number; b: number; o: number };

const POS: Pose[] = [
  { x: 0, y: 0, r: 0, s: 1, b: 1, o: 1 },
  { x: 12, y: -2, r: 3.5, s: 0.96, b: 0.5, o: 1 },
  { x: 22, y: -4, r: 7, s: 0.92, b: 0.32, o: 1 },
  { x: 28, y: -6, r: 9, s: 0.88, b: 0.25, o: 0 },
];
const PASSED: Pose = { x: -4, y: 4, r: -2, s: 0.94, b: 0.4, o: 0 };
const poseAt = (i: number): Pose => (i < 0 ? PASSED : POS[Math.min(i, 3)]);

const EASE = 'cubic-bezier(.22,1,.36,1)';
const EASE_OUT = 'cubic-bezier(.2,.8,.25,1)';
const SPRING = 'cubic-bezier(.25,1.35,.45,1)';

const clamp = (v: number, a: number, b: number) => Math.max(a, Math.min(b, v));
const lerp = (a: number, b: number, t: number) => a + (b - a) * t;
const mix = (a: Pose, b: Pose, t: number): Pose => ({
  x: lerp(a.x, b.x, t),
  y: lerp(a.y, b.y, t),
  r: lerp(a.r, b.r, t),
  s: lerp(a.s, b.s, t),
  b: lerp(a.b, b.b, t),
  o: lerp(a.o, b.o, t),
});

const SWIPE_HINT_KEY = 'kinjo_swipe_hint_seen';

export const CardDeck: React.FC<CardDeckProps> = ({
  profiles,
  isLoading = false,
  onSwipe,
  onRefresh: _onRefresh,
}) => {
  const [deck, setDeck] = useState<UserProfile[]>(() => [...profiles]);
  const [idx, setIdx] = useState(0);
  const [isDragging, setIsDragging] = useState(false);
  const [pullDistance, setPullDistance] = useState(0);
  const [isLocalRefreshing, setIsLocalRefreshing] = useState(false);
  const [hintGone, setHintGone] = useState(() => {
    try { return localStorage.getItem(SWIPE_HINT_KEY) === '1'; } catch { return false; }
  });

  const stageRef = useRef<HTMLDivElement>(null);
  const cardRefs = useRef<Map<string, HTMLDivElement>>(new Map());
  const shadeRefs = useRef<Map<string, HTMLDivElement>>(new Map());
  const idxRef = useRef(0);
  const busyRef = useRef(false);
  const timersRef = useRef<ReturnType<typeof setTimeout>[]>([]);
  const rafRef = useRef(0);
  const isPullingRef = useRef(false);
  const dragStartRef = useRef({ x: 0, y: 0, t: 0 });
  const dragRef = useRef<{
    id: number; x0: number; y0: number; px: number; py: number;
    lx: number; lt: number; vx: number; dx: number; dy: number;
    side: number; allowed: boolean; moved: boolean;
  } | null>(null);

  useEffect(() => { idxRef.current = idx; }, [idx]);

  // Sync incoming profiles, preserving the current top card when the set is unchanged.
  // No demo/placeholder fallback here — an empty list means nobody nearby, full stop.
  useEffect(() => {
    setDeck((prev) => {
      const sameSet = profiles.length === prev.length && profiles.every((p) => prev.some((q) => q.id === p.id));
      if (sameSet) return prev.map((p) => profiles.find((n) => n.id === p.id) ?? p);
      const oldTopId = prev[idxRef.current]?.id ?? null;
      const keep = profiles.findIndex((p) => p.id === oldTopId);
      const nextIdx = keep >= 0 ? keep : 0;
      idxRef.current = nextIdx;
      setIdx(nextIdx);
      return [...profiles];
    });
  }, [profiles]);

  const width = () => stageRef.current?.clientWidth || 250;

  const setPose = useCallback((id: string, p: Pose, dur?: number, ease?: string) => {
    const el = cardRefs.current.get(id);
    if (!el) return;
    el.style.transition = dur ? `transform ${dur}s ${ease || EASE}, opacity ${Math.min(dur, 0.32)}s ease` : 'none';
    el.style.transform = `translate3d(${p.x.toFixed(2)}px,${p.y.toFixed(2)}px,0) rotate(${p.r.toFixed(3)}deg) scale(${p.s.toFixed(4)})`;
    el.style.opacity = String(p.o);
    const shade = shadeRefs.current.get(id);
    if (shade) {
      shade.style.transition = dur ? `opacity ${dur}s ${ease || EASE}` : 'none';
      shade.style.opacity = (1 - p.b).toFixed(3);
    }
  }, []);

  const stackOrder = useCallback(() => {
    deck.forEach((p, i) => {
      const el = cardRefs.current.get(p.id);
      if (!el) return;
      const rel = i - idxRef.current;
      el.style.zIndex = String(rel < 0 ? 1 : 20 - Math.min(rel, 18));
      el.tabIndex = rel === 0 ? 0 : -1;
      el.setAttribute('aria-hidden', String(rel !== 0));
    });
  }, [deck]);

  const layout = useCallback((animate: boolean, dur?: number, ease?: string) => {
    stackOrder();
    deck.forEach((p, i) => {
      const rel = i - idxRef.current;
      if (animate) setPose(p.id, poseAt(rel), dur ?? 0.5, ease);
      else setPose(p.id, poseAt(rel));
    });
  }, [deck, setPose, stackOrder]);

  // Re-lay-out whenever the deck's membership changes (new fetch, reorder, etc.)
  useEffect(() => {
    layout(false);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [deck]);

  const later = (fn: () => void, ms: number) => { timersRef.current.push(setTimeout(fn, ms)); };
  const settle = useCallback(() => {
    timersRef.current.forEach(clearTimeout);
    timersRef.current = [];
    if (busyRef.current) { busyRef.current = false; layout(false); }
  }, [layout]);

  const hasNext = () => idxRef.current < deck.length - 1;
  const hasPrev = () => idxRef.current > 0;

  const markSwiped = () => {
    if (!hintGone) {
      setHintGone(true);
      try { localStorage.setItem(SWIPE_HINT_KEY, '1'); } catch { /* ignore */ }
    }
    if (navigator.vibrate) { try { navigator.vibrate(6); } catch { /* ignore */ } }
  };

  const outLeft = (): Pose => ({ x: -width() * 0.9 - 120, y: 16, r: -14, s: 0.94, b: 0.9, o: 0.4 });
  const outRight = (): Pose => ({ x: width() * 0.9 + 120, y: 16, r: 14, s: 0.94, b: 0.9, o: 0.4 });
  // rubber band at either end of the stack
  const band = (dx: number) => { const m = width() * 0.22; return Math.sign(dx) * m * (1 - 1 / ((Math.abs(dx) / m) * 0.55 + 1)); };
  const dragPose = (dx: number, dy: number): Pose => ({
    x: dx, y: dy * 0.1 + Math.abs(dx) * 0.025, r: clamp((dx / width()) * 14, -14, 14), s: 1, b: 1, o: 1,
  });

  // a small nudge when you try to go past either end
  const nudge = useCallback((dir: number) => {
    settle();
    const p = deck[idxRef.current];
    if (!p) return;
    setPose(p.id, { x: dir * 14, y: 0, r: dir * 1.5, s: 1, b: 1, o: 1 }, 0.16, EASE_OUT);
    later(() => setPose(p.id, poseAt(0), 0.5, SPRING), 150);
  }, [deck, setPose, settle]);

  // next: the top card slides off to the left, then tucks in behind the stack.
  // Crucially, the "reposition the rest of the stack" loop below skips the
  // outgoing card (i >= idxRef.current, which has already moved past it) — the
  // earlier bug reintroduced here repositioned EVERY card including the one
  // mid-flight, so its exit pose was clobbered in the same frame and it never
  // actually left the screen.
  const goNext = useCallback((dragged?: { dx: number; v: number }) => {
    if (!hasNext()) { if (dragged) layout(true, 0.6, SPRING); else nudge(-1); return; }
    settle();
    busyRef.current = true;
    markSwiped();
    const outgoing = deck[idxRef.current];
    const fast = clamp(dragged?.v || 0, 0, 2.5);
    const d1 = clamp(0.34 - fast * 0.06, 0.2, 0.34);
    idxRef.current += 1;
    stackOrder();
    const el = cardRefs.current.get(outgoing.id);
    if (el) el.style.zIndex = '30';
    setPose(outgoing.id, outLeft(), d1, EASE_OUT);
    deck.forEach((p, i) => { if (i >= idxRef.current) setPose(p.id, poseAt(i - idxRef.current), 0.52); });
    later(() => {
      setPose(outgoing.id, PASSED, 0.5);
      later(() => { busyRef.current = false; stackOrder(); }, 520);
    }, d1 * 1000 - 20);
    setIdx(idxRef.current);
    onSwipe('left', outgoing);
  }, [deck, layout, markSwiped, nudge, onSwipe, setPose, settle, stackOrder]);

  // previous: the card before shows up underneath, the current card tucks in behind it
  const goPrev = useCallback((dragged?: { v: number }) => {
    if (!hasPrev()) { if (dragged) layout(true, 0.6, SPRING); else nudge(1); return; }
    settle();
    busyRef.current = true;
    markSwiped();
    const outgoing = deck[idxRef.current];
    const incoming = deck[idxRef.current - 1];
    idxRef.current -= 1;
    const finish = () => {
      stackOrder();
      setPose(incoming.id, poseAt(0), 0.44);
      setPose(outgoing.id, poseAt(1), 0.6);
      deck.forEach((p, i) => { if (i > idxRef.current + 1) setPose(p.id, poseAt(i - idxRef.current), 0.52); });
      later(() => { busyRef.current = false; }, 620);
    };
    if (dragged) {
      finish();
    } else {
      const incomingEl = cardRefs.current.get(incoming.id);
      if (incomingEl) incomingEl.style.zIndex = '25';
      setPose(incoming.id, { x: 0, y: 5, r: 0, s: 0.95, b: 0.45, o: 1 });
      const outgoingEl = cardRefs.current.get(outgoing.id);
      if (outgoingEl) outgoingEl.style.zIndex = '30';
      setPose(outgoing.id, outRight(), 0.26, EASE_OUT);
      later(finish, 240);
    }
    setIdx(idxRef.current);
    onSwipe('right', outgoing);
  }, [deck, layout, markSwiped, nudge, onSwipe, setPose, settle, stackOrder]);

  // ─── Pointer handlers ──────────────────────────────────────────────────────
  const paint = useCallback(() => {
    rafRef.current = 0;
    const d = dragRef.current;
    if (!d) return;
    const w = width();
    let dx = d.px - d.x0;
    const dy = d.py - d.y0;
    const side = dx < 0 ? -1 : 1;
    const allowed = side < 0 ? hasNext() : hasPrev();
    if (!allowed) dx = band(dx);
    d.dx = dx; d.dy = dy; d.allowed = allowed;
    const top = deck[idxRef.current];
    if (!top) return;
    setPose(top.id, dragPose(dx, allowed ? dy : dy * 0.3));
    if (side !== d.side) {
      stackOrder();
      const el = cardRefs.current.get(top.id);
      if (el) el.style.zIndex = '30';
      deck.forEach((p, i) => { if (i !== idxRef.current) setPose(p.id, poseAt(i - idxRef.current)); });
      d.side = side;
    }
    if (!allowed) return;
    const k = clamp(Math.abs(dx) / (w * 0.55), 0, 1);
    if (side < 0) {
      for (let i = idxRef.current + 1; i < deck.length && i <= idxRef.current + 4; i++) {
        setPose(deck[i].id, mix(poseAt(i - idxRef.current), poseAt(i - idxRef.current - 1), k));
      }
    } else {
      const prevProfile = deck[idxRef.current - 1];
      if (prevProfile) {
        const pc = cardRefs.current.get(prevProfile.id);
        if (pc) pc.style.zIndex = '25';
        setPose(prevProfile.id, mix({ x: 0, y: 5, r: 0, s: 0.95, b: 0.45, o: 1 }, poseAt(0), k));
      }
      for (let i = idxRef.current + 1; i < deck.length && i <= idxRef.current + 3; i++) {
        setPose(deck[i].id, mix(poseAt(i - idxRef.current), poseAt(i - idxRef.current + 1), k));
      }
    }
  }, [deck, setPose, stackOrder]);

  const handlePointerDown = (e: React.PointerEvent) => {
    if (busyRef.current || isLocalRefreshing || dragRef.current) return;
    if (!(e.target as HTMLElement).closest('.kinjo-card')) return;
    settle();
    const top = deck[idxRef.current];
    if (!top) return;
    dragStartRef.current = { x: e.clientX, y: e.clientY, t: performance.now() };
    isPullingRef.current = false;
    dragRef.current = {
      id: e.pointerId, x0: e.clientX, y0: e.clientY, px: e.clientX, py: e.clientY,
      lx: e.clientX, lt: performance.now(), vx: 0, dx: 0, dy: 0, side: 0, allowed: true, moved: false,
    };
    try { (e.currentTarget as HTMLElement).setPointerCapture(e.pointerId); } catch { /* ignore */ }
    const el = cardRefs.current.get(top.id);
    if (el) { el.style.zIndex = '30'; el.style.cursor = 'grabbing'; }
    deck.forEach((p) => { const c = cardRefs.current.get(p.id); if (c) c.style.transition = 'none'; });
    setIsDragging(true);
  };

  const handlePointerMove = (e: React.PointerEvent) => {
    const d = dragRef.current;
    if (d && e.pointerId === d.id) {
      const dx0 = e.clientX - d.x0;
      const dy0 = e.clientY - d.y0;
      if (!d.moved && !isPullingRef.current && dy0 > 35 && dy0 > Math.abs(dx0) * 2.5) {
        // vertical intent: hand off to pull-to-refresh only on a direct downward pull
        isPullingRef.current = true;
        const top = deck[idxRef.current];
        if (top) setPose(top.id, poseAt(0));
        dragRef.current = null;
        setIsDragging(false);
        return;
      }
      d.moved = true;
      const now = performance.now();
      const dt = now - d.lt;
      if (dt > 0) d.vx = 0.7 * ((e.clientX - d.lx) / dt) + 0.3 * d.vx;
      d.lx = e.clientX; d.lt = now; d.px = e.clientX; d.py = e.clientY;
      if (!rafRef.current) rafRef.current = requestAnimationFrame(paint);
      return;
    }
    if (isPullingRef.current) {
      const dy = e.clientY - dragStartRef.current.y;
      setPullDistance(Math.min(Math.max(dy, 0) * 0.45, 80));
    }
  };

  const endDrag = (e: React.PointerEvent) => {
    if (isPullingRef.current) {
      isPullingRef.current = false;
      const dy = e.clientY - dragStartRef.current.y;
      const shouldRefresh = pullDistance > 35 || dy > 45;
      setPullDistance(0);
      setIsDragging(false);
      if (shouldRefresh) {
        setIsLocalRefreshing(true);
        _onRefresh?.();
        setTimeout(() => setIsLocalRefreshing(false), 800);
      }
      return;
    }
    const d = dragRef.current;
    setIsDragging(false);
    if (!d || e.pointerId !== d.id) return;
    if (rafRef.current) { cancelAnimationFrame(rafRef.current); rafRef.current = 0; paint(); }
    dragRef.current = null;
    const top = deck[idxRef.current];
    const el = top ? cardRefs.current.get(top.id) : null;
    if (el) el.style.cursor = 'grab';

    if (!d.moved) {
      stackOrder();
      return;
    }
    if (performance.now() - d.lt > 90) d.vx = 0; // finger stopped before lifting: no flick
    const w = width();
    const far = Math.abs(d.dx) > w * 0.28;
    const flick = Math.abs(d.vx) > 0.28 && Math.abs(d.dx) > 12 && Math.sign(d.vx) === Math.sign(d.dx);
    if (d.allowed && (far || flick) && d.dx < 0) goNext({ dx: d.dx, v: Math.abs(d.vx) });
    else if (d.allowed && (far || flick) && d.dx > 0) goPrev({ v: Math.abs(d.vx) });
    else layout(true, 0.6, d.allowed ? EASE : SPRING);
  };

  const handlePointerCancel = (e: React.PointerEvent) => {
    if (isPullingRef.current) { isPullingRef.current = false; setPullDistance(0); }
    setIsDragging(false);
    endDrag(e);
  };

  // Keyboard parity: ← next, → previous
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (busyRef.current || deck.length === 0) return;
      const target = e.target as HTMLElement | null;
      if (target && /input|textarea/i.test(target.tagName)) return;
      if (e.key === 'ArrowLeft') { e.preventDefault(); goNext(); }
      else if (e.key === 'ArrowRight') { e.preventDefault(); goPrev(); }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [deck.length, goNext, goPrev]);

  // Preload the next few avatars
  useEffect(() => {
    deck.slice(idx, idx + 3).forEach((p) => {
      const url = resolvePhotoUrl(p.avatar);
      if (url && url.startsWith('http')) { const img = new Image(); img.src = url; }
    });
  }, [idx, deck]);

  const headerMounted = useMountedReveal([]);
  const emptyMounted = useMountedReveal([deck.length === 0]);
  const words = 'Just move, and find your people.'.split(' ');

  // ─── Nobody nearby ───────────────────────────────────────────────────────
  if (deck.length === 0) {
    return (
      <div
        className="flex-1 flex flex-col items-start justify-start text-left px-6 pt-6 pb-10 select-none max-w-md mx-auto w-full h-full relative touch-none"
        style={{
          transform: pullDistance > 0 ? `translate3d(0, ${pullDistance * 0.6}px, 0)` : undefined,
          transition: isDragging ? 'none' : 'transform 240ms cubic-bezier(0.25, 1, 0.5, 1)',
        }}
      >
        <h2 className={`mask${emptyMounted ? ' in' : ''}`} style={{ fontSize: 46, fontWeight: 400, letterSpacing: '-.045em', lineHeight: 1.05, color: 'var(--fg)' }}>
          <span>Here is no one?</span>
        </h2>

        <p className={`words-wrap${emptyMounted ? ' in' : ''}`} style={{ fontSize: 23, fontWeight: 400, letterSpacing: '-.03em', lineHeight: 1.25, marginTop: 18, maxWidth: '14ch', color: 'var(--fg)' }}>
          {words.map((w, i) => (
            <React.Fragment key={i}>
              <span className="w" style={{ ['--w-delay' as string]: `${0.35 + i * 0.084}s` }}>{w}</span>{' '}
            </React.Fragment>
          ))}
        </p>

        <p className={`rf-item${emptyMounted ? ' in' : ''} flex items-center gap-2 mt-8 text-[13px]`} style={{ ['--rf-delay' as string]: '.5s', color: 'var(--muted)' }}>
          <span className="w-1.5 h-1.5 shrink-0" style={{ background: 'var(--muted)' }} />
          <span>{isLoading || isLocalRefreshing ? 'Scanning your 30m area…' : 'Still looking around you'}</span>
        </p>

        {_onRefresh && (
          <button
            type="button"
            onClick={() => _onRefresh()}
            className={`rf-item${emptyMounted ? ' in' : ''} mt-6 px-6 py-3 text-sm font-medium transition-opacity hover:opacity-85 active:opacity-60 cursor-pointer`}
            style={{
              ['--rf-delay' as string]: '.6s',
              borderRadius: 0,
              border: '1px solid var(--hairline)',
              background: 'var(--surface)',
              color: 'var(--fg)',
            }}
          >
            Look again
          </button>
        )}
      </div>
    );
  }

  // ─── People nearby ───────────────────────────────────────────────────────
  return (
    <div
      className="relative flex flex-col items-start justify-start w-full h-full select-none max-w-md mx-auto px-6 pt-4 flex-1 touch-none"
      style={{
        transform: pullDistance > 0 ? `translate3d(0, ${pullDistance * 0.6}px, 0)` : undefined,
        transition: isDragging ? 'none' : 'transform 240ms cubic-bezier(0.25, 1, 0.5, 1)',
      }}
    >
      {/* Discreet refresh indicator on manual pull */}
      {isLocalRefreshing && (
        <div className="absolute top-2 left-6 z-50 chip" style={{ borderRadius: 0 }}>
          <span className="w-1.5 h-1.5 shrink-0 animate-pulse" style={{ background: 'currentColor' }} />
          <span>Refreshing…</span>
        </div>
      )}

      {/* Editorial headline */}
      <div className="w-full text-left shrink-0">
        <div className="flex items-center gap-2 mb-1.5">
          <span className="w-1.5 h-1.5 bg-zinc-400 shrink-0" />
          <span className="text-[11px] tracking-widest text-zinc-400 uppercase font-medium">
            AROUND YOU, RIGHT NOW
          </span>
        </div>
        <h1 className="mt-1" style={{ fontSize: 34, fontWeight: 400, lineHeight: 1.08, letterSpacing: '-.035em' }}>
          <span className={`block mask${headerMounted ? ' in' : ''}`}><span style={{ color: 'var(--muted)' }}>Look up.</span></span>
          <span className={`block mask${headerMounted ? ' in' : ''}`} style={{ ['--rv-delay' as string]: '.08s' }}><span style={{ color: 'var(--fg)' }}>People are right here.</span></span>
        </h1>
      </div>

      {/* Card stack stage — spaced with a clean gap below headline */}
      <div
        ref={stageRef}
        onPointerDown={handlePointerDown}
        onPointerMove={handlePointerMove}
        onPointerUp={endDrag}
        onPointerCancel={handlePointerCancel}
        className="w-full flex items-center justify-center mt-24 shrink-0 touch-none select-none"
        style={{ minHeight: '388px', touchAction: 'none' }}
      >
        <div
          className="relative pointer-events-none"
          style={{ width: 'min(78vw, 268px)', height: 'min(53vh, 388px)', touchAction: 'none' }}
        >
          {deck.map((p) => (
            <div
              key={p.id}
              ref={(el) => { if (el) cardRefs.current.set(p.id, el); else cardRefs.current.delete(p.id); }}
              className="kinjo-card absolute inset-0 rounded-[24px] pointer-events-auto"
              style={{
                transformOrigin: '50% 62%',
                cursor: 'grab',
                willChange: 'transform',
                borderRadius: 24,
                touchAction: 'none',
              }}
            >
              <ProfileCard profile={p} />
              <div
                ref={(el) => { if (el) shadeRefs.current.set(p.id, el); else shadeRefs.current.delete(p.id); }}
                className="absolute inset-0 pointer-events-none rounded-[24px]"
                style={{ background: '#000', opacity: 0, borderRadius: 24 }}
              />
            </div>
          ))}
        </div>
      </div>

      {!hintGone && (
        <p className="swipe-hint w-full mt-4">Swipe left or right</p>
      )}
    </div>
  );
};

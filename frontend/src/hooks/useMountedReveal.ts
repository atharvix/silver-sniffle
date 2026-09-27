import { useEffect, useState } from 'react';

/**
 * Mirrors the reference design system's `.scr.on` trigger: false on first
 * render, flips true one animation frame after mount so the CSS transition
 * (from the `.mask`/`.rf-item` resting state) actually plays instead of
 * snapping straight to its end state.
 */
export function useMountedReveal(deps: React.DependencyList = []) {
  const [mounted, setMounted] = useState(false);

  useEffect(() => {
    setMounted(false);
    const id = requestAnimationFrame(() => setMounted(true));
    return () => cancelAnimationFrame(id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, deps);

  return mounted;
}

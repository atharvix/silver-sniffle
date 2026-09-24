import React, { createContext, useCallback, useContext, useRef, useState } from 'react';
import { AlertTriangle, CheckCircle2, Info, Loader2, X } from 'lucide-react';

export type ToastKind = 'success' | 'error' | 'info' | 'loading';

export interface Toast {
  id: number;
  kind: ToastKind;
  message: string;
  /** Auto-dismiss delay in ms. 0 = sticky until dismissed/replaced. */
  duration: number;
}

interface ToastContextValue {
  showToast: (message: string, kind?: ToastKind, duration?: number) => number;
  updateToast: (id: number, message: string, kind?: ToastKind, duration?: number) => void;
  dismissToast: (id: number) => void;
}

const ToastContext = createContext<ToastContextValue | null>(null);

/** Convenient hook: const toast = useToast(); toast.error('…'); */
export function useToast() {
  const ctx = useContext(ToastContext);
  if (!ctx) {
    // Safe no-op fallback so components outside the provider never crash.
    return {
      success: () => -1,
      error: () => -1,
      info: () => -1,
      loading: () => -1,
      update: () => {},
      dismiss: () => {},
    };
  }
  return {
    success: (msg: string, duration = 3000) => ctx.showToast(msg, 'success', duration),
    error: (msg: string, duration = 4500) => ctx.showToast(msg, 'error', duration),
    info: (msg: string, duration = 3500) => ctx.showToast(msg, 'info', duration),
    loading: (msg: string) => ctx.showToast(msg, 'loading', 0),
    update: ctx.updateToast,
    dismiss: ctx.dismissToast,
  };
}

const DEFAULT_DURATION: Record<ToastKind, number> = {
  success: 3000,
  error: 4500,
  info: 3500,
  loading: 0,
};

const ICONS: Record<ToastKind, React.ReactNode> = {
  success: <CheckCircle2 className="w-4 h-4 text-emerald-400 shrink-0" />,
  error: <AlertTriangle className="w-4 h-4 text-rose-400 shrink-0" />,
  info: <Info className="w-4 h-4 text-sky-400 shrink-0" />,
  loading: <Loader2 className="w-4 h-4 text-white/80 shrink-0 animate-spin" />,
};

const ACCENT: Record<ToastKind, string> = {
  success: 'border-emerald-500/30',
  error: 'border-rose-500/30',
  info: 'border-sky-500/30',
  loading: 'border-white/20',
};

export const ToastProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [toasts, setToasts] = useState<Toast[]>([]);
  const nextId = useRef(1);
  const timers = useRef(new Map<number, ReturnType<typeof setTimeout>>());

  const clearTimer = useCallback((id: number) => {
    const t = timers.current.get(id);
    if (t) {
      clearTimeout(t);
      timers.current.delete(id);
    }
  }, []);

  const scheduleDismiss = useCallback((id: number, duration: number) => {
    clearTimer(id);
    if (duration > 0) {
      timers.current.set(
        id,
        setTimeout(() => {
          setToasts((prev) => prev.filter((t) => t.id !== id));
          timers.current.delete(id);
        }, duration)
      );
    }
  }, [clearTimer]);

  const showToast = useCallback((message: string, kind: ToastKind = 'info', duration?: number) => {
    const id = nextId.current++;
    const d = duration ?? DEFAULT_DURATION[kind];
    // Cap visible toasts; oldest dismissible toasts drop first.
    setToasts((prev) => {
      const kept = prev.filter((t) => t.kind === 'loading' || t.duration === 0);
      return [...kept.slice(-2), { id, kind, message, duration: d }];
    });
    scheduleDismiss(id, d);
    return id;
  }, [scheduleDismiss]);

  const updateToast = useCallback((id: number, message: string, kind?: ToastKind, duration?: number) => {
    setToasts((prev) =>
      prev.map((t) =>
        t.id === id ? { ...t, message, kind: kind ?? t.kind, duration: duration ?? t.duration } : t
      )
    );
    if (duration !== 0) scheduleDismiss(id, duration ?? DEFAULT_DURATION[kind ?? 'info']);
  }, [scheduleDismiss]);

  const dismissToast = useCallback((id: number) => {
    clearTimer(id);
    setToasts((prev) => prev.filter((t) => t.id !== id));
  }, [clearTimer]);

  return (
    <ToastContext.Provider value={{ showToast, updateToast, dismissToast }}>
      {children}
      <div
        className="fixed top-[max(14px,env(safe-area-inset-top))] left-1/2 -translate-x-1/2 z-[100] flex flex-col items-center gap-2 w-full max-w-sm px-4 pointer-events-none"
        role="status"
        aria-live="polite"
      >
        {toasts.map((t) => (
          <div
            key={t.id}
            className={`toast-item pointer-events-auto flex items-center gap-2.5 w-full px-4 py-3 rounded-2xl bg-[#161618]/95 backdrop-blur-xl border ${ACCENT[t.kind]} shadow-2xl animate-in fade-in slide-in-from-top-2`}
          >
            {ICONS[t.kind]}
            <p className="flex-1 text-xs font-semibold text-white/90 leading-snug">{t.message}</p>
            {t.kind !== 'loading' && (
              <button
                type="button"
                onClick={() => dismissToast(t.id)}
                className="p-1 -m-1 rounded-full text-white/40 hover:text-white transition-colors"
                aria-label="Dismiss"
              >
                <X className="w-3.5 h-3.5" />
              </button>
            )}
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
};

import { useSyncExternalStore } from "react";

export type ToastKind = "success" | "error" | "info";

export interface Toast {
  id: number;
  kind: ToastKind;
  message: string;
}

/**
 * Tiny toast store. Justified as module state because toasts are raised from
 * the QueryClient's global error handlers, outside the React tree.
 */
let toasts: Toast[] = [];
let nextId = 1;
const listeners = new Set<() => void>();

function emit() {
  for (const l of listeners) l();
}

export function showToast(kind: ToastKind, message: string, ttlMs = 6000): number {
  const id = nextId++;
  toasts = [...toasts, { id, kind, message }];
  emit();
  if (ttlMs > 0) {
    setTimeout(() => dismissToast(id), ttlMs);
  }
  return id;
}

export function dismissToast(id: number) {
  toasts = toasts.filter((t) => t.id !== id);
  emit();
}

export function clearToasts() {
  toasts = [];
  emit();
}

function subscribe(listener: () => void) {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

export function useToasts(): Toast[] {
  return useSyncExternalStore(
    subscribe,
    () => toasts,
    () => toasts,
  );
}

import { useCallback, useState } from "react";

/**
 * Server-side cursor pagination over `next_page_token`. Keeps the stack of
 * tokens already visited so "Previous" works without offset support.
 */
export function useCursorPagination() {
  const [stack, setStack] = useState<string[]>([]);
  const pageToken = stack[stack.length - 1];
  const next = useCallback((token: string | undefined) => {
    if (token) setStack((s) => [...s, token]);
  }, []);
  const previous = useCallback(() => setStack((s) => s.slice(0, -1)), []);
  const reset = useCallback(() => setStack([]), []);
  return {
    pageToken,
    page: stack.length + 1,
    hasPrevious: stack.length > 0,
    next,
    previous,
    reset,
  };
}

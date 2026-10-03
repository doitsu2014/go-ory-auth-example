import { useCallback } from "react";
import { useLocation, useNavigate } from "react-router";

import { authRedirectFor } from "./requireAdmin";

/**
 * Applies the global auth-error routing (401, aal2_required, …) to calls that
 * deliberately bypass TanStack Query (PII reveal / lookup) and so do not reach
 * the QueryClient's error handlers. Returns true when it navigated.
 */
export function useAuthRedirect(): (err: unknown) => boolean {
  const navigate = useNavigate();
  const { pathname, search } = useLocation();
  return useCallback(
    (err: unknown) => {
      const target = authRedirectFor(err, `${pathname}${search}`);
      if (!target) return false;
      void navigate(target);
      return true;
    },
    [navigate, pathname, search],
  );
}

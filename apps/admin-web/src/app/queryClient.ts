import { MutationCache, QueryCache, QueryClient } from "@tanstack/react-query";

import { isApiError } from "../api/client";
import { meKeys } from "../auth/me";
import { authRedirectFor } from "../auth/requireAdmin";
import i18n from "../i18n";
import { problemMessage } from "../shared/problem";
import { showToast } from "../shared/toast";

export interface QueryClientDeps {
  /** Client-side navigation (router.navigate). */
  navigate: (path: string) => void;
  /** Current in-app path, used as return_to. */
  currentPath: () => string;
}

/**
 * Global API error policy: auth errors (401, aal2_required,
 * mfa_enrollment_required, not_admin) route the admin; everything else is a
 * toast with the problem `code` mapped through i18n.
 */
export function createQueryClient(deps: QueryClientDeps): QueryClient {
  function onError(err: unknown, opts: { silent?: boolean } = {}) {
    const target = authRedirectFor(err, deps.currentPath());
    if (target) {
      deps.navigate(target);
      return;
    }
    if (!opts.silent) showToast("error", problemMessage(i18n.t, err));
  }

  return new QueryClient({
    queryCache: new QueryCache({
      onError: (err, query) => {
        // The guard loader handles /me itself.
        if (query.queryKey[0] === meKeys.all[0]) return;
        onError(err);
      },
    }),
    mutationCache: new MutationCache({
      onError: (err) => onError(err),
    }),
    defaultOptions: {
      queries: {
        retry: (count, err) =>
          !(isApiError(err) && err.status >= 400 && err.status < 500) && count < 2,
        refetchOnWindowFocus: false,
      },
      mutations: { retry: false },
    },
  });
}

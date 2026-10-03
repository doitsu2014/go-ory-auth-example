import { useCallback } from "react";
import { useNavigate } from "react-router";

import { env } from "../shared/env";

/**
 * Follows a Kratos `redirect_browser_to` / continue_with URL safely:
 * - same origin as the app -> client-side navigation
 * - Kratos public origin   -> full browser navigation (Kratos sets cookies, then redirects back)
 * - anything else          -> refused (open-redirect protection)
 */
export function resolveRedirect(
  target: string,
): { kind: "spa"; path: string } | { kind: "external"; url: string } | { kind: "refused" } {
  let url: URL;
  try {
    url = new URL(target, window.location.origin);
  } catch {
    return { kind: "refused" };
  }
  if (url.origin === window.location.origin) {
    return { kind: "spa", path: `${url.pathname}${url.search}${url.hash}` };
  }
  if (url.origin === new URL(env.kratosPublicUrl).origin) {
    return { kind: "external", url: url.toString() };
  }
  return { kind: "refused" };
}

export function useFollowRedirect(): (target: string) => void {
  const navigate = useNavigate();
  return useCallback(
    (target: string) => {
      const r = resolveRedirect(target);
      if (r.kind === "spa") void navigate(r.path);
      else if (r.kind === "external") window.location.assign(r.url);
      else void navigate("/error");
    },
    [navigate],
  );
}

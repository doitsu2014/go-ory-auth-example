export const DEFAULT_AFTER_LOGIN = "/customers";

/**
 * Returns a safe in-app relative path, or undefined.
 * Accepts "/path?x#y" (not "//host" or "/\\host") and absolute URLs on the
 * current origin (converted to a relative path). Everything else is rejected
 * to prevent open redirects.
 */
export function safeReturnTo(value: string | null | undefined): string | undefined {
  if (!value) return undefined;
  const origin = window.location.origin;
  let url: URL;
  try {
    url = new URL(value, origin);
  } catch {
    return undefined;
  }
  if (url.origin !== origin) return undefined;
  if (!value.startsWith("/") && !value.startsWith(origin)) return undefined;
  if (value.startsWith("//") || value.startsWith("/\\")) return undefined;
  const path = `${url.pathname}${url.search}${url.hash}`;
  // Never bounce back into an auth page.
  if (/^\/(login|logout|registration)(\/|$|\?)/.test(url.pathname)) return undefined;
  return path;
}

export function currentPath(request: Request): string {
  const url = new URL(request.url);
  return `${url.pathname}${url.search}`;
}

export function loginPath(
  opts: { returnTo?: string; aal2?: boolean; refresh?: boolean } = {},
): string {
  const params = new URLSearchParams();
  if (opts.aal2) params.set("aal", "aal2");
  if (opts.refresh) params.set("refresh", "true");
  const rt = safeReturnTo(opts.returnTo);
  if (rt) params.set("return_to", rt);
  const qs = params.toString();
  return qs ? `/login?${qs}` : "/login";
}

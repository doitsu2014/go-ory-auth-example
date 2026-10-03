import {
  type ContinueWith,
  isRedirectBrowserTo,
  isShowSettingsUi,
  isShowVerificationUi,
} from "@ory/client-fetch";

/**
 * Picks where a successful Kratos call wants the browser to go next.
 * Returns an in-app path or a URL for useFollowRedirect, or undefined.
 * Settings beats verification (recovery -> privileged settings flow).
 */
export function continueWithTarget(list: ContinueWith[] | undefined): string | undefined {
  if (!list) return undefined;
  for (const c of list) {
    if (isShowSettingsUi(c)) return `/settings?flow=${encodeURIComponent(c.flow.id)}`;
  }
  for (const c of list) {
    if (isRedirectBrowserTo(c)) return c.redirect_browser_to;
  }
  for (const c of list) {
    if (isShowVerificationUi(c)) return `/verification?flow=${encodeURIComponent(c.flow.id)}`;
  }
  return undefined;
}

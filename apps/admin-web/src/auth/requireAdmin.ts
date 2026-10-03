import type { QueryClient } from "@tanstack/react-query";
import { type LoaderFunctionArgs, redirect } from "react-router";

import { type AdminMe, isApiError } from "../api/client";
import { meQuery } from "./me";
import { currentPath, loginPath } from "./returnTo";

export const NO_ACCESS_PATH = "/no-access";
export const ENROLL_TOTP_PATH = "/settings?enroll=totp";

/**
 * Maps an identity-service error on /admin/v1/me (or any admin call) to the
 * route the admin must go to (principles/03 §3.4). Undefined = not an auth error.
 */
export function authRedirectFor(err: unknown, returnTo: string): string | undefined {
  if (!isApiError(err)) return undefined;
  if (err.status === 401) return loginPath({ returnTo });
  if (err.status === 403) {
    switch (err.code) {
      case "aal2_required":
        return loginPath({ aal2: true, returnTo });
      case "mfa_enrollment_required":
        return ENROLL_TOTP_PATH;
      case "not_admin":
        return NO_ACCESS_PATH;
      default:
        return undefined;
    }
  }
  return undefined;
}

/**
 * `RequireAdmin` loader: asks the API (GET /admin/v1/me) instead of guessing
 * from Kratos. query() reuses a fresh cached value (meQuery.staleTime). On success the {roles, permissions} stay in the query cache
 * for UI gating.
 */
export function requireAdminLoader(queryClient: QueryClient) {
  return async ({ request }: LoaderFunctionArgs): Promise<AdminMe> => {
    try {
      return await queryClient.query(meQuery);
    } catch (err) {
      const returnTo = currentPath(request);
      const target = authRedirectFor(err, returnTo);
      if (target) throw redirect(target);
      // 403 forbidden on /me (or other unexpected 403): no access at all.
      if (isApiError(err) && err.status === 403) throw redirect(NO_ACCESS_PATH);
      throw err;
    }
  };
}

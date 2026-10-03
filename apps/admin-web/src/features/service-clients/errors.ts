import type { TFunction } from "i18next";

import { isApiError } from "../../api/client";
import { problemMessage } from "../../shared/problem";

type Key =
  | "unauthenticated"
  | "forbidden"
  | "not_found"
  | "conflict"
  | "validation_failed"
  | "dependency_unavailable";

const BY_STATUS: Partial<Record<number, Key>> = {
  401: "unauthenticated",
  403: "forbidden",
  404: "not_found",
  409: "conflict",
  422: "validation_failed",
  503: "dependency_unavailable",
};

/**
 * Problem message for the service-client endpoints: a specific text per
 * expected status (401/403/404/409/422/503), the generic problem text otherwise.
 * Auth codes that route the admin (aal2_required, …) are handled before this
 * by useAuthRedirect.
 */
export function serviceClientErrorMessage(t: TFunction, err: unknown): string {
  if (isApiError(err)) {
    const key = BY_STATUS[err.status];
    if (key) return t(`serviceClients.errors.${key}`);
  }
  return problemMessage(t, err);
}

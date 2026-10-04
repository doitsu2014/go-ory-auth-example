import type { TFunction } from "i18next";

import { isApiError } from "../../api/client";
import { problemMessage } from "../../shared/problem";

const PII_CODES = ["rate_limited", "forbidden", "not_found", "dependency_unavailable"] as const;

/** Problem message for the PII endpoints: specific texts for the expected codes. */
export function piiErrorMessage(
  t: TFunction,
  err: unknown,
  context: "reveal" | "lookup" | "loginLookup" | "masked",
): string {
  if (isApiError(err)) {
    const code = PII_CODES.find((c) => c === err.code);
    // Both lookups share one endpoint, rate limit and permission.
    if (code) return t(`pii.errors.${context === "loginLookup" ? "lookup" : context}.${code}`);
    if (context === "lookup" && err.code === "validation_failed") {
      return t("pii.lookup.invalid");
    }
    if (context === "loginLookup" && err.code === "validation_failed") {
      return t("customers.loginLookup.invalid");
    }
  }
  return problemMessage(t, err);
}

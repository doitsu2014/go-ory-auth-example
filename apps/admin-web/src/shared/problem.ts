import type { TFunction } from "i18next";

import { isApiError } from "../api/client";

/** Maps an API problem `code` to a localised message (unknown codes tolerated). */
export function problemMessage(t: TFunction, err: unknown): string {
  if (isApiError(err)) {
    return t(`problem.${err.code}`, {
      defaultValue: t("problem.unknown", { code: err.code }),
    });
  }
  return t("problem.internal");
}

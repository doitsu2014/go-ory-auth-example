import type { UiText } from "@ory/client-fetch";
import type { TFunction } from "i18next";

/** Renders a Kratos message by its stable id via i18n, falling back to `text`. */
export function kratosText(t: TFunction, message: UiText): string {
  const vars: Record<string, string | number> = {};
  const ctx = message.context as Record<string, unknown> | undefined;
  if (ctx) {
    for (const [k, v] of Object.entries(ctx)) {
      if (typeof v === "string" || typeof v === "number") vars[k] = v;
    }
  }
  return t(`kratos.${String(message.id)}`, { ...vars, defaultValue: message.text });
}

/**
 * Text for a Kratos generic error (`error.id`): i18n by id, falling back to
 * Kratos' `reason`/`message`, then to a generic message.
 */
export function kratosErrorText(
  t: TFunction,
  error: { id?: string; reason?: string; message?: string } | undefined,
): string {
  const fallback = error?.reason ?? error?.message ?? t("auth.errorGeneric");
  if (!error?.id) return fallback;
  return t(`kratosError.${error.id}`, { defaultValue: fallback });
}

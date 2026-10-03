/** String value of a form field ("" for missing or File). */
export function formString(data: FormData, key: string): string {
  const v = data.get(key);
  return typeof v === "string" ? v : "";
}

export function formatDateTime(value: string | undefined, lng: string): string {
  if (!value) return "—";
  const d = new Date(value);
  if (Number.isNaN(d.getTime())) return value;
  return new Intl.DateTimeFormat(lng === "vi" ? "vi-VN" : "en-GB", {
    dateStyle: "medium",
    timeStyle: "short",
  }).format(d);
}

export function personName(name: { first?: string; last?: string } | undefined): string {
  if (!name) return "—";
  const full = [name.first, name.last].filter(Boolean).join(" ");
  return full || "—";
}

/**
 * A person's name in the locale's order: Vietnamese writes the family name
 * first ("Nguyễn An"), English the given name first ("An Nguyễn").
 * Returns "" when neither part is present.
 */
export function localizedName(
  name: { first?: string | null; last?: string | null } | null | undefined,
  lng: string,
): string {
  if (!name) return "";
  const parts = lng.startsWith("vi") ? [name.last, name.first] : [name.first, name.last];
  return parts.filter(Boolean).join(" ");
}

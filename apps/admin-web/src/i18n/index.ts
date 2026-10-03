import i18n from "i18next";
import { initReactI18next } from "react-i18next";

import { en } from "./en";
import { kratosVi } from "./kratos.vi";
import { vi } from "./vi";

export const LANGUAGES = ["vi", "en"] as const;
export type Language = (typeof LANGUAGES)[number];

void i18n.use(initReactI18next).init({
  resources: {
    vi: { translation: { ...vi, kratos: kratosVi } },
    en: { translation: en },
  },
  lng: "vi",
  // vi falls back to en for any missing UI key; Kratos ids missing in both
  // fall back to the Kratos `text` via `defaultValue`.
  fallbackLng: "en",
  supportedLngs: LANGUAGES,
  interpolation: { escapeValue: false }, // React escapes output.
  returnNull: false,
});

i18n.on("languageChanged", (lng) => {
  document.documentElement.lang = lng;
});

export default i18n;

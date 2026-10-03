import { useId } from "react";
import { useTranslation } from "react-i18next";

import { LANGUAGES } from "../../i18n";

export function LanguageSwitcher() {
  const { t, i18n } = useTranslation();
  const id = useId();
  return (
    <div className="flex items-center gap-2 text-sm">
      <label htmlFor={id} className="text-slate-600">
        {t("lang.label")}
      </label>
      <select
        id={id}
        value={i18n.resolvedLanguage ?? i18n.language}
        onChange={(e) => void i18n.changeLanguage(e.target.value)}
        className="rounded-md border-0 py-1 pr-8 pl-2 text-sm ring-1 ring-slate-300 ring-inset"
      >
        {LANGUAGES.map((lng) => (
          <option key={lng} value={lng}>
            {t(`lang.${lng}`)}
          </option>
        ))}
      </select>
    </div>
  );
}

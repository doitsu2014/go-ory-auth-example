import { Outlet } from "react-router";
import { useTranslation } from "react-i18next";

import { LanguageSwitcher } from "../shared/ui/LanguageSwitcher";

/** Public layout for Kratos self-service pages. No registration link, ever. */
export function AuthLayout() {
  const { t } = useTranslation();
  return (
    <div className="flex min-h-screen flex-col">
      <header className="flex items-center justify-between px-6 py-4">
        <span className="font-semibold text-slate-900">{t("common.appName")}</span>
        <LanguageSwitcher />
      </header>
      <main id="main" className="flex flex-1 items-start justify-center px-4 pt-8 pb-16">
        <div className="w-full max-w-md">
          <Outlet />
        </div>
      </main>
    </div>
  );
}

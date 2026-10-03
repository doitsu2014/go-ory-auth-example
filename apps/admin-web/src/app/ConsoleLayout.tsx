import { NavLink, Outlet } from "react-router";
import { useTranslation } from "react-i18next";

import type { Permission } from "../api/client";
import { useAdminMe } from "../auth/me";
import { useLogout } from "../auth/useLogout";
import { Button } from "../shared/ui/Button";
import { cx } from "../shared/ui/cx";
import { LanguageSwitcher } from "../shared/ui/LanguageSwitcher";

const NAV: { to: string; label: string; permission: Permission }[] = [
  { to: "/customers", label: "nav.customers", permission: "view_customers" },
  { to: "/admins", label: "nav.admins", permission: "manage_admins" },
  { to: "/audit", label: "nav.audit", permission: "view_audit" },
];

/** Layout of the protected console (rendered below the RequireAdmin loader). */
export function ConsoleLayout() {
  const { t } = useTranslation();
  const me = useAdminMe();
  const { logout, pending } = useLogout();
  const items = NAV.filter((n) => me?.permissions.includes(n.permission));

  return (
    <div className="min-h-screen">
      <a href="#main" className="sr-only focus:not-sr-only focus:absolute focus:p-2">
        {t("common.skipToContent")}
      </a>
      <header className="border-b border-slate-200 bg-white">
        <div className="mx-auto flex max-w-6xl flex-wrap items-center justify-between gap-4 px-6 py-3">
          <div className="flex items-center gap-6">
            <span className="font-semibold text-slate-900">{t("common.appName")}</span>
            <nav aria-label={t("nav.main")}>
              <ul className="flex gap-1">
                {items.map((n) => (
                  <li key={n.to}>
                    <NavLink
                      to={n.to}
                      className={({ isActive }) =>
                        cx(
                          "rounded-md px-3 py-2 text-sm font-medium",
                          isActive
                            ? "bg-blue-50 text-blue-800"
                            : "text-slate-700 hover:bg-slate-100",
                        )
                      }
                    >
                      {t(n.label)}
                    </NavLink>
                  </li>
                ))}
              </ul>
            </nav>
          </div>
          <div className="flex items-center gap-4">
            <LanguageSwitcher />
            <NavLink to="/settings" className="text-sm text-slate-700 hover:underline">
              {t("nav.settings")}
            </NavLink>
            <span className="text-sm text-slate-500" data-testid="me-email">
              {me?.email}
            </span>
            <Button variant="secondary" onClick={() => void logout()} disabled={pending}>
              {t("common.signOut")}
            </Button>
          </div>
        </div>
      </header>
      <main id="main" className="mx-auto max-w-6xl px-6 py-8">
        <Outlet />
      </main>
    </div>
  );
}

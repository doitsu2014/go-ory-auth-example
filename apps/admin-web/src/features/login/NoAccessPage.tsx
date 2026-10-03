import { useTranslation } from "react-i18next";

import { useLogout } from "../../auth/useLogout";
import { Button } from "../../shared/ui/Button";
import { Card } from "../../shared/ui/Card";

/** Shown for 403 not_admin / forbidden on /admin/v1/me (e.g. a customer session). */
export function NoAccessPage() {
  const { t } = useTranslation();
  const { logout, pending } = useLogout();
  return (
    <Card>
      <h1 className="mb-2 text-xl font-semibold text-slate-900">{t("auth.noAccessTitle")}</h1>
      <p className="mb-6 text-sm text-slate-600">{t("auth.noAccessBody")}</p>
      <Button onClick={() => void logout()} disabled={pending}>
        {t("common.signOut")}
      </Button>
    </Card>
  );
}

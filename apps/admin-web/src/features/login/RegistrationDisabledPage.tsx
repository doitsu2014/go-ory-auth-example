import { useTranslation } from "react-i18next";
import { Link } from "react-router";

import { Card } from "../../shared/ui/Card";

/** Kratos' registration ui_url points here; admins are invite-only, so nothing is rendered. */
export function RegistrationDisabledPage() {
  const { t } = useTranslation();
  return (
    <Card>
      <h1 className="mb-2 text-xl font-semibold text-slate-900">
        {t("auth.registrationDisabledTitle")}
      </h1>
      <p className="text-sm text-slate-600">{t("auth.registrationDisabledBody")}</p>
      <div className="mt-6 text-sm">
        <Link to="/login" className="text-blue-700 underline">
          {t("auth.backToLogin")}
        </Link>
      </div>
    </Card>
  );
}

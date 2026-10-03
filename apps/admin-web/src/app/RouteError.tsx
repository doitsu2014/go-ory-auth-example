import { useTranslation } from "react-i18next";
import { Link, isRouteErrorResponse, useRouteError } from "react-router";

import { problemMessage } from "../shared/problem";
import { Alert } from "../shared/ui/Alert";
import { Card } from "../shared/ui/Card";

export function RouteError() {
  const { t } = useTranslation();
  const error = useRouteError();
  const message = isRouteErrorResponse(error)
    ? `${String(error.status)} ${error.statusText}`
    : problemMessage(t, error);
  return (
    <div className="mx-auto max-w-md p-8">
      <Card>
        <h1 className="mb-4 text-xl font-semibold">{t("auth.errorTitle")}</h1>
        <Alert kind="error">{message}</Alert>
        <Link to="/" className="mt-4 block text-sm text-blue-700 underline">
          {t("common.retry")}
        </Link>
      </Card>
    </div>
  );
}

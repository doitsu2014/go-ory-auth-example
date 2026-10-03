import { useQuery } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Link, useSearchParams } from "react-router";

import { frontend } from "../../auth/ory";
import { kratosErrorText } from "../../i18n/kratos";
import { Alert } from "../../shared/ui/Alert";
import { Card } from "../../shared/ui/Card";
import { Spinner } from "../../shared/ui/Spinner";

interface KratosErrorDetail {
  id?: string;
  code?: number;
  message?: string;
  reason?: string;
}

/** /error?id= — Kratos self-service error (selfservice.flows.error.ui_url). */
export function ErrorPage() {
  const { t } = useTranslation();
  const [params] = useSearchParams();
  const id = params.get("id");

  const query = useQuery({
    queryKey: ["kratos-flow-error", id],
    queryFn: () => frontend.getFlowError({ id: id ?? "" }),
    enabled: !!id,
    retry: false,
  });

  const detail = query.data?.error as KratosErrorDetail | undefined;

  return (
    <Card>
      <h1 className="mb-4 text-xl font-semibold text-slate-900">{t("auth.errorTitle")}</h1>
      {id && query.isPending ? (
        <Spinner />
      ) : (
        <Alert kind="error">
          <span>{kratosErrorText(t, detail)}</span>
          {detail?.id ? <code className="ml-2 text-xs">({detail.id})</code> : null}
        </Alert>
      )}
      <div className="mt-6 text-sm">
        <Link to="/login" className="text-blue-700 underline">
          {t("auth.backToLogin")}
        </Link>
      </div>
    </Card>
  );
}

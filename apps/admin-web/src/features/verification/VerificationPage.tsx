import type { UpdateVerificationFlowBody, VerificationFlow } from "@ory/client-fetch";
import { useTranslation } from "react-i18next";
import { Link, useSearchParams } from "react-router";

import { frontend } from "../../auth/ory";
import { FlowErrorView } from "../../auth/ui/FlowErrorView";
import { KratosFlowForms } from "../../auth/ui/KratosFlowForms";
import { useKratosFlow } from "../../auth/useKratosFlow";
import { Card } from "../../shared/ui/Card";
import { Spinner } from "../../shared/ui/Spinner";

/** /verification — email verification (code method), browser flow. */
export function VerificationPage() {
  const { t } = useTranslation();
  const [params, setParams] = useSearchParams();

  const { flow, flowKey, setFlow, submitting, error, retry, run } = useKratosFlow<VerificationFlow>(
    {
      flowId: params.get("flow"),
      create: () => frontend.createBrowserVerificationFlow(),
      get: (id) => frontend.getVerificationFlow({ id }),
      onFlowCreated: (f) =>
        setParams(
          (p) => {
            p.set("flow", f.id);
            return p;
          },
          { replace: true },
        ),
    },
  );

  function onSubmit(body: Record<string, unknown>) {
    void run(
      (f) =>
        frontend.updateVerificationFlow({
          flow: f.id,
          updateVerificationFlowBody: body as unknown as UpdateVerificationFlowBody,
        }),
      (next) => setFlow(next),
    );
  }

  return (
    <Card>
      <h1 className="mb-4 text-xl font-semibold text-slate-900">{t("auth.verificationTitle")}</h1>
      {error ? (
        <FlowErrorView error={error} onRetry={retry} />
      ) : flow ? (
        <KratosFlowForms key={flowKey} ui={flow.ui} submitting={submitting} onSubmit={onSubmit} />
      ) : (
        <Spinner />
      )}
      <div className="mt-6 text-sm">
        <Link to="/login" className="text-blue-700 underline">
          {t("auth.backToLogin")}
        </Link>
      </div>
    </Card>
  );
}

import type { RecoveryFlow, UpdateRecoveryFlowBody } from "@ory/client-fetch";
import { useTranslation } from "react-i18next";
import { Link, useSearchParams } from "react-router";

import { continueWithTarget } from "../../auth/continueWith";
import { frontend } from "../../auth/ory";
import { FlowErrorView } from "../../auth/ui/FlowErrorView";
import { KratosFlowForms } from "../../auth/ui/KratosFlowForms";
import { useFollowRedirect } from "../../auth/useFollowRedirect";
import { useKratosFlow } from "../../auth/useKratosFlow";
import { Card } from "../../shared/ui/Card";
import { Spinner } from "../../shared/ui/Spinner";

/**
 * /recovery — code method. After a valid code Kratos answers either 200 with
 * continue_with show_settings_ui or 422 browser_location_change_required
 * (handled in useKratosFlow); both lead to the privileged /settings flow.
 * This is also the landing page of an admin invitation link.
 */
export function RecoveryPage() {
  const { t } = useTranslation();
  const [params, setParams] = useSearchParams();
  const follow = useFollowRedirect();

  const { flow, flowKey, setFlow, submitting, error, retry, run } = useKratosFlow<RecoveryFlow>({
    flowId: params.get("flow"),
    create: () => frontend.createBrowserRecoveryFlow(),
    get: (id) => frontend.getRecoveryFlow({ id }),
    onFlowCreated: (f) =>
      setParams(
        (p) => {
          p.set("flow", f.id);
          return p;
        },
        { replace: true },
      ),
  });

  function onSubmit(body: Record<string, unknown>) {
    void run(
      (f) =>
        frontend.updateRecoveryFlow({
          flow: f.id,
          updateRecoveryFlowBody: body as unknown as UpdateRecoveryFlowBody,
        }),
      (next) => {
        const target = continueWithTarget(next.continue_with);
        if (target) follow(target);
        else setFlow(next);
      },
    );
  }

  return (
    <Card>
      <h1 className="mb-2 text-xl font-semibold text-slate-900">{t("auth.recoveryTitle")}</h1>
      <p className="mb-4 text-sm text-slate-600">{t("auth.recoveryHint")}</p>
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

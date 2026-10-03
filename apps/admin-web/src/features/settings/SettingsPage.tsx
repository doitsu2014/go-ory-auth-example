import type { SettingsFlow, UpdateSettingsFlowBody } from "@ory/client-fetch";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { Link, useSearchParams } from "react-router";

import { continueWithTarget } from "../../auth/continueWith";
import { meKeys } from "../../auth/me";
import { frontend } from "../../auth/ory";
import { FlowErrorView } from "../../auth/ui/FlowErrorView";
import { FlowMessages, KratosFlowForms } from "../../auth/ui/KratosFlowForms";
import { useFollowRedirect } from "../../auth/useFollowRedirect";
import { useKratosFlow } from "../../auth/useKratosFlow";
import { useLogout } from "../../auth/useLogout";
import { Alert } from "../../shared/ui/Alert";
import { Button } from "../../shared/ui/Button";
import { Card } from "../../shared/ui/Card";
import { Spinner } from "../../shared/ui/Spinner";

/** Credential groups an admin manages here. Profile traits are managed by invitation, not self-service. */
const SETTINGS_GROUPS = ["password", "totp", "lookup_secret"];

/**
 * /settings — browser settings flow: password, TOTP enrol/unlink, backup codes.
 * Reachable without RequireAdmin (new admins enrol TOTP here at AAL1).
 * `?enroll=totp` shows the mandatory-MFA hint.
 */
export function SettingsPage() {
  const { t } = useTranslation();
  const [params, setParams] = useSearchParams();
  const queryClient = useQueryClient();
  const follow = useFollowRedirect();
  const { logout, pending } = useLogout();
  const enroll = params.get("enroll") === "totp";

  const { flow, flowKey, setFlow, submitting, error, retry, run } = useKratosFlow<SettingsFlow>({
    flowId: params.get("flow"),
    create: () => frontend.createBrowserSettingsFlow(),
    get: (id) => frontend.getSettingsFlow({ id }),
    onFlowCreated: (f) =>
      setParams(
        (p) => {
          p.set("flow", f.id);
          return p;
        },
        { replace: true },
      ),
    returnTo: enroll ? "/settings?enroll=totp" : "/settings",
  });

  function onSubmit(body: Record<string, unknown>) {
    void run(
      (f) =>
        frontend.updateSettingsFlow({
          flow: f.id,
          updateSettingsFlowBody: body as unknown as UpdateSettingsFlowBody,
        }),
      (next) => {
        setFlow(next);
        // MFA state may have changed (e.g. TOTP enrolled): re-ask the API.
        void queryClient.invalidateQueries({ queryKey: meKeys.all });
        const target = continueWithTarget(next.continue_with);
        if (target) follow(target);
      },
    );
  }

  return (
    <Card>
      <div className="mb-4 flex items-center justify-between gap-2">
        <h1 className="text-xl font-semibold text-slate-900">{t("auth.settingsTitle")}</h1>
        <Link to="/customers" className="text-sm text-blue-700 underline">
          {t("common.backToConsole")}
        </Link>
      </div>
      {enroll ? (
        <div className="mb-4">
          <Alert kind="warning">{t("auth.enrollTotpHint")}</Alert>
        </div>
      ) : null}
      {error ? (
        <FlowErrorView error={error} onRetry={retry} />
      ) : flow ? (
        <div className="space-y-6">
          <FlowMessages ui={flow.ui} />
          <KratosFlowForms
            key={flowKey}
            ui={flow.ui}
            submitting={submitting}
            onSubmit={onSubmit}
            groups={SETTINGS_GROUPS}
            groupTitles
            hideMessages
          />
        </div>
      ) : (
        <Spinner />
      )}
      <div className="mt-6 border-t border-slate-200 pt-4">
        <Button variant="ghost" onClick={() => void logout()} disabled={pending}>
          {t("common.signOut")}
        </Button>
      </div>
    </Card>
  );
}

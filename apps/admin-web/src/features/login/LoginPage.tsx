import type { LoginFlow, UpdateLoginFlowBody } from "@ory/client-fetch";
import { useQueryClient } from "@tanstack/react-query";
import { useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Link, useNavigate, useSearchParams } from "react-router";

import { meKeys } from "../../auth/me";
import { frontend } from "../../auth/ory";
import { DEFAULT_AFTER_LOGIN, safeReturnTo } from "../../auth/returnTo";
import { FlowErrorView } from "../../auth/ui/FlowErrorView";
import { KratosFlowForms } from "../../auth/ui/KratosFlowForms";
import { useKratosFlow } from "../../auth/useKratosFlow";
import { useLogout } from "../../auth/useLogout";
import { Alert } from "../../shared/ui/Alert";
import { Button } from "../../shared/ui/Button";
import { Card } from "../../shared/ui/Card";
import { Spinner } from "../../shared/ui/Spinner";

/**
 * /login — browser login flow. Supports ?flow=, ?aal=aal2 (MFA step-up),
 * ?refresh=true (privileged re-auth) and ?return_to= (relative paths only).
 */
export function LoginPage() {
  const { t } = useTranslation();
  const [params, setParams] = useSearchParams();
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const { logout, pending: loggingOut } = useLogout();
  const [alreadySignedIn, setAlreadySignedIn] = useState(false);

  const flowId = params.get("flow");
  const wantAal2 = params.get("aal") === "aal2";
  const wantRefresh = params.get("refresh") === "true";
  const returnTo = safeReturnTo(params.get("return_to"));

  // Parameters of the last rendered flow, so a retry of a Kratos-initiated
  // (?flow= only) aal2/refresh flow keeps its kind.
  const lastFlow = useRef<LoginFlow | undefined>(undefined);

  const { flow, flowKey, submitting, error, retry, run } = useKratosFlow<LoginFlow>({
    flowId,
    create: () =>
      frontend.createBrowserLoginFlow({
        aal: wantAal2 || lastFlow.current?.requested_aal === "aal2" ? "aal2" : undefined,
        refresh: wantRefresh || lastFlow.current?.refresh === true ? true : undefined,
      }),
    get: (id) => frontend.getLoginFlow({ id }),
    onFlowCreated: (f) =>
      setParams(
        (p) => {
          p.set("flow", f.id);
          return p;
        },
        { replace: true },
      ),
    returnTo,
    onAlreadySignedIn: () => setAlreadySignedIn(true),
    // A 422 after password login (aal2 step) already set a new session cookie.
    onRedirect: () => queryClient.removeQueries({ queryKey: meKeys.all }),
  });
  lastFlow.current = flow;

  const isAal2 = wantAal2 || flow?.requested_aal === "aal2";
  const isRefresh = wantRefresh || flow?.refresh === true;
  const title = isAal2
    ? t("auth.aal2Title")
    : isRefresh
      ? t("auth.refreshTitle")
      : t("auth.signInTitle");
  const destination = returnTo ?? safeReturnTo(flow?.return_to) ?? DEFAULT_AFTER_LOGIN;

  function onSubmit(body: Record<string, unknown>) {
    void run(
      (f) =>
        frontend.updateLoginFlow({
          flow: f.id,
          updateLoginFlowBody: body as unknown as UpdateLoginFlowBody,
        }),
      async () => {
        // The session cookie changed (aal1 -> aal2 …): drop cached identity and let the guard decide.
        queryClient.removeQueries({ queryKey: meKeys.all });
        await navigate(destination, { replace: true });
      },
    );
  }

  return (
    <Card>
      <h1 className="mb-2 text-xl font-semibold text-slate-900">{title}</h1>
      {isAal2 ? <p className="mb-4 text-sm text-slate-600">{t("auth.aal2Hint")}</p> : null}
      {isRefresh && !isAal2 ? (
        <p className="mb-4 text-sm text-slate-600">{t("auth.refreshHint")}</p>
      ) : null}

      {alreadySignedIn ? (
        <div className="space-y-4">
          <Alert>{t("auth.alreadySignedIn")}</Alert>
          <Link to={destination} className="block text-sm text-blue-700 underline">
            {t("auth.continue")}
          </Link>
        </div>
      ) : error ? (
        <FlowErrorView error={error} onRetry={retry} />
      ) : flow ? (
        <KratosFlowForms key={flowKey} ui={flow.ui} submitting={submitting} onSubmit={onSubmit} />
      ) : (
        <Spinner />
      )}

      <div className="mt-6 flex flex-col gap-2 text-sm">
        {!isAal2 && !isRefresh ? (
          <Link to="/recovery" className="text-blue-700 underline">
            {t("auth.forgotPassword")}
          </Link>
        ) : null}
        {isAal2 || isRefresh || alreadySignedIn ? (
          <Button variant="ghost" onClick={() => void logout()} disabled={loggingOut}>
            {t("auth.useAnotherAccount")}
          </Button>
        ) : null}
      </div>
    </Card>
  );
}

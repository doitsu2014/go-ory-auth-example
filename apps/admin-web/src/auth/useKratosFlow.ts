import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router";

import { showToast } from "../shared/toast";
import { type FlowErrorAction, type KratosFlow, parseFlowError } from "./kratosErrors";
import { DEFAULT_AFTER_LOGIN, loginPath } from "./returnTo";
import { resolveRedirect, useFollowRedirect } from "./useFollowRedirect";

const MAX_RESTARTS = 2;

/**
 * Kratos may redirect the browser to its own login init endpoint (e.g. 422
 * browser_location_change_required to the aal2 step after a password login, or
 * session_refresh_required). Map that to the SPA login route so the flow is
 * created by the app with the same aal/refresh parameters.
 */
export function kratosLoginInitAsSpaPath(target: string, returnTo?: string): string | undefined {
  const r = resolveRedirect(target);
  if (r.kind !== "external") return undefined;
  const url = new URL(r.url);
  if (!url.pathname.endsWith("/self-service/login/browser")) return undefined;
  return loginPath({
    aal2: url.searchParams.get("aal") === "aal2",
    refresh: url.searchParams.get("refresh") === "true",
    returnTo: returnTo ?? url.searchParams.get("return_to") ?? undefined,
  });
}

export interface UseKratosFlowOptions<F extends KratosFlow> {
  /** `?flow=` from the URL, if any. */
  flowId: string | null;
  create: () => Promise<F>;
  get: (id: string) => Promise<F>;
  /** Called after a new flow was created so the page can put `?flow=` in the URL. */
  onFlowCreated?: (flow: F) => void;
  /** Where to come back after an auth redirect (relative path). */
  returnTo?: string;
  onAlreadySignedIn?: () => void;
  /** Called before following a Kratos redirect (e.g. a 422 after login set a new cookie). */
  onRedirect?: () => void;
}

export interface UseKratosFlowResult<F extends KratosFlow> {
  flow: F | undefined;
  /** Changes whenever a new flow object is rendered; use as React `key` to reset inputs. */
  flowKey: number;
  setFlow: (flow: F) => void;
  error: Extract<FlowErrorAction, { type: "error" }> | undefined;
  submitting: boolean;
  /** Clears the error and creates a fresh flow with the page's current parameters. */
  retry: () => void;
  /** Runs a Kratos update call; errors are handled per auth-flows §3.9. */
  run: <R>(
    call: (flow: F) => Promise<R>,
    onSuccess: (result: R) => void | Promise<void>,
  ) => Promise<void>;
}

/**
 * Shared lifecycle for Kratos browser flows: fetch `?flow=` or create a new
 * flow, and react to Kratos error responses (§3.9).
 */
export function useKratosFlow<F extends KratosFlow>(
  opts: UseKratosFlowOptions<F>,
): UseKratosFlowResult<F> {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const follow = useFollowRedirect();
  const [flow, setFlowState] = useState<F>();
  const [flowKey, setFlowKey] = useState(0);
  const [error, setError] = useState<Extract<FlowErrorAction, { type: "error" }>>();
  const [submitting, setSubmitting] = useState(false);
  const restarts = useRef(0);
  const optsRef = useRef(opts);
  optsRef.current = opts;

  const setFlow = useCallback((next: F) => {
    setError(undefined);
    setFlowState(next);
    setFlowKey((k) => k + 1);
  }, []);

  const handleError = useCallback(
    async (err: unknown): Promise<void> => {
      const o = optsRef.current;
      const action = await parseFlowError(err);
      switch (action.type) {
        case "rerender":
          // A 400 for this flow returns the same flow type with messages.
          setFlow(action.flow as F);
          return;
        case "restart": {
          if (restarts.current >= MAX_RESTARTS) {
            setError({ type: "error", status: 0 });
            return;
          }
          restarts.current += 1;
          showToast("info", t("auth.flowRestarted"));
          try {
            const next = action.useFlowId ? await o.get(action.useFlowId) : await o.create();
            setFlow(next);
            o.onFlowCreated?.(next);
          } catch (e) {
            await handleError(e);
          }
          return;
        }
        case "aal2":
          void navigate(loginPath({ aal2: true, returnTo: o.returnTo }));
          return;
        case "refresh":
          void navigate(loginPath({ refresh: true, returnTo: o.returnTo }));
          return;
        case "login":
          void navigate(loginPath({ returnTo: o.returnTo }));
          return;
        case "redirect": {
          o.onRedirect?.();
          const login = kratosLoginInitAsSpaPath(action.to, o.returnTo);
          if (login) void navigate(login);
          else follow(action.to);
          return;
        }
        case "already":
          if (o.onAlreadySignedIn) o.onAlreadySignedIn();
          else void navigate(o.returnTo ?? DEFAULT_AFTER_LOGIN);
          return;
        case "error":
          setError(action);
          return;
      }
    },
    [follow, navigate, setFlow, t],
  );

  const flowIdRef = useRef<string | undefined>(undefined);
  flowIdRef.current = flow?.id;

  useEffect(() => {
    const { flowId } = opts;
    if (flowId && flowIdRef.current === flowId) return;
    let ignore = false;
    const o = optsRef.current;
    const load = flowId ? o.get(flowId) : o.create();
    load
      .then((f) => {
        if (ignore) return;
        setFlow(f);
        if (!flowId) o.onFlowCreated?.(f);
      })
      .catch((e: unknown) => {
        if (!ignore) void handleError(e);
      });
    return () => {
      ignore = true;
    };
  }, [opts.flowId, handleError, setFlow]); // eslint-disable-line react-hooks/exhaustive-deps

  const run = useCallback(
    async <R>(call: (f: F) => Promise<R>, onSuccess: (result: R) => void | Promise<void>) => {
      if (!flow) return;
      setSubmitting(true);
      try {
        const result = await call(flow);
        restarts.current = 0;
        await onSuccess(result);
      } catch (e) {
        await handleError(e);
      } finally {
        setSubmitting(false);
      }
    },
    [flow, handleError],
  );

  const retry = useCallback(() => {
    const o = optsRef.current;
    setError(undefined);
    restarts.current = 0;
    o.create()
      .then((f) => {
        setFlow(f);
        o.onFlowCreated?.(f);
      })
      .catch((e: unknown) => void handleError(e));
  }, [handleError, setFlow]);

  return { flow, flowKey, setFlow, error, submitting, retry, run };
}

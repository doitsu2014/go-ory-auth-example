import { ResponseError } from "@ory/client-fetch";
import { useQueryClient } from "@tanstack/react-query";
import { useCallback, useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router";

import { clearToasts, showToast } from "../shared/toast";
import { frontend } from "./ory";

function isNoSession(err: unknown): boolean {
  return err instanceof ResponseError && err.response.status === 401;
}

/**
 * Logout (auth-flows §3.8): createBrowserLogoutFlow() -> call its logout
 * endpoint (AJAX, Kratos clears the cookie) -> clear all cached server state.
 * 401 (no session) counts as signed out. Any other failure keeps the user on
 * the page with an error toast: we never claim a logout that did not happen (FR-13).
 */
export function useLogout() {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const [pending, setPending] = useState(false);

  const logout = useCallback(async () => {
    setPending(true);
    try {
      const flow = await frontend.createBrowserLogoutFlow();
      await frontend.updateLogoutFlow({ token: flow.logout_token });
    } catch (err) {
      if (!isNoSession(err)) {
        setPending(false);
        showToast("error", t("auth.logoutFailed"));
        return;
      }
    }
    queryClient.clear();
    clearToasts();
    setPending(false);
    await navigate("/login", { replace: true });
  }, [navigate, queryClient, t]);

  return { logout, pending };
}

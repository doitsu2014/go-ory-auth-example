import { ResponseError, type UiContainer } from "@ory/client-fetch";

/** Minimal shape shared by all Kratos self-service flows. */
export interface KratosFlow {
  id: string;
  ui: UiContainer;
}

/**
 * What the page must do after a Kratos call failed (auth-flows §3.9).
 * - rerender:   400 + flow -> render the same flow with its messages
 * - aal2:       403 session_aal2_required -> login with aal=aal2
 * - refresh:    403 session_refresh_required -> re-authenticate (privileged session)
 * - restart:    403 security_csrf_violation, 410 expired, 404 unknown flow -> new flow
 * - redirect:   422 browser_location_change_required (or any redirect_browser_to)
 * - login:      401 no session (settings) -> login
 * - already:    session_already_available -> user is already signed in
 * - error:      anything else; shown as a generic error
 */
export type FlowErrorAction =
  | { type: "rerender"; flow: KratosFlow }
  | { type: "aal2"; redirectTo?: string }
  | { type: "refresh"; redirectTo?: string }
  | { type: "restart"; useFlowId?: string }
  | { type: "redirect"; to: string }
  | { type: "login" }
  | { type: "already" }
  | { type: "error"; status: number; id?: string; message?: string };

interface GenericErrorBody {
  error?: { id?: string; code?: number; message?: string; reason?: string };
  redirect_browser_to?: string;
  use_flow_id?: string;
  ui?: UiContainer;
  id?: string;
}

function isFlow(body: GenericErrorBody): body is GenericErrorBody & KratosFlow {
  return typeof body.id === "string" && typeof body.ui === "object";
}

async function readBody(response: Response): Promise<GenericErrorBody> {
  try {
    return (await response.clone().json()) as GenericErrorBody;
  } catch {
    return {};
  }
}

export async function parseFlowError(err: unknown): Promise<FlowErrorAction> {
  if (!(err instanceof ResponseError)) {
    return { type: "error", status: 0, message: err instanceof Error ? err.message : undefined };
  }
  const { status } = err.response;
  const body = await readBody(err.response);
  const id = body.error?.id;

  if (status === 400 && isFlow(body)) {
    return { type: "rerender", flow: body };
  }
  if (id === "session_already_available") return { type: "already" };
  if (id === "session_aal2_required") {
    return { type: "aal2", redirectTo: body.redirect_browser_to };
  }
  if (id === "session_refresh_required") {
    return { type: "refresh", redirectTo: body.redirect_browser_to };
  }
  if (id === "security_csrf_violation") return { type: "restart" };
  if (status === 410 || id === "self_service_flow_expired") {
    return { type: "restart", useFlowId: body.use_flow_id };
  }
  if (status === 404) return { type: "restart" };
  if (status === 422 || id === "browser_location_change_required") {
    if (body.redirect_browser_to) return { type: "redirect", to: body.redirect_browser_to };
  }
  if (status === 401 || id === "session_inactive") return { type: "login" };
  if (body.redirect_browser_to) return { type: "redirect", to: body.redirect_browser_to };

  return {
    type: "error",
    status,
    id,
    message: body.error?.reason ?? body.error?.message,
  };
}

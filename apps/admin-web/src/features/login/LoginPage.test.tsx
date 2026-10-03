import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";

import { CSRF, adminMe, loginFlow, passwordLoginNodes, text } from "../../test/fixtures";
import { renderApp } from "../../test/render";
import { API, KRATOS, server } from "../../test/server";

const FLOW_ID = "11111111-1111-4111-8111-111111111111";

function kratosLogin(onSubmit: (body: Record<string, unknown>, call: number) => Response) {
  const bodies: Record<string, unknown>[] = [];
  const seen = { createParams: new URLSearchParams() };
  server.use(
    http.get(`${KRATOS}/self-service/login/browser`, ({ request }) => {
      seen.createParams = new URL(request.url).searchParams;
      return HttpResponse.json(loginFlow(FLOW_ID, passwordLoginNodes()));
    }),
    http.post(`${KRATOS}/self-service/login`, async ({ request }) => {
      expect(new URL(request.url).searchParams.get("flow")).toBe(FLOW_ID);
      expect(request.credentials).toBe("include");
      const body = (await request.json()) as Record<string, unknown>;
      bodies.push(body);
      return onSubmit(body, bodies.length);
    }),
    http.get(`${API}/admin/v1/me`, () => HttpResponse.json(adminMe())),
    http.get(`${API}/admin/v1/customers`, () => HttpResponse.json({ items: [] })),
    http.get(`${API}/admin/v1/audit-events`, () => HttpResponse.json({ items: [] })),
  );
  return { bodies, seen };
}

async function fillAndSubmit(email: string, password: string) {
  const user = userEvent.setup();
  const id = await screen.findByLabelText("E-Mail");
  await user.clear(id);
  await user.type(id, email);
  await user.type(screen.getByLabelText("Password"), password);
  await user.click(screen.getByRole("button", { name: "Sign in with password" }));
}

describe("LoginPage", () => {
  it("creates a browser flow, puts ?flow= in the URL, submits and follows return_to", async () => {
    const { bodies, seen } = kratosLogin(() =>
      HttpResponse.json({ session: { id: "s1", active: true, identity: { id: "i1" } } }),
    );
    const { router } = renderApp("/login?return_to=%2Faudit");

    await waitFor(() => expect(router.state.location.search).toContain(`flow=${FLOW_ID}`));
    expect(seen.createParams.get("aal")).toBeNull();
    expect(screen.queryByRole("link", { name: /regist/i })).toBeNull();

    await fillAndSubmit("ops@example.com", "correct horse battery");

    await waitFor(() => expect(router.state.location.pathname).toBe("/audit"));
    expect(bodies[0]).toEqual({
      csrf_token: CSRF,
      identifier: "ops@example.com",
      password: "correct horse battery",
      method: "password",
    });
  });

  it("re-renders the same flow with messages on 400", async () => {
    kratosLogin((_body, call) =>
      call === 1
        ? HttpResponse.json(
            loginFlow(FLOW_ID, passwordLoginNodes(), [
              text(
                4000006,
                "The provided credentials are invalid, check for spelling mistakes.",
                "error",
              ),
            ]),
            { status: 400 },
          )
        : HttpResponse.json({ session: { id: "s1" } }),
    );
    const { router } = renderApp("/login");
    await fillAndSubmit("ops@example.com", "wrong-password");

    expect(await screen.findByText(/The provided credentials are invalid/)).toBeInTheDocument();
    expect(router.state.location.pathname).toBe("/login");
    expect(router.state.location.search).toContain(`flow=${FLOW_ID}`);
    // Inputs were reset from the re-rendered flow; retry works on the same flow.
    await fillAndSubmit("ops@example.com", "correct horse battery");
    await waitFor(() => expect(router.state.location.pathname).toBe("/customers"));
  });

  it("requests an aal2 flow for ?aal=aal2", async () => {
    const { seen } = kratosLogin(() => HttpResponse.json({}));
    renderApp("/login?aal=aal2&return_to=%2Fcustomers");
    expect(
      await screen.findByRole("heading", { name: "Two-factor verification" }),
    ).toBeInTheDocument();
    await waitFor(() => expect(seen.createParams.get("aal")).toBe("aal2"));
    expect(screen.queryByRole("link", { name: "Forgot password?" })).toBeNull();
  });

  it("restarts the flow on CSRF violation", async () => {
    let creates = 0;
    server.use(
      http.get(`${KRATOS}/self-service/login/browser`, () => {
        creates += 1;
        return HttpResponse.json(loginFlow(FLOW_ID, passwordLoginNodes()));
      }),
      http.post(`${KRATOS}/self-service/login`, () =>
        HttpResponse.json(
          { error: { id: "security_csrf_violation", code: 403, message: "csrf" } },
          { status: 403 },
        ),
      ),
    );
    renderApp("/login");
    await fillAndSubmit("ops@example.com", "pw-pw-pw-pw-pw");
    await waitFor(() => expect(creates).toBe(2));
    expect(
      await screen.findByText("Your session for this form expired. Please try again."),
    ).toBeInTheDocument();
  });

  it("/registration shows that registration is not available", async () => {
    renderApp("/registration");
    expect(
      await screen.findByRole("heading", { name: "Registration is not available" }),
    ).toBeInTheDocument();
    expect(document.querySelector("form")).toBeNull();
  });
});

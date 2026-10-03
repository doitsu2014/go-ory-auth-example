import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";

import {
  adminMe,
  csrfNode,
  inputNode,
  loginFlow,
  passwordLoginNodes,
  text,
} from "../test/fixtures";
import { renderApp } from "../test/render";
import { API, KRATOS, server } from "../test/server";

const A = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa";
const B = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb";

function totpLoginNodes() {
  return [
    csrfNode(),
    inputNode("totp", "totp_code", "text", { label: text(1010006, "Authentication code") }),
    inputNode("totp", "method", "submit", { value: "totp", label: text(1010005, "Verify") }),
  ];
}

async function submitPassword() {
  const user = userEvent.setup();
  await user.type(await screen.findByLabelText("E-Mail"), "ops@example.com");
  await user.type(screen.getByLabelText("Password"), "correct horse battery");
  await user.click(screen.getByRole("button", { name: "Sign in with password" }));
}

describe("flow error + retry", () => {
  it("retry clears the error and creates a new flow with the same aal2 parameters", async () => {
    const creates: (string | null)[] = [];
    server.use(
      http.get(`${KRATOS}/self-service/login/browser`, ({ request }) => {
        creates.push(new URL(request.url).searchParams.get("aal"));
        return creates.length === 1
          ? HttpResponse.json(
              { error: { code: 500, reason: "database unavailable" } },
              { status: 500 },
            )
          : HttpResponse.json(loginFlow(A, totpLoginNodes(), [], { requested_aal: "aal2" }));
      }),
    );
    const { router } = renderApp("/login?aal=aal2&return_to=%2Faudit");
    expect(await screen.findByText("database unavailable")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Try again" }));

    expect(await screen.findByLabelText("Authentication code")).toBeInTheDocument();
    expect(screen.queryByText("database unavailable")).toBeNull();
    expect(creates).toEqual(["aal2", "aal2"]);
    const search = new URLSearchParams(router.state.location.search);
    expect(search.get("aal")).toBe("aal2");
    expect(search.get("return_to")).toBe("/audit");
    expect(search.get("flow")).toBe(A);
  });

  it("renders flow errors by Kratos error id, falling back to reason", async () => {
    server.use(
      http.get(`${KRATOS}/self-service/login/browser`, () =>
        HttpResponse.json(
          { error: { id: "security_identity_mismatch", code: 400, reason: "English reason" } },
          { status: 400 },
        ),
      ),
    );
    renderApp("/login");
    expect(
      await screen.findByText("The request was made for a different account."),
    ).toBeInTheDocument();
  });

  it("recovery page offers retry too", async () => {
    let calls = 0;
    server.use(
      http.get(`${KRATOS}/self-service/recovery/browser`, () => {
        calls += 1;
        return calls === 1
          ? HttpResponse.json({ error: { code: 500, reason: "boom" } }, { status: 500 })
          : HttpResponse.json(
              loginFlow(A, [
                csrfNode(),
                inputNode("code", "email", "email", { label: text(1070007, "Email") }),
              ]),
            );
      }),
    );
    renderApp("/recovery");
    await userEvent.click(await screen.findByRole("button", { name: "Try again" }));
    expect(await screen.findByLabelText("Email")).toBeInTheDocument();
  });
});

describe("Kratos response handling (§3.9)", () => {
  it("410 with use_flow_id loads the replacement flow", async () => {
    const fetched: string[] = [];
    server.use(
      http.get(`${KRATOS}/self-service/login/browser`, () =>
        HttpResponse.json(loginFlow(A, passwordLoginNodes())),
      ),
      http.post(`${KRATOS}/self-service/login`, () =>
        HttpResponse.json(
          { error: { id: "self_service_flow_expired", code: 410 }, use_flow_id: B },
          { status: 410 },
        ),
      ),
      http.get(`${KRATOS}/self-service/login/flows`, ({ request }) => {
        const id = new URL(request.url).searchParams.get("id") ?? "";
        fetched.push(id);
        return HttpResponse.json(loginFlow(id, passwordLoginNodes()));
      }),
    );
    const { router } = renderApp("/login");
    await submitPassword();
    await waitFor(() => expect(router.state.location.search).toContain(`flow=${B}`));
    expect(fetched).toEqual([B]);
    expect(
      await screen.findByText("Your session for this form expired. Please try again."),
    ).toBeInTheDocument();
  });

  it("422 after password login (TOTP admin) continues to the aal2 step", async () => {
    const creates: (string | null)[] = [];
    server.use(
      http.get(`${KRATOS}/self-service/login/browser`, ({ request }) => {
        const aal = new URL(request.url).searchParams.get("aal");
        creates.push(aal);
        return aal === "aal2"
          ? HttpResponse.json(loginFlow(B, totpLoginNodes(), [], { requested_aal: "aal2" }))
          : HttpResponse.json(loginFlow(A, passwordLoginNodes()));
      }),
      http.post(`${KRATOS}/self-service/login`, () =>
        HttpResponse.json(
          {
            error: { id: "browser_location_change_required", code: 422 },
            redirect_browser_to: `${KRATOS}/self-service/login/browser?aal=aal2&return_to=${encodeURIComponent(`${window.location.origin}/audit`)}`,
          },
          { status: 422 },
        ),
      ),
    );
    const { router } = renderApp("/login?return_to=%2Faudit");
    await submitPassword();

    expect(
      await screen.findByRole("heading", { name: "Two-factor verification" }),
    ).toBeInTheDocument();
    expect(await screen.findByLabelText("Authentication code")).toBeInTheDocument();
    expect(creates).toEqual([null, "aal2"]);
    const search = new URLSearchParams(router.state.location.search);
    expect(search.get("aal")).toBe("aal2");
    expect(search.get("return_to")).toBe("/audit");
    expect(search.get("flow")).toBe(B);
  });

  it("422 to the app's own /login?flow= is followed in the SPA", async () => {
    server.use(
      http.get(`${KRATOS}/self-service/login/browser`, () =>
        HttpResponse.json(loginFlow(A, passwordLoginNodes())),
      ),
      http.post(`${KRATOS}/self-service/login`, () =>
        HttpResponse.json(
          {
            error: { id: "browser_location_change_required", code: 422 },
            redirect_browser_to: `${window.location.origin}/login?flow=${B}`,
          },
          { status: 422 },
        ),
      ),
      http.get(`${KRATOS}/self-service/login/flows`, () =>
        HttpResponse.json(loginFlow(B, totpLoginNodes(), [], { requested_aal: "aal2" })),
      ),
      http.get(`${API}/admin/v1/me`, () => HttpResponse.json(adminMe())),
    );
    const { router } = renderApp("/login");
    await submitPassword();
    expect(await screen.findByLabelText("Authentication code")).toBeInTheDocument();
    expect(router.state.location.search).toBe(`?flow=${B}`);
    expect(screen.getByRole("heading", { name: "Two-factor verification" })).toBeInTheDocument();
  });
});

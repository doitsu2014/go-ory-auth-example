import type { UiNode } from "@ory/client-fetch";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { HttpResponse, http } from "msw";
import { describe, expect, it } from "vitest";

import { CSRF, csrfNode, inputNode, loginFlow, text } from "../../test/fixtures";
import { renderApp } from "../../test/render";
import { KRATOS, server } from "../../test/server";

const SID = "44444444-4444-4444-8444-444444444444";
const QR = "data:image/png;base64,iVBORw0KGgo=";

function settingsFlow(messages = [] as ReturnType<typeof text>[]) {
  const nodes: UiNode[] = [
    csrfNode(),
    inputNode("password", "password", "password", { label: text(1070001, "Password") }),
    inputNode("password", "method", "submit", { value: "password", label: text(1070003, "Save") }),
    {
      type: "img",
      group: "totp",
      attributes: { node_type: "img", id: "totp_qr", src: QR, width: 256, height: 256 },
      messages: [],
      meta: { label: text(1050005, "Authenticator app QR code") },
    } as unknown as UiNode,
    inputNode("totp", "totp_code", "text", { label: text(1070006, "Verify code") }),
    inputNode("totp", "method", "submit", { value: "totp", label: text(1070005, "Submit") }),
  ];
  return {
    ...loginFlow(SID, nodes, messages),
    state: "show_form",
    identity: { id: "i1", schema_id: "admin", schema_url: "", traits: {} },
  };
}

describe("SettingsPage", () => {
  it("enrols TOTP: shows the enrol hint and QR, submits the code, renders success", async () => {
    const bodies: unknown[] = [];
    server.use(
      http.get(`${KRATOS}/self-service/settings/browser`, () => HttpResponse.json(settingsFlow())),
      http.post(`${KRATOS}/self-service/settings`, async ({ request }) => {
        expect(new URL(request.url).searchParams.get("flow")).toBe(SID);
        bodies.push(await request.json());
        return HttpResponse.json({
          ...settingsFlow([text(1050001, "Your changes have been saved!", "success")]),
          state: "success",
        });
      }),
    );
    const user = userEvent.setup();
    const { router } = renderApp("/settings?enroll=totp");

    expect(await screen.findByText(/Two-factor authentication is mandatory/)).toBeInTheDocument();
    expect(await screen.findByRole("img", { name: /QR code/ })).toHaveAttribute("src", QR);
    await user.type(screen.getByLabelText("Verify code"), "123456");
    await user.click(screen.getByRole("button", { name: "Submit" }));

    expect(await screen.findByText("Your changes have been saved!")).toBeInTheDocument();
    expect(bodies).toEqual([{ csrf_token: CSRF, totp_code: "123456", method: "totp" }]);
    expect(router.state.location.search).toContain("enroll=totp");
  });

  it("session_refresh_required sends the admin to a refresh login and back to settings", async () => {
    server.use(
      http.get(`${KRATOS}/self-service/settings/browser`, () => HttpResponse.json(settingsFlow())),
      http.post(`${KRATOS}/self-service/settings`, () =>
        HttpResponse.json(
          {
            error: { id: "session_refresh_required", code: 403 },
            redirect_browser_to: `${KRATOS}/self-service/login/browser?refresh=true`,
          },
          { status: 403 },
        ),
      ),
      http.get(`${KRATOS}/self-service/login/browser`, ({ request }) => {
        expect(new URL(request.url).searchParams.get("refresh")).toBe("true");
        return HttpResponse.json(
          loginFlow("55555555-5555-4555-8555-555555555555", [csrfNode()], [], { refresh: true }),
        );
      }),
    );
    const user = userEvent.setup();
    const { router } = renderApp("/settings");
    await user.type(
      await screen.findByLabelText("Password", { selector: "input" }),
      "a-brand-new-password",
    );
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(router.state.location.pathname).toBe("/login"));
    const search = new URLSearchParams(router.state.location.search);
    expect(search.get("refresh")).toBe("true");
    expect(search.get("return_to")).toBe("/settings");
    expect(await screen.findByRole("heading", { name: "Confirm it's you" })).toBeInTheDocument();
  });
});
